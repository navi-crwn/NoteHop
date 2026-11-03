package app

import (
	"html/template"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// A template that fails halfway must give a 500, not a 200 with half a page.
func TestRenderErrorDoesNotSendHalfAPage(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	a, err := New(Config{Registration: true}, d)
	if err != nil {
		t.Fatal(err)
	}
	a.pages["about"] = template.Must(template.New("base").Parse(`<!doctype html><html><body><h1>start</h1>{{index .Langs 99}}<p>end</p></body></html>`))
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/about", nil))
	body := rec.Body.String()
	if rec.Code != 500 || strings.Contains(body, "<h1>start</h1>") {
		t.Fatalf("status %d, body starts %q", rec.Code, body[:min(len(body), 60)])
	}
}
