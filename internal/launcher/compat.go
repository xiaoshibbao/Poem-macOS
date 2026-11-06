package launcher

import (
	"fmt"
	"os"
	"path/filepath"

	"poem/internal/helperproto"
	"poem/internal/wine"
)

// Game settings keys: the CN client stores them under miHoYo, others under Cognosphere.
var gameRegistryKeys = []string{`HKCU\Software\miHoYo\崩坏：星穹铁道`, `HKCU\Software\Cognosphere\Star Rail`}

const internetSettings = `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`

func regString(key, name, value string) helperproto.RegValue {
	return helperproto.RegValue{Key: key, Name: name, Kind: helperproto.RegString, String: value}
}

func yesNo(v bool) string {
	if v {
		return "y"
	}
	return "n"
}

// launchRegistry collects the per-launch Wine and game settings.
func launchRegistry(cfg Settings, proxyPort int, dxmt bool) []helperproto.RegValue {
	values := []helperproto.RegValue{
		regString(`HKCU\Software\Wine\Mac Driver`, "RetinaMode", yesNo(cfg.RetinaMode)),
		regString(`HKCU\Software\Wine\Mac Driver`, "LeftCommandIsCtrl", yesNo(cfg.LeftCommandIsCtrl)),
		// Without an explicit driver Wine can end up with no audio output on macOS.
		regString(`HKCU\Software\Wine\Drivers`, "Audio", "coreaudio"),
	}
	// The SDK's default webview renderer shows a blank login page under Wine;
	// removing the cached choice lets it fall back to a working one.
	for _, key := range gameRegistryKeys {
		values = append(values, helperproto.RegValue{Key: key, Name: "MIHOYOSDK_WEBVIEW_RENDER_METHOD_h1573598267", Kind: helperproto.RegDelete})
	}
	if dxmt {
		// DXMT reports an NVIDIA GPU; the game then expects the driver's registry markers.
		marker := "{41FCC608-8496-4DEF-B43E-7D9BD675A6FF}"
		values = append(values,
			helperproto.RegValue{Key: `HKLM\SOFTWARE\NVIDIA Corporation\Global`, Name: marker, Kind: helperproto.RegBinary, Binary: []byte{1}},
			helperproto.RegValue{Key: `HKLM\SYSTEM\ControlSet001\Services\nvlddmkm`, Name: marker, Kind: helperproto.RegBinary, Binary: []byte{1}},
			regString(`HKLM\SOFTWARE\NVIDIA Corporation\Global\NGXCore`, "FullPath", `C:\Windows\System32`),
		)
	}
	if proxyPort > 0 {
		// The SDK webview reads the WinINet proxy, not the process environment.
		values = append(values,
			helperproto.RegValue{Key: internetSettings, Name: "ProxyEnable", Kind: helperproto.RegDWord, DWord: 1},
			regString(internetSettings, "ProxyServer", fmt.Sprintf("127.0.0.1:%d", proxyPort)),
			regString(internetSettings, "ProxyOverride", "<local>;localhost;127.0.0.1"),
		)
	}
	return values
}

const (
	fontsKey        = `HKLM\Software\Microsoft\Windows NT\CurrentVersion\Fonts`
	fontSubstitutes = `HKLM\Software\Microsoft\Windows NT\CurrentVersion\FontSubstitutes`
	fontSystemLink  = `HKLM\Software\Microsoft\Windows NT\CurrentVersion\FontLink\SystemLink`
)

// Chinese UI fonts that Windows programs ask for by name, and the file that
// provides each one when it is installed in the prefix.
var cjkFontNames = []struct{ face, file string }{
	{"Microsoft YaHei", "msyh.ttc"},
	{"Microsoft YaHei UI", "msyh.ttc"},
	{"Microsoft YaHei Bold", "msyhbd.ttc"},
	{"SimSun", "simsun.ttc"},
	{"NSimSun", "simsun.ttc"},
	{"SimHei", "simhei.ttf"},
}

// Default UI faces that fall back to the CJK font for characters they lack.
var cjkLinkedFaces = []string{"Tahoma", "Microsoft Sans Serif", "Arial", "Segoe UI"}

// cjkFontRegistry registers the copied CJK font, maps Chinese font names that
// the prefix does not have to it, and adds it as the first fallback of the
// default UI faces. fonts lists the prefix's font files, lowercased.
func cjkFontRegistry(fonts map[string]bool) []helperproto.RegValue {
	values := []helperproto.RegValue{regString(fontsKey, wine.CJKFontFace+" (TrueType)", wine.CJKFontFile)}
	for _, f := range cjkFontNames {
		if !fonts[f.file] {
			values = append(values, regString(fontSubstitutes, f.face, wine.CJKFontFace))
		}
	}
	link := wine.CJKFontFile + "," + wine.CJKFontFace
	for _, face := range cjkLinkedFaces {
		values = append(values, helperproto.RegValue{Key: fontSystemLink, Name: face, Kind: helperproto.RegPrepend, String: link})
	}
	return values
}

// proxyOffRegistry undoes the WinINet proxy once no game in the prefix needs it.
func proxyOffRegistry() []helperproto.RegValue {
	return []helperproto.RegValue{{Key: internetSettings, Name: "ProxyEnable", Kind: helperproto.RegDWord, DWord: 0}}
}

// compatEnvironment adds Wine and Metal variables; the user's own variables win.
func compatEnvironment(cfg Settings, w wine.Config, dxmt bool) map[string]string {
	env := map[string]string{
		// Prefer the macOS decoders for in-game video.
		"GST_PLUGIN_FEATURE_RANK": "atdec:MAX,avdec_h264:MAX",
		"MTL_DEBUG_LAYER":         "0",
		"MTL_SHADER_VALIDATION":   "0",
	}
	if cfg.WineMsync {
		env["WINEMSYNC"] = "1"
	} else {
		env["WINEESYNC"] = "1"
	}
	if cfg.MetalHUD {
		env["MTL_HUD_ENABLED"] = "1"
	}
	if dxmt {
		fps := 60
		if cfg.UnlockFPS {
			fps = cfg.TargetFPS
		}
		// Present as an NVIDIA GPU: the game's renderer picks broken paths otherwise.
		env["DXMT_CONFIG"] = fmt.Sprintf("d3d11.preferredMaxFrameRate=%d;dxgi.customVendorId=10de;dxgi.customDeviceId=2684", fps)
		env["DXMT_ENABLE_NVEXT"] = "1"
	}
	for key, value := range w.Env {
		env[key] = value
	}
	return env
}

// Crash reporters hang under Wine and the bundled Vulkan loader breaks Metal
// translation; they are set aside while Poem's games run and restored afterwards.
// Current clients keep them under Plugins/x86_64; older layouts are kept for compatibility.
var setAsideFiles = []string{
	"StarRail_Data/upload_crash.exe",
	"StarRail_Data/Plugins/x86_64/crashreport.exe",
	"StarRail_Data/Plugins/x86_64/vulkan-1.dll",
	"StarRail_Data/Plugins/crashreport.exe",
	"StarRail_Data/Plugins/vulkan-1.dll",
}

const setAsideSuffix = ".poem-off"

func setAside(game string) {
	dir := filepath.Dir(game)
	for _, name := range setAsideFiles {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if _, err := os.Stat(path + setAsideSuffix); err == nil {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			continue
		}
		_ = os.Rename(path, path+setAsideSuffix)
	}
}

func restoreSetAside(game string) {
	dir := filepath.Dir(game)
	for _, name := range setAsideFiles {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if _, err := os.Stat(path); err == nil {
			continue
		}
		_ = os.Rename(path+setAsideSuffix, path)
	}
}
