package app

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

func ownerBlobOf(t *testing.T, e *recheckEnv, ownerID int64, token string) string {
	t.Helper()
	var b string
	err := e.db.QueryRow(`SELECT n.owner_blob FROM notes n JOIN links l ON l.note_id = n.id WHERE l.token = ? AND n.owner_id = ?`, token, ownerID).Scan(&b)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A page loaded before a vault reset seals the owner blob with a key that no longer exists. Saving from it keeps the
// text but leaves the stored blob alone; the current generation, and callers that send none, still update it.
func TestSaveFromAnOldVaultGenerationKeepsTheOwnerBlob(t *testing.T) {
	e := newRecheckEnv(t)
	a := e.member("8101", 0)
	adm := map[string]string{"X-Admin": a.basic.admin, "X-Write": a.basic.write}
	save := func(version int, content, epoch, blob string) int {
		h := map[string]string{"X-Admin": adm["X-Admin"], "X-Write": adm["X-Write"]}
		if epoch != "" {
			h["X-Vault-Epoch"] = epoch
		}
		code, _, raw := a.c.do("PUT", "/api/n/"+a.basic.token, map[string]any{"content": content, "version": version, "owner_blob": blob}, h)
		if code != http.StatusOK {
			t.Fatalf("save v%d epoch %q: %d %s", version, epoch, code, raw)
		}
		return code
	}
	if got := ownerBlobOf(t, e, a.user.ID, a.basic.token); got != "sealed-old" {
		t.Fatalf("setup: %q", got)
	}
	save(1, `{"t":"","b":"old1"}`, "7", "x") // a generation that is not the account's (0)
	if got := ownerBlobOf(t, e, a.user.ID, a.basic.token); got != "sealed-old" {
		t.Fatalf("a stale page overwrote the owner blob: %q", got)
	}
	var content string
	e.db.QueryRow(`SELECT n.content FROM notes n JOIN links l ON l.note_id = n.id WHERE l.token = ?`, a.basic.token).Scan(&content)
	if content != `{"t":"","b":"old1"}` {
		t.Fatalf("the text must still be saved: %q", content)
	}
	save(2, `{"t":"","b":"cur2"}`, "0", "sealed-cur")
	if got := ownerBlobOf(t, e, a.user.ID, a.basic.token); got != "sealed-cur" {
		t.Fatalf("the current generation must update the blob: %q", got)
	}
	save(3, `{"t":"","b":"cli3"}`, "", "sealed-cli")
	if got := ownerBlobOf(t, e, a.user.ID, a.basic.token); got != "sealed-cli" {
		t.Fatalf("a caller that sends no generation must update the blob: %q", got)
	}
}

// Claiming a guest note with a page from before a vault reset is refused (409); the current generation passes.
func TestClaimFromAnOldVaultGenerationIsRefused(t *testing.T) {
	e := newRecheckEnv(t)
	a := e.member("8102", 0)
	if code, _, raw := a.c.do("POST", resetPath, map[string]string{"confirm": "RESET"}, nil); code != http.StatusOK {
		t.Fatalf("reset: %d %s", code, raw)
	}
	guest := newClient(t, e.srv)
	n := guest.create(nil)
	claim := func(epoch string) (int, map[string]any) {
		code, m, _ := a.c.do("POST", "/api/n/"+n.token+"/claim", map[string]string{"owner_blob": "sealed"}, map[string]string{"X-Admin": n.admin, "X-Vault-Epoch": epoch})
		return code, m
	}
	if code, m := claim("0"); code != http.StatusConflict || m["code"] != "err.vault_stale" {
		t.Fatalf("an old generation: %d %v", code, m)
	}
	if e.count(`SELECT COUNT(*) FROM notes n JOIN links l ON l.note_id = n.id WHERE l.token = ? AND n.owner_id IS NOT NULL`, n.token) != 0 {
		t.Fatal("a refused claim still took the note")
	}
	if code, m := claim("1"); code != http.StatusOK {
		t.Fatalf("the current generation: %d %v", code, m)
	}
}

// A snapshot that was frozen before the note had an owner has no owner of its own; the new keys still reach it.
func TestSetNoteKeysAlsoReachesASnapshotFrozenBeforeTheClaim(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	u := &db.User{Username: "amir", SSOID: "sso-amir", Email: "amir@example.test"}
	if err := d.CreateSSOUser(u); err != nil {
		t.Fatal(err)
	}
	n := &db.Note{Content: `{"t":"","b":"x"}`, AdminHash: "old-admin", WriteHash: "old-write"}
	if err := d.CreateNote(n, &db.Link{Token: "guest-note", Mode: "read"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Freeze(n, "guest-note-snap"); err != nil { // frozen while the note belonged to nobody
		t.Fatal(err)
	}
	if _, err := d.Exec(`UPDATE notes SET owner_id = ? WHERE id = ?`, u.ID, n.ID); err != nil { // the later claim touches the parent only
		t.Fatal(err)
	}
	if err := d.SetNoteKeys(n.ID, u.ID, "new-admin", "new-write"); err != nil {
		t.Fatal(err)
	}
	var snap string
	d.QueryRow(`SELECT admin_hash FROM notes WHERE parent_id = ? AND frozen = 1`, n.ID).Scan(&snap)
	if snap != "new-admin" {
		t.Fatalf("the snapshot kept the old owner key: %q", snap)
	}
}

// Taking a note over closes its live room: the old writer's connection must not outlive the old keys.
func TestTakeoverClosesTheLiveRoom(t *testing.T) {
	e := newRecheckEnv(t)
	a := e.member("8103", 0)
	n := a.c.create(map[string]any{"slug": "live-takeover", "mode": "edit"})
	writer, hello := dialLive(t, a.c, n.token, n.write, n.admin)
	if hello == nil || hello["w"] != true {
		t.Fatalf("writer hello: %v", hello)
	}
	if code, _, raw := a.c.do("POST", "/api/me/notes/"+n.token+"/takeover", map[string]string{"admin": key(), "write": key()}, nil); code != http.StatusOK {
		t.Fatalf("takeover: %d %s", code, raw)
	}
	if !writer.serverClosed() {
		t.Fatal("the old writer is still connected after the keys changed")
	}
	liveRoomsGone(t, e.app)
}

// One address cannot hold every upload slot: its third slow body is refused while another address still gets in.
func TestOneAddressCannotHoldAllUploadSlots(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	a, err := New(Config{FileMB: 20, Registration: true, TrustProxy: true, FilesPerIPMB: 1000, FilesTotalMB: 10000}, d)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	post := func(ip string, body io.Reader) (int, error) {
		req, _ := http.NewRequest("POST", srv.URL+"/api/files", body)
		req.Header.Set("X-Upload-Draft", key())
		req.Header.Set("X-Forwarded-For", ip)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return -1, err
		}
		res.Body.Close()
		return res.StatusCode, nil
	}
	var wg sync.WaitGroup
	var writers []*io.PipeWriter
	defer func() { // a failed check must not leave the slow bodies open: srv.Close would wait for them
		for _, pw := range writers {
			pw.Close()
		}
	}()
	for i := 0; i < uploadSlotsPerIP; i++ {
		pr, pw := io.Pipe()
		writers = append(writers, pw)
		wg.Add(1)
		go func() { defer wg.Done(); post("203.0.113.9", pr) }()
		pw.Write(make([]byte, 100)) // started, then quiet
	}
	time.Sleep(300 * time.Millisecond)
	if code, _ := post("203.0.113.9", bytes.NewReader(make([]byte, 100))); code != http.StatusServiceUnavailable {
		t.Fatalf("the same address, one more upload: %d", code)
	}
	if code, _ := post("203.0.113.10", bytes.NewReader(make([]byte, 5000))); code != http.StatusCreated {
		t.Fatalf("another address: %d", code)
	}
	for _, pw := range writers {
		pw.Close()
	}
	wg.Wait()
	// slots come back: the first address can upload again
	if code, _ := post("203.0.113.9", bytes.NewReader(make([]byte, 5000))); code != http.StatusCreated {
		t.Fatalf("after the slow uploads ended: %d", code)
	}
}
