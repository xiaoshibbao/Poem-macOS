package launcher

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
)

// File names written into the game directory by early releases; only removed
// when their contents match the embedded modules.
const (
	legacyFeatureName = "KomariFeatures.dll"
	legacyVisualName  = "HSRUnlock.dll"
)

// Delete only files generated in our direct temporary runtime directory.
func cleanupRuntimeModules(dir string) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	temp, err := filepath.Abs(os.TempDir())
	if err != nil {
		return
	}
	if !strings.EqualFold(filepath.Dir(abs), temp) || !strings.HasPrefix(filepath.Base(abs), "Poem-runtime-") {
		return
	}
	cleanupLegacyModules(abs)
}
func cleanupLegacyModules(dir string, expected ...[]byte) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := entry.Name()
		known := name == "Poem.Lua.dll" || name == "lua-request.bin" || name == "lua-request.tmp" || name == "lua-processing.bin" || name == "lua-status.txt" || name == "lua-status.tmp" || name == "Poem.Network.dll" || name == "Poem.Game.dll" || name == "Poem.Proxy.dll" || name == "Poem.Visual.dll" || strings.HasPrefix(name, "Poem.Proxy-") && strings.HasSuffix(name, ".dll")
		if len(expected) > 0 {
			known = known || name == legacyFeatureName || name == legacyVisualName
		}
		if !known {
			continue
		}
		path := filepath.Join(dir, name)
		if len(expected) > 0 {
			b, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			hash := sha256.Sum256(b)
			match := false
			for _, original := range expected {
				if len(original) > 0 && hash == sha256.Sum256(original) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		_ = os.Remove(path)
	}
	_ = os.Remove(dir) // Succeeds only when empty; never recursively deletes unknown files.
}
