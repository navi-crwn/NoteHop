package app

import (
	"io"
	"strings"
	"testing"
)

// The signed-in dashboard has the same chrome as the other HOP dashboards: this app's menu and the HOP family in
// the sidebar, settings-type controls in the top bar (no second list of apps), a compact footer.
func TestDashboardShell(t *testing.T) {
	_, _, c := newAppDB(t)
	if code, _, raw := c.do("POST", "/api/register", map[string]string{"username": "mira", "auth": key()}, nil); code != 200 && code != 201 {
		t.Fatalf("register: %d %s", code, raw)
	}
	get := func(path string) string {
		resp, err := c.hc.Get(c.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	body := get("/dashboard")
	for _, want := range []string{`class="shell"`, `class="shell-side"`, `class="shell-nav shell-family"`, `class="shell-top"`, `class="shell-foot"`, `id="logout"`} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard lacks %s", want)
		}
	}
	top, _, _ := strings.Cut(strings.SplitN(body, `class="shell-top"`, 2)[1], "</header>")
	if strings.Contains(top, "family-switcher") {
		t.Error("the app grid is back in the top bar")
	}
	for _, want := range []string{`id="lang-menu"`, `id="theme"`, `id="menu"`} {
		if !strings.Contains(top, want) {
			t.Errorf("top bar lacks %s", want)
		}
	}
	// The vault: its state line, the banners that explain a locked row and offer the way out, the
	// "On this device" group, and the lock/unlock entry in the account menu.
	for _, want := range []string{`id="reseal"`, `id="nokey"`, `id="menu-vault"`, `id="dash-lead"`, `data-view="basic"`, `id="vault-btn"`, `href="/new?kind=basic"`} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard lacks %s", want)
		}
	}
	// The home of the dashboard is the basic notes and needs no vault: no vault banners, and only the filters that
	// mean something for a basic list. The Vault view has the banners and the rest of the filters.
	for _, gone := range []string{`id="vault-state"`, `id="vault-locked"`, `id="vault-mismatch"`, `data-group="encrypted"`, `data-group="basic"`, `data-group="code"`, `data-group="snapshots"`, `data-group="device"`} {
		if strings.Contains(body, gone) {
			t.Errorf("the basic notes view still has %s", gone)
		}
	}
	if n := strings.Count(body, `data-group=`); n != 5 {
		t.Errorf("%d filters on the basic view, want 5 (all, to-dos, links, timed, archived)", n)
	}
	vbody := get("/dashboard?view=vault")
	for _, want := range []string{`id="vault-state"`, `id="vault-locked"`, `id="vault-mismatch"`, `data-group="device"`, `data-group="code"`, `data-view="vault"`} {
		if !strings.Contains(vbody, want) {
			t.Errorf("the Vault view lacks %s", want)
		}
	}
	if !strings.Contains(vbody, `<a href="/dashboard?view=vault" aria-current="page">`) || strings.Contains(vbody, `<a href="/dashboard" aria-current="page">`) {
		t.Error("the sidebar does not mark the Vault as the current page")
	}
	if !strings.Contains(body, `<a href="/dashboard" aria-current="page">`) || !strings.Contains(body, `<a href="/dashboard?view=vault" >`) {
		t.Error("the sidebar does not mark My notes as the current page, with the Vault beside it")
	}
	// Pages about the product that are not tools stay public for everyone.
	if pub := get("/terms"); strings.Contains(pub, `class="shell-side"`) || !strings.Contains(pub, `class="footer-columns"`) {
		t.Error("public pages keep the public chrome")
	}
}

// The tools and help pages opened from the sidebar stay inside the frame for a member, with their own entry lit
// and no way "back" to the public home page; a guest keeps the public page. Rule 6 below it: the "Part of Hop
// Easy Link" line is the last thing in the sidebar and is not repeated in the footer.
func TestSidebarPagesStayInTheFrame(t *testing.T) {
	_, _, c := newAppDB(t)
	if code, _, raw := c.do("POST", "/api/register", map[string]string{"username": "mira", "auth": key()}, nil); code != 200 && code != 201 {
		t.Fatalf("register: %d %s", code, raw)
	}
	guest := newClient(t, c.srv)
	get := func(who *client, path string) string {
		resp, err := who.hc.Get(who.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	for _, path := range []string{"/nearby", "/diff", "/about", "/faq"} {
		m := get(c, path)
		if !strings.Contains(m, `class="shell-side"`) || !strings.Contains(m, `class="shell-top"`) {
			t.Errorf("%s: a member is outside the dashboard frame", path)
		}
		if strings.Contains(m, `class="back-row"`) || strings.Contains(m, `class="wrap back-row"`) {
			t.Errorf("%s: a member still has a back row to the home page", path)
		}
		if !strings.Contains(m, `<a href="`+path+`" aria-current="page">`) {
			t.Errorf("%s: its sidebar entry is not marked as the current page", path)
		}
		g := get(guest, path)
		if strings.Contains(g, `class="shell-side"`) || !strings.Contains(g, `class="back" href="/"`) {
			t.Errorf("%s: a guest lost the public page", path)
		}
	}
	// The way back of the pages that stay public: the dashboard for a member, the home page for a guest.
	for _, path := range []string{"/changelog", "/contact", "/status", "/terms", "/privacy", "/no-such-page-at-all"} {
		m, g := get(c, path), get(guest, path)
		if !strings.Contains(m, `class="brand" href="/dashboard"`) {
			t.Errorf("%s: the logo takes a member to the home page", path)
		}
		if strings.Contains(m, `class="back" href="/"`) || strings.Contains(m, `<a class="btn primary" href="/">`) {
			t.Errorf("%s: a member is sent to the home page", path)
		}
		if !strings.Contains(g, `class="brand" href="/"`) {
			t.Errorf("%s: a guest's logo does not go to the home page", path)
		}
	}
	for _, path := range []string{"/changelog", "/contact", "/status", "/terms", "/privacy"} {
		if m := get(c, path); !strings.Contains(m, `class="back" href="/dashboard"`) {
			t.Errorf("%s: a member's way back is not the dashboard", path)
		}
		if g := get(guest, path); !strings.Contains(g, `class="back" href="/"`) {
			t.Errorf("%s: a guest's way back is not the home page", path)
		}
	}
	// the sidebar
	body := get(c, "/dashboard")
	if !strings.Contains(body, `class="shell-brand" href="/dashboard"`) {
		t.Error("the sidebar logo does not go to the dashboard")
	}
	if n := strings.Count(body, `class="part-of"`); n != 1 {
		t.Errorf("%d \"Part of\" lines on the dashboard, want exactly one", n)
	}
	side, rest, _ := strings.Cut(body, "</aside>")
	foot := side[strings.Index(side, `class="shell-side-foot"`):]
	if !strings.Contains(foot, `class="part-of" href="https://hel.ink"`) || strings.Index(foot, "GitHub") > strings.Index(foot, `class="part-of"`) {
		t.Error("the \"Part of\" line is not the last thing in the sidebar foot")
	}
	if strings.Contains(rest, `class="part-of"`) {
		t.Error("the \"Part of\" line is repeated in the page footer")
	}
}
