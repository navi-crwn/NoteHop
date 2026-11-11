package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/internal/i18n"
)

func getPage(t *testing.T, srv *httptest.Server, path, lang string) (int, string) {
	t.Helper()
	if lang != "" { // the language is chosen with ?lang= (Accept-Language is not used)
		path += "?lang=" + lang
	}
	req, _ := http.NewRequest("GET", srv.URL+path, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// /secret and /embargo are the new-note form with a preset named on it: same page,
// same fields, nothing new for the server to store.
func TestPresetPages(t *testing.T) {
	srv := newTestApp(t)
	_, plain := getPage(t, srv, "/new", "")
	if strings.Contains(plain, "data-preset") || strings.Contains(plain, `class="preset-head"`) {
		t.Fatal("/new must not carry a preset")
	}
	for _, c := range []struct{ path, preset, titleEn, titleDe string }{
		{"/secret", "secret", "Send a secret message", "Eine geheime Nachricht senden"},
		{"/embargo", "embargo", "A note that opens later", "Eine Notiz, die sich später öffnet"},
	} {
		code, body := getPage(t, srv, c.path, "")
		if code != 200 || !strings.Contains(body, `data-preset="`+c.preset+`"`) || !strings.Contains(body, "<title>"+c.titleEn) {
			t.Fatalf("%s: %d", c.path, code)
		}
		// It is the whole form: the same compose script and fields as /new.
		for _, must := range []string{`id="compose"`, `data-feature="capsule"`, `id="open-at"`, `data-starter="secret"`, `value="86400"`} {
			if !strings.Contains(body, must) {
				t.Errorf("%s lacks %s", c.path, must)
			}
		}
		if _, de := getPage(t, srv, c.path, "de"); !strings.Contains(de, c.titleDe) {
			t.Errorf("%s is not translated (de)", c.path)
		}
	}
}

// The pages render in every language with that language's own words, not the English fallback.
func TestPresetPagesAllLanguages(t *testing.T) {
	srv := newTestApp(t)
	en := map[string]string{}
	for _, p := range []string{"/secret", "/embargo"} {
		_, body := getPage(t, srv, p, "en")
		en[p] = between(body, "<title>", "</title>")
	}
	for _, lang := range []string{"zh-CN", "de", "id", "ja", "pt-BR", "es", "fr", "ru"} {
		for _, p := range []string{"/secret", "/embargo"} {
			code, body := getPage(t, srv, p, lang)
			title := between(body, "<title>", "</title>")
			if code != 200 || title == "" || title == en[p] {
				t.Errorf("%s in %s: %d %q", p, lang, code, title)
			}
		}
	}
}

func between(s, a, b string) string {
	i := strings.Index(s, a)
	if i < 0 {
		return ""
	}
	s = s[i+len(a):]
	if j := strings.Index(s, b); j >= 0 {
		return s[:j]
	}
	return s
}

// Both names can never be a link name or a username; the routes are in the sitemap and
// linked from the home page tour.
func TestPresetNamesReservedAndLinked(t *testing.T) {
	a := &App{}
	for _, n := range []string{"secret", "embargo"} {
		if !a.reservedName(n) {
			t.Errorf("%s must be reserved", n)
		}
	}
	srv := newTestApp(t)
	c := newClient(t, srv)
	for _, n := range []string{"secret", "embargo"} {
		if code, m, _ := c.do("POST", "/api/register", map[string]any{"username": n, "auth": key()}, nil); code != 400 || m["code"] != "err.username_reserved" {
			t.Errorf("username %s: %d %v", n, code, m)
		}
		// "secret" is too short to be a link name at all (7+ characters); "embargo" is the reserved check.
		code, m, _ := c.do("POST", "/api/notes", map[string]any{"encrypted": false, "content": `{"t":"x","b":"x"}`, "mode": "read", "admin": key(), "write": key(), "slug": n}, nil)
		if code != 400 || (m["code"] != "err.slug_reserved" && m["code"] != "err.slug_short") {
			t.Errorf("link name %s: %d %v", n, code, m)
		}
		if n == "embargo" && m["code"] != "err.slug_reserved" {
			t.Errorf("embargo must be refused as reserved: %v", m)
		}
	}
	_, sm := getPage(t, srv, "/sitemap.xml", "")
	_, home := getPage(t, srv, "/", "")
	for _, p := range []string{"/secret", "/embargo"} {
		if !strings.Contains(sm, p+"</loc>") {
			t.Errorf("sitemap lacks %s", p)
		}
		if !strings.Contains(home, `href="`+p+`"`) {
			t.Errorf("home lacks a link to %s", p)
		}
	}
	// Each link wraps the text of its own tour item, not whatever sits at that position.
	for key, p := range tourLinks {
		want := `<a href="` + p + `">` + i18n.Locale{Lang: i18n.Get(i18n.Default)}.T(key) + "</a>"
		if !strings.Contains(home, want) {
			t.Errorf("%s: home lacks %s", key, want)
		}
	}
	// And no other tour item is a link.
	if n := strings.Count(home, `<b><a href=`); n != len(tourLinks) {
		t.Errorf("%d linked tour items, want %d", n, len(tourLinks))
	}
}
