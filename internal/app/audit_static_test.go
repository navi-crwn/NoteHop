package app

import (
	"encoding/json"
	"io"
	"io/fs"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/web"
)

func staticJS(t *testing.T, name string) string {
	t.Helper()
	b, err := fs.ReadFile(web.Static, "static/js/"+name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func catalog(t *testing.T, lang string) map[string]string {
	t.Helper()
	b, err := fs.ReadFile(web.I18n, "i18n/"+lang+".json")
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]string{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

var allLangs = []string{"en", "zh-CN", "de", "id", "ja", "pt-BR", "es", "fr", "ru"}

// A2-S2-10: the Open link of an embed leaves no Referer, like the other external links.
func TestEmbedLinkHasNoReferrer(t *testing.T) {
	js := staticJS(t, "blocks.js")
	if strings.Contains(js, "rel: 'noopener' }") || strings.Contains(js, "rel: 'noopener noreferrer' }") || strings.Count(js, "rel: 'noopener noreferrer nofollow'") < 2 {
		t.Error("blocks.js: both external links of an embed must use rel='noopener noreferrer nofollow'")
	}
}

// A2-S2-09: every button that copies a link with the key in it says that the link is the key.
func TestCopyingAKeyedLinkSaysSo(t *testing.T) {
	for _, f := range []string{"dashboard.js", "diffpage.js", "extras.js"} {
		if !strings.Contains(staticJS(t, f), "copy.key_hint") {
			t.Errorf("%s copies a link with its key without the hint", f)
		}
	}
	for _, lang := range allLangs {
		if catalog(t, lang)["copy.key_hint"] == "" {
			t.Errorf("%s: copy.key_hint missing", lang)
		}
	}
}

// A2-S2-22: the privacy page says what a report keeps and that the owner part of a link is sent to the server.
func TestPrivacyNamesReportsAndOwnerToken(t *testing.T) {
	c := newClient(t, newTestApp(t))
	marks := map[string]string{
		"en": "owner part", "zh-CN": "所有者部分", "de": "Inhaber-Teil", "id": "bagian pemilik", "ja": "オーナー部分",
		"pt-BR": "parte do dono", "es": "parte del propietario", "fr": "partie propriétaire", "ru": "часть владельца",
	}
	for _, lang := range allLangs {
		res, err := c.hc.Get(c.srv.URL + "/privacy?lang=" + lang)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if !strings.Contains(string(b), marks[lang]) {
			t.Errorf("%s: /privacy does not mention %q", lang, marks[lang])
		}
	}
}
