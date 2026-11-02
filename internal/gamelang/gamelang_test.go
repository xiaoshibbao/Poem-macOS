package gamelang

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLanguageBackupBoundedWritesAndExternalChange(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "table.bytes")
	original := []byte{1, 2, 3, 4, 5, 6}
	whole := append(append([]byte("before"), original...), []byte("after")...)
	if err := os.WriteFile(file, whole, 0600); err != nil {
		t.Fatal(err)
	}
	plan := Plan{File: file, Offset: 6, Original: original, Patched: []byte{9, 9, 0, 0, 0, 0}, Index: "a"}
	backups := filepath.Join(dir, "backups")
	if err := Apply(plan, backups); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(file)
	if !bytes.Equal(got[:6], whole[:6]) || !bytes.Equal(got[12:], whole[12:]) {
		t.Fatal("adjacent entries overwritten")
	}
	plan.Original = plan.Patched
	plan.Patched = []byte{8, 8, 0, 0, 0, 0}
	if err := Apply(plan, backups); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(backupPath(plan, backups))
	var backup Backup
	if err := json.Unmarshal(b, &backup); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(backup.Original, original) {
		t.Fatal("first backup lost")
	}
	if err := writeSegment(file, 6, backup.Patched, backup.Original); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(file)
	if !bytes.Equal(got, whole) {
		t.Fatal("restore failed")
	}
	if err := writeSegment(file, 6, plan.Patched, original); err == nil {
		t.Fatal("external modification not detected")
	}
	if err := writeSegment(file, 6, original, []byte{1}); err == nil {
		t.Fatal("length mismatch allowed")
	}
}
