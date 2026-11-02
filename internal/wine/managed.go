package wine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Distribution is a Wine build Poem can download and manage itself.
type Distribution struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"-"`
	// DXMT builds can take the Direct3D 11 → Metal layer into their lib directory.
	DXMT bool `json:"dxmt"`
}

// Distributions lists the builds offered in the 运行环境 panel; the first is the default.
// All are signed macOS builds, so they start without Gatekeeper prompts once unquarantined.
var Distributions = []Distribution{
	{"wine-11.8-signed", "Wine 11.8（签名版，推荐）", "https://github.com/yaagl/anime-game-wine/releases/download/wine-11.8-signed/wine-devel-11.8-osx64-signed.tar.xz", true},
	{"wine-crossover-11.0-signed", "Wine CrossOver 11.0（签名版）", "https://github.com/yaagl/anime-game-wine/releases/download/wine-crossover-11.0-1-signed/wine-crossover-11.0-1-osx64-signed.tar.xz", true},
	{"wine-stable-11.0-signed", "Wine 11.0 Stable（签名版）", "https://github.com/dawn-winery/dawn-signed/releases/download/wine-stable-gcenx-11.0-osx64/wine-stable-11.0-osx64-signed.tar.xz", true},
}

// FindDistribution returns the distribution with id, or the default.
func FindDistribution(id string) Distribution {
	for _, d := range Distributions {
		if d.ID == id {
			return d
		}
	}
	return Distributions[0]
}

// Progress reports a stage and, while downloading, a fraction in [0,1] (-1 when unknown).
type Progress func(stage string, fraction float64)

// Managed is the layout of a Poem-managed Wine under dir.
type Managed struct{ Dir string }

func (m Managed) Root() string   { return filepath.Join(m.Dir, "wine") }
func (m Managed) Prefix() string { return filepath.Join(m.Dir, "prefix") }

// Binary returns the managed wine executable, preferring wine64 when present.
func (m Managed) Binary() string {
	return binaryIn(m.Root())
}

func binaryIn(root string) string {
	for _, name := range []string{"wine64", "wine"} {
		if path := filepath.Join(root, "bin", name); executable(path) {
			return path
		}
	}
	return filepath.Join(root, "bin", "wine")
}

// Installed reports which distribution is installed, or "".
func (m Managed) Installed() string {
	b, err := os.ReadFile(filepath.Join(m.Root(), ".poem-distribution"))
	if err != nil || !executable(m.Binary()) {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func download(ctx context.Context, url, dest string, progress Progress, stage string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 30 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败：HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	total := resp.ContentLength
	var done int64
	buf := make([]byte, 256<<10)
	last := time.Time{}
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}
			done += int64(n)
			if time.Since(last) > 200*time.Millisecond {
				last = time.Now()
				fraction := -1.0
				if total > 0 {
					fraction = float64(done) / float64(total)
				}
				progress(stage, fraction)
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

// findRoot locates the directory containing bin/wine(64) inside an extracted archive.
func findRoot(dir string) (string, error) {
	var found string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return err
		}
		if !d.IsDir() && (d.Name() == "wine" || d.Name() == "wine64") && filepath.Base(filepath.Dir(path)) == "bin" {
			found = filepath.Dir(filepath.Dir(path))
			return filepath.SkipAll
		}
		return nil
	})
	if err == nil && found == "" {
		err = fmt.Errorf("压缩包中没有找到 bin/wine")
	}
	return found, err
}

// Install downloads dist into the managed directory, replacing any earlier build,
// then makes sure the managed prefix exists.
func (m Managed) Install(ctx context.Context, dist Distribution, progress Progress) error {
	if err := os.MkdirAll(m.Dir, 0755); err != nil {
		return err
	}
	work, err := os.MkdirTemp(m.Dir, "download-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	archive := filepath.Join(work, filepath.Base(dist.URL))
	if err := download(ctx, dist.URL, archive, progress, "正在下载 "+dist.Name); err != nil {
		return err
	}
	progress("正在解压 Wine", -1)
	extracted := filepath.Join(work, "x")
	if err := os.Mkdir(extracted, 0755); err != nil {
		return err
	}
	if out, err := exec.CommandContext(ctx, "/usr/bin/tar", "-xf", archive, "-C", extracted).CombinedOutput(); err != nil {
		return fmt.Errorf("解压失败：%s", strings.TrimSpace(string(out)))
	}
	root, err := findRoot(extracted)
	if err != nil {
		return err
	}
	// Downloaded files carry quarantine; Gatekeeper would block every Wine binary.
	_ = exec.Command("/usr/bin/xattr", "-dr", "com.apple.quarantine", root).Run()
	if err := os.RemoveAll(m.Root()); err != nil {
		return err
	}
	if err := os.Rename(root, m.Root()); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(m.Root(), ".poem-distribution"), []byte(dist.ID), 0644); err != nil {
		return err
	}
	if dist.DXMT {
		if err := InstallDXMT(ctx, m.Dir, m.Root(), progress); err != nil {
			return fmt.Errorf("DXMT 安装失败：%w", err)
		}
	}
	return m.InitPrefix(ctx, progress)
}

// InitPrefix creates or updates the managed prefix and selects Windows 10.
func (m Managed) InitPrefix(ctx context.Context, progress Progress) error {
	progress("正在初始化 Wine 容器（首次可能需要一两分钟）", -1)
	if err := os.MkdirAll(m.Prefix(), 0755); err != nil {
		return err
	}
	cfg := Config{Binary: m.Binary(), Prefix: m.Prefix()}
	for _, args := range [][]string{{"wineboot", "-u"}, {"winecfg", "-v", "win10"}} {
		cmd := cfg.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s 失败：%v %s", args[0], err, lastLine(out))
		}
	}
	cfg.WaitServer(2 * time.Minute)
	if dxmtVersion(m.Root()) != "" {
		if err := placeDXMTPrefix(m.Dir, m.Prefix()); err != nil {
			return err
		}
	}
	return nil
}

func lastLine(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return lines[len(lines)-1]
}

// WaitServer waits for this prefix's wineserver to exit, up to timeout.
func (c Config) WaitServer(timeout time.Duration) {
	server := filepath.Join(filepath.Dir(c.Binary), "wineserver")
	if !executable(server) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, server, "-w")
	cmd.Env = c.environ()
	_ = cmd.Run()
}
