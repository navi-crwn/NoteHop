package app

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

func TestScheduledLock(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(Config{Registration: true}, d)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() { srv.Close(); d.Close() })
	c := newClient(t, srv)
	n := c.create(map[string]any{"mode": "edit", "encrypted": true, "content": fakeCipher("- [ ] a")})
	adm := map[string]string{"X-Admin": n.admin, "X-Write": n.write}
	vis := map[string]string{"X-Write": n.write}
	cfg := "/api/n/" + n.token + "/config"

	if code, _, _ := c.do("POST", cfg, map[string]any{"lock_at": time.Now().Unix() + 3600}, vis); code != 403 {
		t.Fatalf("non-admin config: %d", code)
	}
	if code, _, _ := c.do("POST", cfg, map[string]any{"lock_at": time.Now().Unix() - 10}, adm); code != 400 {
		t.Fatalf("lock in the past: %d", code)
	}
	future := time.Now().Unix() + 3600
	if code, m, _ := c.do("POST", cfg, map[string]any{"lock_at": future}, adm); code != 200 || m["lock_at"].(float64) != float64(future) || m["locked"] != nil {
		t.Fatalf("set lock: %d %v", code, m)
	}
	// Before the time, visitors still write.
	if code, _, raw := c.do("POST", "/api/n/"+n.token+"/checks", map[string]any{"index": 0, "checked": true}, vis); code != 200 {
		t.Fatalf("check before lock: %d %s", code, raw)
	}

	// Jump past the lock time.
	note, _, _ := d.NoteByLink(n.token)
	if err := d.SetConfig(note.ID, fmt.Sprintf(`{"lock_at":%d}`, time.Now().Unix()-1)); err != nil {
		t.Fatal(err)
	}
	if _, m, _ := c.do("GET", "/api/n/"+n.token, nil, nil); m["locked"] != true {
		t.Fatalf("meta after lock: %v", m)
	}
	writes := []struct {
		method, path string
		body         any
	}{
		{"PUT", "/api/n/" + n.token, map[string]any{"content": fakeCipher("x"), "version": 1}},
		{"POST", "/api/n/" + n.token + "/checks", map[string]any{"index": 0, "checked": false}},
		{"POST", "/api/n/" + n.token + "/entries", map[string]any{"content": fakeCipher("baris")}},
	}
	for _, wr := range writes {
		code, m, _ := c.do(wr.method, wr.path, wr.body, vis)
		if code != 403 || !strings.Contains(fmt.Sprint(m["error"]), "dikunci") {
			t.Errorf("%s %s after lock: %d %v", wr.method, wr.path, code, m)
		}
	}
	// Reading still works, and the admin can still edit and lift the lock.
	if code, _, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, nil); code != 200 {
		t.Fatalf("read after lock: %d", code)
	}
	if code, _, raw := c.do("PUT", "/api/n/"+n.token, map[string]any{"content": fakeCipher("admin"), "version": 1}, adm); code != 200 {
		t.Fatalf("admin save after lock: %d %s", code, raw)
	}
	if code, m, _ := c.do("POST", cfg, map[string]any{"lock_at": 0}, adm); code != 200 || m["lock_at"] != nil {
		t.Fatalf("clear lock: %d %v", code, m)
	}
	if code, _, _ := c.do("POST", "/api/n/"+n.token+"/checks", map[string]any{"index": 0, "checked": false}, vis); code != 200 {
		t.Fatalf("check after unlock: %d", code)
	}
}

func TestParseConfig(t *testing.T) {
	if c := parseConfig("{}"); c.LockAt != 0 {
		t.Fatal(c)
	}
	if c := parseConfig("not json"); c.LockAt != 0 {
		t.Fatal(c)
	}
	if s := (NoteConfig{}).String(); s != "{}" {
		t.Fatalf("empty config = %s", s)
	}
}
