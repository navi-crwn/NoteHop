package app

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// A2-S2-20: a panic in a handler gives a translated 500 page (or the JSON
// error for the API), is logged, and does not take the connection down.
func TestRecoverGivesAnErrorPage(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	a, err := New(Config{Registration: true}, d)
	if err != nil {
		t.Fatal(err)
	}
	a.mux.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) { panic("boom secret-detail") })
	a.mux.HandleFunc("GET /api/boom", func(http.ResponseWriter, *http.Request) { panic("boom") })
	a.mux.HandleFunc("GET /half", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "partial")
		w.(http.Flusher).Flush()
		panic("late")
	})
	a.mux.HandleFunc("GET /abort", func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })
	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(io.Discard)
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()

	get := func(path string) (int, string, http.Header) {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b), res.Header
	}
	for _, lang := range allLangs {
		code, body, h := get("/boom?lang=" + lang)
		want := catalog(t, lang)["error.fault_title"]
		if code != 500 || want == "" || !strings.Contains(body, want) {
			t.Errorf("%s: status %d, title %q missing in page", lang, code, want)
		}
		if strings.Contains(body, "secret-detail") || strings.Contains(body, "goroutine") {
			t.Errorf("%s: the page leaks the panic", lang)
		}
		if h.Get("Content-Security-Policy") == "" || !strings.Contains(h.Get("Content-Type"), "text/html") {
			t.Errorf("%s: headers %v", lang, h)
		}
	}
	code, body, _ := get("/api/boom")
	var m map[string]any
	if json.Unmarshal([]byte(body), &m) != nil || code != 500 || m["code"] != "err.internal" {
		t.Errorf("api: %d %s", code, body)
	}
	if !strings.Contains(logs.String(), "boom secret-detail") || !strings.Contains(logs.String(), "panic") {
		t.Errorf("the panic is not logged: %q", logs.String())
	}
	// Once the page has started there is nothing to swap in: the connection is cut, not finished as if all was well.
	if res, err := http.Get(srv.URL + "/half"); err == nil {
		_, err = io.ReadAll(res.Body)
		res.Body.Close()
		if err == nil {
			t.Error("a page that failed after it started ended cleanly")
		}
	}
	if res, err := http.Get(srv.URL + "/abort"); err == nil {
		res.Body.Close()
		t.Errorf("ErrAbortHandler should abort the request, got %d", res.StatusCode)
	}
	// And the server still answers.
	if code, _, _ := get("/about"); code != 200 {
		t.Errorf("after a panic /about gives %d", code)
	}
}
