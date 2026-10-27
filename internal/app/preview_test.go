package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/internal/db"
)

func TestPreviewText(t *testing.T) {
	title, desc := previewText(&db.Note{Encrypted: true, Password: true, MaxViews: 1})
	if title != "Catatan terenkripsi" || !strings.Contains(desc, "butuh password") || !strings.Contains(desc, "hancur setelah dibaca") {
		t.Fatalf("encrypted: %q %q", title, desc)
	}
	title, _ = previewText(&db.Note{Content: `{"t":"Menu rapat","b":"rahasia"}`})
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
	if !strings.Contains(bot, `og:title" content="Catatan terenkripsi"`) || strings.Contains(bot, "<script") ||
		!strings.Contains(bot, "hancur setelah dibaca") || !strings.Contains(bot, "/static/img/og.png") {
		t.Fatalf("bot page: %s", bot)
	}
	human := get("Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 Chrome/126 Mobile Safari/537.36")
	if !strings.Contains(human, "/static/js/note.js") || !strings.Contains(human, `og:title" content="Catatan terenkripsi"`) {
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
	for _, p := range []string{"/n/nope-nope", "/api/n/nope-nope", "/healthz"} {
		if res := req("nh.test", p); res.StatusCode == http.StatusFound {
			t.Fatalf("share host %s redirected", p)
		}
	}
	res := req("note.test", "/")
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || !strings.Contains(string(body), `data-share="https://nh.test"`) || !strings.Contains(string(body), `data-main="https://note.test"`) {
		t.Fatalf("main site page: %d", res.StatusCode)
	}
}
