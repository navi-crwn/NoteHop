package app

import (
	"io"
	"net/http"
	"net/http/httptest"
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
	title, _ = previewText(&db.Note{Content: `{"t":"Menu rapat","b":"rahasia"}`}, en)
	if title != "Menu rapat" {
		t.Fatalf("public note title: %q", title)
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
	req := func(host, path string) *http.Response {
		r, _ := http.NewRequest("GET", "http://"+host+path, nil)
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Result()
	}
	if res := req("nh.test", "/dashboard?x=1"); res.StatusCode != http.StatusFound || res.Header.Get("Location") != "https://note.test/dashboard?x=1" {
		t.Fatalf("share host dashboard: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	if res := req("nh.test", "/"); res.StatusCode != http.StatusFound {
		t.Fatalf("share host root: %d", res.StatusCode)
	}
	// What a note page loads is served here too: a redirect to the main site
	// would be blocked by the page's own CSP.
	for _, p := range []string{"/api/n/nope-nope", "/healthz", "/i18n/en.js", "/run", "/render/mermaid", "/sw.js", "/static/js/app.js", "/api/files/abc"} {
		if res := req("nh.test", p); res.StatusCode == http.StatusFound || res.StatusCode == http.StatusMovedPermanently {
			t.Fatalf("share host %s redirected to %s", p, res.Header.Get("Location"))
		}
	}
	// Short links: nh.test/TOKEN is the note page; the long form moves there.
	share, admin, err := a.createPublic("Rapat", "isi")
	if err != nil || !strings.HasPrefix(share, "https://nh.test/") || strings.Contains(share, "/n/") || !strings.HasPrefix(admin, "https://note.test/n/") {
		t.Fatalf("links: %v %s %s", err, share, admin)
	}
	tok, _, _ := strings.Cut(strings.TrimPrefix(share, "https://nh.test/"), "#")
	res := req("nh.test", "/"+tok)
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK || !strings.Contains(string(body), `data-token="`+tok+`"`) {
		t.Fatalf("share host short link: %d", res.StatusCode)
	}
	if res := req("nh.test", "/n/rapat-senin?x=1"); res.StatusCode != http.StatusMovedPermanently || res.Header.Get("Location") != "/rapat-senin?x=1" {
		t.Fatalf("share host long link: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	// Site pages are reserved names, so they still go to the main site.
	for _, p := range []string{"/new", "/faq", "/login", "/dashboard", "/@someone", "/recover"} {
		if res := req("nh.test", p); res.StatusCode != http.StatusFound || res.Header.Get("Location") != "https://note.test"+p {
			t.Fatalf("share host %s: %d %s", p, res.StatusCode, res.Header.Get("Location"))
		}
	}
	// The main site keeps the long form (owners, and old links).
	if res := req("note.test", "/n/"+tok); res.StatusCode != http.StatusOK {
		t.Fatalf("main site note page: %d", res.StatusCode)
	}
	if got := a.readerLink("rapat-senin"); got != "https://nh.test/rapat-senin" {
		t.Fatalf("reader link: %s", got)
	}
	res = req("note.test", "/")
	body, _ = io.ReadAll(res.Body)
	if res.StatusCode != 200 || !strings.Contains(string(body), `data-share="https://nh.test"`) || !strings.Contains(string(body), `data-main="https://note.test"`) {
		t.Fatalf("main site page: %d", res.StatusCode)
	}
}
