package app

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// A Web note's run box has no internet on purpose. When the code asks for something anyway, the frame
// tells the panel (securitypolicyviolation), the panel says so in the reader's language, and the box
// itself is no more open than before.
func TestWebRunReportsBlockedRequests(t *testing.T) {
	js, err := os.ReadFile("../../web/static/js/web.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"securitypolicyviolation", "e.data.blocked", "NH.t('web.blocked'", "blockedNote", "seen[d + ' ' + u]"} {
		if !strings.Contains(string(js), want) {
			t.Errorf("web.js lacks %q", want)
		}
	}
	if !strings.Contains(string(js), "if (!frameWin || e.source !== frameWin") {
		t.Error("the panel must still accept messages only from the frame it created")
	}
	if !strings.Contains(string(js), "sandbox: 'allow-scripts allow-forms allow-modals'") || strings.Contains(string(js), "allow-same-origin") {
		t.Error("the run frame sandbox must stay allow-scripts allow-forms allow-modals")
	}
	for _, bad := range []string{"connect-src 'none'", "img-src data: blob:"} {
		if !strings.Contains(runCSP, bad) {
			t.Errorf("runCSP lost %q", bad)
		}
	}
	for _, lang := range []string{"en", "zh-CN", "de", "id", "ja", "pt-BR", "es", "fr", "ru"} {
		b, err := os.ReadFile("../../web/i18n/" + lang + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var cat map[string]string
		if err := json.Unmarshal(b, &cat); err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{"web.blocked", "web.blocked_note", "web.blk_img", "web.blk_script", "web.blk_font", "web.blk_net", "web.blk_style", "web.blk_media", "web.blk_other"} {
			if strings.TrimSpace(cat[k]) == "" {
				t.Errorf("%s lacks %s", lang, k)
			}
		}
		if !strings.Contains(cat["web.blocked"], "{what}") || !strings.Contains(cat["web.blocked"], "{uri}") {
			t.Errorf("%s: web.blocked needs {what} and {uri}", lang)
		}
	}
}
