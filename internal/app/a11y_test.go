package app

import (
	"io"
	"regexp"
	"testing"
)

// Icon-only formatting buttons need an accessible name. A title tooltip is
// not a reliable substitute on touch devices or for assistive technology.
func TestFormattingButtonsHaveAccessibleNames(t *testing.T) {
	c := newClient(t, newTestApp(t))
	for _, lang := range []string{"en", "zh-CN", "de", "id", "ja", "pt-BR", "es", "fr", "ru"} {
		t.Run(lang, func(t *testing.T) {
			res, err := c.hc.Get(c.srv.URL + "/new?lang=" + lang)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(res.Body)
			res.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			html := string(body)
			for _, item := range []struct{ cmd, key string }{
				{"bold", "ed.bold"}, {"h2", "ed.heading"}, {"todo", "ed.todo"},
				{"code", "ed.code"}, {"table", "ed.table"}, {"spoiler", "ed.spoiler"}, {"locked", "ed.locked"},
			} {
				re := regexp.MustCompile(`<button[^>]*data-cmd="` + item.cmd + `"[^>]*>`)
				tag := re.FindString(html)
				if tag == "" {
					t.Errorf("data-cmd=%q button not found", item.cmd)
					continue
				}
				name := regexp.MustCompile(`aria-label="([^"]*)"`).FindStringSubmatch(tag)
				if len(name) != 2 || name[1] == "" || name[1] == item.key {
					t.Errorf("data-cmd=%q has no localized aria-label: %s", item.cmd, tag)
				}
			}
		})
	}
}
