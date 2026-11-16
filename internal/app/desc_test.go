package app

import (
	"html"
	"regexp"
	"testing"
	"unicode/utf8"
)

var metaDescRe = regexp.MustCompile(`<meta name="description" content="([^"]*)"`)

// Every page in the sitemap carries its own description, 50 to 160 characters long,
// in every language. Two pages of one language never share a description, because
// search engines then show the same snippet for different pages.
func TestSitemapPagesHaveOwnDescriptions(t *testing.T) {
	srv := newTestApp(t)
	for _, lang := range []string{"en", "zh-CN", "de", "id", "ja", "pt-BR", "es", "fr", "ru"} {
		seen := map[string]string{}
		for _, p := range indexablePages {
			_, body := seoGet(t, srv, p+"?lang="+lang, "")
			m := metaDescRe.FindStringSubmatch(body)
			if m == nil {
				t.Errorf("%s %s: no meta description", lang, p)
				continue
			}
			d := html.UnescapeString(m[1])
			min := 50
			if n := utf8.RuneCountInString(d); n < min || n > 160 {
				t.Errorf("%s %s: description is %d characters (want %d-160): %q", lang, p, n, min, d)
			}
			if other, dup := seen[d]; dup {
				t.Errorf("%s: %s and %s share the description %q", lang, p, other, d)
			}
			seen[d] = p
		}
	}
}
