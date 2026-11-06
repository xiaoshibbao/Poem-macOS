package wine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlaceCJKFont(t *testing.T) {
	src, err := os.Stat(hostCJKFont)
	if err != nil {
		t.Skip("host has no Arial Unicode")
	}
	prefix := t.TempDir()
	for range 2 {
		ok, err := PlaceCJKFont(prefix)
		if err != nil || !ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
	}
	info, err := os.Stat(filepath.Join(prefix, "drive_c", "windows", "Fonts", CJKFontFile))
	if err != nil || info.Size() != src.Size() {
		t.Fatalf("font not copied: %v", err)
	}
	if !PrefixFonts(prefix)["arialunicodems.ttf"] {
		t.Fatal("copied font not listed")
	}
}
