package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

func newAppDB(t *testing.T) (*App, *db.DB, *client) {
	t.Helper()
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
	return a, d, newClient(t, srv)
}

func TestHistory(t *testing.T) {
	_, _, c := newAppDB(t)
	n := c.create(map[string]any{"mode": "edit"})
	w := map[string]string{"X-Write": n.write}
	c.do("PUT", "/api/n/"+n.token, map[string]any{"content": fakeCipher("v2"), "version": 1}, w)
	c.do("PUT", "/api/n/"+n.token, map[string]any{"content": fakeCipher("v3"), "version": 2}, w)
	code, m, _ := c.do("GET", "/api/n/"+n.token+"/versions", nil, w)
	vs := m["versions"].([]any)
	// The second save came right after the first: one draft, not two.
	if code != 200 || len(vs) != 1 || vs[0].(map[string]any)["version"].(float64) != 1 || m["current"].(float64) != 3 {
		t.Fatalf("versions: %d %v", code, m)
	}
	if code, m, _ := c.do("GET", "/api/n/"+n.token+"/versions/1", nil, w); code != 200 || m["content"] != fakeCipher("hello") {
		t.Fatalf("version 1: %d %v", code, m)
	}
	if code, _, _ := c.do("GET", "/api/n/"+n.token+"/versions", nil, nil); code != 403 {
		t.Fatalf("history without key: %d", code)
	}
}

func TestSuggestions(t *testing.T) {
	_, _, c := newAppDB(t)
	n := c.create(map[string]any{"mode": "suggest"})
	vis, adm := map[string]string{"X-Write": n.write}, map[string]string{"X-Admin": n.admin}
	if code, _, _ := c.do("PUT", "/api/n/"+n.token, map[string]any{"content": fakeCipher("x"), "version": 1}, vis); code != 403 {
		t.Fatalf("suggest link must not edit: %d", code)
	}
	if code, _, raw := c.do("POST", "/api/n/"+n.token+"/suggestions", map[string]any{"content": fakeCipher("better"), "base_version": 1}, vis); code != 201 {
		t.Fatalf("suggest: %d %s", code, raw)
	}
	if _, m, _ := c.do("GET", "/api/n/"+n.token, nil, adm); m["suggestions"].(float64) != 1 {
		t.Fatalf("admin count: %v", m)
	}
	_, m, _ := c.do("GET", "/api/n/"+n.token+"/suggestions", nil, adm)
	s := m["suggestions"].([]any)[0].(map[string]any)
	if s["content"] != fakeCipher("better") {
		t.Fatalf("suggestion: %v", s)
	}
	if code, _, _ := c.do("GET", "/api/n/"+n.token+"/suggestions", nil, vis); code != 403 {
		t.Fatalf("reader listing suggestions: %d", code)
	}
	if code, _, _ := c.do("DELETE", fmt.Sprintf("/api/n/%s/suggestions/%v", n.token, s["id"]), nil, adm); code != 200 {
		t.Fatalf("reject: %d", code)
	}
}

func TestSignOff(t *testing.T) {
	_, _, c := newAppDB(t)
	n := c.create(map[string]any{"sign_off": true})
	vis := map[string]string{"X-Write": n.write}
	if code, _, _ := c.do("POST", "/api/n/"+n.token+"/signatures", map[string]any{"name": fakeCipher("Budi")}, nil); code != 403 {
		t.Fatalf("sign without key: %d", code)
	}
	code, s1, raw := c.do("POST", "/api/n/"+n.token+"/signatures", map[string]any{"name": fakeCipher("Budi")}, vis)
	if code != 201 || s1["snapshot"] == "" || s1["version"].(float64) != 1 {
		t.Fatalf("sign: %d %s", code, raw)
	}
	_, s2, _ := c.do("POST", "/api/n/"+n.token+"/signatures", map[string]any{"name": fakeCipher("Ani")}, vis)
	if s2["snapshot"] != s1["snapshot"] {
		t.Fatal("same version should share one snapshot")
	}
	if _, m, _ := c.do("GET", "/api/n/"+s1["snapshot"].(string), nil, nil); m["frozen"] != true {
		t.Fatalf("snapshot: %v", m)
	}
	_, m, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, nil)
	if len(m["signatures"].([]any)) != 2 {
		t.Fatalf("signatures in open: %v", m["signatures"])
	}
	off := c.create(nil)
	if code, _, _ := c.do("POST", "/api/n/"+off.token+"/signatures", map[string]any{"name": fakeCipher("x")}, map[string]string{"X-Write": off.write}); code != 403 {
		t.Fatalf("sign-off disabled: %d", code)
	}
}

func TestVotes(t *testing.T) {
	_, _, c := newAppDB(t)
	n := c.create(nil)
	vis := map[string]string{"X-Write": n.write}
	c.do("POST", "/api/n/"+n.token+"/votes", map[string]any{"poll": 0, "option": 1}, vis)
	c.do("POST", "/api/n/"+n.token+"/votes", map[string]any{"poll": 0, "option": 1}, vis)
	_, m, _ := c.do("POST", "/api/n/"+n.token+"/votes", map[string]any{"poll": 0, "option": 0, "prev": 1}, vis)
	v := m["votes"].(map[string]any)["0"].(map[string]any)
	if v["0"].(float64) != 1 || v["1"].(float64) != 1 {
		t.Fatalf("votes: %v", v)
	}
	for _, bad := range []map[string]any{{"poll": -1, "option": 0}, {"poll": 0, "option": 99}, {"poll": 0, "option": -1}} {
		if code, _, _ := c.do("POST", "/api/n/"+n.token+"/votes", bad, vis); code != 400 {
			t.Errorf("bad vote %v: %d", bad, code)
		}
	}
	if code, _, _ := c.do("POST", "/api/n/"+n.token+"/votes", map[string]any{"poll": 0, "option": 0}, nil); code != 403 {
		t.Fatalf("vote without key: %d", code)
	}
}

func TestDecoy(t *testing.T) {
	_, _, c := newAppDB(t)
	decoyWrite := key()
	n := c.create(map[string]any{"password": true, "salt": key(), "mode": "edit",
		"decoy": map[string]any{"content": fakeCipher("harmless"), "write": decoyWrite}})
	// Real password: real note.
	if _, m, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, map[string]string{"X-Write": n.write}); m["content"] != fakeCipher("hello") || m["mode"] != "edit" {
		t.Fatalf("real open: %v", m)
	}
	// Decoy password: decoy text, read-only, and the admin finds out.
	_, m, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, map[string]string{"X-Write": decoyWrite})
	if m["content"] != fakeCipher("harmless") || m["mode"] != "read" {
		t.Fatalf("decoy open: %v", m)
	}
	if code, _, _ := c.do("PUT", "/api/n/"+n.token, map[string]any{"content": fakeCipher("x"), "version": 1}, map[string]string{"X-Write": decoyWrite}); code != 403 {
		t.Fatalf("decoy can't save: %d", code)
	}
	if _, m, _ := c.do("GET", "/api/n/"+n.token, nil, map[string]string{"X-Admin": n.admin}); m["decoy"].(map[string]any)["opened"].(float64) != 1 {
		t.Fatalf("admin sees decoy opened: %v", m["decoy"])
	}
	if code, _, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, map[string]string{"X-Write": key()}); code != 403 {
		t.Fatalf("wrong password: %d", code)
	}
	bad := map[string]any{"encrypted": true, "content": fakeCipher("x"), "mode": "read", "admin": key(), "write": key(),
		"decoy": map[string]any{"content": fakeCipher("d"), "write": key()}}
	if code, _, _ := c.do("POST", "/api/notes", bad, nil); code != 400 {
		t.Fatalf("decoy without password: %d", code)
	}
}

func TestBurnBlocks(t *testing.T) {
	_, _, c := newAppDB(t)
	bk := key()
	n := c.create(map[string]any{"burns": []map[string]any{{"id": "blk001", "key": bk, "max_views": 2}}})
	vis, adm := map[string]string{"X-Write": n.write}, map[string]string{"X-Admin": n.admin, "X-Write": n.write}
	if _, m, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, nil); m["burn_blocks"].(map[string]any)["blk001"] == nil {
		t.Fatalf("open lists burn blocks: %v", m["burn_blocks"])
	}
	if code, _, _ := c.do("POST", "/api/n/"+n.token+"/burns/blk001/reveal", nil, nil); code != 403 {
		t.Fatalf("reveal without key: %d", code)
	}
	if _, m, _ := c.do("POST", "/api/n/"+n.token+"/burns/blk001/reveal", nil, adm); m["key"] != bk || m["left"].(float64) != 2 {
		t.Fatalf("admin reveal must not count: %v", m)
	}
	c.do("POST", "/api/n/"+n.token+"/burns/blk001/reveal", nil, vis)
	if _, m, _ := c.do("POST", "/api/n/"+n.token+"/burns/blk001/reveal", nil, vis); m["left"].(float64) != 0 {
		t.Fatalf("second view: %v", m)
	}
	if code, _, _ := c.do("POST", "/api/n/"+n.token+"/burns/blk001/reveal", nil, vis); code != 404 {
		t.Fatalf("burned block: %d", code)
	}
	pub := map[string]any{"encrypted": false, "content": `{"t":"","b":"x"}`, "mode": "read", "admin": key(), "write": key(),
		"burns": []map[string]any{{"id": "blk002", "key": key(), "max_views": 1}}}
	if code, _, _ := c.do("POST", "/api/notes", pub, nil); code != 400 {
		t.Fatalf("burn block on a public note: %d", code)
	}
}

func TestClaimAndMargin(t *testing.T) {
	_, _, c := newAppDB(t)
	n := c.create(nil) // made before signing in
	c.do("POST", "/api/register", map[string]string{"username": "claimer", "auth": key()}, nil)
	blob := fakeCipher("vault-sealed")
	if code, _, _ := c.do("POST", "/api/n/"+n.token+"/claim", map[string]any{"owner_blob": blob}, nil); code != 403 {
		t.Fatalf("claim without admin: %d", code)
	}
	if code, _, raw := c.do("POST", "/api/n/"+n.token+"/claim", map[string]any{"owner_blob": blob}, map[string]string{"X-Admin": n.admin}); code != 200 {
		t.Fatalf("claim: %d %s", code, raw)
	}
	_, m, _ := c.do("GET", "/api/me/notes", nil, nil)
	if len(m["notes"].([]any)) != 1 {
		t.Fatalf("claimed note in list: %v", m)
	}
	if code, _, _ := c.do("POST", "/api/n/"+n.token+"/claim", map[string]any{"owner_blob": blob}, map[string]string{"X-Admin": n.admin}); code != 409 {
		t.Fatalf("second claim: %d", code)
	}
	ref := fmt.Sprintf("%064x", 7)
	c.do("PUT", "/api/margins/"+ref, map[string]any{"content": "c2VjcmV0"}, nil)
	if _, m, _ := c.do("GET", "/api/margins/"+ref, nil, nil); m["content"] != "c2VjcmV0" {
		t.Fatalf("margin: %v", m)
	}
	other := newClient(t, c.srv)
	if code, _, _ := other.do("GET", "/api/margins/"+ref, nil, nil); code != 401 {
		t.Fatalf("margin without account: %d", code)
	}
}

func TestIPAllowResetDMS(t *testing.T) {
	a, d, c := newAppDB(t)
	n := c.create(map[string]any{"mode": "check", "content": fakeCipher("- [ ] a")})
	adm, vis := map[string]string{"X-Admin": n.admin}, map[string]string{"X-Write": n.write}
	cfg := "/api/n/" + n.token + "/config"
	if code, _, _ := c.do("POST", cfg, map[string]any{"ip_allow": []string{"not-an-ip"}}, adm); code != 400 {
		t.Fatalf("bad ip list: %d", code)
	}
	c.do("POST", cfg, map[string]any{"ip_allow": []string{"10.0.0.0/8"}}, adm)
	if code, m, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, vis); code != 403 || m["code"] != "err.ip_blocked" {
		t.Fatalf("blocked ip: %d %v", code, m)
	}
	if code, _, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, adm); code != 200 {
		t.Fatalf("admin is never blocked: %d", code)
	}
	c.do("POST", cfg, map[string]any{"ip_allow": []string{"127.0.0.1"}}, adm)
	if code, _, _ := c.do("POST", "/api/n/"+n.token+"/checks", map[string]any{"index": 0, "checked": true}, vis); code != 200 {
		t.Fatalf("allowed ip: %d", code)
	}

	// Recurring reset: due → ticks cleared, next date scheduled.
	if code, m, _ := c.do("POST", cfg, map[string]any{"reset": "daily"}, adm); code != 200 || m["reset"] != "daily" {
		t.Fatalf("set reset: %d %v", code, m)
	}
	note, _, _ := d.NoteByLink(n.token)
	cf := parseConfig(note.Config)
	cf.ResetAt = time.Now().Unix() - 1
	d.SetConfig(note.ID, cf.String())
	a.runJobs(context.Background(), time.Now())
	if _, m, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, vis); fmt.Sprint(m["checks"]) != "map[]" || m["reset_at"].(float64) <= float64(time.Now().Unix()) {
		t.Fatalf("after reset: checks=%v reset_at=%v", m["checks"], m["reset_at"])
	}

	// Dead man's switch: closed for readers while the admin keeps visiting.
	if code, m, _ := c.do("POST", cfg, map[string]any{"dms_days": 7}, adm); code != 200 || m["dms_days"].(float64) != 7 {
		t.Fatalf("dms: %d %v", code, m)
	}
	if code, _, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, vis); code != http.StatusTooEarly {
		t.Fatalf("dms hold: %d", code)
	}
	if code, _, _ := c.do("POST", cfg, map[string]any{"open_at": time.Now().Unix() + 3600}, adm); code != 400 {
		t.Fatalf("manual capsule with dms on: %d", code)
	}
	note, _, _ = d.NoteByLink(n.token)
	cf = parseConfig(note.Config)
	cf.OpenAt = time.Now().Unix() - 1 // the owner stopped checking in
	d.SetConfig(note.ID, cf.String())
	if code, _, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, vis); code != 200 {
		t.Fatalf("dms released: %d", code)
	}
}
