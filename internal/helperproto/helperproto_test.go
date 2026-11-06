package helperproto

import (
	"slices"
	"testing"
)

func TestPrependEntry(t *testing.T) {
	entry := "ArialUnicodeMS.ttf,Arial Unicode MS"
	list := []string{"MSGOTHIC.TTC,MS UI Gothic", "arialunicodems.ttf,arial unicode ms", "SIMSUN.TTC,SimSun"}
	want := []string{entry, "MSGOTHIC.TTC,MS UI Gothic", "SIMSUN.TTC,SimSun"}
	got := PrependEntry(list, entry)
	if !slices.Equal(got, want) {
		t.Fatalf("got %q", got)
	}
	if again := PrependEntry(got, entry); !slices.Equal(again, want) {
		t.Fatalf("not idempotent: %q", again)
	}
	if got := PrependEntry(nil, entry); !slices.Equal(got, []string{entry}) {
		t.Fatalf("empty list: %q", got)
	}
}
