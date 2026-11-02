package wine

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWindowsPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("macOS path mapping")
	}
	prefix := "/Users/a/.wine"
	cases := map[string]string{
		"/Users/a/.wine/drive_c/Games/StarRail/StarRail.exe": `C:\Games\StarRail\StarRail.exe`,
		"/Users/a/.wine/drive_c":                             `C:\`,
		"/Users/a/Games/StarRail.exe":                        `Z:\Users\a\Games\StarRail.exe`,
		"/Users/a/.wine/drive_cx/file":                       `Z:\Users\a\.wine\drive_cx\file`,
	}
	for in, want := range cases {
		if got := WindowsPath(prefix, in); got != want {
			t.Errorf("WindowsPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseEnv(t *testing.T) {
	env, err := ParseEnv("# comment\nWINEESYNC=1\n\n DXVK_HUD = fps \n")
	if err != nil || env["WINEESYNC"] != "1" || env["DXVK_HUD"] != "fps" || len(env) != 2 {
		t.Fatalf("ParseEnv = %v, %v", env, err)
	}
	if _, err := ParseEnv("NOVALUE"); err == nil {
		t.Fatal("expected error for a line without =")
	}
}

func TestPrefixValidation(t *testing.T) {
	dir := t.TempDir()
	if ValidPrefix(dir) {
		t.Fatal("empty directory accepted as prefix")
	}
	if err := os.Mkdir(filepath.Join(dir, "drive_c"), 0755); err != nil {
		t.Fatal(err)
	}
	if !ValidPrefix(dir) {
		t.Fatal("prefix with drive_c rejected")
	}
}

func TestCrossOverBottleEnvironment(t *testing.T) {
	c := Config{Binary: "/Applications/CrossOver.app/Contents/SharedSupport/CrossOver/bin/wine", Prefix: "/Users/a/Library/Application Support/CrossOver/Bottles/StarRail", Env: map[string]string{"A": "1"}}
	env := map[string]bool{}
	for _, e := range c.environ() {
		env[e] = true
	}
	if !env["CX_BOTTLE=StarRail"] || !env["WINEPREFIX="+c.Prefix] || !env["A=1"] {
		t.Fatalf("missing CrossOver variables")
	}
}
