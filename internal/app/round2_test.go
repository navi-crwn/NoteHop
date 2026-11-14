package app

import (
	"bytes"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

func (e *recheckEnv) fresh() {
	e.t.Helper()
	if _, err := e.db.Exec(`UPDATE sessions SET verified_at = ?`, time.Now().Unix()); err != nil {
		e.t.Fatal(err)
	}
}

// Taking a note over uses the same strict proof as the vault reset: a stale session is refused even with the cooldown
// cookie (which anybody can set), with the account server unreachable, and with sign-on switched off while the
// session is fresh; a fresh session passes; a password account (no account server) passes; and the attempts are limited.
func TestTakeoverGateIsStrict(t *testing.T) {
	e := newRecheckEnv(t)
	a := e.member("8001", 1)
	e.age()
	path := "/api/me/notes/" + a.basic.token + "/takeover"
	body := func() map[string]string { return map[string]string{"admin": key(), "write": key()} }
	before := func() string {
		var h string
		if err := e.db.QueryRow(`SELECT admin_hash FROM notes WHERE owner_id = ? AND encrypted = 0`, a.user.ID).Scan(&h); err != nil {
			t.Fatal(err)
		}
		return h
	}
	h0 := before()
	if code, _, _ := a.c.do("POST", path, body(), map[string]string{"Cookie": recheckCooldown + "=1"}); code != http.StatusUnauthorized {
		t.Fatalf("stale + cooldown cookie: %d", code)
	}
	e.hubUp = false
	if code, _, _ := a.c.do("POST", path, body(), nil); code != http.StatusUnauthorized {
		t.Fatalf("stale + hub down: %d", code)
	}
	e.hubUp = true
	e.fresh()
	e.app.cfg.SSOSecret = "" // sign-on switched off, the session itself fresh: that clause alone
	if code, _, _ := a.c.do("POST", path, body(), nil); code != http.StatusUnauthorized {
		t.Fatalf("fresh session, sign-on off: %d", code)
	}
	e.app.cfg.SSOSecret = testSecret
	if before() != h0 {
		t.Fatal("a refused takeover changed the keys")
	}
	if code, _, raw := a.c.do("POST", path, body(), nil); code != http.StatusOK {
		t.Fatalf("a freshly vouched session: %d %s", code, raw)
	}

	// a password account has no account server: its session is all there is
	e2 := newRecheckEnv(t)
	local := noRedirect(t, e2.srv)
	if code, _, raw := local.do("POST", "/api/register", map[string]any{"username": "lokal", "auth": key()}, nil); code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("register: %d %s", code, raw)
	}
	n := local.create(map[string]any{"slug": "lokalbasic"})
	if code, _, raw := local.do("POST", "/api/me/notes/"+n.token+"/takeover", body(), nil); code != http.StatusOK {
		t.Fatalf("a password account: %d %s", code, raw)
	}
	// attempts are limited per account (20 a minute)
	limited := false
	for i := 0; i < 25; i++ {
		if code, _, _ := local.do("POST", "/api/me/notes/nosuchtoken/takeover", body(), nil); code == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("no limit on takeover attempts")
	}
}

// The vault reset's own check also stands alone on the sign-on clause: a fresh session with sign-on off is refused.
func TestResetGateRefusesAFreshSessionWhenSignOnIsOff(t *testing.T) {
	e := newRecheckEnv(t)
	a := e.member("8002", 1)
	e.fresh()
	e.app.cfg.SSOSecret = ""
	if code, _, _ := a.c.do("POST", resetPath, map[string]string{"confirm": "RESET"}, nil); code != http.StatusUnauthorized {
		t.Fatalf("fresh session, sign-on off: %d", code)
	}
	e.app.cfg.SSOSecret = testSecret
	e.intact(a, "sign-on off")
}

// The old owner link stops opening the frozen snapshots of the note as admin too.
func TestTakeoverMovesTheSnapshotsToo(t *testing.T) {
	f := seedVaultForRound2(t)
	if err := f.d.SetNoteKeys(f.aNotes[1].ID, f.a.ID, "new-admin", "new-write"); err != nil {
		t.Fatal(err)
	}
	var snap, other string
	f.d.QueryRow(`SELECT admin_hash FROM notes WHERE parent_id = ? AND frozen = 1`, f.aNotes[1].ID).Scan(&snap)
	f.d.QueryRow(`SELECT admin_hash FROM notes WHERE id = ?`, f.bNote.ID).Scan(&other)
	if snap != "new-admin" {
		t.Fatalf("the snapshot kept the old admin hash: %q", snap)
	}
	if other == "new-admin" {
		t.Fatal("another account's note changed")
	}
}

func seedVaultForRound2(t *testing.T) *struct {
	d      *db.DB
	a, b   *db.User
	aNotes []*db.Note
	bNote  *db.Note
} {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	mk := func(name string) *db.User {
		u := &db.User{Username: name, SSOID: "sso-" + name, Email: name + "@example.test"}
		if err := d.CreateSSOUser(u); err != nil {
			t.Fatal(err)
		}
		return u
	}
	f := &struct {
		d      *db.DB
		a, b   *db.User
		aNotes []*db.Note
		bNote  *db.Note
	}{d: d, a: mk("amir"), b: mk("bela")}
	mkn := func(owner int64, tok string) *db.Note {
		n := &db.Note{OwnerID: sql.NullInt64{Int64: owner, Valid: true}, Content: `{"t":"","b":"x"}`, AdminHash: "old-admin", WriteHash: "old-write"}
		if err := d.CreateNote(n, &db.Link{Token: tok, Mode: "read"}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	f.aNotes = []*db.Note{mkn(f.a.ID, "amir-one"), mkn(f.a.ID, "amir-two")}
	f.bNote = mkn(f.b.ID, "bela-one")
	if _, err := d.Freeze(f.aNotes[1], "amir-two-snap"); err != nil {
		t.Fatal(err)
	}
	return f
}

// A note's attachment totals are for people who may write to it: its keys or its signed-in owner, not anybody who
// merely knows a link name.
func TestFileStatusOfANoteIsNotForEveryone(t *testing.T) {
	srv, _ := quotaApp(t, Config{FileMB: 20})
	owner := signedIn(t, srv, "ada_lovelace")
	n := owner.create(map[string]any{"slug": "attach-status"})
	req, _ := http.NewRequest("POST", srv.URL+"/api/n/"+n.token+"/files", bytes.NewReader(make([]byte, 5000)))
	req.Header.Set("X-Admin", n.admin)
	req.Header.Set("X-Write", n.write)
	res, err := owner.hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 201 {
		t.Fatalf("upload to the note: %d", res.StatusCode)
	}
	get := func(c *client, hdr map[string]string) map[string]any {
		_, m, _ := c.do("GET", "/api/files/status?note="+n.token, nil, hdr)
		return m
	}
	if m := get(owner, nil); m["note_used_bytes"] != float64(5000) {
		t.Fatalf("the owner's session: %v", m)
	}
	if m := get(newClient(t, srv), map[string]string{"X-Admin": n.admin}); m["note_used_bytes"] != float64(5000) {
		t.Fatalf("the admin key: %v", m)
	}
	if m := get(newClient(t, srv), nil); m["note_used_bytes"] != nil {
		t.Fatalf("a stranger who knows the link name learned the size: %v", m)
	}
	if m := get(signedIn(t, srv, "bob_barker"), nil); m["note_used_bytes"] != nil {
		t.Fatalf("another account learned the size: %v", m)
	}
}

// After a vault reset a page that was loaded before it must not seal anything with its old key: the server refuses.
func TestWritesSealedWithAnOldVaultKeyAreRefused(t *testing.T) {
	e := newRecheckEnv(t)
	a := e.member("8003", 0)
	cur := "0"
	hdr := func(ep string) map[string]string { return map[string]string{"X-Vault-Epoch": ep} }
	if code, _, raw := a.c.do("POST", resetPath, map[string]string{"confirm": "RESET"}, nil); code != http.StatusOK {
		t.Fatalf("reset: %d %s", code, raw)
	}
	cur = "1"
	body := map[string]any{"encrypted": true, "content": fakeCipher("x"), "mode": "read", "admin": key(), "write": key(), "owner_blob": "sealed-with-the-old-key"}
	if code, m, _ := a.c.do("POST", "/api/notes", body, hdr("0")); code != http.StatusConflict || m["code"] != "err.vault_stale" {
		t.Fatalf("a note sealed with the old key: %d %v", code, m)
	}
	if code, m, _ := a.c.do("PUT", "/api/margins/"+strings.Repeat("a", 64), map[string]any{"content": "old"}, hdr("0")); code == http.StatusOK {
		t.Fatalf("a margin sealed with the old key was stored: %d %v", code, m)
	}
	if e.count(`SELECT COUNT(*) FROM notes WHERE owner_id = ?`, a.user.ID) != 1 { // only the basic note that stayed
		t.Fatal("a refused write made a note")
	}
	// the current generation, and callers that send none (the command line), pass
	body["admin"], body["write"] = key(), key()
	if code, _, raw := a.c.do("POST", "/api/notes", body, hdr(cur)); code != http.StatusCreated {
		t.Fatalf("the current generation: %d %s", code, raw)
	}
	body["admin"], body["write"] = key(), key()
	if code, _, raw := a.c.do("POST", "/api/notes", body, nil); code != http.StatusCreated {
		t.Fatalf("no generation sent: %d %s", code, raw)
	}
}

// A slow uploader no longer holds everybody else up, and the number of bodies read at once is bounded.
func TestASlowUploadDoesNotBlockOthers(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	a, err := New(Config{FileMB: 20, Registration: true, FilesPerIPMB: 1000, FilesTotalMB: 10000}, d)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()

	pr, pw := io.Pipe()
	req, _ := http.NewRequest("POST", srv.URL+"/api/files", pr)
	req.Header.Set("X-Upload-Draft", key())
	done := make(chan int, 1)
	go func() {
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			done <- -1
			return
		}
		res.Body.Close()
		done <- res.StatusCode
	}()
	pw.Write(make([]byte, 100)) // the first upload has started and then goes quiet
	time.Sleep(200 * time.Millisecond)

	finished := make(chan int, 1)
	go func() {
		c := newClient(t, srv)
		code, _ := c.uploadTo(key(), 5000)
		finished <- code
	}()
	select {
	case code := <-finished:
		if code != 201 {
			t.Fatalf("the second upload: %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the second upload waited for the slow one")
	}
	// all slots taken: the next one is told to try again at once
	var hold sync.WaitGroup
	for i := 0; i < uploadSlots-1; i++ { // one slot is the slow upload above
		a.uploadSem <- struct{}{}
		hold.Add(1)
	}
	c := newClient(t, srv)
	if code, m := c.uploadTo(key(), 5000); code != http.StatusServiceUnavailable || m["code"] != "err.files_busy" {
		t.Fatalf("every slot taken: %d %v", code, m)
	}
	for i := 0; i < uploadSlots-1; i++ {
		<-a.uploadSem
	}
	pw.Close()
	if code := <-done; code != http.StatusBadRequest && code != http.StatusCreated {
		t.Fatalf("the slow upload ended with %d", code)
	}
	_ = bytes.MinRead
	_ = strings.TrimSpace
}
