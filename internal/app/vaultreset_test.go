package app

import (
	"bytes"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// A HOP ID account (sub) with a vault set up, signed in on its own client, with two notes, a margin, a
// recovery code and an unclaimed upload.
type resetMember struct {
	c     *client
	user  *db.User
	notes []note // encrypted
	basic note   // a basic note: it stays through a reset
}

func (e *recheckEnv) member(sub string, notes int) *resetMember {
	e.t.Helper()
	c := noRedirect(e.t, e.srv)
	e.signIn(c, sub)
	u, err := e.db.UserBySSO(sub)
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.db.SetVaultCheck(u.ID, "check-"+sub); err != nil {
		e.t.Fatal(err)
	}
	if err := e.db.SetSecret(u.ID, true, "check-"+sub, "WRAPSECRET"+sub); err != nil {
		e.t.Fatal(err)
	}
	m := &resetMember{c: c, user: u}
	for i := 0; i < notes; i++ {
		m.notes = append(m.notes, c.create(nil))
	}
	m.basic = c.create(map[string]any{"slug": "keep" + sub})
	if _, err := e.db.Exec(`UPDATE notes SET owner_blob = 'sealed-old' WHERE owner_id = ?`, u.ID); err != nil {
		e.t.Fatal(err)
	}
	if err := e.db.SetRecoveryCodes(u.ID, []db.RecoveryCode{{Slot: 0, ProofHash: "p" + sub, Wrap: "w"}}); err != nil {
		e.t.Fatal(err)
	}
	if err := e.db.SetMargin(u.ID, "ref-"+sub, "private words"); err != nil {
		e.t.Fatal(err)
	}
	f := db.File{ID: "draft-" + sub, Size: 4, Data: []byte("data"), Created: time.Now().Unix(), Expires: time.Now().Add(time.Hour).Unix(), Scope: "draft:" + sub, OwnerID: u.ID}
	if ok, err := e.db.AddFileFor(f, 0, 0); err != nil || !ok {
		e.t.Fatalf("draft upload: %v %v", ok, err)
	}
	return m
}

func (e *recheckEnv) count(q string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.db.QueryRow(q, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// everything of the account is still there
func (e *recheckEnv) intact(m *resetMember, why string) {
	e.t.Helper()
	if n := e.count(`SELECT COUNT(*) FROM notes WHERE owner_id = ?`, m.user.ID); n != len(m.notes)+1 {
		e.t.Fatalf("%s: %d notes left of %d", why, n, len(m.notes)+1)
	}
	if n := e.count(`SELECT COUNT(*) FROM notes WHERE owner_id = ? AND owner_blob = 'sealed-old'`, m.user.ID); n != len(m.notes)+1 {
		e.t.Fatalf("%s: owner blobs changed", why)
	}
	if n := e.count(`SELECT COUNT(*) FROM margins WHERE user_id = ?`, m.user.ID); n != 1 {
		e.t.Fatalf("%s: margins %d", why, n)
	}
	if n := e.count(`SELECT COUNT(*) FROM recovery_codes WHERE user_id = ?`, m.user.ID); n != 1 {
		e.t.Fatalf("%s: recovery codes %d", why, n)
	}
	if n := e.count(`SELECT COUNT(*) FROM files WHERE id = ?`, "draft-"+m.user.SSOID); n != 1 {
		e.t.Fatalf("%s: draft upload %d", why, n)
	}
	u, _ := e.db.UserByID(m.user.ID)
	if u.VaultCheck != "check-"+m.user.SSOID || u.VaultWrap != "WRAPSECRET"+m.user.SSOID {
		e.t.Fatalf("%s: unlock secrets changed", why)
	}
}

var resetPath = "/api/me/vault/reset"

// The wipe is for the owner, who types the word, and for nobody else: no session, a session the account server no
// longer vouches for, another origin, a wrong or missing word, a password account. Every refusal deletes nothing.
func TestResetVaultRefusals(t *testing.T) {
	e := newRecheckEnv(t)
	a := e.member("1001", 2)
	b := e.member("1002", 1)

	anon := noRedirect(t, e.srv)
	if code, _, _ := anon.do("POST", resetPath, map[string]string{"confirm": "RESET"}, nil); code != http.StatusUnauthorized {
		t.Fatalf("without a session: %d", code)
	}
	if code, _, _ := anon.do("GET", resetPath, nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("preview without a session: %d", code)
	}
	if code, _, raw := a.c.do("POST", resetPath, map[string]string{"confirm": "RESET"}, map[string]string{"Sec-Fetch-Site": "cross-site"}); code != http.StatusForbidden {
		t.Fatalf("from another site: %d %s", code, raw)
	}
	if code, _, raw := a.c.do("POST", resetPath, map[string]string{"confirm": "RESET"}, map[string]string{"Origin": "https://evil.test"}); code != http.StatusForbidden {
		t.Fatalf("from another origin: %d %s", code, raw)
	}
	for _, word := range []string{"", "reset", " RESET", "RESET ", "DELETE"} { // five tries is the most a minute allows; the page trims, the server does not
		if code, m, raw := a.c.do("POST", resetPath, map[string]string{"confirm": word}, nil); code != http.StatusBadRequest || m["code"] != "err.reset_confirm" {
			t.Fatalf("word %q: %d %s", word, code, raw)
		}
	}
	e.intact(a, "after wrong words")

	// the account server has not vouched for this session lately: it counts as nobody until re-checked
	e.age()
	if code, _, _ := a.c.do("POST", resetPath, map[string]string{"confirm": "RESET"}, nil); code != http.StatusUnauthorized {
		t.Fatalf("stale session: %d", code)
	}
	e.intact(a, "stale session")
	e.intact(b, "stale session (other account)")

	// a password account has no separate vault key to forget
	e2 := newRecheckEnv(t)
	local := noRedirect(t, e2.srv)
	if code, _, raw := local.do("POST", "/api/register", map[string]any{"username": "lokal", "auth": key()}, nil); code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("register: %d %s", code, raw)
	}
	local.create(nil)
	if code, m, _ := local.do("POST", resetPath, map[string]string{"confirm": "RESET"}, nil); code != http.StatusBadRequest || m["code"] != "err.reset_hopid_only" {
		t.Fatalf("password account: %d %v", code, m)
	}
	if n := e2.count(`SELECT COUNT(*) FROM notes`); n != 1 {
		t.Fatalf("a refused reset of a password account deleted notes: %d left", n)
	}
	if code, _, _ := local.do("GET", resetPath, nil, nil); code != http.StatusBadRequest {
		t.Fatalf("preview for a password account: %d", code)
	}
}

// The word is tried a few times a minute per account; the sixth try is turned away even with the right word.
func TestResetVaultIsRateLimited(t *testing.T) {
	e := newRecheckEnv(t)
	a := e.member("2001", 1)
	for i := 0; i < 5; i++ {
		if code, _, _ := a.c.do("POST", resetPath, map[string]string{"confirm": "nope"}, nil); code != http.StatusBadRequest {
			t.Fatalf("try %d: %d", i+1, code)
		}
	}
	if code, _, _ := a.c.do("POST", resetPath, map[string]string{"confirm": "RESET"}, nil); code != http.StatusTooManyRequests {
		t.Fatalf("sixth try: %d", code)
	}
	e.intact(a, "rate limited")
}

// The whole flow as a member lives it: the preview says what goes, the reset takes it, the old links die like any
// deleted note's, other devices are signed out, nobody else is touched, a new phrase can be made and new notes
// written. Nothing about keys reaches the log or the audit entry.
func TestResetVaultFlow(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	e := newRecheckEnv(t)
	a := e.member("3001", 2)
	other := e.member("3002", 1)
	device2 := noRedirect(t, e.srv) // the same account signed in on another device
	e.signIn(device2, "3001")
	if e.count(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, a.user.ID) != 2 {
		t.Fatal("test setup: two sessions expected")
	}

	code, m, raw := a.c.do("GET", resetPath, nil, nil)
	counts, _ := m["counts"].(map[string]any)
	if code != http.StatusOK || m["word"] != "RESET" || counts["notes"] != float64(2) || counts["kept"] != float64(1) || counts["files"] != float64(1) || counts["margins"] != float64(1) || counts["codes"] != float64(1) || counts["links"] != float64(2) {
		t.Fatalf("preview: %d %s", code, raw)
	}
	if e.count(`SELECT COUNT(*) FROM notes WHERE owner_id = ?`, a.user.ID) != 3 {
		t.Fatal("the preview deleted something")
	}

	code, m, raw = a.c.do("POST", resetPath, map[string]string{"confirm": "RESET"}, nil)
	if code != http.StatusOK || m["ok"] != true || m["next"] != "/vault" {
		t.Fatalf("reset: %d %s", code, raw)
	}
	got, _ := m["counts"].(map[string]any)
	if got["notes"] != float64(2) || got["files"] != float64(1) || got["links"] != float64(2) {
		t.Fatalf("what it took: %s", raw)
	}

	// gone from every table, and the old links answer as deleted notes do
	for table, q := range map[string]string{
		"encrypted notes": `SELECT COUNT(*) FROM notes WHERE owner_id = ? AND encrypted = 1`,
		"margins":         `SELECT COUNT(*) FROM margins WHERE user_id = ?`,
		"codes":           `SELECT COUNT(*) FROM recovery_codes WHERE user_id = ?`,
		"files":           `SELECT COUNT(*) FROM files WHERE owner_id = ?`,
	} {
		if n := e.count(q, a.user.ID); n != 0 {
			t.Errorf("%s: %d rows left", table, n)
		}
	}
	for _, n := range a.notes {
		if code, _, _ := a.c.do("GET", "/api/n/"+n.token, nil, nil); code != http.StatusNotFound && code != http.StatusGone {
			t.Errorf("old link %s answers %d", n.token, code)
		}
		if code, _, _ := noRedirect(t, e.srv).do("GET", "/api/n/"+n.token, nil, nil); code != http.StatusNotFound && code != http.StatusGone {
			t.Errorf("old link %s for a visitor answers %d", n.token, code)
		}
	}
	// the basic note stays: readable by anyone with its link, still the account's, no blob sealed with the old key
	if code, _, _ := noRedirect(t, e.srv).do("GET", "/api/n/"+a.basic.token, nil, nil); code != http.StatusOK {
		t.Errorf("the basic note after the reset: %d", code)
	}
	if e.count(`SELECT COUNT(*) FROM notes WHERE owner_id = ? AND encrypted = 0 AND owner_blob = ''`, a.user.ID) != 1 {
		t.Error("the basic note lost its owner or kept its old blob")
	}
	u, _ := e.db.UserByID(a.user.ID)
	if u.VaultCheck != "" || u.VaultWrap != "" {
		t.Errorf("unlock secrets left: %q %q", u.VaultCheck, u.VaultWrap)
	}

	// this device is still in; the other is out
	if code, name := e.me(a.c); code != http.StatusOK || name == "" {
		t.Errorf("the device that reset was signed out: %d", code)
	}
	if code, _ := e.me(device2); code != http.StatusUnauthorized {
		t.Errorf("another device is still signed in: %d", code)
	}

	// nobody else was touched
	e.intact(other, "after another account's reset")
	if code, _, _ := other.c.do("GET", "/api/n/"+other.notes[0].token, nil, nil); code != http.StatusOK {
		t.Errorf("the other account's note: %d", code)
	}

	// a new vault key can be made, and notes written again
	if code, m, raw := a.c.do("POST", "/api/vault", map[string]string{"check": key()}, nil); code != http.StatusCreated || m["created"] != true {
		t.Fatalf("new vault key: %d %s", code, raw)
	}
	a.c.create(nil)
	if e.count(`SELECT COUNT(*) FROM notes WHERE owner_id = ?`, a.user.ID) != 2 {
		t.Error("a new note after the reset")
	}

	// the record: one audit entry with the counts; no key material anywhere in the log or the entry
	var action, detail string
	if err := e.db.QueryRow(`SELECT action, detail FROM hop_audit WHERE action = 'user.vault_reset_self'`).Scan(&action, &detail); err != nil {
		t.Fatalf("no audit entry: %v", err)
	}
	if !strings.Contains(detail, `"notes":2`) || !strings.Contains(detail, `"kept":1`) {
		t.Errorf("audit detail: %s", detail)
	}
	for _, secret := range []string{"WRAPSECRET3001", "check-3001", "RESET", e.sessionCookie(a.c)} {
		if strings.Contains(logs.String(), secret) || strings.Contains(detail, secret) {
			t.Errorf("%q reached the log or the audit entry", secret)
		}
	}
}

// The way in: a danger zone on the security page, and a link to it where people get stuck (the locked notice on the
// dashboard, the vault page next to "forgot"). Only HOP ID accounts see any of it; a password account does not.
func TestResetVaultIsOfferedWhereAPersonGetsStuck(t *testing.T) {
	e := newRecheckEnv(t)
	a := e.member("4001", 0)
	for path, want := range map[string][]string{
		"/security":  {`id="reset"`, `id="reset-open"`, `id="reset-dlg"`, `id="reset-go"`, `disabled`, `sec-danger`},
		"/dashboard": {`id="vault-locked"`, `href="/security#reset"`},
		"/vault":     {`href="/recover"`, `href="/security#reset"`},
	} {
		code, _, body := e.status(a.c, path)
		if code != http.StatusOK {
			t.Fatalf("%s: %d", path, code)
		}
		for _, w := range want {
			if !strings.Contains(body, w) {
				t.Errorf("%s (HOP ID account) lacks %s", path, w)
			}
		}
	}
	e2 := newRecheckEnv(t)
	local := noRedirect(t, e2.srv)
	local.do("POST", "/api/register", map[string]any{"username": "lokal", "auth": key()}, nil)
	for _, path := range []string{"/security", "/dashboard"} {
		code, _, body := e2.status(local, path)
		if code != http.StatusOK {
			t.Fatalf("%s: %d", path, code)
		}
		for _, w := range []string{`id="reset"`, `id="reset-dlg"`, `/security#reset`} {
			if strings.Contains(body, w) {
				t.Errorf("%s (password account) offers the reset: %s", path, w)
			}
		}
	}
}

// The proof is strict. A stale session is refused even when the cooldown cookie (which anybody can set) is
// present, when the account server cannot be reached, and when single sign-on is switched off; nothing is deleted.
func TestResetVaultGateIgnoresTheLenientShortcuts(t *testing.T) {
	e := newRecheckEnv(t)
	a := e.member("5001", 2)
	e.age()
	post := func(hdr map[string]string) int {
		code, _, _ := a.c.do("POST", resetPath, map[string]string{"confirm": "RESET"}, hdr)
		return code
	}
	// the page itself still opens for this stale session while the cooldown cookie is set: that leniency is for pages
	if code, _, _ := e.status(a.c, "/about"); code != http.StatusOK {
		t.Fatalf("setup: %d", code)
	}
	if code := post(map[string]string{"Cookie": recheckCooldown + "=1"}); code != http.StatusUnauthorized {
		t.Fatalf("stale session with the cooldown cookie: %d", code)
	}
	e.hubUp = false
	if code := post(nil); code != http.StatusUnauthorized {
		t.Fatalf("stale session with the hub down: %d", code)
	}
	if code, m, _ := a.c.do("GET", resetPath, nil, nil); code != http.StatusUnauthorized || m["code"] != "err.reset_recheck" {
		t.Fatalf("preview with the hub down: %d %v", code, m)
	}
	e.hubUp = true
	e.app.cfg.SSOSecret = "" // single sign-on switched off
	if code := post(nil); code != http.StatusUnauthorized {
		t.Fatalf("stale session with sign-on off: %d", code)
	}
	e.app.cfg.SSOSecret = testSecret
	e.intact(a, "every lenient shortcut")
	// a session the account server vouched for a moment ago goes through (strict, not broken)
	if _, err := e.db.Exec(`UPDATE sessions SET verified_at = ?`, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	if code := post(nil); code != http.StatusOK {
		t.Fatalf("a freshly vouched session: %d", code)
	}
}

// The page carries the vault generation, and it goes up with a reset, so a browser that kept an older key drops it.
func TestResetVaultRaisesTheGenerationOnThePage(t *testing.T) {
	e := newRecheckEnv(t)
	a := e.member("6001", 1)
	_, _, before := e.status(a.c, "/dashboard")
	if !strings.Contains(before, `data-epoch="0"`) {
		t.Fatal("the page does not carry the vault generation")
	}
	if code, _, raw := a.c.do("POST", resetPath, map[string]string{"confirm": "RESET"}, nil); code != http.StatusOK {
		t.Fatalf("reset: %d %s", code, raw)
	}
	_, _, after := e.status(a.c, "/vault")
	if !strings.Contains(after, `data-epoch="1"`) {
		t.Fatal("the generation did not go up after a reset")
	}
	// the browser half: crypto.js keeps the generation with the key and drops a key of an older one
	js, err := os.ReadFile("../../web/static/js/crypto.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"e: +document.body.dataset.epoch", "(v.e || 0) !== (+document.body.dataset.epoch || 0)"} {
		if !strings.Contains(string(js), want) {
			t.Errorf("crypto.js lacks %q", want)
		}
	}
}
