package launcher

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeSDK(t *testing.T, path string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for name, text := range files {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestChannelINIPreservesOtherValues(t *testing.T) {
	source := []byte("[General]\r\ngame_version=4.6.0\r\nchannel=1\r\ncps=gw_PC\r\nsub_channel=1\r\n[Other]\r\nchannel=99\r\n")
	bili := string(changeChannelINI(source, "bilibili"))
	for _, want := range []string{"game_version=4.6.0", "channel=14", "cps=bilibili_PC", "sub_channel=0", "[Other]\r\nchannel=99"} {
		if !strings.Contains(bili, want) {
			t.Fatalf("lost %s: %s", want, bili)
		}
	}
	official := string(changeChannelINI([]byte(bili), "official"))
	if !strings.Contains(official, "cps=gw_PC") || strings.Contains(official, "cps=bilibili_PC") {
		t.Fatal(official)
	}
}

func TestRetailKeepsGameFeaturesWithoutPrivateProxy(t *testing.T) {
	for _, channel := range []string{"official", "bilibili"} {
		saved := Settings{LoginChannel: channel, ProxyEnabled: true, RSAPatch: true, WebRedirect: true, LuaEnabled: true, TimeStop: true, UnlockFPS: true, MultiInstance: true, TouchUI: true, RemoveBlur: true, TargetFPS: 240}
		runtime, retail := channelRuntimeSettings(saved)
		if !retail || runtime.ProxyEnabled || runtime.RSAPatch || runtime.WebRedirect {
			t.Fatal("private modules enabled on official channels")
		}
		if !runtime.LuaEnabled || !runtime.TimeStop || !runtime.UnlockFPS || !runtime.MultiInstance || !runtime.TouchUI || !runtime.RemoveBlur || runtime.TargetFPS != 240 {
			t.Fatal("game features lost")
		}
		if !saved.ProxyEnabled || !saved.LuaEnabled {
			t.Fatal("saved preferences changed")
		}
	}
}

func TestChannelSDKSwitchAndPrivateRestore(t *testing.T) {
	root := t.TempDir()
	gameDir := filepath.Join(root, "game")
	dir := filepath.Join(root, "Poem-data")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}
	game := filepath.Join(gameDir, "StarRail.exe")
	original := []byte("[General]\ngame_version=4.6.0\ncps=custom\nchannel=1\nsub_channel=1\n")
	if err := os.WriteFile(filepath.Join(gameDir, "config.ini"), original, 0644); err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(gameDir, "StarRail_Data", "Plugins", "x86_64", "PCGameSDK.dll")
	if err := os.MkdirAll(filepath.Dir(plugin), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plugin, []byte("original SDK"), 0644); err != nil {
		t.Fatal(err)
	}
	makeSDK(t, sdkCache(dir), map[string]string{"StarRail_Data/Plugins/x86_64/PCGameSDK.dll": "bili SDK", "StarRail_Data/Plugins/x86_64/BLPlatform64/new.dat": "new", "sdk_pkg_version": "metadata"})
	cfg := Settings{GamePath: game, LoginChannel: "bilibili"}
	if err := prepareRetailChannel(cfg, dir); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(plugin); string(b) != "bili SDK" {
		t.Fatalf("SDK not installed: %s", b)
	}
	cfg.LoginChannel = "official"
	if err := prepareRetailChannel(cfg, dir); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(plugin); string(b) != "original SDK" {
		t.Fatalf("SDK not restored: %s", b)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(plugin), "BLPlatform64", "new.dat")); !os.IsNotExist(err) {
		t.Fatal("Bili-only file remains enabled")
	}
	if err := restorePrivateChannel(game, dir); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(gameDir, "config.ini")); string(b) != string(original) {
		t.Fatalf("private ini not restored: %s", b)
	}
	// A subsequent private start must not overwrite newly updated configuration.
	updated := []byte("[General]\ngame_version=4.7.0\n")
	if err := os.WriteFile(filepath.Join(gameDir, "config.ini"), updated, 0644); err != nil {
		t.Fatal(err)
	}
	if err := restorePrivateChannel(game, dir); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(gameDir, "config.ini")); string(b) != string(updated) {
		t.Fatal("inactive backup overwrote update")
	}
}

func TestChannelArchiveRejectsTraversal(t *testing.T) {
	for _, name := range []string{"../outside.dll", "StarRail.exe", "StarRail_Data/Plugins/../../../outside.dll"} {
		path := filepath.Join(t.TempDir(), "sdk.zip")
		makeSDK(t, path, map[string]string{name: "bad"})
		if _, err := readSDKFiles(path); err == nil {
			t.Fatalf("unsafe archive path accepted: %s", name)
		}
	}
	if officialDownloadURL("https://mihoyo.com.bad.example/sdk.zip") || officialDownloadURL("http://launcher-webstatic.mihoyo.com/sdk.zip") {
		t.Fatal("non-official URL accepted")
	}
}

func TestRealOfficialSDKArchiveLayout(t *testing.T) {
	path := os.Getenv("POEM_BILI_SDK_FIXTURE")
	if path == "" {
		t.Skip("official SDK archive fixture not configured")
	}
	files, err := readSDKFiles(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(files[filepath.FromSlash("StarRail_Data/Plugins/x86_64/PCGameSDK.dll")]) == 0 {
		t.Fatal("official SDK DLL missing")
	}
	t.Logf("official Bili SDK: %d managed files", len(files))
}

func TestWindowsProcessListedIgnoresMentions(t *testing.T) {
	list := "/usr/bin/grep StarRail.exe\nvim notes-starrail.exe.md\n"
	if windowsProcessListed(list, "StarRail.exe") {
		t.Fatal("a command merely mentioning the name was treated as the game")
	}
	for _, game := range []string{`C:\Program Files\Star Rail\Game\StarRail.exe -disable-gpu-skinning`, `Z:\Users\a\Games\StarRail.exe`, `"C:\Games\StarRail.exe"`} {
		if !windowsProcessListed("init\n"+game+"\n", "StarRail.exe") {
			t.Fatalf("Wine game process not detected: %s", game)
		}
	}
}
