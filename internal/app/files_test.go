package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

func upload(t *testing.T, srv *httptest.Server, body []byte) (int, map[string]any) {
	t.Helper()
	res, err := http.Post(srv.URL+"/api/files", "application/octet-stream", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(res.Body).Decode(&m)
	return res.StatusCode, m
}

func TestFiles(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := New(Config{FileMB: 1, FilesTotalMB: 2}, d)
	srv := httptest.NewServer(a.Handler())
	defer func() { srv.Close(); d.Close() }()

	blob := bytes.Repeat([]byte{7}, 1000)
	code, m := upload(t, srv, blob)
	if code != 201 || len(m["id"].(string)) != 20 {
		t.Fatalf("upload: %d %v", code, m)
	}
	res, _ := http.Get(srv.URL + "/api/files/" + m["id"].(string))
	got, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !bytes.Equal(got, blob) {
		t.Fatalf("download: %d %d bytes", res.StatusCode, len(got))
	}
	if code, _ := upload(t, srv, []byte("short")); code != 400 {
		t.Fatalf("tiny file: %d", code)
	}
	if code, m := upload(t, srv, make([]byte, 2<<20)); code != 413 || m["code"] != "err.file_too_large" {
		t.Fatalf("too large: %d %v", code, m)
	}
	// 2 MB budget: the third ~0.9 MB file doesn't fit.
	big := make([]byte, 900<<10)
	upload(t, srv, big)
	upload(t, srv, big)
	if code, m := upload(t, srv, big); code != 507 || m["code"] != "err.files_full" {
		t.Fatalf("budget: %d %v", code, m)
	}
	if res, _ := http.Get(srv.URL + "/api/files/AAAAAAAAAAAAAAAAAAAA"); res.StatusCode != 404 {
		t.Fatalf("missing: %d", res.StatusCode)
	}
	// Expired files are gone even before the janitor runs, then swept.
	d.Exec(`UPDATE files SET expires = 1`)
	if res, _ := http.Get(srv.URL + "/api/files/" + m["id"].(string)); res.StatusCode != 404 {
		t.Fatalf("expired: %d", res.StatusCode)
	}
	d.Sweep()
	var n int
	d.QueryRow(`SELECT COUNT(*) FROM files`).Scan(&n)
	if n != 0 {
		t.Fatalf("swept: %d left", n)
	}

	off, _ := New(Config{}, d)
	srv2 := httptest.NewServer(off.Handler())
	defer srv2.Close()
	if code, _ := upload(t, srv2, blob); code != 403 {
		t.Fatalf("off: %d", code)
	}
}

func TestDayQuota(t *testing.T) {
	q := &dayQuota{m: map[string]*dayUse{}}
	now := time.Now()
	if !q.take("a", 6, 10, now) || q.take("a", 6, 10, now) || !q.take("b", 6, 10, now) {
		t.Fatal("budget per address")
	}
	if !q.take("a", 6, 10, now.Add(25*time.Hour)) {
		t.Fatal("window resets after a day")
	}
	if !q.take("c", 1<<40, 0, now) {
		t.Fatal("0 means no limit")
	}
}
