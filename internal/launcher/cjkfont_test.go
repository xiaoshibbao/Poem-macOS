package launcher

import (
	"testing"

	"poem/internal/helperproto"
)

func TestCJKFontRegistrySkipsInstalledFonts(t *testing.T) {
	values := cjkFontRegistry(map[string]bool{"simsun.ttc": true})
	substitutes := map[string]string{}
	links := 0
	for _, v := range values {
		switch {
		case v.Key == fontSubstitutes:
			substitutes[v.Name] = v.String
		case v.Key == fontSystemLink && v.Kind == helperproto.RegPrepend:
			links++
		}
	}
	if substitutes["Microsoft YaHei"] != "Arial Unicode MS" || substitutes["SimHei"] != "Arial Unicode MS" {
		t.Fatalf("missing substitutes: %v", substitutes)
	}
	if _, ok := substitutes["SimSun"]; ok {
		t.Fatal("installed SimSun was substituted")
	}
	if links != len(cjkLinkedFaces) {
		t.Fatalf("links=%d", links)
	}
	if values[0].Key != fontsKey || values[0].String != "ArialUnicodeMS.ttf" {
		t.Fatalf("font not registered: %+v", values[0])
	}
}
