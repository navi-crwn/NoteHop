package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/internal/db"
	"github.com/navi-crwn/NoteHop/internal/i18n"
)

func TestPreviewText(t *testing.T) {
	en, id := i18n.Locale{Lang: i18n.Get("en")}, i18n.Locale{Lang: i18n.Get("id")}
	title, desc := previewText(&db.Note{Encrypted: true, Password: true, MaxViews: 1}, en)
	if title != "Encrypted note" || !strings.Contains(desc, "needs a password") || !strings.Contains(desc, "destroyed after reading") {
		t.Fatalf("encrypted: %q %q", title, desc)
	}
	if title, _ := previewText(&db.Note{Encrypted: true}, id); title != "Catatan terenkripsi" {
		t.Fatalf("id: %q", title)
	}
	if _, desc := previewText(&db.Note{MaxViews: 1}, en); !strings.HasPrefix(desc, "Destroyed after reading.") {
		t.Fatalf("capitalised: %q", desc)
	}
	// A basic note's words never reach a preview.
	title, desc = previewText(&db.Note{Content: `{"t":"Menu rapat","b":"rahasia"}`}, en)
	if title != "Basic note" || strings.Contains(desc, "Menu rapat") || strings.Contains(desc, "rahasia") {
		t.Fatalf("basic note preview: %q %q", title, desc)
	}
}

func TestPreviewBot(t *testing.T) {
	c := newClient(t, newTestApp(t))
	n := c.create(map[string]any{"max_views": 1})
	get := func(ua string) string {
		req, _ := http.NewRequest("GET", c.srv.URL+"/n/"+n.token, nil)
		req.Header.Set("User-Agent", ua)
		res, err := c.hc.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return string(b)
	}
	bot := get("WhatsApp/2.23.20.0 A")
	if !strings.Contains(bot, `og:title" content="Encrypted note"`) || strings.Contains(bot, "<script") ||
		!strings.Contains(bot, "destroyed after reading") || !strings.Contains(bot, "/static/img/og.png") {
		t.Fatalf("bot page: %s", bot)
	}
	human := get("Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 Chrome/126 Mobile Safari/537.36")
	if !strings.Contains(human, "/static/js/note.js") || !strings.Contains(human, `og:title" content="Encrypted note"`) {
		t.Fatal("human page should be the app, with the same preview tags")
	}
	// Neither request spends the single view.
	if code, m, _ := c.do("GET", "/api/n/"+n.token, nil, nil); code != 200 || m["views"].(float64) != 0 {
		t.Fatalf("views after previews: %d %v", code, m)
	}
}

func TestShareDomain(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(Config{Registration: true, BaseURL: "https://note.test", ShareURL: "https://nh.test"}, d)
	if err != nil {
		t.Fatal(err)
	}
	h := a.Handler()
	req := func(method, host, path string) *http.Response {
		r, _ := http.NewRequest(method, "http://"+host+path, nil)
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Result()
	}
	share, admin, err := a.createPublic("Rapat", "isi")
	if err != nil || !strings.HasPrefix(share, "https://nh.test/") || strings.Contains(share, "/n/") || !strings.HasPrefix(admin, "https://note.test/") || strings.Contains(admin, "/n/") {
		t.Fatalf("links: %v %s %s", err, share, admin)
	}
	tok, _, _ := strings.Cut(strings.TrimPrefix(share, "https://nh.test/"), "#")

	// The short domain serves nothing: every path goes to the same one on the main site.
	for path, want := range map[string]string{
		"/" + tok:          "https://note.test/" + tok,
		"/n/" + tok + "?x": "https://note.test/" + tok + "?x",
		"/new":             "https://note.test/new",
		"/":                "https://note.test/",
		"/@someone":        "https://note.test/@someone",
	} {
		if res := req("GET", "nh.test", path); res.StatusCode != http.StatusMovedPermanently || res.Header.Get("Location") != want {
			t.Fatalf("short domain %s: %d %s", path, res.StatusCode, res.Header.Get("Location"))
		}
	}
	if res := req("POST", "nh.test", "/api/n/"+tok+"/open"); res.StatusCode != http.StatusPermanentRedirect {
		t.Fatalf("short domain POST keeps its method: %d", res.StatusCode)
	}
	if res := req("GET", "nh.test", "/healthz"); res.StatusCode != http.StatusOK {
		t.Fatalf("short domain healthz: %d", res.StatusCode)
	}

	// The main site: the note at /TOKEN, the old /n/TOKEN moves there.
	res := req("GET", "note.test", "/"+tok)
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK || !strings.Contains(string(body), `data-token="`+tok+`"`) {
		t.Fatalf("main site note page: %d", res.StatusCode)
	}
	if res := req("GET", "note.test", "/n/"+tok+"?x=1"); res.StatusCode != http.StatusMovedPermanently || res.Header.Get("Location") != "/"+tok+"?x=1" {
		t.Fatalf("old note link: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	if res := req("GET", "note.test", "/link-that-does-not-exist"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("missing note: %d", res.StatusCode)
	}
	for _, p := range []string{"/new", "/faq", "/login"} {
		if res := req("GET", "note.test", p); res.StatusCode != http.StatusOK {
			t.Fatalf("site page %s: %d", p, res.StatusCode)
		}
	}
	if a.readerLink("rapat-senin") != "https://nh.test/rapat-senin" || a.ownerLink("rapat-senin") != "https://note.test/rapat-senin" || a.readerLink("faq") != "https://note.test/n/faq" {
		t.Fatalf("links: %s %s %s", a.readerLink("rapat-senin"), a.ownerLink("rapat-senin"), a.readerLink("faq"))
	}
	res = req("GET", "note.test", "/")
	body, _ = io.ReadAll(res.Body)
	if res.StatusCode != 200 || !strings.Contains(string(body), `data-share="https://nh.test"`) || !strings.Contains(string(body), `data-main="https://note.test"`) {
		t.Fatalf("main site page: %d", res.StatusCode)
	}
}

// Notes live at /NAME, so every site page must be a reserved name: otherwise
// someone could take /faq as a link name. A new page needs a line in names/reserved.txt.
func TestSitePagesAreReserved(t *testing.T) {
	src, err := os.ReadFile("app.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile(`"(?:GET|POST) /([a-z0-9-]+)"`).FindAllStringSubmatch(string(src), -1) {
		if shortName(m[1]) {
			t.Errorf("/%s is a site page but not a reserved name", m[1])
		}
	}
}
