package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/internal/db"
)

func getBody(t *testing.T, srv *httptest.Server, path, lang string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", srv.URL+path, nil)
	if lang != "" {
		req.AddCookie(&http.Cookie{Name: langCookie, Value: lang})
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func TestDedicatedPages(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(Config{Registration: true, GitHub: "navi-crwn/NoteHop", Version: "9.9.9"}, d)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() { srv.Close(); d.Close() })

	for path, want := range map[string]string{
		"/features": "Features",
		"/status":   "NoteHop is up.",
		"/contact":  "https://github.com/navi-crwn/NoteHop/issues/new/choose",
		"/about":    "",
		"/faq":      "",
		"/privacy":  "",
	} {
		code, body := getBody(t, srv, path, "")
		if code != 200 {
			t.Errorf("%s: status %d", path, code)
			continue
		}
		if want != "" && !strings.Contains(body, want) {
			t.Errorf("%s lacks %q", path, want)
		}
		if !strings.Contains(body, `class="back" href="/"`) || !strings.Contains(body, "Back to NoteHop") {
			t.Errorf("%s: no back link", path)
		}
	}
	if _, body := getBody(t, srv, "/status", ""); !strings.Contains(body, "9.9.9") {
		t.Error("/status does not show the version")
	}
	if _, body := getBody(t, srv, "/contact", ""); !strings.Contains(body, "/security/advisories/new") {
		t.Error("/contact lacks the private security report link")
	}
	if _, body := getBody(t, srv, "/", ""); strings.Contains(body, `class="back"`) {
		t.Error("the home page should not show a back link")
	}
	if _, body := getBody(t, srv, "/features", "id"); !strings.Contains(body, "Kembali ke NoteHop") {
		t.Error("the back link is not translated")
	}
}
