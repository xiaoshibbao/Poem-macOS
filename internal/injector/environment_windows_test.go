//go:build windows

package injector

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestEnvironmentOverrideCaseAndOrdering(t *testing.T) {
	block := mergeEnvBlock([]string{"z=last", "Path=old", "POEM_PROXY=old", "=C:=C:\\work"}, map[string]string{"path": "new", "poem_proxy": "127.0.0.1:8080", "POEM_UNICODE": "旅途"})
	if len(block) < 2 || block[len(block)-1] != 0 || block[len(block)-2] != 0 {
		t.Fatal("missing double terminator")
	}
	entries := strings.Split(string(utf16.Decode(block[:len(block)-2])), "\x00")
	want := []string{"=C:=C:\\work", "PATH=new", "POEM_PROXY=127.0.0.1:8080", "POEM_UNICODE=旅途", "Z=last"}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("got %q, want %q", entries, want)
	}
}

func TestEmptyEnvironmentIsDoubleTerminated(t *testing.T) {
	block := mergeEnvBlock(nil, nil)
	if !reflect.DeepEqual(block, []uint16{0, 0}) {
		t.Fatalf("invalid empty block: %v", block)
	}
}
