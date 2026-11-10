package app

import (
	"strings"
	"testing"
)

// Every page footer says the app belongs to Hop Easy Link, in the reader's language.
func TestFooterPartOfFamily(t *testing.T) {
	srv := newTestApp(t)
	for lang, want := range map[string]string{"en": "Part of <b>Hop Easy Link</b>", "de": "Teil von <b>Hop Easy Link</b>"} {
		body, _ := fetch(t, srv, "/about?lang="+lang, "en-US,en;q=0.9")
		if !strings.Contains(body, `class="part-of" href="https://hel.ink"`) || !strings.Contains(body, want) {
			t.Fatalf("%s: footer row missing %q", lang, want)
		}
	}
}
