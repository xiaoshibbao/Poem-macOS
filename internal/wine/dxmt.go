package wine

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DXMT translates Direct3D 11 to Metal. Builtin releases (0.74+) replace the
// Wine build's own d3d11/dxgi, so it is installed only into the managed Wine.
const (
	dxmtVersionTag = "0.80"
	dxmtURL        = "https://github.com/3Shain/dxmt/releases/download/v0.80/dxmt-v0.80-builtin.tar.gz"
)

var dxmtCore = []string{"d3d10core.dll", "d3d11.dll", "dxgi.dll"}

func dxmtVersion(root string) string {
	b, err := os.ReadFile(filepath.Join(root, ".poem-dxmt"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// DXMTVersion reports the DXMT version installed into the Wine owning binary, or "".
func DXMTVersion(binary string) string {
	return dxmtVersion(filepath.Dir(filepath.Dir(binary)))
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	_ = os.Remove(dst)
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// dxmtFiles is where the extracted DXMT release is kept for later prefixes.
func dxmtFiles(dir string) string { return filepath.Join(dir, "dxmt") }

// InstallDXMT downloads DXMT and places it into the managed Wine at root.
func InstallDXMT(ctx context.Context, dir, root string, progress Progress) error {
	work, err := os.MkdirTemp(dir, "dxmt-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	archive := filepath.Join(work, "dxmt.tar.gz")
	if err := download(ctx, dxmtURL, archive, progress, "正在下载 DXMT "+dxmtVersionTag); err != nil {
		return err
	}
	if out, err := exec.CommandContext(ctx, "/usr/bin/tar", "-xzf", archive, "-C", work).CombinedOutput(); err != nil {
		return fmt.Errorf("解压失败：%s", strings.TrimSpace(string(out)))
	}
	var winDir, unixDir string
	_ = filepath.WalkDir(work, func(path string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			switch d.Name() {
			case "x86_64-windows":
				winDir = path
			case "x86_64-unix":
				unixDir = path
			}
		}
		return nil
	})
	if winDir == "" || unixDir == "" {
		return fmt.Errorf("DXMT 压缩包结构不符合预期")
	}
	store := dxmtFiles(dir)
	_ = os.RemoveAll(store)
	for _, sub := range []struct{ from, to string }{{winDir, "x86_64-windows"}, {unixDir, "x86_64-unix"}} {
		entries, err := os.ReadDir(sub.from)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if !e.IsDir() {
				if err := copyFile(filepath.Join(sub.from, e.Name()), filepath.Join(store, sub.to, e.Name())); err != nil {
					return err
				}
			}
		}
	}
	libWin := filepath.Join(root, "lib", "wine", "x86_64-windows")
	libUnix := filepath.Join(root, "lib", "wine", "x86_64-unix")
	if _, err := os.Stat(libWin); err != nil {
		return fmt.Errorf("此 Wine 没有 lib/wine/x86_64-windows，无法安装 DXMT")
	}
	for _, name := range append(append([]string{}, dxmtCore...), "winemetal.dll", "nvngx.dll") {
		src := filepath.Join(store, "x86_64-windows", name)
		if _, err := os.Stat(src); err == nil {
			if err := copyFile(src, filepath.Join(libWin, name)); err != nil {
				return err
			}
		}
	}
	if err := copyFile(filepath.Join(store, "x86_64-unix", "winemetal.so"), filepath.Join(libUnix, "winemetal.so")); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, ".poem-dxmt"), []byte(dxmtVersionTag), 0644)
}

// The prefix needs winemetal.dll (and nvngx.dll for the NVIDIA path) in system32.
func placeDXMTPrefix(dir, prefix string) error {
	system32 := filepath.Join(prefix, "drive_c", "windows", "system32")
	for _, name := range []string{"winemetal.dll", "nvngx.dll"} {
		src := filepath.Join(dxmtFiles(dir), "x86_64-windows", name)
		if _, err := os.Stat(src); err == nil {
			if err := copyFile(src, filepath.Join(system32, name)); err != nil {
				return err
			}
		}
	}
	return nil
}
