// Package wine locates a Wine runtime on macOS and runs Windows programs in a
// chosen prefix. The game, its modules and the Poem helper all run inside the
// same prefix, so they share one wineserver and can see each other's processes.
package wine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Runtime is a detected Wine installation.
type Runtime struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Prefix is a detected Wine prefix (bottle).
type Prefix struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func home() string {
	dir, _ := os.UserHomeDir()
	return dir
}

func executable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0111 != 0
}

// DetectRuntimes lists Wine binaries from common macOS distributions.
func DetectRuntimes() []Runtime {
	h := home()
	candidates := []Runtime{
		{"CrossOver", "/Applications/CrossOver.app/Contents/SharedSupport/CrossOver/bin/wine"},
		{"CrossOver (用户目录)", filepath.Join(h, "Applications/CrossOver.app/Contents/SharedSupport/CrossOver/bin/wine")},
		{"Whisky", filepath.Join(h, "Library/Application Support/com.isaacmarovitz.Whisky/Libraries/Wine/bin/wine64")},
		{"Game Porting Toolkit", "/usr/local/opt/game-porting-toolkit/bin/wine64"},
		{"Wine Stable", "/Applications/Wine Stable.app/Contents/Resources/wine/bin/wine"},
		{"Wine Staging", "/Applications/Wine Staging.app/Contents/Resources/wine/bin/wine"},
		{"Wine Devel", "/Applications/Wine Devel.app/Contents/Resources/wine/bin/wine"},
		{"Homebrew (Apple Silicon)", "/opt/homebrew/bin/wine"},
		{"Homebrew (Apple Silicon)", "/opt/homebrew/bin/wine64"},
		{"Homebrew (Intel)", "/usr/local/bin/wine"},
		{"Homebrew (Intel)", "/usr/local/bin/wine64"},
	}
	result := []Runtime{}
	seen := map[string]bool{}
	for _, c := range candidates {
		if !executable(c.Path) {
			continue
		}
		real, err := filepath.EvalSymlinks(c.Path)
		if err != nil {
			real = c.Path
		}
		if seen[real] {
			continue
		}
		seen[real] = true
		result = append(result, c)
	}
	return result
}

// DetectPrefixes lists prefixes created by CrossOver, Whisky or plain Wine.
func DetectPrefixes() []Prefix {
	h := home()
	result := []Prefix{}
	add := func(name, path string) {
		if ValidPrefix(path) {
			result = append(result, Prefix{name, path})
		}
	}
	add("默认 (~/.wine)", filepath.Join(h, ".wine"))
	for _, root := range []struct{ label, dir string }{
		{"CrossOver", filepath.Join(h, "Library/Application Support/CrossOver/Bottles")},
		{"Whisky", filepath.Join(h, "Library/Containers/com.isaacmarovitz.Whisky/Bottles")},
	} {
		entries, err := os.ReadDir(root.dir)
		if err != nil {
			continue
		}
		names := []string{}
		for _, e := range entries {
			if e.IsDir() {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			add(root.label+" · "+bottleName(filepath.Join(root.dir, name)), filepath.Join(root.dir, name))
		}
	}
	return result
}

// Whisky names bottle directories by UUID; its display name is in Metadata.plist.
func bottleName(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "Metadata.plist"))
	if err == nil {
		text := string(b)
		if at := strings.Index(text, "<key>name</key>"); at >= 0 {
			rest := text[at:]
			if start := strings.Index(rest, "<string>"); start >= 0 {
				rest = rest[start+len("<string>"):]
				if end := strings.Index(rest, "</string>"); end > 0 {
					return rest[:end]
				}
			}
		}
	}
	return filepath.Base(dir)
}

// ValidPrefix reports whether dir looks like an initialised Wine prefix.
func ValidPrefix(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "drive_c"))
	return err == nil && info.IsDir()
}

// WindowsPath maps a macOS path to the path Wine programs in prefix see.
// Files inside drive_c use C:, everything else goes through Wine's Z: drive.
func WindowsPath(prefix, path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if prefix != "" {
		driveC := filepath.Join(prefix, "drive_c")
		if rel, err := filepath.Rel(driveC, abs); err == nil && rel != ".." && !strings.HasPrefix(rel, "../") {
			if rel == "." {
				return `C:\`
			}
			return `C:\` + strings.ReplaceAll(rel, "/", `\`)
		}
	}
	return "Z:" + strings.ReplaceAll(abs, "/", `\`)
}

// ParseEnv reads KEY=VALUE lines; blank lines and # comments are ignored.
func ParseEnv(text string) (map[string]string, error) {
	result := map[string]string{}
	for n, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" || strings.ContainsAny(key, " \t") {
			return nil, fmt.Errorf("第 %d 行环境变量格式应为 KEY=VALUE", n+1)
		}
		result[key] = strings.TrimSpace(value)
	}
	return result, nil
}

// Config selects the Wine binary, prefix and any extra environment.
type Config struct {
	Binary string
	Prefix string
	Env    map[string]string
}

// Validate checks the configured binary and prefix exist.
func (c Config) Validate() error {
	if c.Binary == "" {
		return fmt.Errorf("请先在「运行环境」中选择 Wine")
	}
	if !executable(c.Binary) {
		return fmt.Errorf("Wine 程序不存在或不可执行：%s", c.Binary)
	}
	if c.Prefix == "" {
		return fmt.Errorf("请先在「运行环境」中选择 Wine 容器（prefix）")
	}
	if !ValidPrefix(c.Prefix) {
		return fmt.Errorf("所选目录不是有效的 Wine 容器（缺少 drive_c）：%s", c.Prefix)
	}
	return nil
}

// Command prepares `wine program args...` for the configured prefix.
func (c Config) Command(program string, args ...string) *exec.Cmd {
	cmd := exec.Command(c.Binary, append([]string{program}, args...)...)
	cmd.Env = c.environ()
	return cmd
}

func (c Config) environ() []string {
	env := map[string]string{}
	for _, entry := range os.Environ() {
		if key, value, ok := strings.Cut(entry, "="); ok {
			env[key] = value
		}
	}
	env["WINEPREFIX"] = c.Prefix
	if _, ok := env["WINEDEBUG"]; !ok {
		env["WINEDEBUG"] = "-all"
	}
	// CrossOver's wine wrapper selects bottles by name rather than WINEPREFIX.
	if strings.Contains(c.Binary, "CrossOver.app") && filepath.Base(filepath.Dir(c.Prefix)) == "Bottles" {
		env["CX_BOTTLE"] = filepath.Base(c.Prefix)
	}
	for key, value := range c.Env {
		env[key] = value
	}
	result := make([]string, 0, len(env))
	for key, value := range env {
		result = append(result, key+"="+value)
	}
	sort.Strings(result)
	return result
}
