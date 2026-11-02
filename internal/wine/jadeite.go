package wine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Jadeite (https://codeberg.org/mkrsym1/jadeite) makes the game start under
// Wine, where its kernel anti-cheat driver cannot load. Poem injects its
// game_payload.dll first, before every other module, as jadeite's own
// launcher does. It is downloaded from the official release, never bundled.
const (
	JadeiteVersion = "4.1.0"
	jadeiteURL     = "https://codeberg.org/mkrsym1/jadeite/releases/download/v4.1.0/v4.1.0.zip"
)

// JadeiteArgs are the game arguments jadeite recommends for Star Rail.
var JadeiteArgs = []string{"-disable-gpu-skinning"}

func jadeiteDir(dir string) string { return filepath.Join(dir, "jadeite") }

// JadeitePayload returns the installed game_payload.dll, or "".
func JadeitePayload(dir string) string {
	b, err := os.ReadFile(filepath.Join(jadeiteDir(dir), ".poem-version"))
	path := filepath.Join(jadeiteDir(dir), "game_payload.dll")
	if err != nil || strings.TrimSpace(string(b)) != JadeiteVersion {
		return ""
	}
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

// EnsureJadeite downloads jadeite into dir when the expected version is missing.
func EnsureJadeite(ctx context.Context, dir string, progress Progress) (string, error) {
	if path := JadeitePayload(dir); path != "" {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	work, err := os.MkdirTemp(dir, "jadeite-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(work)
	archive := filepath.Join(work, "jadeite.zip")
	if err := download(ctx, jadeiteURL, archive, progress, "正在下载 Jadeite "+JadeiteVersion); err != nil {
		return "", err
	}
	extracted := filepath.Join(work, "x")
	if out, err := exec.CommandContext(ctx, "/usr/bin/unzip", "-oq", archive, "-d", extracted).CombinedOutput(); err != nil {
		return "", fmt.Errorf("解压 Jadeite 失败：%s", strings.TrimSpace(string(out)))
	}
	var payload string
	_ = filepath.WalkDir(extracted, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == "game_payload.dll" {
			payload = path
			return filepath.SkipAll
		}
		return nil
	})
	if payload == "" {
		return "", fmt.Errorf("Jadeite 发布包中没有 game_payload.dll")
	}
	target := jadeiteDir(dir)
	_ = os.RemoveAll(target)
	if err := os.Rename(filepath.Dir(payload), target); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(target, ".poem-version"), []byte(JadeiteVersion), 0644); err != nil {
		return "", err
	}
	return filepath.Join(target, "game_payload.dll"), nil
}
