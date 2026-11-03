package app

import (
	"io"
	"strings"
	"testing"
)

// The banner at the top of /privacy and /terms links to the HOP-wide documents.
// It comes from the catalogs, so it reads in the reader's language; before, the
// English text was fixed in the template in every language.
func TestLegalBannerIsTranslated(t *testing.T) {
	c := newClient(t, newTestApp(t))
	for _, page := range []struct{ path, english, sentence, link string }{
		{"/privacy", "Master HOP Privacy Policy", "Both documents govern your data protection rights", "hel.ink/privacy"},
		{"/terms", "Master HOP Agreement", "Both documents are legally binding", "hel.ink/terms"},
	} {
		for _, lang := range []string{"en", "zh-CN", "de", "id", "ja", "pt-BR", "es", "fr", "ru"} {
			t.Run(page.path+"/"+lang, func(t *testing.T) {
				res, err := c.hc.Get(c.srv.URL + page.path + "?lang=" + lang)
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(res.Body)
				res.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				html := string(body)
				if !strings.Contains(html, page.link) {
					t.Errorf("banner link %q missing", page.link)
				}
				for _, text := range []string{page.english, page.sentence} {
					if has := strings.Contains(html, text); has != (lang == "en") {
						t.Errorf("English banner text %q present=%v in %s", text, has, lang)
					}
				}
			})
		}
	}
}
