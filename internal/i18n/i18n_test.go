package i18n

import (
	"encoding/json"
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/navi-crwn/NoteHop/web"
)

var (
	placeholderRe = regexp.MustCompile(`\{[a-z_]+\}`)
	tagRe         = regexp.MustCompile(`</?[a-z]+[^>]*>`)
)

func raw(t *testing.T, code string) map[string]string {
	b, err := fs.ReadFile(web.I18n, "i18n/"+code+".json")
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]string{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%s: %v", code, err)
	}
	return m
}

func sorted(re *regexp.Regexp, s string) []string {
	x := re.FindAllString(s, -1)
	slices.Sort(x)
	return x
}

// Every translation must have exactly the English keys, and keep the same
// placeholders and markup, or text silently goes missing or breaks pages.
func TestCatalogs(t *testing.T) {
	en := raw(t, Default)
	for _, l := range Langs {
		if l.Code == Default {
			continue
		}
		c := raw(t, l.Code)
		for k := range c {
			if _, ok := en[k]; !ok {
				t.Errorf("%s: unknown key %q", l.Code, k)
			}
		}
		for k, e := range en {
			v, ok := c[k]
			if !ok {
				t.Errorf("%s: missing %q", l.Code, k)
				continue
			}
			if !slices.Equal(sorted(placeholderRe, e), sorted(placeholderRe, v)) {
				t.Errorf("%s: %q placeholders differ: %q vs %q", l.Code, k, e, v)
			}
			if !slices.Equal(sorted(tagRe, e), sorted(tagRe, v)) {
				t.Errorf("%s: %q markup differs: %q vs %q", l.Code, k, e, v)
			}
			if e != "" && strings.TrimSpace(v) == "" && !strings.HasSuffix(k, "_keys") {
				t.Errorf("%s: %q is empty", l.Code, k)
			}
		}
	}
}

func TestT(t *testing.T) {
	if got := T("en", "pill.opened", "n", 2, "max", 3); got != "Opened 2/3" {
		t.Fatalf("T = %q", got)
	}
	if got := T("xx", "nav.new"); got != "New note" {
		t.Fatalf("unknown language should fall back to English, got %q", got)
	}
	if got := T("en", "no.such.key"); got != "no.such.key" {
		t.Fatalf("missing key = %q", got)
	}
	if got := T("en", "err.locked", "time", time.Unix(0, 0)); !strings.Contains(got, "1970-01-01 00:00 UTC") {
		t.Fatalf("time value = %q", got)
	}
	l := Locale{Get("en")}
	if got := string(l.H("new.saved_to", "user", "<b>x</b>")); !strings.Contains(got, "&lt;b&gt;x&lt;/b&gt;") || !strings.Contains(got, "<b>@") {
		t.Fatalf("H must escape values but keep catalog markup: %q", got)
	}
	for _, code := range []string{"en", "ar", "de", "ru"} {
		if (Locale{Get(code)}).Dir() != "ltr" {
			t.Fatalf("%s must be left to right", code)
		}
	}
}

func TestFromHeader(t *testing.T) {
	for h, want := range map[string]string{
		"":                           "en",
		"fr-CA,fr;q=0.9,en;q=0.8":    "fr",
		"pt-BR":                      "pt-BR",
		"pt-PT,pt;q=0.9":             "pt-BR",
		"de-DE,de;q=0.9":             "de",
		"de;q=0.9,id;q=0.8,en;q=0.7": "de",
		"zh-TW,zh;q=0.9":             "zh-CN",
		"ja;q=0.5,ar;q=0.6":          "ja",
		"ar,ar-SA;q=0.9":             "en",
		"es,fr":                      "es",
		"en-US,en;q=0.9,id;q=0.8":    "en",
		"xx;q=1,ru;q=0.1":            "ru",
		"id-ID;q=bogus,ja":           "ja",
	} {
		if got := FromHeader(h); got != want {
			t.Errorf("FromHeader(%q) = %q, want %q", h, got, want)
		}
	}
}

func TestLangs(t *testing.T) {
	expected := []string{"en", "zh-CN", "de", "id", "ja", "pt-BR", "es", "fr", "ru"}
	if len(Langs) != len(expected) {
		t.Fatalf("expected %d languages, got %d", len(expected), len(Langs))
	}
	for i, code := range expected {
		if Langs[i].Code != code {
			t.Errorf("Langs[%d] = %q, want %q", i, Langs[i].Code, code)
		}
		if Langs[i].Name == "" || Langs[i].Short == "" {
			t.Errorf("%s has no menu name or short code", code)
		}
	}
}

func TestResolve(t *testing.T) {
	for in, want := range map[string]string{"en": "en", "EN": "en", "zh": "zh-CN", "zh-cn": "zh-CN", "pt": "pt-BR", "PT-br": "pt-BR", " de ": "de", "ru": "ru"} {
		if got, ok := Resolve(in); !ok || got != want {
			t.Errorf("Resolve(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "ar", "xx", "zh-TW", "pt-PT"} {
		if got, ok := Resolve(in); ok {
			t.Errorf("Resolve(%q) = %q; a retired or unknown language must not resolve", in, got)
		}
	}
	if Get("ar").Code != "en" || Get("zh").Code != "zh-CN" {
		t.Error("Get must fall back to English and map the old short codes")
	}
}
