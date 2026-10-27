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
		if code != 403 || m["code"] != "err.locked" || m["vars"] == nil {
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

func TestTimeCapsule(t *testing.T) {
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
	n := c.create(map[string]any{"max_views": 1})
	adm := map[string]string{"X-Admin": n.admin, "X-Write": n.write}
	vis := map[string]string{"X-Write": n.write}
	openAt := time.Now().Unix() + 3600
	if code, m, _ := c.do("POST", "/api/n/"+n.token+"/config", map[string]any{"open_at": openAt}, adm); code != 200 || m["open_at"].(float64) != float64(openAt) {
		t.Fatalf("set open_at: %d %v", code, m)
	}
	// Too early: refused without spending the single view, writes refused too.
	for i := 0; i < 2; i++ {
		if code, m, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, vis); code != 425 || m["code"] != "err.capsule_closed" || !strings.Contains(fmt.Sprint(m["error"]), "can only be opened") {
			t.Fatalf("early open: %d %v", code, m)
		}
	}
	e := c.create(map[string]any{"mode": "edit", "open_at": openAt})
	if code, _, _ := c.do("PUT", "/api/n/"+e.token, map[string]any{"content": fakeCipher("x"), "version": 1}, map[string]string{"X-Write": e.write}); code != 425 {
		t.Fatalf("early write: %d", code)
	}
	if _, m, _ := c.do("GET", "/api/n/"+n.token, nil, nil); m["views"].(float64) != 0 {
		t.Fatalf("views spent: %v", m)
	}
	// The admin can look before the time.
	if code, _, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, adm); code != 200 {
		t.Fatalf("admin early open: %d", code)
	}
	// Once the time has passed it opens (and here burns) normally.
	note, _, _ := d.NoteByLink(n.token)
	d.SetConfig(note.ID, fmt.Sprintf(`{"open_at":%d}`, time.Now().Unix()-1))
	if _, m, _ := c.do("GET", "/api/n/"+n.token, nil, nil); m["open_at"] != nil {
		t.Fatalf("open_at still shown after opening: %v", m)
	}
	if code, m, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, vis); code != 200 || m["burned"] != true {
		t.Fatalf("open after time: %d %v", code, m)
	}
}

func TestFeatureConflicts(t *testing.T) {
	c := newClient(t, newTestApp(t))
	now := time.Now().Unix()
	bad := []map[string]any{
		{"mode": "edit", "max_views": 1},                               // burn needs read
		{"mode": "read", "lock_at": now + 3600},                        // lock needs a writable link
		{"mode": "edit", "open_at": now + 7200, "lock_at": now + 3600}, // lock before opening
		{"open_at": now + 7200, "expires_in": 3600},                    // expires before opening
		{"mode": "check", "lock_at": now + 7200, "expires_in": 3600},   // expires before lock
		{"open_at": now - 10},                                          // capsule in the past
		{"mode": "edit", "lock_at": now + 400*24*3600},                 // too far ahead
	}
	for i, extra := range bad {
		body := map[string]any{"encrypted": true, "content": fakeCipher("x"), "mode": "read", "admin": key(), "write": key()}
		for k, v := range extra {
			body[k] = v
		}
		if code, _, raw := c.do("POST", "/api/notes", body, nil); code != 400 {
			t.Errorf("case %d: want 400, got %d %s", i, code, raw)
		}
	}
	ok := c.create(map[string]any{"mode": "edit", "open_at": now + 3600, "lock_at": now + 7200, "expires_in": 86400})
	if _, m, _ := c.do("GET", "/api/n/"+ok.token, nil, nil); m["open_at"].(float64) != float64(now+3600) || m["lock_at"].(float64) != float64(now+7200) {
		t.Fatalf("config at create: %v", m)
	}
	// A burn note can't get an edit link later either.
	b := c.create(map[string]any{"max_views": 1})
	if code, _, _ := c.do("POST", "/api/n/"+b.token+"/links", map[string]any{"mode": "edit"}, map[string]string{"X-Admin": b.admin}); code != 400 {
		t.Fatalf("edit link on burn note: %d", code)
	}
	// Config changes are checked against what's already set.
	adm := map[string]string{"X-Admin": ok.admin}
	if code, _, _ := c.do("POST", "/api/n/"+ok.token+"/config", map[string]any{"open_at": now + 9000}, adm); code != 400 {
		t.Fatalf("capsule after lock via config: %d", code)
	}
}
