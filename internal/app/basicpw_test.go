package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// A basic note with a password: the server can read the text, so the password is a gate and every way to read the
// text has to honour it. These tests are the "who may do what" table for such a note.

const secretBody = `{"t":"Rahasia Helena","b":"isi yang tidak boleh bocor"}`

type lockedEnv struct {
	t   *testing.T
	a   *App
	db  *db.DB
	srv *httptest.Server
}

func newLockedEnv(t *testing.T, cfg Config) *lockedEnv {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Registration = true
	a, err := New(cfg, d)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() { srv.Close(); d.Close() })
	return &lockedEnv{t: t, a: a, db: d, srv: srv}
}

// lockedNote makes a basic note with a password; n.write stands for the proof derived from the password.
func lockedNote(c *client, extra map[string]any) note {
	body := map[string]any{"encrypted": false, "content": secretBody, "password": true, "salt": key()[:22]}
	for k, v := range extra {
		body[k] = v
	}
	return c.create(body)
}

func TestBasicNoteCanHaveAPassword(t *testing.T) {
	c := newClient(t, newTestApp(t))
	n := lockedNote(c, nil)
	if _, m, _ := c.do("GET", "/api/n/"+n.token, nil, nil); m["password"] != true || m["encrypted"] != false || m["salt"] == "" {
		t.Fatalf("meta: %v", m)
	}
	for name, body := range map[string]map[string]any{
		"a salt that is too short":  {"encrypted": false, "content": secretBody, "password": true, "salt": "abc", "mode": "read", "admin": key(), "write": key()},
		"a salt that is not base64": {"encrypted": false, "content": secretBody, "password": true, "salt": strings.Repeat("!", 22), "mode": "read", "admin": key(), "write": key()},
		"no salt":                   {"encrypted": false, "content": secretBody, "password": true, "mode": "read", "admin": key(), "write": key()},
		"a decoy on a basic note":   {"encrypted": false, "content": secretBody, "password": true, "salt": key()[:22], "mode": "read", "admin": key(), "write": key(), "decoy": map[string]any{"content": fakeCipher("x"), "write": key()}},
	} {
		if code, m, _ := c.do("POST", "/api/notes", body, nil); code != 400 {
			t.Errorf("%s: %d %v", name, code, m)
		}
	}
	// the decoy cannot be added later to a basic note either
	if code, m, _ := c.do("PUT", "/api/n/"+n.token+"/decoy", map[string]any{"content": fakeCipher("x"), "write": key()}, map[string]string{"X-Admin": n.admin, "X-Write": n.write}); code != 400 || m["code"] != "err.decoy_needs_password" {
		t.Errorf("decoy on a basic note: %d %v", code, m)
	}
}

// Every way to read the text of a locked basic note needs the password; the wrong one and none at all are refused,
// whatever the link or the admin key.
func TestLockedBasicNoteIsClosedOnEveryReadPath(t *testing.T) {
	e := newLockedEnv(t, Config{FileMB: 20})
	owner := signedIn(t, e.srv, "helena")
	n := lockedNote(owner, map[string]any{"slug": "kunci-helena", "mode": "edit"})
	guest := newClient(t, e.srv)
	read := func(c *client, path string, hdr map[string]string) (int, string) {
		code, _, raw := c.do("GET", path, nil, hdr)
		return code, string(raw)
	}
	noLeak := func(what, body string) {
		t.Helper()
		if strings.Contains(body, "isi yang tidak boleh bocor") || strings.Contains(body, "Rahasia Helena") {
			t.Errorf("%s leaked the text: %s", what, body)
		}
	}
	right := map[string]string{"X-Write": n.write}
	wrong := map[string]string{"X-Write": key()}
	adminOnly := map[string]string{"X-Admin": n.admin}

	// /open: none, wrong, the admin key without the password, right
	for name, h := range map[string]map[string]string{"no password": nil, "wrong password": wrong, "admin key only": adminOnly} {
		code, _, raw := guest.do("POST", "/api/n/"+n.token+"/open", nil, h)
		if code != http.StatusForbidden {
			t.Errorf("open with %s: %d", name, code)
		}
		noLeak("open with "+name, string(raw))
	}
	if code, m, _ := guest.do("POST", "/api/n/"+n.token+"/open", nil, right); code != 200 || m["content"] != secretBody {
		t.Fatalf("open with the password: %d %v", code, m)
	}

	// /raw (curl), /mock (JSON), for the long link and the short name alike
	code, body := read(guest, "/raw/"+n.token, nil)
	noLeak("raw", body)
	if code != http.StatusForbidden || !strings.Contains(body, "password") {
		t.Errorf("raw: %d %q", code, body)
	}
	if code, body := read(guest, "/raw/"+n.token, right); code != http.StatusForbidden { // /raw cannot ask, headers or not
		t.Errorf("raw with the proof header: %d %s", code, body)
	}
	e.db.Exec(`UPDATE notes SET content = '{"t":"x","b":"{\"k\":\"geheim\"}"}' WHERE id = (SELECT note_id FROM links WHERE token = ?)`, n.token)
	if code, body := read(guest, "/mock/"+n.token, nil); code != http.StatusForbidden || strings.Contains(body, "geheim") {
		t.Errorf("mock: %d %s", code, body)
	}
	e.db.Exec(`UPDATE notes SET content = ? WHERE id = (SELECT note_id FROM links WHERE token = ?)`, secretBody, n.token)

	// SSH get
	if out := e.a.sshGet(n.token); strings.Contains(out, "isi yang tidak boleh bocor") || !strings.Contains(out, "password") {
		t.Errorf("ssh get: %q", out)
	}

	// profile: refused when asked for, and left out even if the flag is forced in the database
	if code, m, _ := owner.do("POST", "/api/n/"+n.token+"/config", map[string]any{"profile": true}, map[string]string{"X-Admin": n.admin, "X-Write": n.write}); code != 400 || m["code"] != "err.profile_public" {
		t.Errorf("profile flag: %d %v", code, m)
	}
	e.db.Exec(`UPDATE notes SET config = '{"profile":true}' WHERE id = (SELECT note_id FROM links WHERE token = ?)`, n.token)
	if code, body := read(guest, "/@helena", nil); code != http.StatusNotFound {
		t.Errorf("profile page lists a locked note: %d", code)
		noLeak("profile", body)
	}

	// history and suggestions: the admin key alone is not enough
	owner.do("PUT", "/api/n/"+n.token, map[string]any{"content": `{"t":"Rahasia Helena","b":"isi yang tidak boleh bocor, dua"}`, "version": 1}, map[string]string{"X-Admin": n.admin, "X-Write": n.write})
	for name, path := range map[string]string{"versions": "/api/n/" + n.token + "/versions", "one version": "/api/n/" + n.token + "/versions/1", "suggestions": "/api/n/" + n.token + "/suggestions"} {
		for who, h := range map[string]map[string]string{"admin key only": adminOnly, "admin key, wrong password": {"X-Admin": n.admin, "X-Write": key()}, "no keys": nil} {
			code, body := read(guest, path, h)
			noLeak(name+" for "+who, body)
			if code == 200 {
				t.Errorf("%s for %s: %d", name, who, code)
			}
		}
		if code, body := read(guest, path, map[string]string{"X-Admin": n.admin, "X-Write": n.write}); code != 200 {
			t.Errorf("%s for the admin with the password: %d %s", name, code, body)
		}
	}

	// attachment totals: only with the password (or as the owner)
	if _, m, _ := guest.do("GET", "/api/files/status?note="+n.token, nil, wrong); m["note_used_bytes"] != nil {
		t.Errorf("file status answered a wrong password: %v", m)
	}
	if _, m, _ := guest.do("GET", "/api/files/status?note="+n.token, nil, right); m["note_used_bytes"] == nil {
		t.Errorf("file status refused the right password: %v", m)
	}

	// live: no room without the password
	w, hello := dialLive(t, guest, n.token, key(), "")
	if hello != nil || !w.serverClosed() {
		t.Errorf("live accepted a wrong password: %v", hello)
	}
	w, hello = dialLive(t, guest, n.token, n.write, "")
	if hello == nil || hello["w"] != true {
		t.Errorf("live with the password: %v", hello)
	}
	_ = w

	// the writes need it as well: a wrong password changes nothing
	for name, h := range map[string]map[string]string{"no password": {"X-Admin": n.admin}, "wrong password": {"X-Admin": n.admin, "X-Write": key()}} {
		if code, _, _ := guest.do("PUT", "/api/n/"+n.token, map[string]any{"content": `{"t":"x","b":"overwritten"}`, "version": 2}, h); code != http.StatusForbidden {
			t.Errorf("save with %s: %d", name, code)
		}
	}

	// a snapshot of the note is locked the same way
	_, fr, _ := guest.do("POST", "/api/n/"+n.token+"/freeze", nil, map[string]string{"X-Admin": n.admin})
	snap, _ := fr["token"].(string)
	if snap == "" {
		t.Fatalf("freeze: %v", fr)
	}
	if code, _, raw := guest.do("POST", "/api/n/"+snap+"/open", nil, nil); code != http.StatusForbidden {
		t.Errorf("snapshot open without the password: %d", code)
	} else {
		noLeak("snapshot", string(raw))
	}
	if code, m, _ := guest.do("POST", "/api/n/"+snap+"/open", nil, right); code != 200 || m["content"] == nil {
		t.Errorf("snapshot open with the password: %d %v", code, m)
	}
}

// Wrong guesses are limited per address and per note; the right proof of a working editor is never counted, and
// the owner (signed in) or the admin key are not locked out.
func TestPasswordGuessesAreLimited(t *testing.T) {
	e := newLockedEnv(t, Config{TrustProxy: true})
	owner := signedIn(t, e.srv, "helena")
	n := lockedNote(owner, map[string]any{"slug": "batas-helena", "mode": "edit"})
	from := func(ip string, proof string) int {
		g := newClient(t, e.srv)
		code, _, _ := g.do("POST", "/api/n/"+n.token+"/open", nil, map[string]string{"X-Write": proof, "X-Forwarded-For": ip})
		return code
	}
	// a working editor polls with the right proof all day
	for i := 0; i < 60; i++ {
		if code := from("198.51.100.1", n.write); code != 200 {
			t.Fatalf("poll %d: %d", i, code)
		}
	}
	// ten wrong guesses from one address are answered (403); then even the right proof from it is refused (429)
	for i := 0; i < pwTriesPerMinute; i++ {
		if code := from("198.51.100.2", key()); code != http.StatusForbidden {
			t.Fatalf("guess %d: %d", i, code)
		}
	}
	if code := from("198.51.100.2", key()); code != http.StatusTooManyRequests {
		t.Fatalf("guess after the limit: %d", code)
	}
	if code := from("198.51.100.2", n.write); code != http.StatusTooManyRequests {
		t.Fatalf("the right proof from a blocked address: %d", code)
	}
	// other addresses are not affected, and the editor above keeps working
	if code := from("198.51.100.3", n.write); code != 200 {
		t.Fatalf("another address: %d", code)
	}
	// the same limit holds for the other places that compare the proof
	if _, m, _ := newClient(t, e.srv).do("GET", "/api/files/status?note="+n.token, nil, map[string]string{"X-Write": n.write, "X-Forwarded-For": "198.51.100.2"}); m["note_used_bytes"] != nil {
		t.Errorf("file status answered from a blocked address: %v", m)
	}
	if _, hello := dialLive(t, newClient(t, e.srv), n.token, n.write, ""); hello == nil {
		t.Fatal("the editor's live connection was refused") // (the test client connects from 127.0.0.1, not blocked)
	}

	// the whole note: many addresses, one wrong guess each, past the hourly allowance
	for i := 0; i < pwTriesPerHour; i++ {
		from("203.0.113."+strconv.Itoa(i+1), key()) // one wrong guess from each of many addresses
	}
	if code := from("192.0.2.77", n.write); code != http.StatusTooManyRequests {
		t.Fatalf("the right proof once the note is over its hourly allowance: %d", code)
	}
	// the owner's session and the admin key are never locked out
	if code, m, _ := owner.do("POST", "/api/n/"+n.token+"/open", nil, map[string]string{"X-Write": n.write, "X-Forwarded-For": "192.0.2.78"}); code != 200 {
		t.Fatalf("owner session: %d %v", code, m)
	}
	if code, m, _ := newClient(t, e.srv).do("POST", "/api/n/"+n.token+"/open", nil, map[string]string{"X-Write": n.write, "X-Admin": n.admin, "X-Forwarded-For": "192.0.2.79"}); code != 200 {
		t.Fatalf("admin key: %d %v", code, m)
	}
}

// A wrong password does not spend a view of a self-destructing note, and an embargo still holds after the password.
func TestLockedNoteWithBurnAndEmbargo(t *testing.T) {
	c := newClient(t, newTestApp(t))
	b := lockedNote(c, map[string]any{"max_views": 1})
	for i := 0; i < 3; i++ {
		if code, _, _ := c.do("POST", "/api/n/"+b.token+"/open", nil, map[string]string{"X-Write": key()}); code != http.StatusForbidden {
			t.Fatalf("wrong password %d: %d", i, code)
		}
	}
	if code, m, _ := c.do("POST", "/api/n/"+b.token+"/open", nil, map[string]string{"X-Write": b.write}); code != 200 || m["burned"] != true || m["content"] != secretBody {
		t.Fatalf("the one view: %d %v", code, m)
	}
	if code, _, _ := c.do("POST", "/api/n/"+b.token+"/open", nil, map[string]string{"X-Write": b.write}); code != http.StatusNotFound {
		t.Fatalf("after the view: %d", code)
	}
	em := lockedNote(c, map[string]any{"open_at": time.Now().Add(time.Hour).Unix()})
	if code, _, raw := c.do("POST", "/api/n/"+em.token+"/open", nil, map[string]string{"X-Write": em.write}); code != http.StatusTooEarly || strings.Contains(string(raw), "isi yang tidak boleh bocor") {
		t.Fatalf("embargo with the right password: %d %s", code, raw)
	}
}

// A locked note can neither be turned into an encrypted one nor the other way round while it has a password: the
// proof is made differently in the two forms, so the note would be left impossible to open or edit.
func TestPasswordBlocksConversion(t *testing.T) {
	c := newClient(t, newTestApp(t))
	adm := func(n note) map[string]string { return map[string]string{"X-Admin": n.admin, "X-Write": n.write} }
	b := lockedNote(c, nil)
	if code, m, _ := c.do("POST", "/api/n/"+b.token+"/convert", map[string]any{"to": "encrypted", "content": fakeCipher("x"), "version": 1}, adm(b)); code != 409 || m["code"] != "err.convert_blocked" {
		t.Fatalf("basic with a password to encrypted: %d %v", code, m)
	}
	e := c.create(map[string]any{"password": true, "salt": key()[:22]})
	if code, m, _ := c.do("POST", "/api/n/"+e.token+"/convert", map[string]any{"to": "basic", "content": basicBody, "version": 1}, adm(e)); code != 409 || m["code"] != "err.convert_blocked" {
		t.Fatalf("encrypted with a password to basic: %d %v", code, m)
	}
	if _, m, _ := c.do("GET", "/api/n/"+e.token, nil, nil); m["encrypted"] != true || m["password"] != true {
		t.Fatalf("a refused conversion changed the note: %v", m)
	}
}

// The control center's list and search show no title or text of a locked note, and cannot find one by its text;
// the row still exists (so it can be removed) and says it has a password.
func TestImperiumDoesNotShowLockedNotes(t *testing.T) {
	srv, _, _ := hopApp(t, Config{})
	c := newClient(t, srv)
	lockedNote(c, map[string]any{"slug": "kunci-satu"})
	basic(c, map[string]any{"slug": "terbuka-satu", "content": `{"t":"Terbuka","b":"isi yang tidak boleh bocor tetapi umum"}`})
	_, env := call(t, srv, "imperium", "GET", "/hop/content?type=note", nil)
	var sawLocked, sawOpen bool
	for _, r := range list(env) {
		row := r.(map[string]any)
		if row["password"] == true {
			sawLocked = true
			if row["title"] != "" || row["preview"] != "" {
				t.Errorf("a locked note shows its text in the list: %v", row)
			}
		} else if row["title"] == "Terbuka" {
			sawOpen = true
		}
	}
	if !sawLocked || !sawOpen {
		t.Fatalf("the list: locked=%v open=%v", sawLocked, sawOpen)
	}
	for _, q := range []string{"Rahasia Helena", "yang tidak boleh bocor"} {
		_, env = call(t, srv, "imperium", "GET", "/hop/content?q="+strings.ReplaceAll(q, " ", "%20"), nil)
		for _, r := range list(env) {
			if r.(map[string]any)["password"] == true {
				t.Errorf("search %q found a locked note: %v", q, r)
			}
		}
	}
	_, env = call(t, srv, "imperium", "GET", "/hop/search?q=Rahasia", nil)
	for _, r := range list(env) {
		if strings.Contains(strings.ToLower(r.(map[string]any)["label"].(string)), "rahasia") {
			t.Errorf("the title search found a locked note: %v", r)
		}
	}
}

// The service worker keeps no copy of a locked note: offline it would open without the password.
func TestServiceWorkerSkipsLockedNotes(t *testing.T) {
	b, err := os.ReadFile("../../web/static/js/sw.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	if !strings.Contains(js, "d.password && !d.encrypted") {
		t.Fatal("sw.js does not skip locked basic notes")
	}
}

// A signed-in owner can take a locked note over with a NEW password (the old one is not needed, and a vault reset
// takes it away); a note without a password cannot be given one that way; nobody else can.
func TestTakeoverOfALockedNoteSetsANewPassword(t *testing.T) {
	e := newRecheckEnv(t)
	a := e.member("8301", 0)
	other := e.member("8302", 0)
	n := lockedNote(a.c, map[string]any{"slug": "kunci-8301", "mode": "edit"})
	snapTok := ""
	if _, fr, _ := a.c.do("POST", "/api/n/"+n.token+"/freeze", nil, map[string]string{"X-Admin": n.admin}); fr != nil {
		snapTok, _ = fr["token"].(string)
	}
	path := "/api/me/notes/" + n.token + "/takeover"
	newSalt, newWrite, newAdmin := key()[:22], key(), key()
	if code, _, _ := other.c.do("POST", path, map[string]any{"admin": newAdmin, "write": newWrite, "salt": newSalt}, nil); code != http.StatusForbidden {
		t.Errorf("another account: %d", code)
	}
	if code, _, _ := a.c.do("POST", path, map[string]any{"admin": newAdmin, "write": newWrite}, nil); code != http.StatusBadRequest {
		t.Errorf("a locked note without the salt of a new password: %d", code)
	}
	if code, _, _ := a.c.do("POST", path, map[string]any{"admin": newAdmin, "write": newWrite, "salt": "tiny"}, nil); code != http.StatusBadRequest {
		t.Errorf("a bad salt: %d", code)
	}
	if code, m, _ := newClient(t, e.srv).do("POST", "/api/n/"+n.token+"/open", nil, map[string]string{"X-Write": newWrite}); code != http.StatusForbidden {
		t.Fatalf("before the takeover the new password must not work: %d %v", code, m)
	}
	if code, _, raw := a.c.do("POST", path, map[string]any{"admin": newAdmin, "write": newWrite, "salt": newSalt}, nil); code != http.StatusOK {
		t.Fatalf("takeover: %d %s", code, raw)
	}
	g := newClient(t, e.srv)
	if _, m, _ := g.do("GET", "/api/n/"+n.token, nil, nil); m["password"] != true || m["salt"] != newSalt {
		t.Fatalf("meta after: %v", m)
	}
	if code, _, _ := g.do("POST", "/api/n/"+n.token+"/open", nil, map[string]string{"X-Write": n.write}); code != http.StatusForbidden {
		t.Errorf("the old password still opens the note: %d", code)
	}
	if code, m, _ := g.do("POST", "/api/n/"+n.token+"/open", nil, map[string]string{"X-Write": newWrite}); code != 200 || m["content"] != secretBody {
		t.Errorf("the new password: %d %v", code, m)
	}
	if snapTok != "" {
		if code, _, _ := g.do("POST", "/api/n/"+snapTok+"/open", nil, map[string]string{"X-Write": n.write}); code != http.StatusForbidden {
			t.Errorf("the old password still opens a snapshot: %d", code)
		}
		if code, _, _ := g.do("POST", "/api/n/"+snapTok+"/open", nil, map[string]string{"X-Write": newWrite}); code != 200 {
			t.Errorf("the new password on a snapshot: %d", code)
		}
	}
	// a note without a password cannot be given one this way
	plain := basic(a.c, map[string]any{"slug": "tanpa-8301"})
	if code, _, _ := a.c.do("POST", "/api/me/notes/"+plain.token+"/takeover", map[string]any{"admin": key(), "write": key(), "salt": key()[:22]}, nil); code != http.StatusBadRequest {
		t.Errorf("a salt for a note with no password: %d", code)
	}
	if code, _, _ := a.c.do("POST", "/api/me/notes/"+plain.token+"/takeover", map[string]any{"admin": key(), "write": key()}, nil); code != http.StatusOK {
		t.Errorf("a plain takeover: %d", code)
	}
}

// After a vault reset the owner has lost the password the vault held, but the locked note is still there and the
// owner can set a new one; the vault reset itself never touches the note.
func TestVaultResetKeepsALockedNoteAndTakeoverRecoversIt(t *testing.T) {
	e := newRecheckEnv(t)
	a := e.member("8303", 0)
	n := lockedNote(a.c, map[string]any{"slug": "kunci-8303", "owner_blob": "sealed-with-p"})
	if code, _, raw := a.c.do("POST", resetPath, map[string]string{"confirm": "RESET"}, nil); code != http.StatusOK {
		t.Fatalf("reset: %d %s", code, raw)
	}
	if got := ownerBlobOf(t, e, a.user.ID, n.token); got != "" {
		t.Fatalf("the sealed blob (it held the password) must be gone: %q", got)
	}
	var pw int
	e.db.QueryRow(`SELECT password FROM notes n JOIN links l ON l.note_id = n.id WHERE l.token = ?`, n.token).Scan(&pw)
	if pw != 1 {
		t.Fatal("the reset must leave the note locked")
	}
	if code, _, _ := newClient(t, e.srv).do("POST", "/api/n/"+n.token+"/open", nil, nil); code != http.StatusForbidden {
		t.Fatalf("after the reset the note is still closed to strangers: %d", code)
	}
	e.fresh()
	ns, nw := key()[:22], key()
	if code, _, raw := a.c.do("POST", "/api/me/notes/"+n.token+"/takeover", map[string]any{"admin": key(), "write": nw, "salt": ns}, nil); code != http.StatusOK {
		t.Fatalf("takeover after the reset: %d %s", code, raw)
	}
	if code, m, _ := newClient(t, e.srv).do("POST", "/api/n/"+n.token+"/open", nil, map[string]string{"X-Write": nw}); code != 200 || m["content"] != secretBody {
		t.Fatalf("the new password: %d %v", code, m)
	}
}
