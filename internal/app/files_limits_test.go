package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// uploadTo sends one file into the draft of one note being written; the same draft is the same note.
func (c *client) uploadTo(draft string, size int) (int, map[string]any) {
	c.t.Helper()
	req, _ := http.NewRequest("POST", c.srv.URL+"/api/files", bytes.NewReader(make([]byte, size)))
	req.Header.Set("X-Upload-Draft", draft)
	res, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(res.Body).Decode(&m)
	return res.StatusCode, m
}

// One note holds at most 20 MB of attachments in all, and one file may use all of it; the account's own limit is a
// separate, larger one (100 MB by default) over all its notes. The messages name the numbers that are configured.
func TestAttachmentLimitsPerNoteAndPerAccount(t *testing.T) {
	srv, _ := quotaApp(t, Config{FileMB: 20, FilesAccountMB: 100, FilesPerIPMB: 1000, FilesTotalMB: 10000})
	ada := signedIn(t, srv, "ada_lovelace")
	const MB = 1 << 20

	// 20 MB is accepted as one file; one byte more is refused with the NOTE message
	draft := key()
	if code, m := ada.uploadTo(draft, 20*MB); code != 201 {
		t.Fatalf("20 MB: %d %v", code, m)
	}
	if code, m := ada.uploadTo(key(), 20*MB+1); code != 413 || m["code"] != "err.files_note_full" || !strings.Contains(m["error"].(string), "20 MB") {
		t.Fatalf("20 MB + 1 byte: %d %v", code, m)
	}
	// a second file into the same note: the note is full
	if code, m := ada.uploadTo(draft, 1000); code != 413 || m["code"] != "err.files_note_full" {
		t.Fatalf("a full note: %d %v", code, m)
	}
	// far over: refused as too large, naming the configured size
	if code, m := ada.uploadTo(key(), 21*MB); code != 413 || m["code"] != "err.file_too_large" || !strings.Contains(m["error"].(string), "20 MB") {
		t.Fatalf("21 MB: %d %v", code, m)
	}
	// what the editor shows: this note's own number and the account's, apart
	req, _ := http.NewRequest("GET", srv.URL+"/api/files/status", nil)
	req.Header.Set("X-Upload-Draft", draft)
	res, err := ada.hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var st map[string]any
	_ = json.NewDecoder(res.Body).Decode(&st)
	res.Body.Close()
	if st["note_limit_bytes"] != float64(20*MB) || st["note_used_bytes"] != float64(20*MB) || st["account_limit_bytes"] != float64(100*MB) || st["account_used_bytes"] != float64(20*MB) {
		t.Fatalf("status: %v", st)
	}

	// the account: 100 MB in all, over many notes (four more full notes make five); an empty note is refused too
	for i := 0; i < 4; i++ {
		if code, m := ada.uploadTo(key(), 20*MB); code != 201 {
			t.Fatalf("note %d: %d %v", i+2, code, m)
		}
	}
	code, m := ada.uploadTo(key(), 1000)
	if code != 429 || m["code"] != "err.files_account_quota" || !strings.Contains(m["error"].(string), "100 MB") {
		t.Fatalf("the account's 100 MB: %d %v", code, m)
	}
	// splitting into many small notes does not get round it
	for i := 0; i < 3; i++ {
		if code, m := ada.uploadTo(key(), 100); code != 429 || m["code"] != "err.files_account_quota" {
			t.Fatalf("small notes: %d %v", code, m)
		}
	}
	// another account has its own share
	bob := signedIn(t, srv, "bob_barker")
	if code, m := bob.uploadTo(key(), 20*MB); code != 201 {
		t.Fatalf("another account: %d %v", code, m)
	}
}

// A guest is not counted against an account, but the per-address budget still stops them; the configured numbers are
// the ones in the messages, and a file size above the cap is brought down to it.
func TestAttachmentLimitsGuestAndConfiguredNumbers(t *testing.T) {
	srv, _ := quotaApp(t, Config{FileMB: 20, FilesAccountMB: 7, FilesPerIPMB: 30, FilesTotalMB: 10000})
	const MB = 1 << 20
	guest := newClient(t, srv)
	if code, m := guest.uploadTo(key(), 20*MB); code != 201 {
		t.Fatalf("guest 20 MB: %d %v", code, m)
	}
	if code, m := guest.uploadTo(key(), 20*MB); code != 429 || m["code"] != "err.files_quota" || !strings.Contains(m["error"].(string), "30 MB") {
		t.Fatalf("guest over the address budget: %d %v", code, m)
	}
	if code, _, _ := guest.do("GET", "/api/files/status", nil, nil); code != 200 {
		t.Fatal("status")
	}
	ada := signedIn(t, srv, "ada_lovelace")
	if code, m := ada.uploadTo(key(), 8*MB); code != 429 || m["code"] != "err.files_account_quota" || !strings.Contains(m["error"].(string), "7 MB") {
		t.Fatalf("a configured 7 MB account limit: %d %v", code, m)
	}
	// NOTEHOP_FILE_MB above 20 is brought down to 20
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	a2, err := New(Config{FileMB: 50}, d)
	if err != nil {
		t.Fatal(err)
	}
	if a2.cfg.FileMB != 20 {
		t.Fatalf("FileMB 50 became %d", a2.cfg.FileMB)
	}
}
