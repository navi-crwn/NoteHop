package app

import (
	"io"
	"net/http"
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
