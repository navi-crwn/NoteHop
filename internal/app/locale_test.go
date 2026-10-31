package app

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/navi-crwn/NoteHop/internal/db"
)

func mustURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}

// fetch returns the HTML body of a GET with optional Accept-Language header.
func fetch(t *testing.T, srv *httptest.Server, path, lang string) (string, *http.Response) {
	t.Helper()
	req, _ := http.NewRequest("GET", srv.URL+path, nil)
	if lang != "" {
		req.Header.Set("Accept-Language", lang)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return string(b), res
}

// htmlLang extracts the <html lang> attribute from a page.
func htmlLang(s string) string {
	start := strings.Index(s, `<html lang="`)
	if start == -1 {
		return ""
	}
	start += len(`<html lang="`)
	end := strings.Index(s[start:], `"`)
	if end == -1 {
		return ""
	}
	return s[start : start+end]
}

// assertJarContains fails if jar does not contain a cookie with name and value for u.
func assertJarContains(t *testing.T, jar *cookiejar.Jar, u *url.URL, name, value string) {
	t.Helper()
	for _, c := range jar.Cookies(u) {
		if c.Name == name && c.Value == value {
			return
		}
	}
	t.Fatalf("jar for %q does not contain %s=%s; got %v", u.String(), name, value, jar.Cookies(u))
}

// cookieCaptureHandler wraps h and records every Cookie header received.
func cookieCaptureHandler(h http.Handler) (http.Handler, *[]string) {
	var cookies []string
	var mu sync.Mutex
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		cookies = append(cookies, r.Header.Get("Cookie"))
		mu.Unlock()
		h.ServeHTTP(w, r)
	}), &cookies
}

// newTestAppWithCookieCapture returns a test server and a slice that records
// the Cookie header of every request.
func newTestAppWithCookieCapture(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(Config{Registration: true}, d)
	if err != nil {
		t.Fatal(err)
	}
	h, cookies := cookieCaptureHandler(a.Handler())
	srv := httptest.NewServer(h)
	t.Cleanup(func() { srv.Close(); d.Close() })
	return srv, cookies
}

// pageTitle returns the visible header title from the catalog.
func pageTitle(s string) string {
	start := strings.Index(s, `<title>`)
	if start == -1 {
		return ""
	}
	start += len(`<title>`)
	end := strings.Index(s[start:], `</title>`)
	if end == -1 {
		return ""
	}
	return s[start : start+end]
}

func TestLocaleDefaultEnglishForAnonymous(t *testing.T) {
	srv := newTestApp(t)

	cases := []struct {
		accept, wantLang, wantTitle string
	}{
		{"", "en", "New note"},                        // no header
		{"id-ID,id;q=0.9", "en", "New note"},          // Indonesian header ignored
		{"fr-CA,fr;q=0.9,en;q=0.8", "en", "New note"}, // French header ignored
		{"*;q=1", "en", "New note"},                   // wildcard ignored
		{"de-DE,de;q=0.9", "en", "New note"},          // unsupported header ignored
		{"id;q=0.8,xx,en;q=0.7", "en", "New note"},    // mix ignored
	}
	for _, c := range cases {
		body, res := fetch(t, srv, "/new", c.accept)
		if got := htmlLang(body); got != c.wantLang {
			t.Errorf("Accept-Language %q: html lang = %q, want %q", c.accept, got, c.wantLang)
		}
		if !strings.Contains(pageTitle(body), c.wantTitle) {
			t.Errorf("Accept-Language %q: title %q does not contain %q", c.accept, pageTitle(body), c.wantTitle)
		}
		vary := res.Header.Get("Vary")
		if strings.Contains(vary, "Accept-Language") {
			t.Errorf("Accept-Language %q: Vary still claims Accept-Language: %q", c.accept, vary)
		}
		if !strings.Contains(vary, "Cookie") {
			t.Errorf("Accept-Language %q: Vary should still mention Cookie: %q", c.accept, vary)
		}
	}
}

func TestLocaleExplicitChoiceWins(t *testing.T) {
	srv := newTestApp(t)

	// ?lang= overrides anonymous English default.
	body, _ := fetch(t, srv, "/new?lang=ru", "en-US,en;q=0.9")
	if got := htmlLang(body); got != "ru" {
		t.Fatalf("?lang=ru: html lang = %q, want ru", got)
	}
	if !strings.Contains(pageTitle(body), "Новая заметка") {
		t.Fatalf("?lang=ru: title %q does not contain Russian title", pageTitle(body))
	}

	// Cookie overrides anonymous default.
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	set, _ := http.NewRequest("GET", srv.URL+"/new?lang=id", nil)
	if _, err := client.Do(set); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", srv.URL+"/new", nil)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if got := htmlLang(string(b)); got != "id" {
		t.Fatalf("nh_lang=id cookie: html lang = %q, want id", got)
	}
	if !strings.Contains(pageTitle(string(b)), "Catatan baru") {
		t.Fatalf("nh_lang=id cookie: title %q does not contain Indonesian title", pageTitle(string(b)))
	}

	// Cookie wins over a conflicting Accept-Language header.
	conflictReq, _ := http.NewRequest("GET", srv.URL+"/new", nil)
	conflictReq.Header.Set("Accept-Language", "fr-CA,fr;q=0.9")
	conflictRes, err := client.Do(conflictReq)
	if err != nil {
		t.Fatal(err)
	}
	defer conflictRes.Body.Close()
	conflictBody, _ := io.ReadAll(conflictRes.Body)
	if got := htmlLang(string(conflictBody)); got != "id" {
		t.Fatalf("nh_lang=id cookie vs fr-CA header: html lang = %q, want id", got)
	}

	// ?lang= still overrides a conflicting Accept-Language header.
	body, _ = fetch(t, srv, "/new?lang=fr", "id-ID,id;q=0.9")
	if got := htmlLang(body); got != "fr" {
		t.Fatalf("?lang=fr: html lang = %q, want fr", got)
	}

	// Invalid query does not overwrite a valid cookie.
	invalidReq, _ := http.NewRequest("GET", srv.URL+"/new?lang=xx", nil)
	invalidRes, err := client.Do(invalidReq)
	if err != nil {
		t.Fatal(err)
	}
	invalidBody, _ := io.ReadAll(invalidRes.Body)
	invalidRes.Body.Close()
	if got := htmlLang(string(invalidBody)); got != "id" {
		t.Fatalf("invalid ?lang with cookie: html lang = %q, want id", got)
	}

	// Verify the actual cookie name and value that the server set.
	cookies := jar.Cookies(set.URL)
	var langCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "nh_lang" {
			langCookie = c
			break
		}
	}
	if langCookie == nil {
		t.Fatalf("expected nh_lang cookie to be set, got cookies: %v", cookies)
	}
	if langCookie.Value != "id" {
		t.Fatalf("nh_lang cookie value = %q, want id", langCookie.Value)
	}
}

func TestLocaleInvalidFallback(t *testing.T) {
	srv := newTestApp(t)
	for _, c := range []string{"?lang=xx", "?lang=", "?lang=123"} {
		body, _ := fetch(t, srv, "/new"+c, "id")
		if got := htmlLang(body); got != "en" {
			t.Errorf("%q: html lang = %q, want en", c, got)
		}
	}

	// An unsupported cookie falls back to English without panic.
	srv, captured := newTestAppWithCookieCapture(t)
	jar, _ := cookiejar.New(nil)
	jar.SetCookies(mustURL(srv.URL+"/new"), []*http.Cookie{{Name: "nh_lang", Value: "xx"}})
	assertJarContains(t, jar, mustURL(srv.URL+"/new"), "nh_lang", "xx")
	client := &http.Client{Jar: jar}
	req, _ := http.NewRequest("GET", srv.URL+"/new", nil)
	req.Header.Set("Accept-Language", "id-ID,id;q=0.9")
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if got := htmlLang(string(b)); got != "en" {
		t.Fatalf("unsupported nh_lang=xx cookie: html lang = %q, want en", got)
	}
	// Verify the cookie was actually transported to the server.
	found := false
	for _, c := range *captured {
		if strings.Contains(c, "nh_lang=xx") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("server never received nh_lang=xx cookie; captured Cookie headers: %v", *captured)
	}
}

func TestPreviewHTMLLangMatchesEffectiveLocale(t *testing.T) {
	c := newClient(t, newTestApp(t))
	n := c.create(map[string]any{"max_views": 1})

	bot := func(path, lang string) (string, *http.Response) {
		req, _ := http.NewRequest("GET", c.srv.URL+path, nil)
		req.Header.Set("User-Agent", "WhatsApp/2.23.20.0 A")
		if lang != "" {
			req.Header.Set("Accept-Language", lang)
		}
		res, err := c.hc.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return string(b), res
	}

	// Anonymous bot preview must be English despite Indonesian Accept-Language.
	body, _ := bot("/n/"+n.token, "id-ID,id;q=0.9")
	if got := htmlLang(body); got != "en" {
		t.Fatalf("anonymous preview html lang = %q, want en", got)
	}

	// Explicit ?lang= applies to the preview too.
	body, _ = bot("/n/"+n.token+"?lang=ru", "en-US,en;q=0.9")
	if got := htmlLang(body); got != "ru" {
		t.Fatalf("?lang=ru preview html lang = %q, want ru", got)
	}

	// Bot preview of a public note uses the catalog title in the chosen language.
	content := `{"t":"Hello world","b":"body"}`
	pub := c.create(map[string]any{"encrypted": false, "content": content})
	body, _ = bot("/n/"+pub.token+"?lang=id", "en")
	if got := htmlLang(body); got != "id" {
		t.Fatalf("?lang=id public preview html lang = %q, want id", got)
	}
}
