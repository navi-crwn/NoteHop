package app

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/web"
)

// Everything that opens over the page closes with Escape and gives the focus
// back: the dashboard drawer, the public header's menu on phones, the dropdown
// menus, and the chips' menus on the new-note page.
func TestDrawersAndMenusCloseWithEscape(t *testing.T) {
	read := func(path string) string {
		b, err := fs.ReadFile(web.Static, path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		return string(b)
	}
	for path, wants := range map[string][]string{
		"static/js/shell.js": {"e.key === 'Escape' && isOpen()", "burger.focus()", "side.inert"},
		"static/js/app.js":   {"e.key === 'Escape' && nav.classList.contains('open')", "if (e.key !== 'Escape') return;"},
		"static/js/new.js":   {"e.key === 'Escape' && open"},
	} {
		js := read(path)
		for _, want := range wants {
			if !strings.Contains(js, want) {
				t.Errorf("%s lacks %q", path, want)
			}
		}
	}
}
