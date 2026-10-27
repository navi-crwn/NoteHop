package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// Own-key AI: the editor pages may call the providers directly; nothing else may.
func TestAICSP(t *testing.T) {
	on := &App{cfg: Config{AIOrigins: []string{"https://ai.example.com"}}}
	for _, p := range []string{"/new", "/n/abc"} {
		c := on.cspFor(p)
		if !strings.Contains(c, "connect-src 'self' https://api.anthropic.com") || !strings.Contains(c, "https://ai.example.com") {
			t.Errorf("%s: %s", p, c)
		}
	}
	for _, p := range []string{"/", "/dashboard", "/security", "/raw/abc", "/api/notes"} {
		if c := on.cspFor(p); strings.Contains(c, "anthropic") || !strings.Contains(c, "connect-src 'self';") {
			t.Errorf("%s must keep connect-src 'self': %s", p, c)
		}
	}
	for _, mode := range []string{"device", "off"} {
		if c := (&App{cfg: Config{AI: mode}}).cspFor("/new"); strings.Contains(c, "anthropic") {
			t.Errorf("AI=%s opens connect-src: %s", mode, c)
		}
	}
}

// NOTEHOP_AI=off: no AI button at all; device: the button, but no API origins.
func TestAIButton(t *testing.T) {
	for mode, want := range map[string]string{"off": "", "device": `data-ai="device"`, "": `data-ai="on"`} {
		d, err := db.Open(":memory:")
		if err != nil {
			t.Fatal(err)
		}
		a, err := New(Config{AI: mode}, d)
		if err != nil {
			t.Fatal(err)
		}
		srv := httptest.NewServer(a.Handler())
		res, err := http.Get(srv.URL + "/new")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		srv.Close()
		d.Close()
		has := strings.Contains(string(b), `class="btn sm ed-ai"`)
		if want == "" && has || want != "" && !strings.Contains(string(b), want) {
			t.Errorf("AI=%q: button %v", mode, has)
		}
	}
}
