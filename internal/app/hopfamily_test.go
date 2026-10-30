package app

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestHopFamilyTypography(t *testing.T) {
	srv := newTestApp(t)
	res, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	body := string(b)
	for _, want := range []string{
		`href="https://hel.ink/hop-assets/v1/fonts.css" integrity="sha384-`,
		`https://hel.ink/hop-assets/v1/apps/pixelhop/mark.svg`,
		`>LinkHop<`,
		`Part of the HOP family`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("home page lacks %q", want)
		}
	}
	if strings.Contains(body, "apps/notehop/mark.svg") {
		t.Error("the family list should not link NoteHop to itself")
	}
	csp := res.Header.Get("Content-Security-Policy")
	for _, d := range []string{"script-src 'self';", "style-src 'self' https://hel.ink;", "font-src 'self' https://hel.ink;"} {
		if !strings.Contains(csp, d) {
			t.Errorf("CSP lacks %q: %s", d, csp)
		}
	}
}
