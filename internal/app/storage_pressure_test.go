package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

func TestStoragePauseAndRecovery(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	a, err := New(Config{FileMB: 5, FilesTotalMB: 999999}, d)
	if err != nil {
		t.Fatal(err)
	}
	if a.cfg.FilesTotalMB != 10240 {
		t.Fatal("global cap not enforced")
	}
	free := int64(1 << 30)
	a.space = func(string) (int64, int64) { return free, 20 << 30 }
	h := a.Handler()
	send := func() int {
		r := httptest.NewRequest("POST", "/api/files", bytes.NewReader(make([]byte, 100)))
		r.Header.Set("X-Upload-Draft", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if code := send(); code != 507 {
		t.Fatalf("low disk: %d", code)
	}
	free = 4 << 30
	if code := send(); code != 201 {
		t.Fatalf("recovered: %d", code)
	}
	// Physical usage includes expired blobs until the janitor actually deletes them.
	d.Exec(`UPDATE files SET size = ?, expires = 1`, int64(10)<<30)
	if code := send(); code != 507 {
		t.Fatalf("global full: %d", code)
	}
	d.Sweep()
	if code := send(); code != 201 {
		t.Fatalf("after cleanup: %d", code)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/files/status", nil))
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"available":true`)) {
		t.Fatal(w.Body.String())
	}
}

func TestConcurrentNoteAttachmentBudgetAndDraftClaim(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := d.AddFile(db.File{ID: randToken(20), Scope: "draft:test", Size: 2 << 20, Data: []byte{1}, Expires: time.Now().Add(time.Hour).Unix()}, 10<<30)
			if ok {
				accepted.Add(1)
			} else if err != db.ErrNoteFilesFull {
				t.Errorf("rejection: %v", err)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 2 {
		t.Fatalf("accepted %d", accepted.Load())
	}
	n := &db.Note{Content: "{}", UploadDraft: "draft:test"}
	if err := d.CreateNote(n, &db.Link{Token: "draftnote", Mode: "edit"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.AddFile(db.File{ID: randToken(20), Scope: "draft:test", Size: 1, Data: []byte{1}}, 10<<30); err != db.ErrTaken {
		t.Fatalf("claimed draft reusable: %v", err)
	}
	var total int64
	d.QueryRow(`SELECT SUM(size) FROM files WHERE scope LIKE 'note:%'`).Scan(&total)
	if total != 4<<20 {
		t.Fatalf("claim lost accounting: %d", total)
	}
	if err := d.CreateNote(&db.Note{Content: "{}", UploadDraft: "draft:test"}, &db.Link{Token: "secondnote", Mode: "edit"}); err != db.ErrTaken {
		t.Fatalf("draft claimed twice: %v", err)
	}
}
