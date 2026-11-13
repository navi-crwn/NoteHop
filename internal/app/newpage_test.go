package app

import (
	"io"
	"strings"
	"testing"
)

// The new-note page: one chip per setting, the editor first, and the chrome
// that fits who is looking (the dashboard shell when signed in, the public
// header and a one-line footer otherwise).
func TestNewNotePage(t *testing.T) {
	_, _, c := newAppDB(t)
	get := func(path string) string {
		resp, err := c.hc.Get(c.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	anon := get("/new")
	for _, want := range []string{
		`id="opt-kind"`, `id="opt-password"`, `id="opt-until"`, `id="opt-opens"`, `id="opt-who"`, `id="extra-chips"`, `id="adv-btn"`,
		`data-starter="note" aria-pressed="true"`, `id="start-more"`, `class="btn sm ed-type-btn"`,
		`id="summary"`, `id="create"`, `id="why-password"`, `id="why-burn"`,
		`class="footer-mini"`, `id="help-menu"`, `id="recent"`,
	} {
		if !strings.Contains(anon, want) {
			t.Errorf("/new (signed out) lacks %s", want)
		}
	}
	for _, gone := range []string{`data-starter="code"`, `data-starter="web"`, `name="privacy"`, `class="footer-columns"`, `class="shell-side"`, `id="vault-note"`, `class="compose-head"`} {
		if strings.Contains(anon, gone) {
			t.Errorf("/new (signed out) still has %s", gone)
		}
	}
	// The summary comes before the button it explains.
	if strings.Index(anon, `id="summary"`) > strings.Index(anon, `id="create"`) {
		t.Error("the summary must be read before the Create button")
	}

	if code, _, raw := c.do("POST", "/api/register", map[string]string{"username": "mira", "auth": key()}, nil); code != 200 && code != 201 {
		t.Fatalf("register: %d %s", code, raw)
	}
	in := get("/new")
	for _, want := range []string{`class="shell-side"`, `class="shell-nav shell-family"`, `id="vault-note"`, `id="menu-vault"`, `id="opt-password"`} {
		if !strings.Contains(in, want) {
			t.Errorf("/new (signed in) lacks %s", want)
		}
	}
	// Signed in, the device list lives in My notes, and the top bar has no app grid.
	for _, gone := range []string{`id="recent"`, `data-family-switcher`, `class="footer-columns"`} {
		if strings.Contains(in, gone) {
			t.Errorf("/new (signed in) still has %s", gone)
		}
	}
	if sec := get("/security"); !strings.Contains(sec, `class="shell-side"`) || strings.Contains(sec, `class="back"`) {
		t.Error("/security (signed in) uses the dashboard chrome, without a Back row")
	}
	// The pages that present NoteHop keep the public chrome and the columns.
	if home := get("/"); strings.Contains(home, `class="shell-side"`) || !strings.Contains(home, `class="footer-columns"`) {
		t.Error("the home page keeps the public chrome")
	}
}
