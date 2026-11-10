package app

import (
	"strings"
	"testing"
)

// The home page's family section: one 3D tile per other app (FileHop included),
// name and host under it.
func TestHomeFamilySection(t *testing.T) {
	body, _ := fetch(t, newTestApp(t), "/", "en-US,en;q=0.9")
	_, sec, _ := strings.Cut(body, `id="family"`)
	sec, _, _ = strings.Cut(sec, "</section>")
	if n := strings.Count(sec, `class="hop-fam-tile"`); n != 5 {
		t.Fatalf("%d tiles, want 5", n)
	}
	for _, want := range []string{`data-hop-hero="filehop"`, `/static/hop-home/home/filehop-light.webp`, `<span>file.hel.ink</span>`} {
		if !strings.Contains(sec, want) {
			t.Errorf("family section lacks %q", want)
		}
	}
	if strings.Contains(sec, `data-hop-hero="notehop"`) {
		t.Error("the section lists the app itself")
	}
}
