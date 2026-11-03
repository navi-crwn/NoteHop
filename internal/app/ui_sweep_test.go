package app

import (
	"encoding/json"
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/web"
)

var (
	tplAction  = regexp.MustCompile(`(?s)\{\{.*?\}\}`)
	tplComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	tplScript  = regexp.MustCompile(`(?s)<(script|style)\b.*?</(script|style)>`)
)

// templates returns every page template with its template actions, comments and scripts removed.
func templates(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	files, err := fs.Glob(web.Templates, "templates/*.html")
	if err != nil || len(files) == 0 {
		t.Fatalf("no templates: %v", err)
	}
	for _, f := range files {
		b, err := fs.ReadFile(web.Templates, f)
		if err != nil {
			t.Fatal(err)
		}
		s := tplComment.ReplaceAllString(string(b), "")
		s = tplScript.ReplaceAllString(s, "")
		out[strings.TrimPrefix(f, "templates/")] = tplAction.ReplaceAllString(s, "")
	}
	return out
}

// A stray closing tag once ended the "More settings" panel early, so half of its
// settings were drawn on the page behind it. Every container must close once.
func TestTemplatesAreBalanced(t *testing.T) {
	for name, s := range templates(t) {
		for _, tag := range []string{"div", "details", "dialog", "section", "form", "nav", "aside", "ul", "ol"} {
			open := len(regexp.MustCompile(`<`+tag+`[\s>]`).FindAllString(s, -1))
			shut := strings.Count(s, "</"+tag+">")
			if open != shut {
				t.Errorf("%s: %d <%s> but %d </%s>", name, open, tag, shut, tag)
			}
		}
	}
}

// All the "More settings" groups live inside the dialog, and nothing of them outside.
func TestMoreSettingsAreAllInTheDialog(t *testing.T) {
	s := templates(t)["new.html"]
	a := strings.Index(s, `<dialog class="drawer" id="adv"`)
	b := strings.Index(s, `</dialog>`)
	if a < 0 || b < a {
		t.Fatal(`new.html: "More settings" must be a <dialog id="adv">`)
	}
	in, out := strings.Count(s[a:b], `class="adv-group`), strings.Count(s[:a], `class="adv-group`)+strings.Count(s[b:], `class="adv-group`)
	if in < 6 || out != 0 {
		t.Errorf("settings groups: %d inside the dialog (want at least 6), %d outside (want 0)", in, out)
	}
}

// The content security policy has no 'unsafe-inline' for styles: an inline style
// attribute is blocked and logged as an error on every visit.
func TestTemplatesHaveNoInlineStyle(t *testing.T) {
	re := regexp.MustCompile(`\sstyle\s*=`)
	for name, s := range templates(t) {
		if re.MatchString(s) {
			t.Errorf("%s: inline style attribute (use a class in app.css)", name)
		}
	}
}

// Labels that screen readers announce are text too: they need to be translated.
func TestTemplatesHaveNoUntranslatedLabels(t *testing.T) {
	re := regexp.MustCompile(`(?:aria-label|title|alt|placeholder)="[^"]*[A-Za-z]{3,}[^"]*"`)
	allowed := map[string]bool{`placeholder="XXXX-XXXX-XXXX-XXXX"`: true}
	for name, s := range templates(t) {
		for _, m := range re.FindAllString(s, -1) {
			if !allowed[m] {
				t.Errorf("%s: untranslated text in %s", name, m)
			}
		}
	}
	for name, s := range templates(t) {
		for _, w := range []string{">Raw<", "Support<", "Policies<", "Master HOP"} {
			if strings.Contains(s, w) {
				t.Errorf("%s: untranslated text %q", name, w)
			}
		}
	}
}

// The Share dialog tells people that an encrypted note's link is its key, in every language.
func TestShareKeyHintInEveryLanguage(t *testing.T) {
	load := func(code string) map[string]string {
		b, err := fs.ReadFile(web.I18n, "i18n/"+code+".json")
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]string{}
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	en := load("en")
	for _, code := range []string{"en", "zh-CN", "de", "id", "ja", "pt-BR", "es", "fr", "ru"} {
		c := load(code)
		for _, k := range []string{"share.key_hint", "footer.support", "footer.policies", "footer.languages", "more.raw", "legal.master_terms_p", "legal.master_privacy_p"} {
			v := strings.TrimSpace(c[k])
			if v == "" {
				t.Errorf("%s: %q is missing", code, k)
			} else if code != "en" && v == strings.TrimSpace(en[k]) {
				t.Errorf("%s: %q is not translated", code, k)
			}
		}
	}
}
