package app

import (
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"
)

// The dashboard home has a place for a notice from the people who run the site. The control center writes it
// through the settings it already uses; it shows on the dashboard home only, as plain text, with its own close
// button, and with no notice there is no card at all.
func TestAnnouncementOnTheDashboardHome(t *testing.T) {
	srv, a, d := hopApp(t, Config{})
	member := ssoUser(t, srv, "501", "ann@example.com", nil)
	page := func(c *client, path string) string {
		res, err := c.hc.Get(c.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var sb strings.Builder
		buf := make([]byte, 8192)
		for {
			n, err := res.Body.Read(buf)
			sb.Write(buf[:n])
			if err != nil {
				break
			}
		}
		return sb.String()
	}

	// no notice: no card, no empty space
	if body := page(member, "/dashboard"); strings.Contains(body, `id="announce"`) || strings.Contains(body, "announce-body") {
		t.Fatal("a card without a notice")
	}

	// the control center sets it; the manifest and the settings list advertise it
	_, env := call(t, srv, "imperium", "GET", "/hop/settings", nil)
	var found bool
	for _, s := range list(env) {
		if m := s.(map[string]any); m["key"] == "announcement" {
			found = m["type"] == "string" && m["value"] == "" && m["readonly"] != true
		}
	}
	if !found {
		t.Fatalf("settings do not offer an editable announcement: %v", env)
	}
	_, env = call(t, srv, "imperium", "GET", "/hop/manifest", nil)
	var advertised bool
	for _, s := range data(env)["settings_schema"].([]any) {
		if s.(map[string]any)["key"] == "announcement" {
			advertised = true
		}
	}
	if !advertised {
		t.Fatal("the manifest does not advertise the announcement setting")
	}
	if code, env := call(t, srv, "imperium", "PUT", "/hop/settings", map[string]any{"announcement": "  Maintenance on Sunday <b>03:00</b> & more  "}); code != http.StatusOK {
		t.Fatalf("put: %d %v", code, env)
	}
	body := page(member, "/dashboard")
	card := body[strings.Index(body, `id="announce"`):]
	card, _, _ = strings.Cut(card, "</section>")
	for _, want := range []string{"Announcement", "Maintenance on Sunday &lt;b&gt;03:00&lt;/b&gt; &amp; more", ` hidden`, `id="announce-close"`, `data-id="`} {
		if !strings.Contains(card, want) {
			t.Errorf("the card lacks %q:\n%s", want, card)
		}
	}
	// plain text: nothing from the notice is read as markup
	if strings.Contains(body, "<b>03:00</b>") {
		t.Error("the notice was written into the page as HTML")
	}
	// it is above the first heading of the page, on the dashboard home only
	if strings.Index(body, `id="announce"`) > strings.Index(body, `<h1>`) {
		t.Error("the card is not above the page heading")
	}
	for _, path := range []string{"/new", "/security", "/about", "/nearby"} {
		if strings.Contains(page(member, path), `id="announce"`) {
			t.Errorf("%s shows the notice; only the dashboard home does", path)
		}
	}
	guest := noRedirect(t, srv)
	for _, path := range []string{"/", "/about", "/terms", "/login", "/changelog"} {
		if strings.Contains(page(guest, path), "Maintenance on Sunday") {
			t.Errorf("%s (public) shows the notice", path)
		}
	}
	// a different notice has a different id, so closing one does not hide the next
	id1 := a.announcementID()
	call(t, srv, "imperium", "PUT", "/hop/settings", map[string]any{"announcement": "Another one"})
	if id2 := a.announcementID(); id1 == "" || id2 == "" || id1 == id2 {
		t.Errorf("notice ids: %q %q", id1, id2)
	}

	// a notice that is too long, or not text, changes nothing
	long := strings.Repeat("é", 301)
	if code, env := call(t, srv, "imperium", "PUT", "/hop/settings", map[string]any{"announcement": long}); code != http.StatusBadRequest || env["error"] != "bad_value" {
		t.Fatalf("too long: %d %v", code, env)
	}
	if code, env := call(t, srv, "imperium", "PUT", "/hop/settings", map[string]any{"announcement": 5}); code != http.StatusBadRequest || env["error"] != "bad_value" {
		t.Fatalf("not text: %d %v", code, env)
	}
	if a.announcement() != "Another one" {
		t.Fatalf("a refused change moved the notice to %q", a.announcement())
	}
	if ok := utf8.RuneCountInString(strings.Repeat("é", 300)); ok != 300 {
		t.Fatal("test setup")
	}
	if code, env := call(t, srv, "imperium", "PUT", "/hop/settings", map[string]any{"announcement": strings.Repeat("é", 300)}); code != http.StatusOK {
		t.Fatalf("exactly 300 characters: %d %v", code, env)
	}

	// it survives a restart, and an empty value takes it away again
	a2, err := New(Config{Registration: true, BaseURL: "https://note.test", SSOURL: "https://accounts.test", SSOSecret: testSecret, HopClients: []string{"imperium"}}, d)
	if err != nil {
		t.Fatal(err)
	}
	if utf8.RuneCountInString(a2.announcement()) != 300 {
		t.Fatal("the notice was lost after a restart")
	}
	call(t, srv, "imperium", "PUT", "/hop/settings", map[string]any{"announcement": ""})
	if strings.Contains(page(member, "/dashboard"), `id="announce"`) {
		t.Error("the card stayed after the notice was emptied")
	}
	// the change is in the control center's audit log
	_, env = call(t, srv, "imperium", "GET", "/hop/audit", nil)
	var audited bool
	for _, e := range list(env) {
		if m := e.(map[string]any); m["action"] == "settings.update" && m["target"] == "announcement" {
			audited = true
		}
	}
	if !audited {
		t.Error("the announcement change was not audited")
	}
}
