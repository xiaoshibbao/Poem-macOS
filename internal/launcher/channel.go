package launcher

import (
	"archive/zip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const biliSDKAPI = "https://hyp-api.mihoyo.com/hyp/hyp-connect/api/getGameChannelSDKs?launcher_id=6P5gHMNyK3&language=zh-cn&game_ids[]=EdtUqXfCHh&channel=14&sub_channel=0"

func sdkCache(dir string) string { return filepath.Join(dir, "channels", "bilibili-sdk.zip") }

func officialDownloadURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.User == nil && (u.Hostname() == "mihoyo.com" || strings.HasSuffix(u.Hostname(), ".mihoyo.com") || strings.HasSuffix(u.Hostname(), ".bhsr.com"))
}

// This downloads an official component package, without executing it or
// changing the game. No file hashes or fingerprints are required.
func (s *Service) PrepareBilibiliSDK() (string, error) {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	return downloadBilibiliSDK(dir)
}

func downloadBilibiliSDK(dir string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 8 || !officialDownloadURL(r.URL.String()) {
			return fmt.Errorf("组件下载跳转到了非米哈游地址")
		}
		return nil
	}}
	response, err := client.Get(biliSDKAPI)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", fmt.Errorf("渠道接口返回 HTTP %d", response.StatusCode)
	}
	var api struct {
		Retcode int    `json:"retcode"`
		Message string `json:"message"`
		Data    struct {
			SDKs []struct {
				Game struct {
					Biz string `json:"biz"`
				} `json:"game"`
				Version string `json:"version"`
				Package struct {
					URL string `json:"url"`
				} `json:"channel_sdk_pkg"`
			} `json:"game_channel_sdks"`
		} `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&api); err != nil {
		return "", err
	}
	if api.Retcode != 0 {
		return "", fmt.Errorf("渠道接口错误：%s", api.Message)
	}
	var address, version string
	for _, sdk := range api.Data.SDKs {
		if sdk.Game.Biz == "hkrpg_cn" {
			address = sdk.Package.URL
			version = sdk.Version
			break
		}
	}
	if !officialDownloadURL(address) {
		return "", fmt.Errorf("接口未提供可用的星铁 B服官方组件")
	}
	pkg, err := client.Get(address)
	if err != nil {
		return "", err
	}
	defer pkg.Body.Close()
	if pkg.StatusCode != 200 {
		return "", fmt.Errorf("组件下载返回 HTTP %d", pkg.StatusCode)
	}
	cache := sdkCache(dir)
	if err = os.MkdirAll(filepath.Dir(cache), 0755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(filepath.Dir(cache), "sdk-*.zip")
	if err != nil {
		return "", err
	}
	temp := f.Name()
	defer os.Remove(temp)
	count, copyErr := io.Copy(f, io.LimitReader(pkg.Body, (512<<20)+1))
	closeErr := f.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if count > 512<<20 {
		return "", fmt.Errorf("组件包过大")
	}
	if _, err = readSDKFiles(temp); err != nil {
		return "", err
	}
	if err = os.Rename(temp, cache); err != nil {
		return "", err
	}
	return "B服官方组件已准备，版本 " + version, nil
}

type sdkFile struct {
	Name    string `json:"name"`
	Existed bool   `json:"existed"`
}
type channelBackup struct {
	Active        bool      `json:"active"`
	ConfigExisted bool      `json:"configExisted"`
	Files         []sdkFile `json:"files"`
}

func channelDir(game, dir string) string {
	absolute, _ := filepath.Abs(filepath.Dir(game))
	return filepath.Join(dir, "channel-backups", base64.RawURLEncoding.EncodeToString([]byte(strings.ToLower(absolute))))
}

// Validate archive paths and sizes before touching any installed files.
func readSDKFiles(path string) (map[string][]byte, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	result := map[string][]byte{}
	var total uint64
	for _, f := range archive.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.Clean(filepath.FromSlash(strings.ReplaceAll(f.Name, "\\", "/")))
		if filepath.IsAbs(name) || filepath.VolumeName(name) != "" || name == ".." || strings.HasPrefix(name, ".."+string(os.PathSeparator)) || strings.Contains(name, ":") || f.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("组件包路径无效：%s", f.Name)
		}
		// SDK packages must not replace game executables, assemblies or channel ini.
		lower := strings.ToLower(filepath.ToSlash(name))
		if lower == "deletefiles.txt" {
			continue
		}
		if lower != "sdk_pkg_version" && !strings.HasPrefix(lower, "starrail_data/plugins/") {
			return nil, fmt.Errorf("组件包包含非 SDK 文件：%s", f.Name)
		}
		total += f.UncompressedSize64
		if total > 512<<20 {
			return nil, fmt.Errorf("解压后的组件包过大")
		}
		r, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(r, (512<<20)+1))
		r.Close()
		if err != nil {
			return nil, err
		}
		if len(data) > 512<<20 {
			return nil, fmt.Errorf("组件文件过大")
		}
		if _, ok := result[name]; ok {
			return nil, fmt.Errorf("组件包包含重复路径")
		}
		result[name] = data
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("组件包为空")
	}
	// The current official SDK retires this pre-x86_64-layout plugin.
	result[filepath.FromSlash("StarRail_Data/Plugins/PCGameSDK.dll")] = nil
	return result, nil
}

func changeChannelINI(source []byte, channel string) []byte {
	newline := "\n"
	if strings.Contains(string(source), "\r\n") {
		newline = "\r\n"
	}
	text := strings.TrimPrefix(string(source), "\ufeff")
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	values := "channel=1\nsub_channel=1\ncps=gw_PC"
	if channel == "bilibili" {
		values = "channel=14\nsub_channel=0\ncps=bilibili_PC"
	}
	out := []string{}
	inGeneral := false
	found := false
	for _, line := range lines {
		clean := strings.TrimSpace(line)
		if strings.HasPrefix(clean, "[") && strings.HasSuffix(clean, "]") {
			if inGeneral {
				out = append(out, strings.Split(values, "\n")...)
			}
			inGeneral = strings.EqualFold(clean, "[General]")
			if inGeneral {
				found = true
			}
		}
		key, _, has := strings.Cut(clean, "=")
		if inGeneral && has && (strings.EqualFold(strings.TrimSpace(key), "channel") || strings.EqualFold(strings.TrimSpace(key), "sub_channel") || strings.EqualFold(strings.TrimSpace(key), "cps")) {
			continue
		}
		out = append(out, line)
	}
	if inGeneral {
		out = append(out, strings.Split(values, "\n")...)
	}
	if !found {
		out = append(out, "[General]")
		out = append(out, strings.Split(values, "\n")...)
	}
	return []byte(strings.Join(out, newline))
}

// Wine shows a Windows process with its Windows path (C:\… or Z:\…) as the
// command; any StarRail.exe in any prefix counts as running.
func gameIsRunning(game string) bool {
	out, err := exec.Command("ps", "-axo", "command").Output()
	if err != nil {
		return true
	}
	return windowsProcessListed(string(out), filepath.Base(game))
}

// Only a backslash path ending in the executable matches, so other command
// lines that merely mention the name (such as a grep) do not.
func windowsProcessListed(list, name string) bool {
	needle := `\` + strings.ToLower(name)
	for _, line := range strings.Split(strings.ToLower(list), "\n") {
		for _, field := range strings.Fields(line) {
			if strings.HasSuffix(strings.Trim(field, `"`), needle) {
				return true
			}
		}
	}
	return false
}

func restoreSDK(game, backup string, manifest channelBackup) error {
	for i, f := range manifest.Files {
		target := filepath.Join(filepath.Dir(game), f.Name)
		if f.Existed {
			data, err := os.ReadFile(filepath.Join(backup, fmt.Sprintf("sdk-%d", i)))
			if err != nil {
				return err
			}
			if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if err = os.WriteFile(target, data, 0644); err != nil {
				return err
			}
		} else if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func prepareRetailChannel(cfg Settings, dir string) error {
	if gameIsRunning(cfg.GamePath) {
		return fmt.Errorf("请先退出使用此目录的游戏，再切换渠道")
	}
	backup := channelDir(cfg.GamePath, dir)
	manifestPath := filepath.Join(backup, "manifest.json")
	var manifest channelBackup
	data, err := os.ReadFile(manifestPath)
	if os.IsNotExist(err) {
		if err = os.MkdirAll(backup, 0755); err != nil {
			return err
		}
		original, e := os.ReadFile(filepath.Join(filepath.Dir(cfg.GamePath), "config.ini"))
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		manifest.ConfigExisted = e == nil
		if err = os.WriteFile(filepath.Join(backup, "config.original"), original, 0600); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if err = json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	if !manifest.Active {
		original, e := os.ReadFile(filepath.Join(filepath.Dir(cfg.GamePath), "config.ini"))
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		manifest = channelBackup{ConfigExisted: e == nil}
		if err = os.WriteFile(filepath.Join(backup, "config.original"), original, 0600); err != nil {
			return err
		}
	}
	var files map[string][]byte
	if cfg.LoginChannel == "bilibili" {
		files, err = readSDKFiles(sdkCache(dir))
		if err != nil {
			return fmt.Errorf("请先在渠道设置中下载 B服官方组件：%w", err)
		}
		// Back up each original before the manifest authorizes any overwrite.
		for name := range files {
			known := false
			for _, f := range manifest.Files {
				if strings.EqualFold(f.Name, name) {
					known = true
					break
				}
			}
			if known {
				continue
			}
			original, e := os.ReadFile(filepath.Join(filepath.Dir(cfg.GamePath), name))
			if e != nil && !os.IsNotExist(e) {
				return e
			}
			index := len(manifest.Files)
			if e == nil {
				if err = os.WriteFile(filepath.Join(backup, fmt.Sprintf("sdk-%d", index)), original, 0600); err != nil {
					return err
				}
			}
			manifest.Files = append(manifest.Files, sdkFile{Name: name, Existed: e == nil})
		}
	}
	manifest.Active = true
	data, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(manifestPath, data, 0600); err != nil {
		return err
	}
	if err = restoreSDK(cfg.GamePath, backup, manifest); err != nil {
		return err
	}
	for name, data := range files {
		if data == nil {
			err = os.Remove(filepath.Join(filepath.Dir(cfg.GamePath), name))
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		path := filepath.Join(filepath.Dir(cfg.GamePath), name)
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err = os.WriteFile(path, data, 0644); err != nil {
			_ = restoreSDK(cfg.GamePath, backup, manifest)
			return err
		}
	}
	config := filepath.Join(filepath.Dir(cfg.GamePath), "config.ini")
	current, err := os.ReadFile(config)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(config, changeChannelINI(current, cfg.LoginChannel), 0644)
}

func restorePrivateChannel(game, dir string) error {
	backup := channelDir(game, dir)
	data, err := os.ReadFile(filepath.Join(backup, "manifest.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var manifest channelBackup
	if err = json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	if !manifest.Active {
		return nil
	}
	if gameIsRunning(game) {
		return fmt.Errorf("切回私服前请先退出此游戏目录的进程")
	}
	if err = restoreSDK(game, backup, manifest); err != nil {
		return err
	}
	config := filepath.Join(filepath.Dir(game), "config.ini")
	if manifest.ConfigExisted {
		original, e := os.ReadFile(filepath.Join(backup, "config.original"))
		if e != nil {
			return e
		}
		err = os.WriteFile(config, original, 0644)
	} else {
		err = os.Remove(config)
		if os.IsNotExist(err) {
			err = nil
		}
	}
	if err != nil {
		return err
	}
	manifest.Active = false
	data, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(backup, "manifest.json"), data, 0600)
}

// Keep official-channel game features and local Lua while disabling
// private-server traffic handling. This is a runtime copy; saved
// private-server preferences remain.
func channelRuntimeSettings(cfg Settings) (Settings, bool) {
	retail := cfg.LoginChannel == "official" || cfg.LoginChannel == "bilibili"
	if cfg.LoginChannel == "" {
		cfg.LoginChannel = "private"
	}
	if retail {
		cfg.ProxyEnabled = false
		cfg.RSAPatch = false
		cfg.WebRedirect = false
	}
	return cfg, retail
}
