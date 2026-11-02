package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFeatureConfigurationRestore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.ini")
	original := []byte("[General]\r\nCustom=keep\r\nEnableBattleTimeStop=False\r\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	cfg := defaults()
	cfg.TimeStop = true
	cfg.TimeStopKey = 0x75
	cfg.TargetFPS = 144
	cfg.UnlockFPS = true
	cfg.MultiInstance = true
	cfg.TouchUI = true
	restore, err := stageConfig(path, cfg)
	if err != nil {
		t.Fatal(err)
	}
	staged, _ := os.ReadFile(path)
	for _, line := range []string{"EnableBattleTimeStop=True", "BattleTimeStopKey=0x75", "TargetFPS=144", "UnlockMultiInstance=True", "EnableTouchUI=True", "Custom=keep"} {
		if !strings.Contains(string(staged), line) {
			t.Errorf("missing %s", line)
		}
	}
	if strings.Contains(string(staged), "EnableBattleTimeStop=False") {
		t.Error("stale key survived merge")
	}
	if err := restore(); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(original) {
		t.Fatal("did not restore original bytes")
	}
	newPath := filepath.Join(t.TempDir(), "config.ini")
	restore, err = stageConfig(newPath, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := restore(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Fatal("temporary config not removed")
	}
}

func TestValidation(t *testing.T) {
	cfg := defaults()
	if err := validate(cfg, false); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Settings){func(s *Settings) { s.ProxyPort = -1 }, func(s *Settings) { s.ProxyPort = 65536 }, func(s *Settings) { s.ServerURL = "file:///D:/game" }, func(s *Settings) { s.Instances = 0 }, func(s *Settings) { s.RSAKey = "broken" }, func(s *Settings) { s.TimeStopKey = 0 }} {
		c := cfg
		mutate(&c)
		if validate(c, false) == nil {
			t.Errorf("accepted invalid settings: %+v", c)
		}
	}
	cfg.ProxyEnabled = false
	cfg.ServerURL = ""
	cfg.RSAKey = "broken"
	if err := validate(cfg, false); err != nil {
		t.Fatal("disabled proxy validated stale fields", err)
	}
}

func TestNormalizeFixedFeatures(t *testing.T) {
	cfg := normalizeSettings(Settings{Instances: 8, TargetFPS: 144, TimeStopKey: 117, TouchUI: true})
	if cfg.TimeStop || cfg.UnlockFPS || cfg.MultiInstance || cfg.Instances != 1 || cfg.TargetFPS != 144 || cfg.TimeStopKey != 117 || !cfg.TouchUI {
		t.Fatalf("unexpected migration: %+v", cfg)
	}
}

func TestMultiInstancePreferencePreserved(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		cfg := normalizeSettings(Settings{MultiInstance: enabled})
		if cfg.MultiInstance != enabled {
			t.Fatalf("changed multi-instance preference: %v", enabled)
		}
		want := "UnlockMultiInstance=False"
		if enabled {
			want = "UnlockMultiInstance=True"
		}
		if !strings.Contains(featureConfig(cfg), want) {
			t.Fatal("module config ignored preference")
		}
	}
}

func TestFeatureSwitchesRemainSelectable(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		cfg := normalizeSettings(Settings{TimeStop: enabled, UnlockFPS: enabled})
		if cfg.TimeStop != enabled || cfg.UnlockFPS != enabled {
			t.Fatal("feature switches overwritten")
		}
		want := "False"
		if enabled {
			want = "True"
		}
		for _, key := range []string{"EnableBattleTimeStop=", "UnlockFPS="} {
			if !strings.Contains(featureConfig(cfg), key+want) {
				t.Fatal("feature configuration ignores switch")
			}
		}
	}
}
