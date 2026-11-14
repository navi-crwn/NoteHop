package app

import (
	"os"
	"strings"
	"testing"
)

// The dashboard sidebar ends with the family foot: Help, Terms, Privacy, Changelog, GitHub, Announcement (one per
// line, in that order), then "Part of Hop Easy Link"; the palette picker sits above it, no longer in the language menu.
func TestSidebarFootAndPalettePicker(t *testing.T) {
	c := signedIn(t, newTestApp(t), "helena")
	body := pageOf(t, c, "/dashboard")
	side, _, _ := strings.Cut(body, "</aside>")
	foot := side[strings.Index(side, `class="shell-side-foot"`):]
	last := -1
	for _, href := range []string{`href="/faq"`, `href="/terms"`, `href="/privacy"`, `href="/changelog"`, `href="https://github.com/navi-crwn/NoteHop"`, `href="https://hel.ink/announcements"`, `class="part-of"`} {
		i := strings.Index(foot, href)
		if i < 0 || i < last {
			t.Fatalf("foot item %s missing or out of order", href)
		}
		last = i
	}
	if strings.Count(foot, "<li>") != 6 {
		t.Errorf("foot has %d list items, want 6", strings.Count(foot, "<li>"))
	}
	pal := strings.Index(side, `class="shell-pal"`)
	if pal < 0 || pal > strings.Index(side, `class="shell-side-foot"`) || strings.Count(side, `data-palette=`) != 3 {
		t.Error("the palette picker is not in the sidebar above the foot")
	}
	if strings.Contains(body, `class="pal"`) {
		t.Error("a palette picker is left in a language menu")
	}
	if strings.Contains(pageOf(t, newClient(t, newTestApp(t)), "/"), `class="pal"`) {
		t.Error("a palette picker is left in the public bar")
	}
}

// The palette lives in the family cookie hop_palette, read before paint; an older localStorage choice moves once.
func TestPaletteIsAFamilyCookie(t *testing.T) {
	b, err := os.ReadFile("../../web/static/js/theme.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	for _, want := range []string{"hop_palette=", "hopPref('hop_palette', oldPal)", "removeItem('nh_palette')", "pal === 'mist' || pal === 'sand'", "hop_palette_depth=surface"} {
		if !strings.Contains(js, want) {
			t.Errorf("theme.js lacks %q", want)
		}
	}
}
