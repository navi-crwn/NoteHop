// Package i18n holds NoteHop's interface text. Each language is a flat JSON
// catalog in web/i18n (key → text, "{name}" placeholders). English is the
// default and the fallback for any key a translation lacks.
package i18n

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"strings"
	"time"

	"github.com/navi-crwn/NoteHop/web"
)

const Default = "en"

// Lang is one supported language, in the order the picker shows them.
type Lang struct {
	Code  string // BCP 47 tag, also the catalog file name
	Name  string // in its own language (menu, left)
	Short string // short label (menu, right)
}

// The nine languages every HOP app ships, in the order the menu shows them.
// Right-to-left languages are not part of the set.
var Langs = []Lang{
	{"en", "English", "EN"},
	{"zh-CN", "简体中文", "中文"},
	{"de", "Deutsch", "DE"},
	{"id", "Bahasa Indonesia", "ID"},
	{"ja", "日本語", "日本語"},
	{"pt-BR", "Português (Brasil)", "PT"},
	{"es", "Español", "ES"},
	{"fr", "Français", "FR"},
	{"ru", "Русский", "RU"},
}

// legacy are the codes stored before the set was fixed (cookies) and what they mean now.
var legacy = map[string]string{"zh": "zh-CN", "pt": "pt-BR"}

// Resolve maps a requested or stored code to a supported one: "zh" and "pt"
// mean "zh-CN" and "pt-BR", case does not matter, and anything else (such as
// the retired "ar") is not supported.
func Resolve(code string) (string, bool) {
	code = strings.TrimSpace(code)
	if l, ok := legacy[strings.ToLower(code)]; ok {
		code = l
	}
	for _, l := range Langs {
		if strings.EqualFold(l.Code, code) {
			return l.Code, true
		}
	}
	return "", false
}

// catalogs are complete per language: English first, the translation on
// top, so a missing translation shows English rather than a key.
var catalogs = map[string]map[string]string{}

func init() {
	base := load(Default)
	for _, l := range Langs {
		m := make(map[string]string, len(base))
		for k, v := range base {
			m[k] = v
		}
		if l.Code != Default {
			for k, v := range load(l.Code) {
				if v != "" || base[k] == "" {
					m[k] = v
				}
			}
		}
		catalogs[l.Code] = m
	}
}

func load(code string) map[string]string {
	b, err := fs.ReadFile(web.I18n, "i18n/"+code+".json")
	if err != nil {
		panic(fmt.Sprintf("i18n: %s: %v", code, err))
	}
	m := map[string]string{}
	if err := json.Unmarshal(b, &m); err != nil {
		panic(fmt.Sprintf("i18n: %s: %v", code, err))
	}
	return m
}

// Supported reports whether code is one of Langs (or an old code for one).
func Supported(code string) bool {
	_, ok := Resolve(code)
	return ok
}

// FromHeader picks the best supported language from an Accept-Language
// header ("fr-CA,fr;q=0.9,en;q=0.8"), by quality then order; English when
// nothing matches. Regional tags count as their language (fr-CA → fr); the
// supported Chinese and Portuguese are zh-CN and pt-BR, whatever the region.
//
// NoteHop no longer uses Accept-Language to choose the UI locale for
// anonymous visitors, but FromHeader is kept as a reusable utility in case
// a future feature needs it.
func FromHeader(h string) string {
	best, bestQ := Default, 0.0
	for i, part := range strings.Split(h, ",") {
		tag, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		q := 1.0
		if v, ok := strings.CutPrefix(strings.TrimSpace(params), "q="); ok {
			if _, err := fmt.Sscanf(v, "%g", &q); err != nil {
				continue
			}
		}
		code, ok := Resolve(strings.ToLower(strings.SplitN(strings.SplitN(tag, "-", 2)[0], "_", 2)[0]))
		// Later entries break ties in favour of earlier ones.
		q -= float64(i) * 1e-6
		if ok && q > bestQ {
			best, bestQ = code, q
		}
	}
	return best
}

// Get returns the language for code, or the default.
func Get(code string) Lang {
	if c, ok := Resolve(code); ok {
		code = c
	}
	for _, l := range Langs {
		if l.Code == code {
			return l
		}
	}
	return Langs[0]
}

// Catalog returns the raw catalog (for the browser scripts).
func Catalog(code string) map[string]string {
	if r, ok := Resolve(code); ok {
		code = r
	}
	if c, ok := catalogs[code]; ok {
		return c
	}
	return catalogs[Default]
}

// T looks up key in lang (falling back to English, then the key itself) and
// fills "{name}" placeholders from name/value pairs. A value that is a
// time.Time is written as a UTC date, since the server doesn't know the
// reader's zone.
func T(lang, key string, kv ...any) string {
	if r, ok := Resolve(lang); ok {
		lang = r
	}
	c, ok := catalogs[lang]
	if !ok {
		c = catalogs[Default]
	}
	s, ok := c[key]
	if !ok {
		s = key
	}
	for i := 0; i+1 < len(kv); i += 2 {
		name, _ := kv[i].(string)
		var v string
		switch x := kv[i+1].(type) {
		case time.Time:
			v = x.UTC().Format("2006-01-02 15:04 UTC")
		case int64:
			// API errors pass times as unix seconds (the browser formats them).
			if name == "time" {
				v = time.Unix(x, 0).UTC().Format("2006-01-02 15:04 UTC")
			} else {
				v = fmt.Sprint(x)
			}
		default:
			v = fmt.Sprint(x)
		}
		s = strings.ReplaceAll(s, "{"+name+"}", v)
	}
	return s
}

// Locale binds a language for templates: {{.L.T "key"}}, and {{.L.H "key"}}
// for catalog entries that contain markup (catalogs are our own files).
type Locale struct{ Lang }

func (l Locale) T(key string, kv ...any) string { return T(l.Code, key, kv...) }

// H trusts the catalog text but escapes the values filled into it, which
// may come from users (a username, a title).
func (l Locale) H(key string, kv ...any) template.HTML {
	esc := make([]any, len(kv))
	for i, v := range kv {
		if i%2 == 1 {
			if _, isTime := v.(time.Time); !isTime {
				v = template.HTMLEscapeString(fmt.Sprint(v))
			}
		}
		esc[i] = v
	}
	return template.HTML(T(l.Code, key, esc...))
}

// Dir is the text direction for <html dir>: always left to right, the set has no right-to-left language.
func (l Locale) Dir() string { return "ltr" }
