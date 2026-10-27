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
	Code string // BCP 47 tag, also the catalog file name
	Name string // in its own language
	RTL  bool
}

var Langs = []Lang{
	{"en", "English", false},
	{"zh", "中文", false},
	{"es", "Español", false},
	{"ar", "العربية", true},
	{"pt", "Português", false},
	{"ja", "日本語", false},
	{"ru", "Русский", false},
	{"id", "Bahasa Indonesia", false},
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

// Supported reports whether code is one of Langs.
func Supported(code string) bool {
	_, ok := catalogs[code]
	return ok
}

// Get returns the language for code, or the default.
func Get(code string) Lang {
	for _, l := range Langs {
		if l.Code == code {
			return l
		}
	}
	return Langs[0]
}

// Catalog returns the raw catalog (for the browser scripts).
func Catalog(code string) map[string]string {
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

// Dir is the text direction for <html dir>.
func (l Locale) Dir() string {
	if l.RTL {
		return "rtl"
	}
	return "ltr"
}
