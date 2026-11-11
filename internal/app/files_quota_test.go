package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

func quotaApp(t *testing.T, cfg Config) (*httptest.Server, *db.DB) {
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
	return srv, d
}

// upload as this client (its cookie, if signed in); every call is a fresh draft.
func (c *client) upload(size int) (int, map[string]any) {
	c.t.Helper()
	req, _ := http.NewRequest("POST", c.srv.URL+"/api/files", bytes.NewReader(make([]byte, size)))
	req.Header.Set("X-Upload-Draft", key())
	res, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(res.Body).Decode(&m)
	return res.StatusCode, m
}

func signedIn(t *testing.T, srv *httptest.Server, name string) *client {
	t.Helper()
	c := newClient(t, srv)
	if code, m, _ := c.do("POST", "/api/register", map[string]any{"username": name, "auth": key()}, nil); code != 201 {
		t.Fatalf("register %s: %d %v", name, code, m)
	}
	return c
}

func TestAccountAttachmentQuota(t *testing.T) {
	srv, d := quotaApp(t, Config{FileMB: 1, FilesAccountMB: 1})
	ada, bob, guest := signedIn(t, srv, "ada_lovelace"), signedIn(t, srv, "bob_barker"), newClient(t, srv)

	if code, m := ada.upload(600 << 10); code != 201 {
		t.Fatalf("first: %d %v", code, m)
	}
	// 600 KB + 600 KB passes the 1 MB limit.
	code, m := ada.upload(600 << 10)
	if code != 429 || m["code"] != "err.files_account_quota" {
		t.Fatalf("over quota: %d %v", code, m)
	}
	// The limit is per account: another account and a guest are not affected by it.
	if code, m := bob.upload(600 << 10); code != 201 {
		t.Fatalf("other account: %d %v", code, m)
	}
	if code, m := guest.upload(600 << 10); code != 201 {
		t.Fatalf("guest: %d %v", code, m)
	}
	// What the account holds is shown to it, and only to it.
	_, st, _ := ada.do("GET", "/api/files/status", nil, nil)
	if st["account_limit_bytes"] != float64(1<<20) || st["account_used_bytes"] != float64(600<<10) || st["available"] != true {
		t.Fatalf("status: %v", st)
	}
	if _, gs, _ := guest.do("GET", "/api/files/status", nil, nil); gs["account_limit_bytes"] != nil {
		t.Fatalf("guest sees an account limit: %v", gs)
	}
	// Only the size and the uploader are recorded; a guest's row carries no owner.
	var owners, anon int
	d.QueryRow(`SELECT COUNT(DISTINCT owner_id) FROM files WHERE owner_id != 0`).Scan(&owners)
	d.QueryRow(`SELECT COUNT(*) FROM files WHERE owner_id = 0`).Scan(&anon)
	if owners != 2 || anon != 1 {
		t.Fatalf("owners %d, guest rows %d", owners, anon)
	}
	// Expired files stop counting, so the room comes back by itself.
	d.Exec(`UPDATE files SET expires = 1`)
	if code, m := ada.upload(600 << 10); code != 201 {
		t.Fatalf("after expiry: %d %v", code, m)
	}
}

// A full account is told so before it tries, and the editor's upload buttons go dark.
func TestAccountQuotaStatusWhenFull(t *testing.T) {
	srv, _ := quotaApp(t, Config{FileMB: 1, FilesAccountMB: 1})
	ada := signedIn(t, srv, "ada_lovelace")
	ada.upload(700 << 10)
	ada.upload(300 << 10) // 1000 KB held of 1024 KB: still room
	if _, st, _ := ada.do("GET", "/api/files/status", nil, nil); st["available"] != true {
		t.Fatalf("not full yet: %v", st)
	}
	ada.upload(24 << 10)
	if _, st, _ := ada.do("GET", "/api/files/status", nil, nil); st["available"] != false || st["code"] != "err.files_account_quota" {
		t.Fatalf("full: %v", st)
	}
}

func TestAccountQuotaOffByZero(t *testing.T) {
	srv, _ := quotaApp(t, Config{FileMB: 1, FilesAccountMB: 0})
	ada := signedIn(t, srv, "ada_lovelace")
	for i := 0; i < 4; i++ {
		if code, m := ada.upload(600 << 10); code != 201 {
			t.Fatalf("upload %d: %d %v", i, code, m)
		}
	}
	if _, st, _ := ada.do("GET", "/api/files/status", nil, nil); st["account_limit_bytes"] != nil {
		t.Fatalf("no limit should show none: %v", st)
	}
}

// The lifetime is the configured one, 24 hours when nothing (or nonsense) is set.
func TestFileTTL(t *testing.T) {
	for name, c := range map[string]struct {
		cfg  Config
		want time.Duration
	}{
		"default":   {Config{FileMB: 1}, 24 * time.Hour},
		"six hours": {Config{FileMB: 1, FileTTL: 6 * time.Hour}, 6 * time.Hour},
		"a week":    {Config{FileMB: 1, FileTTL: 168 * time.Hour}, 168 * time.Hour},
		"too short": {Config{FileMB: 1, FileTTL: time.Minute}, 24 * time.Hour},
		"too long":  {Config{FileMB: 1, FileTTL: 9999 * time.Hour}, 24 * time.Hour},
		"negative":  {Config{FileMB: 1, FileTTL: -time.Hour}, 24 * time.Hour},
	} {
		srv, d := quotaApp(t, c.cfg)
		before := time.Now().Unix()
		code, m := newClient(t, srv).upload(2000)
		if code != 201 {
			t.Fatalf("%s: %d %v", name, code, m)
		}
		exp := int64(m["expires"].(float64))
		if got := time.Duration(exp-before) * time.Second; got < c.want-2*time.Second || got > c.want+5*time.Second {
			t.Errorf("%s: kept for %v, want %v", name, got, c.want)
		}
		var stored int64
		d.QueryRow(`SELECT expires FROM files`).Scan(&stored)
		if stored != exp {
			t.Errorf("%s: stored %d, told %d", name, stored, exp)
		}
	}
}

// A negative value is nonsense, not "no limit": New uses the default, as for the lifetime.
func TestAccountQuotaNegativeUsesDefault(t *testing.T) {
	d, _ := db.Open(":memory:")
	defer d.Close()
	for in, want := range map[int]int{-1: DefaultFilesAccountMB, -999: DefaultFilesAccountMB, 0: 0, 7: 7} {
		a, err := New(Config{FilesAccountMB: in}, d)
		if err != nil || a.cfg.FilesAccountMB != want {
			t.Errorf("FilesAccountMB %d became %d (err %v), want %d", in, a.cfg.FilesAccountMB, err, want)
		}
	}
}
