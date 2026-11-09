package app

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/web"
)

// The HOP control standard (2026-11-09): one radius and one height scale for
// every input, select and button, buttons as wide as their text, capped
// fields. The pages are measured in a browser (see the pull request); this
// keeps the stylesheet from drifting back between measurements.
func TestControlStandardTokens(t *testing.T) {
	b, err := fs.ReadFile(web.Static, "static/css/app.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)
	for _, want := range []string{
		"--hop-ctl-radius: 8px;", "--hop-ctl-radius-sm: 6px;", "--hop-card-radius: 14px;",
		"--hop-ctl-h: 36px;", "--hop-ctl-h-sm: 32px;", "--hop-ctl-h-lg: 40px;", "--hop-ctl-px: 12px;", "--hop-ctl-font: 14px;", "--hop-ring: 3px;",
		"--hop-field-max: 28rem;", "--hop-field-max-narrow: 24rem;", "--hop-field-max-short: 11rem;", "--hop-field-max-xs: 6rem;", "--hop-search-max: 20rem;",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("app.css lacks the token %s", want)
		}
	}
	rule := func(selector string) string {
		i := strings.Index(css, "\n"+selector+" {")
		if i < 0 {
			t.Fatalf("app.css has no rule %q", selector)
		}
		end := strings.Index(css[i:], "}")
		return css[i : i+end]
	}
	for selector, wants := range map[string][]string{
		".btn":      {"height: var(--hop-ctl-h)", "border-radius: var(--hop-ctl-radius)", "white-space: nowrap", "flex-shrink: 0", "width: auto"},
		".btn.sm":   {"height: var(--hop-ctl-h-sm)"},
		".btn.lg":   {"height: var(--hop-ctl-h-lg)"},
		".icon-btn": {"width: var(--hop-ctl-h)", "height: var(--hop-ctl-h)", "border-radius: var(--hop-ctl-radius)"},
		".chip":     {"height: var(--hop-ctl-h-sm)", "border-radius: var(--hop-ctl-radius)"},
		".starter":  {"height: var(--hop-ctl-h-sm)", "border-radius: var(--hop-ctl-radius)"},
		".prefix":   {"height: var(--hop-ctl-h)", "border-radius: var(--hop-ctl-radius)"},
		".auth":     {"max-width: var(--hop-field-max-narrow)"},
		".search":   {"max-width: var(--hop-search-max)"},
	} {
		body := rule(selector)
		for _, want := range wants {
			if !strings.Contains(body, want) {
				t.Errorf("%s must have %q", selector, want)
			}
		}
	}
	// Text fields: capped, on the standard height and radius, 16px on phones.
	for _, want := range []string{
		"max-width: var(--hop-field-max); height: var(--hop-ctl-h); padding: 0 var(--hop-ctl-px);",
		"input[type=password], input[type=email] { max-width: var(--hop-field-max-narrow); }",
		"select, textarea { font-size: 16px; }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("app.css lacks %q", want)
		}
	}
	// No pill-shaped or odd-radius buttons and inputs: a radius on these rules (the control itself, with or
	// without a modifier class, not something inside it such as a counter badge) is the token.
	odd := regexp.MustCompile(`(?m)^\.(btn|icon-btn|chip|starter|prefix|seg)(\.[a-z-]+)* \{[^}]*border-radius: (\d+px|999px)`)
	if m := odd.FindString(css); m != "" {
		t.Errorf("a control rule sets its own radius: %s", strings.SplitN(m, "{", 2)[0])
	}
}

func TestUIStyleCookie(t *testing.T) {
	for _, tc := range []struct{ cookie, want string }{{"", ""}, {"clean", "clean"}, {"hop", ""}, {"Clean", ""}, {"x", ""}} {
		r := httptest.NewRequest("GET", "/", nil)
		if tc.cookie != "" {
			r.AddCookie(&http.Cookie{Name: "hop_ui", Value: tc.cookie})
		}
		if got := uiStyle(r); got != tc.want {
			t.Errorf("hop_ui=%q: got %q, want %q", tc.cookie, got, tc.want)
		}
	}
}
