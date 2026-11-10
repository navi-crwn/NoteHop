package app

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestHopFamilyTypography(t *testing.T) {
	srv := newTestApp(t)
	res, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	body := string(b)
	for _, want := range []string{
		`href="https://hel.ink/hop-assets/v8/fonts-amnesia.css" integrity="sha384-`,
		`https://hel.ink/hop-assets/v8/apps/pixelhop/mark.svg`,
		`>LinkHop<`,
		`Part of the HOP family`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("home page lacks %q", want)
		}
	}
	_, footer, _ := strings.Cut(body, "<footer")
	if strings.Contains(footer, "apps/notehop/mark.svg") {
		t.Error("the footer family list should not link NoteHop to itself")
	}
	if !strings.Contains(body, `href="https://note.hel.ink" aria-current="true"`) || len(hopFamily("")) != 6 {
		t.Error("the switcher must list all six apps and identify NoteHop")
	}
	csp := res.Header.Get("Content-Security-Policy")
	for _, d := range []string{"script-src 'self' https://hel.ink;", "style-src 'self' https://hel.ink;", "font-src 'self' https://hel.ink;"} {
		if !strings.Contains(csp, d) {
			t.Errorf("CSP lacks %q: %s", d, csp)
		}
	}
	// The shared dialogs: one script from hel.ink, pinned by its hash, and its stylesheet.
	ui := hopUI()
	if ui == nil || !strings.HasPrefix(ui.JS.Integrity, "sha384-") {
		t.Fatal("the bundled manifest must list ui_css and ui_js (with integrity); a sync that drops them turns the dialogs back into the browser's own")
	}
	for _, want := range []string{`<script src="` + ui.JS.URL + `" integrity="` + ui.JS.Integrity + `" crossorigin="anonymous" defer></script>`, `href="` + ui.CSS.URL + `"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page lacks %s", want)
		}
	}
}
