package launcher

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"poem/internal/helperproto"
	"poem/internal/netproxy"
	"poem/internal/wine"
)

type Settings struct {
	LoginChannel       string   `json:"loginChannel"`
	LuaEnabled         bool     `json:"luaEnabled"`
	LuaScripts         []string `json:"luaScripts"`
	LuaDisabledScripts []string `json:"luaDisabledScripts"`
	RemoveBlur         bool     `json:"removeBlur"`
	TextLanguage       string   `json:"textLanguage"`
	VoiceLanguage      string   `json:"voiceLanguage"`
	GamePath           string   `json:"gamePath"`
	ProxyEnabled       bool     `json:"proxyEnabled"`
	ServerURL          string   `json:"serverURL"`
	ProxyPort          int      `json:"proxyPort"`
	RSAPatch           bool     `json:"rsaPatch"`
	RSAKey             string   `json:"rsaKey"`
	WebRedirect        bool     `json:"webRedirect"`
	WebHosts           string   `json:"webHosts"`
	TimeStop           bool     `json:"timeStop"`
	TimeStopKey        int      `json:"timeStopKey"`
	UnlockFPS          bool     `json:"unlockFPS"`
	TargetFPS          int      `json:"targetFPS"`
	MultiInstance      bool     `json:"multiInstance"`
	TouchUI            bool     `json:"touchUI"`
	Instances          int      `json:"instances"`
	WinePath           string   `json:"winePath"`
	WinePrefix         string   `json:"winePrefix"`
	WineEnv            string   `json:"wineEnv"`
}

type Status struct {
	Running int    `json:"running"`
	Ports   []int  `json:"ports"`
	Message string `json:"message"`
}

type session struct {
	port      int
	channel   string
	game      string
	moduleDir string
	luaLoaded bool
	wine      wine.Config
	blurFPS   int
}

type Service struct {
	hook, features []byte
	blur           []byte
	lua            []byte
	helper         []byte
	mu             sync.Mutex
	saveMu         sync.Mutex
	sessions       map[uint32]*session
}

// Modules holds the native modules. Helper and Lua are built with Poem; the
// three prebuilt modules are read from the user's modules folder at launch.
type Modules struct {
	Helper []byte // poem-helper.exe: process creation and injection inside Wine
	Lua    []byte // LuaEntry.dll: local Lua bridge
}

// Prebuilt modules the user supplies, by feature.
const (
	moduleProxy    = "ProxyHook.dll"
	moduleFeatures = "GameFeatures.dll"
	moduleVisual   = "VisualPatch.dll"
)

func New(m Modules) *Service {
	return &Service{helper: m.Helper, lua: m.Lua, sessions: map[uint32]*session{}}
}

func defaults() Settings {
	return Settings{ServerURL: "http://127.0.0.1:21000", ProxyEnabled: true, ProxyPort: 8080, RSAPatch: true, WebRedirect: true, TimeStop: true, UnlockFPS: true, MultiInstance: true, TimeStopKey: 0x20, TargetFPS: 120, Instances: 1}
}

func normalizeSettings(cfg Settings) Settings {
	cfg.Instances = 1
	return cfg
}

// macOS app bundles are read-only once signed; keep data in Application Support.
func dataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "Poem")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	netproxy.SetStorageDir(filepath.Join(dir, "proxy"))
	return dir, nil
}

func modulesDir() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "modules")
	return dir, os.MkdirAll(dir, 0755)
}

func (s *Service) LoadSettings() (Settings, error) {
	dir, err := dataDir()
	if err != nil {
		return Settings{}, err
	}
	cfg := defaults()
	b, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if os.IsNotExist(err) {
		return autoWine(cfg), nil
	}
	if err != nil {
		return cfg, err
	}
	err = json.Unmarshal(b, &cfg)
	return normalizeSettings(cfg), err
}

// First run: preselect the only detected runtime and prefix.
func autoWine(cfg Settings) Settings {
	if runtimes := wine.DetectRuntimes(); len(runtimes) > 0 {
		cfg.WinePath = runtimes[0].Path
	}
	if prefixes := wine.DetectPrefixes(); len(prefixes) == 1 {
		cfg.WinePrefix = prefixes[0].Path
	}
	return cfg
}

func wineConfig(cfg Settings) (wine.Config, error) {
	env, err := wine.ParseEnv(cfg.WineEnv)
	if err != nil {
		return wine.Config{}, err
	}
	return wine.Config{Binary: strings.TrimSpace(cfg.WinePath), Prefix: strings.TrimSpace(cfg.WinePrefix), Env: env}, nil
}

func validate(cfg Settings, launching bool) error {
	for _, language := range []string{cfg.TextLanguage, cfg.VoiceLanguage} {
		if language != "" && language != "cn" && language != "en" && language != "jp" && language != "kr" {
			return errors.New("语言仅支持中文、英文、日文和韩文")
		}
	}
	if cfg.ProxyPort < 0 || cfg.ProxyPort > 65535 {
		return errors.New("代理端口须在 0–65535 之间")
	}
	if cfg.TargetFPS < 30 || cfg.TargetFPS > 1000 {
		return errors.New("帧率上限须在 30–1000 之间")
	}
	if cfg.TimeStopKey < 1 || cfg.TimeStopKey > 254 {
		return errors.New("请设置有效的时停按键")
	}
	if cfg.Instances < 1 || cfg.Instances > 8 {
		return errors.New("启动数量须在 1–8 之间")
	}
	if cfg.Instances > 1 && !cfg.MultiInstance {
		return errors.New("启动多个窗口前请启用解除多开限制")
	}
	if cfg.ProxyEnabled {
		u, err := url.Parse(strings.TrimSpace(cfg.ServerURL))
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
			return errors.New("请输入完整的 HTTP/HTTPS 服务器地址，不含用户名、查询或片段")
		}
		if cfg.RSAPatch {
			if _, err := netproxy.NormalizeRSAKey(cfg.RSAKey); err != nil {
				return err
			}
		}
	}
	if _, err := wine.ParseEnv(cfg.WineEnv); err != nil {
		return err
	}
	if launching {
		if !strings.EqualFold(filepath.Base(cfg.GamePath), "StarRail.exe") {
			return errors.New("请选择游戏目录中的 StarRail.exe")
		}
		if info, err := os.Stat(cfg.GamePath); err != nil || info.IsDir() {
			return errors.New("游戏程序不存在，请重新选择路径")
		}
		if info, err := os.Stat(filepath.Join(filepath.Dir(cfg.GamePath), "StarRail_Data")); err != nil || !info.IsDir() {
			return errors.New("所选路径缺少 StarRail_Data 游戏目录")
		}
		w, _ := wineConfig(cfg)
		if err := w.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) SaveSettings(cfg Settings) error {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	cfg = normalizeSettings(cfg)
	dir, err := dataDir()
	if err != nil {
		return err
	}
	cfg.ServerURL = strings.TrimSpace(cfg.ServerURL)
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "settings.json"), b, 0600)
}

func (s *Service) GetStatus() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked("")
}

func (s *Service) statusLocked(message string) Status {
	ports := []int{}
	for _, session := range s.sessions {
		if session.port > 0 {
			ports = append(ports, session.port)
		}
	}
	return Status{Running: len(s.sessions), Ports: ports, Message: message}
}

// ModuleState reports whether a user-supplied module is present.
type ModuleState struct {
	Name    string `json:"name"`
	Feature string `json:"feature"`
	Present bool   `json:"present"`
}

// Environment is what the 运行环境 panel shows.
type Environment struct {
	Runtimes   []wine.Runtime `json:"runtimes"`
	Prefixes   []wine.Prefix  `json:"prefixes"`
	Modules    []ModuleState  `json:"modules"`
	ModulesDir string         `json:"modulesDir"`
	DataDir    string         `json:"dataDir"`
}

func (s *Service) GetEnvironment() (Environment, error) {
	data, err := dataDir()
	if err != nil {
		return Environment{}, err
	}
	modules, err := modulesDir()
	if err != nil {
		return Environment{}, err
	}
	env := Environment{Runtimes: wine.DetectRuntimes(), Prefixes: wine.DetectPrefixes(), ModulesDir: modules, DataDir: data}
	for _, m := range []struct{ name, feature string }{
		{moduleProxy, "私服网络代理"},
		{moduleFeatures, "时停 / 帧率 / 多开 / 移动端 UI"},
		{moduleVisual, "角色去虚化"},
	} {
		info, err := os.Stat(filepath.Join(modules, m.name))
		env.Modules = append(env.Modules, ModuleState{Name: m.name, Feature: m.feature, Present: err == nil && !info.IsDir() && info.Size() > 0})
	}
	return env, nil
}

// Read the user-supplied modules once per launch so replacements take effect.
func (s *Service) loadModules(cfg Settings, featuresOn bool) error {
	dir, err := modulesDir()
	if err != nil {
		return err
	}
	read := func(name string, needed bool) ([]byte, error) {
		if !needed {
			return nil, nil
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("缺少功能模块 %s，请放入「运行环境」中的模块文件夹，或关闭对应功能", name)
		}
		return b, nil
	}
	if s.hook, err = read(moduleProxy, cfg.ProxyEnabled); err != nil {
		return err
	}
	if s.features, err = read(moduleFeatures, featuresOn); err != nil {
		return err
	}
	s.blur, err = read(moduleVisual, cfg.RemoveBlur)
	return err
}

func writeModule(path string, b []byte) error {
	if len(b) == 0 {
		return fmt.Errorf("缺少功能模块 %s", filepath.Base(path))
	}
	existing, err := os.ReadFile(path)
	if err == nil && sha256.Sum256(existing) == sha256.Sum256(b) {
		return nil
	}
	return os.WriteFile(path, b, 0644)
}

// The helper is shared by every launch; rewrite it only when it changed.
func (s *Service) helperPath() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	if len(s.helper) == 0 {
		return "", errors.New("此构建未包含 poem-helper.exe")
	}
	dir = filepath.Join(dir, "bin")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "poem-helper.exe")
	return path, writeModule(path, s.helper)
}

// The game-features DLL reads <game>/StarRail_Data/config.ini synchronously in DllMain.
// Temporarily merge just our keys, then restore the original bytes after loading.
func featureConfig(cfg Settings) string {
	boolean := func(v bool) string {
		if v {
			return "True"
		}
		return "False"
	}
	return fmt.Sprintf("CompatibilityMode=False\nUnlockMultiInstance=%s\nEnableTouchUI=%s\nEnableBattleTimeStop=%s\nBattleTimeStopKey=0x%X\nUnlockFPS=%s\nTargetFPS=%d\n", boolean(cfg.MultiInstance), boolean(cfg.TouchUI), boolean(cfg.TimeStop), cfg.TimeStopKey, boolean(cfg.UnlockFPS), cfg.TargetFPS)
}

func stageConfig(path string, cfg Settings) (func() error, error) {
	old, err := os.ReadFile(path)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	keys := map[string]bool{"CompatibilityMode": true, "UnlockMultiInstance": true, "EnableTouchUI": true, "EnableBattleTimeStop": true, "BattleTimeStopKey": true, "UnlockFPS": true, "TargetFPS": true}
	lines := []string{}
	for _, line := range strings.Split(strings.ReplaceAll(string(old), "\r\n", "\n"), "\n") {
		key, _, _ := strings.Cut(line, "=")
		if !keys[strings.TrimSpace(key)] {
			lines = append(lines, line)
		}
	}
	merged := strings.Join(lines, "\n") + "\n" + featureConfig(cfg)
	if err := os.WriteFile(path, []byte(merged), 0644); err != nil {
		return nil, err
	}
	return func() error {
		if exists {
			return os.WriteFile(path, old, 0644)
		}
		err := os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}, nil
}

// Start starts a batch serially so every module reads the intended settings.
func (s *Service) Start(cfg Settings) (Status, error) {
	if cfg.LoginChannel != "" && cfg.LoginChannel != "private" && cfg.LoginChannel != "official" && cfg.LoginChannel != "bilibili" {
		return Status{}, errors.New("未知登录渠道")
	}
	cfg = normalizeSettings(cfg)
	savedSettings := cfg
	cfg, retail := channelRuntimeSettings(cfg)
	if err := validate(cfg, true); err != nil {
		return Status{}, err
	}
	wineCfg, err := wineConfig(cfg)
	if err != nil {
		return Status{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sessions) > 0 && !cfg.MultiInstance {
		return s.statusLocked(""), errors.New("游戏已在运行；继续启动需要解除多开限制")
	}
	blurFPS := 0
	if cfg.RemoveBlur {
		blurFPS = cfg.TargetFPS
		if !cfg.UnlockFPS {
			blurFPS = 60
		}
	}
	for _, existing := range s.sessions {
		if existing.channel != cfg.LoginChannel || !strings.EqualFold(existing.game, cfg.GamePath) {
			return s.statusLocked(""), errors.New("多开请使用相同渠道和游戏目录；切换渠道前请退出已有游戏")
		}
		if existing.wine.Prefix != wineCfg.Prefix {
			return s.statusLocked(""), errors.New("多开请使用同一个 Wine 容器")
		}
		if blurFPS > 0 && existing.blurFPS > 0 && existing.blurFPS != blurFPS {
			return s.statusLocked(""), errors.New("去虚化多开窗口共用帧率，请保持相同帧率或先退出已有游戏")
		}
	}
	if err := s.SaveSettings(savedSettings); err != nil {
		return s.statusLocked(""), err
	}
	dir, err := dataDir()
	if err != nil {
		return s.statusLocked(""), err
	}
	featuresOn := cfg.MultiInstance || cfg.TouchUI || cfg.TimeStop || cfg.UnlockFPS
	if err := s.loadModules(cfg, featuresOn); err != nil {
		return s.statusLocked(""), err
	}
	helper, err := s.helperPath()
	if err != nil {
		return s.statusLocked(""), err
	}
	if err = applyLanguage(cfg, dir, len(s.sessions) > 0); err != nil {
		return s.statusLocked(""), err
	}
	if retail {
		if len(s.sessions) == 0 {
			err = prepareRetailChannel(cfg, dir)
		}
	} else {
		err = restorePrivateChannel(cfg.GamePath, dir)
	}
	if err != nil {
		return s.statusLocked(""), err
	}
	moduleDir, err := os.MkdirTemp("", "Poem-runtime-")
	if err != nil {
		return s.statusLocked(""), err
	}
	keepRuntime := false
	defer func() {
		if !keepRuntime {
			cleanupRuntimeModules(moduleDir)
		}
	}()
	win := func(path string) string { return wine.WindowsPath(wineCfg.Prefix, path) }
	featurePath := filepath.Join(moduleDir, "Poem.Game.dll")
	hookPath := filepath.Join(moduleDir, "Poem.Proxy.dll")
	blurPath := filepath.Join(moduleDir, "Poem.Visual.dll")
	hookBytes := s.hook
	if cfg.ProxyEnabled {
		if hookBytes, err = configureProxyThreadNotifications(hookBytes); err != nil {
			return s.statusLocked(""), err
		}
		if cfg.RSAPatch {
			if hookBytes, err = configureHookRSA(hookBytes, cfg.RSAKey); err != nil {
				return s.statusLocked(""), err
			}
			hash := sha256.Sum256(hookBytes)
			hookPath = filepath.Join(moduleDir, fmt.Sprintf("Poem.Proxy-%x.dll", hash[:8]))
		}
		if err = writeModule(hookPath, hookBytes); err != nil {
			return s.statusLocked(""), err
		}
	}
	if featuresOn {
		featureBytes, patchErr := configureFeatureThreadExit(s.features)
		if patchErr != nil {
			return s.statusLocked(""), patchErr
		}
		if err = writeModule(featurePath, featureBytes); err != nil {
			return s.statusLocked(""), err
		}
	}
	if cfg.RemoveBlur {
		if err = writeModule(blurPath, s.blur); err != nil {
			return s.statusLocked(""), err
		}
	}
	logf := func(format string, args ...any) { writeLaunchLog(dir, format, args...) }
	for n := 0; n < cfg.Instances; n++ {
		sessionDir := filepath.Join(dir, "sessions", fmt.Sprintf("%d-%d", time.Now().UnixNano(), n))
		plan := helperproto.Plan{Game: win(cfg.GamePath), Env: proxyRuntimeEnvironment(cfg.ProxyEnabled), BlurFPS: blurFPS}
		port := 0
		var proxy *netproxy.Proxy
		stopProxy := func() {
			if proxy != nil {
				proxy.Stop()
			}
		}
		if cfg.ProxyEnabled {
			if err = os.MkdirAll(sessionDir, 0755); err != nil {
				return s.statusLocked(""), err
			}
			proxy, err = netproxy.New(strings.TrimSpace(cfg.ServerURL), netproxy.PatchOptions{RSAPatch: cfg.RSAPatch, RSAKey: cfg.RSAKey, WebRedirect: cfg.WebRedirect, WebHosts: cfg.WebHosts})
			if err != nil {
				return s.statusLocked(""), err
			}
			ec2b := filepath.Join(sessionDir, "ClientSecretKey.bin")
			proxy.SetEc2bFile(ec2b)
			certFile, certErr := proxy.CA().CertFile()
			if certErr != nil {
				return s.statusLocked(""), fmt.Errorf("代理证书准备失败：%w", certErr)
			}
			plan.TrustCert = win(certFile)
			proxy.SetLogger(logf)
			// Wine shares the host network stack, so the game reaches this loopback port.
			if port, err = proxy.Start(cfg.ProxyPort); err != nil {
				return s.statusLocked(""), err
			}
			plan.Env[hookEnvProxy] = fmt.Sprintf("127.0.0.1:%d", port)
			setGameHTTPProxy(plan.Env, port)
			plan.Env[hookEnvEc2bFile] = win(ec2b)
			plan.Env[hookEnvWebTarget] = strings.TrimSpace(cfg.ServerURL)
			plan.Late = append(plan.Late, win(hookPath))
		}
		var restore func() error
		if featuresOn {
			restore, err = stageConfig(filepath.Join(filepath.Dir(cfg.GamePath), "StarRail_Data", "config.ini"), cfg)
			if err != nil {
				stopProxy()
				return s.statusLocked(""), fmt.Errorf("写入游戏功能配置失败：%w", err)
			}
			plan.Early = append(plan.Early, win(featurePath))
		}
		if cfg.RemoveBlur {
			// The original unlocker loads this module before the main thread.
			// Never initialise its protected loader after the proxy hook starts running.
			plan.Early = append(plan.Early, win(blurPath))
		}
		// Without a late stage everything loads before the main thread resumes.
		if len(plan.Early) == 0 {
			plan.Early, plan.Late = plan.Late, nil
		}
		logf("game=%s prefix=%s early=%v late=%v proxyPort=%d", cfg.GamePath, wineCfg.Prefix, plan.Early, plan.Late, port)
		run, launchErr := startHelper(wineCfg, helper, moduleDir, plan, logf)
		var restoreErr error
		if restore != nil {
			restoreErr = restore()
		}
		if launchErr != nil {
			logf("launch error: %v", launchErr)
			stopProxy()
			return s.statusLocked(""), fmt.Errorf("启动失败：%w", launchErr)
		}
		pid := run.pid
		logf("process resumed pid=%d", pid)
		s.sessions[pid] = &session{port: port, channel: cfg.LoginChannel, game: cfg.GamePath, moduleDir: moduleDir, wine: wineCfg, blurFPS: blurFPS}
		if cfg.LuaEnabled && len(enabledLuaScripts(cfg)) > 0 {
			paths := enabledLuaScripts(cfg)
			go func() {
				if _, luaErr := s.queueLua(pid, paths, true); luaErr != nil {
					logf("automatic Lua pid=%d error=%v", pid, luaErr)
					_ = os.WriteFile(filepath.Join(moduleDir, "lua-status.txt"), []byte("0\nerror\n"+luaErr.Error()), 0600)
				}
			}()
		}
		keepRuntime = true
		go s.monitor(pid, run, proxy, sessionDir, moduleDir, dir)
		if restoreErr != nil {
			return s.statusLocked("游戏已启动，但配置恢复失败"), fmt.Errorf("游戏已启动，原配置恢复失败：%w", restoreErr)
		}
	}
	status := s.statusLocked("游戏已启动")
	application.Get().Event.Emit("poem:status", status)
	return status, nil
}

func writeLaunchLog(dir, format string, args ...any) {
	f, err := os.OpenFile(filepath.Join(dir, "launch.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

func (s *Service) monitor(pid uint32, run *helperRun, proxy *netproxy.Proxy, sessionDir, moduleDir, dir string) {
	exitCode, exitErr := run.wait()
	writeLaunchLog(dir, "process exited pid=%d code=0x%08X error=%v", pid, exitCode, exitErr)
	if luaState := readLuaStatus(pid, moduleDir); luaState.State != "idle" {
		writeLaunchLog(dir, "Lua final pid=%d job=%s state=%s message=%s", pid, luaState.ID, luaState.State, luaState.Message)
	}
	s.mu.Lock()
	delete(s.sessions, pid)
	shared := false
	for _, other := range s.sessions {
		shared = shared || other.moduleDir == moduleDir
	}
	s.mu.Unlock()
	if !shared {
		cleanupRuntimeModules(moduleDir)
	}
	if proxy != nil {
		proxy.Stop()
	}
	// Session directories are created by Poem and contain only its key file.
	_ = os.Remove(filepath.Join(sessionDir, "ClientSecretKey.bin"))
	_ = os.Remove(filepath.Join(sessionDir, "ClientSecretKey.bin.tmp"))
	_ = os.Remove(sessionDir)
	exitMessage := "游戏已退出"
	if exitErr != nil {
		exitMessage = "Poem 助手已退出，无法继续跟踪游戏状态，详情已记录到 launch.log"
	} else if exitCode != 0 {
		exitMessage = fmt.Sprintf("游戏异常退出（0x%08X），详情已记录到 launch.log", exitCode)
	}
	s.mu.Lock()
	status := s.statusLocked(exitMessage)
	s.mu.Unlock()
	application.Get().Event.Emit("poem:status", status)
}
