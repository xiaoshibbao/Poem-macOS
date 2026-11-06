package wine

import (
	"os"
	"path/filepath"
	"strings"
)

// A new prefix has no CJK font, so Chinese text in GDI windows such as the
// Bilibili login component renders as boxes. macOS ships Arial Unicode, which
// covers CJK; it is copied into the prefix instead of being redistributed.
const (
	hostCJKFont = "/System/Library/Fonts/Supplemental/Arial Unicode.ttf"
	CJKFontFile = "ArialUnicodeMS.ttf"
	CJKFontFace = "Arial Unicode MS"
)

// PlaceCJKFont copies the host's Arial Unicode into the prefix's Fonts
// directory. It reports false, without error, when the host has no such font.
func PlaceCJKFont(prefix string) (bool, error) {
	src, err := os.Stat(hostCJKFont)
	if err != nil {
		return false, nil
	}
	dst := filepath.Join(prefix, "drive_c", "windows", "Fonts", CJKFontFile)
	if info, err := os.Stat(dst); err == nil && info.Size() == src.Size() {
		return true, nil
	}
	if err := copyFile(hostCJKFont, dst); err != nil {
		return false, err
	}
	return true, nil
}

// PrefixFonts lists the font files in the prefix's Fonts directory, lowercased.
func PrefixFonts(prefix string) map[string]bool {
	fonts := map[string]bool{}
	entries, _ := os.ReadDir(filepath.Join(prefix, "drive_c", "windows", "Fonts"))
	for _, entry := range entries {
		fonts[strings.ToLower(entry.Name())] = true
	}
	return fonts
}
