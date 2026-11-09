package app

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/web"
)

// dashboard.js had one shape of bug in thirteen places: everything hung on the
// vault having opened the row (`r.k`), so a note without a readable owner blob
// had no name, no link, no export and a "Delete Untitled" dialog. These are the
// fallbacks that replaced it; the test keeps them from being "simplified" away.
func TestDashboardScriptKeepsItsFallbacks(t *testing.T) {
	b, err := fs.ReadFile(web.Static, "static/js/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	for what, want := range map[string]string{
		"a row is named from the server when the vault can't":   "r.title || r.n.title",
		"a nameless basic note is shown by its address":         "if (!r.n.encrypted) return '/' + r.n.token;",
		"the three locked states are told apart":                "why: 'nokey'",
		"a closed vault is its own state":                       "why: 'closed'",
		"a vault that opens nothing is its own state":           "why: 'mismatch'",
		"a basic note links without a key":                      "r.n.encrypted ? null : NH.noteURL(r.n.token, {})",
		"the delete dialog never says Untitled for a named row": "title: name(r) || '/' + n.token",
		"search also matches the address":                       "r.n.token.toLowerCase().indexOf(q)",
		"an empty result says so":                               "dash.no_results",
		"export counts what it leaves out":                      "dash.export_skipped",
		"export explains a fully locked account":                "dash.export_locked",
		"a dead note leaves the claim offer":                    "x.status === 404",
		"keys of owned notes can be sealed later":               "dash.reseal",
		"the vault can be locked again without signing out":     "NH.store.del('nh_vault')",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("dashboard.js: %s (missing %q)", what, want)
		}
	}
	if strings.Contains(js, "r.title || NH.t('common.untitled') }))") {
		t.Error("dashboard.js: the delete dialog is back to the vault-only title")
	}
}
