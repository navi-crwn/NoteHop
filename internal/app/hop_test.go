package app

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
	"github.com/navi-crwn/NoteHop/internal/hop"
)

// hopApp is a server with single sign-on (and so the control center API) on.
func hopApp(t *testing.T, cfg Config) (*httptest.Server, *App, *db.DB) {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL, cfg.SSOURL, cfg.SSOSecret, cfg.SSOClient = "https://note.test", "https://accounts.test", testSecret, "notehop"
	}
	if cfg.HopClients == nil {
		cfg.HopClients = []string{"imperium", "helink"}
	}
	cfg.Registration = true
	a, err := New(cfg, d)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() { srv.Close(); d.Close() })
	return srv, a, d
}

// call makes one signed request as app and returns the status and envelope.
func call(t *testing.T, srv *httptest.Server, app, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var buf []byte
	if body != nil {
		buf, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, srv.URL+path, bytes.NewReader(buf))
	req.Header = hop.Headers(app, testSecret, "7:owner", method, path, buf)
	req.Header.Set("Origin", "https://evil.example") // no CSRF here: the signature is the auth
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var env map[string]any
	if err := json.Unmarshal(raw, &env); err != nil {
		env = map[string]any{"raw": string(raw)}
	}
	return res.StatusCode, env
}

func data(env map[string]any) map[string]any {
	d, _ := env["data"].(map[string]any)
	return d
}

func list(env map[string]any) []any {
	d, _ := env["data"].([]any)
	return d
}

// ssoUser signs a user in through the account server and returns their cookie client.
func ssoUser(t *testing.T, srv *httptest.Server, sub, email string, roles []string) *client {
	t.Helper()
	c := noRedirect(t, srv)
	state := ssoStart(t, c, "/")
	n := time.Now().Unix()
	tk := signTicket(ssoTicket{Sub: sub, Email: email, Name: "N", Roles: roles, State: state, Jti: randToken(16), Iat: n, Exp: n + 120, V: 2}, testSecret)
	if code, _ := ssoCallback(t, c, tk); code != http.StatusSeeOther {
		t.Fatalf("sso login %s: %d", sub, code)
	}
	return c
}

func TestHopSignatureGate(t *testing.T) {
	srv, _, _ := hopApp(t, Config{})
	// Unsigned
	res, _ := http.Get(srv.URL + "/hop/health")
	var env map[string]any
	json.NewDecoder(res.Body).Decode(&env)
	res.Body.Close()
	if res.StatusCode != 401 || env["ok"] != false || env["error"] != "bad_signature" {
		t.Fatalf("unsigned: %d %v", res.StatusCode, env)
	}
	// Wrong app
	if code, env := call(t, srv, "pixelhop", "GET", "/hop/health", nil); code != 401 || env["error"] != "bad_signature" {
		t.Fatalf("unknown app: %d %v", code, env)
	}
	// Wrong secret
	req, _ := http.NewRequest("GET", srv.URL+"/hop/health", nil)
	req.Header = hop.Headers("imperium", strings.Repeat("x", 32), "a", "GET", "/hop/health", nil)
	res, _ = http.DefaultClient.Do(req)
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatalf("wrong secret: %d", res.StatusCode)
	}
	// Replay of the same signed request
	req, _ = http.NewRequest("GET", srv.URL+"/hop/health", nil)
	req.Header = hop.Headers("imperium", testSecret, "a", "GET", "/hop/health", nil)
	res, _ = http.DefaultClient.Do(req)
	res.Body.Close()
	req2, _ := http.NewRequest("GET", srv.URL+"/hop/health", nil)
	req2.Header = req.Header.Clone()
	res, _ = http.DefaultClient.Do(req2)
	json.NewDecoder(res.Body).Decode(&env)
	res.Body.Close()
	if res.StatusCode != 401 || env["error"] != "replay" {
		t.Fatalf("replay: %d %v", res.StatusCode, env)
	}
	// Stale
	req, _ = http.NewRequest("GET", srv.URL+"/hop/health", nil)
	req.Header = hop.Headers("imperium", testSecret, "a", "GET", "/hop/health", nil)
	req.Header.Set("X-HOP-Timestamp", "1000")
	res, _ = http.DefaultClient.Do(req)
	json.NewDecoder(res.Body).Decode(&env)
	res.Body.Close()
	if res.StatusCode != 401 || env["error"] != "stale_request" {
		t.Fatalf("stale: %d %v", res.StatusCode, env)
	}
	// The path with its query is signed: a changed query fails.
	req, _ = http.NewRequest("GET", srv.URL+"/hop/users?q=b", nil)
	req.Header = hop.Headers("imperium", testSecret, "a", "GET", "/hop/users?q=a", nil)
	res, _ = http.DefaultClient.Do(req)
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatalf("query tampered: %d", res.StatusCode)
	}
	// Good: both apps work, unknown routes answer in the envelope.
	if code, env := call(t, srv, "helink", "GET", "/hop/health", nil); code != 200 || env["ok"] != true {
		t.Fatalf("helink: %d %v", code, env)
	}
	if code, env := call(t, srv, "imperium", "GET", "/hop/nope", nil); code != 404 || env["error"] != "not_found" {
		t.Fatalf("unknown route: %d %v", code, env)
	}
	// SSO off: not configured, even with a signature.
	off := newTestApp(t)
	if code, env := call(t, off, "imperium", "GET", "/hop/health", nil); code != 404 || env["error"] != "not_configured" {
		t.Fatalf("sso off: %d %v", code, env)
	}
}

func TestHopHealthManifestStats(t *testing.T) {
	srv, _, _ := hopApp(t, Config{Version: "v9", FilesTotalMB: 1})
	code, env := call(t, srv, "imperium", "GET", "/hop/health", nil)
	d := data(env)
	if code != 200 || d["app"] != "notehop" || d["version"] != "v9" || d["db"] != "ok" || d["sso"] != "ok" || len(d["checks"].([]any)) < 2 {
		t.Fatalf("health: %d %v", code, env)
	}
	_, env = call(t, srv, "imperium", "GET", "/hop/manifest", nil)
	d = data(env)
	if d["app"] != "notehop" || d["icon"] != "note" || d["base_url"] != "https://note.test" {
		t.Fatalf("manifest: %v", d)
	}
	caps, _ := json.Marshal(d["capabilities"])
	if !strings.Contains(string(caps), "zero_knowledge") || len(d["content_types"].([]any)) != 2 || len(d["extensions"].([]any)) != 4 {
		t.Fatalf("manifest capabilities: %v", d)
	}
	c := newClient(t, srv)
	c.create(nil)
	pub, _ := json.Marshal(map[string]string{"t": "Hello", "b": "world"})
	c.create(map[string]any{"encrypted": false, "content": string(pub)})
	c.create(map[string]any{"max_views": 2})
	_, env = call(t, srv, "imperium", "GET", "/hop/stats", nil)
	k := data(env)["kpi"].(map[string]any)
	if k["notes"].(float64) != 3 || k["notes_encrypted"].(float64) != 2 || k["notes_public"].(float64) != 1 || k["notes_burn"].(float64) != 1 || k["files_budget_bytes"].(float64) != 1<<20 {
		t.Fatalf("kpi: %v", k)
	}
	series := data(env)["series"].(map[string]any)["notes_created"].([]any)
	if len(series) != 14 || series[13].([]any)[1].(float64) != 3 {
		t.Fatalf("series: %v", series)
	}
	req, _ := http.NewRequest("GET", srv.URL+"/hop/metrics", nil)
	req.Header = hop.Headers("imperium", testSecret, "a", "GET", "/hop/metrics", nil)
	res, _ := http.DefaultClient.Do(req)
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(raw), "notehop_notes 3\n") || !strings.Contains(res.Header.Get("Content-Type"), "text/plain") {
		t.Fatalf("metrics: %s", raw)
	}
}

func TestHopUsersAndActions(t *testing.T) {
	srv, _, d := hopApp(t, Config{})
	u := ssoUser(t, srv, "42", "budi@example.com", nil)
	u.create(nil)
	u.create(nil)
	newClient(t, srv).do("POST", "/api/register", map[string]any{"username": "lokal", "auth": key()}, nil)

	code, env := call(t, srv, "imperium", "GET", "/hop/users/42", nil)
	x := data(env)
	if code != 200 || x["sub"] != "42" || x["username"] != "budi" || x["role"] != "user" || x["notes"].(float64) != 2 || x["sessions"].(float64) != 1 || x["vault_set"] != false {
		t.Fatalf("user: %d %v", code, x)
	}
	if code, env := call(t, srv, "imperium", "GET", "/hop/users/999", nil); code != 404 || env["error"] != "not_found" {
		t.Fatalf("missing user: %d %v", code, env)
	}
	_, env = call(t, srv, "imperium", "GET", "/hop/users?q=lok", nil)
	if l := list(env); len(l) != 1 || l[0].(map[string]any)["id"] != "local:2" {
		t.Fatalf("list: %v", env)
	}
	if code, env := call(t, srv, "imperium", "GET", "/hop/users/local:2", nil); code != 200 || data(env)["username"] != "lokal" {
		t.Fatalf("local user: %d %v", code, env)
	}

	// set_role makes a site admin (short link names allowed), and back.
	if code, _ := call(t, srv, "imperium", "POST", "/hop/users/42/actions", map[string]any{"action": "set_role", "params": map[string]any{"role": "admin"}}); code != 200 {
		t.Fatalf("set_role: %d", code)
	}
	if code, m, _ := u.do("POST", "/api/notes", map[string]any{"encrypted": true, "content": fakeCipher("x"), "mode": "read", "admin": key(), "write": key(), "slug": "eid"}, nil); code != 201 {
		t.Fatalf("admin short name: %d %v", code, m)
	}
	if code, env := call(t, srv, "imperium", "POST", "/hop/users/42/actions", map[string]any{"action": "set_role", "params": map[string]any{"role": "god"}}); code != 400 || env["error"] != "bad_role" {
		t.Fatalf("bad role: %d %v", code, env)
	}
	if code, env := call(t, srv, "imperium", "POST", "/hop/users/42/actions", map[string]any{"action": "fly"}); code != 400 || env["error"] != "unknown_action" {
		t.Fatalf("unknown action: %d %v", code, env)
	}
	// revoke_sessions signs the user out.
	call(t, srv, "imperium", "POST", "/hop/users/42/actions", map[string]any{"action": "revoke_sessions", "reason": "test"})
	if code, _, _ := u.do("GET", "/api/me", nil, nil); code != 401 {
		t.Fatalf("after revoke: %d", code)
	}
	// delete_notes
	_, env = call(t, srv, "imperium", "POST", "/hop/users/42/actions", map[string]any{"action": "delete_notes"})
	if data(env)["deleted_notes"].(float64) != 3 {
		t.Fatalf("delete_notes: %v", env)
	}
	// reset_vault clears the phrase check so a new one can be set.
	d.Exec(`UPDATE users SET vault_check = 'x' WHERE sso_id = '42'`)
	call(t, srv, "imperium", "POST", "/hop/users/42/actions", map[string]any{"action": "reset_vault"})
	if _, env := call(t, srv, "imperium", "GET", "/hop/users/42", nil); data(env)["vault_set"] != false {
		t.Fatalf("reset_vault: %v", env)
	}
	// purge removes the account.
	_, env = call(t, srv, "imperium", "POST", "/hop/users/42/actions", map[string]any{"action": "purge"})
	if data(env)["purged"] != true {
		t.Fatalf("purge: %v", env)
	}
	if code, _ := call(t, srv, "imperium", "GET", "/hop/users/42", nil); code != 404 {
		t.Fatalf("after purge: %d", code)
	}
	// Everything was audited.
	_, env = call(t, srv, "imperium", "GET", "/hop/audit", nil)
	audits := list(env)
	if len(audits) != 5 || audits[0].(map[string]any)["action"] != "user.purge" || audits[0].(map[string]any)["actor"] != "7:owner" || audits[0].(map[string]any)["app"] != "imperium" {
		t.Fatalf("audit: %v", audits)
	}
}

func TestHopProfileAndRoles(t *testing.T) {
	srv, _, _ := hopApp(t, Config{})
	// A v2 ticket with roles promotes at sign-in; one without roles leaves the role alone; one with [] demotes.
	ssoUser(t, srv, "1", "owner@example.com", []string{"owner"})
	if _, env := call(t, srv, "imperium", "GET", "/hop/users/1", nil); data(env)["role"] != "admin" {
		t.Fatalf("owner role at sign-in: %v", env)
	}
	ssoUser(t, srv, "1", "owner@example.com", nil)
	if _, env := call(t, srv, "imperium", "GET", "/hop/users/1", nil); data(env)["role"] != "admin" {
		t.Fatalf("v1 ticket must not demote: %v", env)
	}
	ssoUser(t, srv, "1", "owner@example.com", []string{"app:pixelhop"})
	if _, env := call(t, srv, "imperium", "GET", "/hop/users/1", nil); data(env)["role"] != "user" {
		t.Fatalf("ticket without an admin role must demote: %v", env)
	}
	// Profile push: promotion, demotion, name and username rules.
	code, env := call(t, srv, "helink", "POST", "/hop/profile", map[string]any{"sub": "1", "username": "owner_new", "name": "Owner", "roles": []string{"admin:notehop"}})
	x := data(env)
	if code != 200 || x["role"] != "admin" || x["username"] != "owner_new" || x["username_kept"] != false {
		t.Fatalf("profile promote: %d %v", code, env)
	}
	_, env = call(t, srv, "helink", "POST", "/hop/profile", map[string]any{"hop_id": "1", "display_name": "O", "roles": []string{}})
	if data(env)["role"] != "user" {
		t.Fatalf("profile demote: %v", env)
	}
	// A reserved or taken username is kept.
	newClient(t, srv).do("POST", "/api/register", map[string]any{"username": "taken_name", "auth": key()}, nil)
	for _, name := range []string{"admin", "taken_name", "Bad Name"} {
		_, env = call(t, srv, "helink", "POST", "/hop/profile", map[string]any{"sub": "1", "username": name})
		if data(env)["username_kept"] != true || data(env)["username"] != "owner_new" {
			t.Fatalf("username %q should be kept: %v", name, env)
		}
	}
	// No account yet: nothing is created.
	if code, env := call(t, srv, "helink", "POST", "/hop/profile", map[string]any{"sub": "77", "username": "nobody"}); code != 404 || env["error"] != "not_found" {
		t.Fatalf("profile for unknown: %d %v", code, env)
	}
	// Username from the ticket is used for a new account when free.
	n := time.Now().Unix()
	c := noRedirect(t, srv)
	state := ssoStart(t, c, "/")
	ssoCallback(t, c, signTicket(ssoTicket{Sub: "2", Username: "siti_r", Email: "siti@example.com", State: state, Jti: randToken(16), Iat: n, Exp: n + 120, V: 2}, testSecret))
	if _, m, _ := c.do("GET", "/api/me", nil, nil); m["username"] != "siti_r" {
		t.Fatalf("ticket username: %v", m)
	}
}

func TestHopEvents(t *testing.T) {
	srv, _, d := hopApp(t, Config{})
	u := ssoUser(t, srv, "5", "ani@example.com", nil)
	ev := func(id, typ string, dataV map[string]any) (int, map[string]any) {
		return call(t, srv, "helink", "POST", "/hop/events", map[string]any{"id": id, "type": typ, "at": "2026-10-27T00:00:00Z", "data": dataV})
	}
	// Suspension signs the user out and keeps them out, through both doors.
	code, env := ev("e1", "account.status", map[string]any{"sub": "5", "status": "suspended"})
	if code != 200 || data(env)["status"] != "suspended" {
		t.Fatalf("suspend: %d %v", code, env)
	}
	if code, _, _ := u.do("GET", "/api/me", nil, nil); code != 401 {
		t.Fatalf("suspended session still works: %d", code)
	}
	c := noRedirect(t, srv)
	state := ssoStart(t, c, "/")
	if code, _ := ssoCallback(t, c, ticketFor("5", state)); code != http.StatusForbidden {
		t.Fatalf("suspended sso login: %d", code)
	}
	// Idempotent: the same id again does nothing more.
	if _, env := ev("e1", "account.status", map[string]any{"sub": "5", "status": "active"}); data(env)["duplicate"] != true {
		t.Fatalf("duplicate: %v", env)
	}
	if _, env := call(t, srv, "imperium", "GET", "/hop/users/5", nil); data(env)["status"] != "suspended" {
		t.Fatalf("duplicate event changed state: %v", env)
	}
	ev("e2", "account.status", map[string]any{"sub": "5", "status": "active"})
	if _, env := call(t, srv, "imperium", "GET", "/hop/users/5", nil); data(env)["status"] != "active" {
		t.Fatalf("reactivate: %v", env)
	}
	// A suspended password account can't log in either (the status column
	// is set by hand for local accounts: the account server doesn't know them).
	auth := key()
	pw := newClient(t, srv)
	pw.do("POST", "/api/register", map[string]any{"username": "pw_user", "auth": auth}, nil)
	d.Exec(`UPDATE users SET status = 'suspended' WHERE username = 'pw_user'`)
	if code, m, _ := newClient(t, srv).do("POST", "/api/login", map[string]any{"username": "pw_user", "auth": auth}, nil); code != 403 || m["code"] != "err.account_suspended" {
		t.Fatalf("suspended password login: %d %v", code, m)
	}
	if code, _, _ := pw.do("GET", "/api/me", nil, nil); code != 401 {
		t.Fatalf("suspended password session: %d", code)
	}
	// profile.updated and session.revoked
	u = ssoUser(t, srv, "5", "ani@example.com", nil)
	ev("e3", "profile.updated", map[string]any{"profile": map[string]any{"sub": "5", "name": "Ani Baru", "roles": []string{"owner"}}})
	if _, env := call(t, srv, "imperium", "GET", "/hop/users/5", nil); data(env)["display_name"] != "Ani Baru" || data(env)["role"] != "admin" {
		t.Fatalf("profile.updated: %v", env)
	}
	ev("e4", "session.revoked", map[string]any{"sub": "5"})
	if code, _, _ := u.do("GET", "/api/me", nil, nil); code != 401 {
		t.Fatalf("session.revoked: %d", code)
	}
	// account.deleted purges; an event for nobody is acknowledged.
	u = ssoUser(t, srv, "5", "ani@example.com", nil)
	u.create(nil)
	_, env = ev("e5", "account.deleted", map[string]any{"sub": "5"})
	if data(env)["purged"] != true || data(env)["counts"].(map[string]any)["notes"].(float64) != 1 {
		t.Fatalf("account.deleted: %v", env)
	}
	if code, _ := call(t, srv, "imperium", "GET", "/hop/users/5", nil); code != 404 {
		t.Fatalf("after account.deleted: %d", code)
	}
	if code, env := ev("e6", "account.deleted", map[string]any{"sub": "5"}); code != 200 || data(env)["ignored"] != true {
		t.Fatalf("event for nobody: %d %v", code, env)
	}
	if code, env := ev("", "x", nil); code != 400 || env["error"] != "bad_request" {
		t.Fatalf("event without id: %d %v", code, env)
	}
}

func TestHopContentZeroKnowledge(t *testing.T) {
	srv, _, _ := hopApp(t, Config{FileMB: 10})
	u := ssoUser(t, srv, "9", "dewi@example.com", nil)
	enc := u.create(map[string]any{"content": fakeCipher("secret body"), "slug": "rahasia-besar"})
	pub, _ := json.Marshal(map[string]string{"t": "Rapat Senin", "b": strings.Repeat("agenda ", 50)})
	open := newClient(t, srv).create(map[string]any{"encrypted": false, "content": string(pub), "mode": "edit"})
	newClient(t, srv).create(map[string]any{"max_views": 1})

	_, env := call(t, srv, "imperium", "GET", "/hop/content?type=note", nil)
	rows := list(env)
	if len(rows) != 3 {
		t.Fatalf("rows: %v", env)
	}
	raw, _ := json.Marshal(env)
	if strings.Contains(string(raw), fakeCipher("secret body")) || strings.Contains(string(raw), "secret body") {
		t.Fatal("encrypted content leaked into the list")
	}
	var encRow, pubRow map[string]any
	for _, r := range rows {
		m := r.(map[string]any)
		if m["url"] == "https://note.test/"+enc.token {
			encRow = m
		}
		if m["url"] == "https://note.test/"+open.token {
			pubRow = m
		}
	}
	if encRow == nil || encRow["encrypted"] != true || encRow["status"] != "encrypted" || encRow["title"] != "" || encRow["preview"] != "" {
		t.Fatalf("encrypted row: %v", encRow)
	}
	if o := encRow["owner"].(map[string]any); o["sub"] != "9" || o["username"] != "dewi" {
		t.Fatalf("owner: %v", encRow)
	}
	if pubRow == nil || pubRow["encrypted"] != false || pubRow["title"] != "Rapat Senin" || len([]rune(pubRow["preview"].(string))) != 200 || pubRow["owner"] != nil {
		t.Fatalf("public row: %v", pubRow)
	}
	if links := pubRow["links"].([]any); len(links) != 1 || links[0].(map[string]any)["mode"] != "edit" {
		t.Fatalf("links: %v", pubRow)
	}
	// Filters: status, owner, q by slug prefix and by public title.
	_, env = call(t, srv, "imperium", "GET", "/hop/content?status=burn", nil)
	if l := list(env); len(l) != 1 || l[0].(map[string]any)["status"] != "burn" {
		t.Fatalf("status=burn: %v", env)
	}
	_, env = call(t, srv, "imperium", "GET", "/hop/content?owner=9", nil)
	if l := list(env); len(l) != 1 || l[0].(map[string]any)["id"] != encRow["id"] {
		t.Fatalf("owner=9: %v", env)
	}
	_, env = call(t, srv, "imperium", "GET", "/hop/content?q=rahasia", nil)
	if l := list(env); len(l) != 1 || l[0].(map[string]any)["id"] != encRow["id"] {
		t.Fatalf("q=slug: %v", env)
	}
	_, env = call(t, srv, "imperium", "GET", "/hop/content?q=senin", nil)
	if l := list(env); len(l) != 1 || l[0].(map[string]any)["id"] != pubRow["id"] {
		t.Fatalf("q=title: %v", env)
	}
	_, env = call(t, srv, "imperium", "GET", "/hop/content?q=agenda", nil)
	if l := list(env); len(l) != 0 {
		t.Fatalf("q must not match the body: %v", env)
	}
	// Paging.
	_, env = call(t, srv, "imperium", "GET", "/hop/content?limit=2", nil)
	next := env["page"].(map[string]any)["next"].(string)
	if len(list(env)) != 2 || next == "" {
		t.Fatalf("page 1: %v", env)
	}
	_, env = call(t, srv, "imperium", "GET", "/hop/content?limit=2&cursor="+next, nil)
	if len(list(env)) != 1 {
		t.Fatalf("page 2: %v", env)
	}
	// Detail of the encrypted note: counts, no content.
	u.do("POST", "/api/n/"+enc.token+"/entries", map[string]any{"content": fakeCipher("e")}, map[string]string{"X-Write": enc.write, "X-Admin": enc.admin})
	code, env := call(t, srv, "imperium", "GET", "/hop/content/note/"+encRow["id"].(string), nil)
	x := data(env)
	if code != 200 || x["entries"].(float64) != 1 || x["inbox"] != false || x["watch"] != false || x["encrypted"] != true {
		t.Fatalf("detail: %d %v", code, env)
	}
	if _, ok := x["content"]; ok {
		t.Fatal("detail must not include content")
	}
	if code, env := call(t, srv, "imperium", "GET", "/hop/content/note/999", nil); code != 404 || env["error"] != "not_found" {
		t.Fatalf("missing note: %d %v", code, env)
	}
	// Attachments.
	req, _ := http.NewRequest("POST", srv.URL+"/api/files", bytes.NewReader(make([]byte, 100)))
	req.Header.Set("X-Upload-Draft", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	req.Header.Set("Content-Type", "application/octet-stream")
	res, _ := http.DefaultClient.Do(req)
	var up map[string]any
	json.NewDecoder(res.Body).Decode(&up)
	res.Body.Close()
	_, env = call(t, srv, "imperium", "GET", "/hop/content?type=file", nil)
	if l := list(env); len(l) != 1 || l[0].(map[string]any)["id"] != up["id"] || l[0].(map[string]any)["size"].(float64) != 100 {
		t.Fatalf("files: %v", env)
	}
	if code, env := call(t, srv, "imperium", "GET", "/hop/content/file/"+up["id"].(string), nil); code != 200 || data(env)["size"].(float64) != 100 || data(env)["encrypted"] != true {
		t.Fatalf("file detail: %d %v", code, env)
	}
	if code, env := call(t, srv, "imperium", "POST", "/hop/content/file/"+up["id"].(string)+"/actions", map[string]any{"action": "delete"}); code != 200 || data(env)["deleted"] != true {
		t.Fatalf("file delete: %d %v", code, env)
	}
	if code, _ := call(t, srv, "imperium", "POST", "/hop/content/file/"+up["id"].(string)+"/actions", map[string]any{"action": "delete"}); code != 404 {
		t.Fatalf("file delete twice: %d", code)
	}
}

func TestHopNoteActions(t *testing.T) {
	srv, _, d := hopApp(t, Config{FileMB: 10})
	c := newClient(t, srv)
	act := func(tok, action string, params map[string]any) (int, map[string]any) {
		n, _, err := d.NoteByLink(tok)
		if err != nil {
			t.Fatalf("note %s: %v", tok, err)
		}
		return call(t, srv, "imperium", "POST", "/hop/content/note/"+strconv.FormatInt(n.ID, 10)+"/actions", map[string]any{"action": action, "reason": "test", "params": params})
	}
	// freeze: nobody can change it any more, the owner included.
	n := c.create(map[string]any{"mode": "edit"})
	if code, env := act(n.token, "freeze", nil); code != 200 || data(env)["frozen"] != true {
		t.Fatalf("freeze: %d %v", code, env)
	}
	if code, _, _ := c.do("PUT", "/api/n/"+n.token, map[string]any{"content": fakeCipher("x"), "version": 1}, map[string]string{"X-Write": n.write, "X-Admin": n.admin}); code != 403 {
		t.Fatalf("edit after freeze: %d", code)
	}
	// set_expiry, then clear it.
	exp := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	if code, env := act(n.token, "set_expiry", map[string]any{"expires": exp.Format(time.RFC3339)}); code != 200 || data(env)["expires"] != exp.Format(time.RFC3339) {
		t.Fatalf("set_expiry: %d %v", code, env)
	}
	if _, m, _ := c.do("GET", "/api/n/"+n.token, nil, nil); int64(m["expires"].(float64)) != exp.Unix() {
		t.Fatalf("expiry not applied: %v", m)
	}
	if code, env := act(n.token, "set_expiry", map[string]any{"expires": nil}); code != 200 || data(env)["expires"] != nil {
		t.Fatalf("clear expiry: %d %v", code, env)
	}
	if code, env := act(n.token, "set_expiry", map[string]any{"expires": "yesterday"}); code != 400 || env["error"] != "bad_expires" {
		t.Fatalf("bad expiry: %d %v", code, env)
	}
	// disable_watch and disable_inbox.
	w := c.create(map[string]any{"mode": "edit"})
	adm := map[string]string{"X-Write": w.write, "X-Admin": w.admin}
	c.do("POST", "/api/n/"+w.token+"/config", map[string]any{"watch": "https://hooks.example/x"}, adm)
	// A real P-256 public key: the inbox endpoint checks it.
	pubKey, _ := ecdh.P256().GenerateKey(rand.Reader)
	if code, _, raw := c.do("POST", "/api/n/"+w.token+"/inbox", map[string]any{"pub": base64.RawURLEncoding.EncodeToString(pubKey.PublicKey().Bytes()), "priv": fakeCipher("priv-priv-priv")}, adm); code != 200 {
		t.Fatalf("inbox on: %d %s", code, raw)
	}
	nn, _, _ := d.NoteByLink(w.token)
	if _, env := call(t, srv, "imperium", "GET", "/hop/content/note/"+strconv.FormatInt(nn.ID, 10), nil); data(env)["watch"] != true || data(env)["inbox"] != true {
		t.Fatalf("watch/inbox on: %v", env)
	}
	act(w.token, "disable_watch", nil)
	act(w.token, "disable_inbox", nil)
	if _, env := call(t, srv, "imperium", "GET", "/hop/content/note/"+strconv.FormatInt(nn.ID, 10), nil); data(env)["watch"] != false || data(env)["inbox"] != false {
		t.Fatalf("watch/inbox off: %v", env)
	}
	if _, m, _ := c.do("GET", "/api/n/"+w.token, nil, adm); m["watch"] != nil {
		t.Fatalf("watch still set: %v", m["watch"])
	}
	// delete: the name is held for 90 days for the owner; release_slug frees it.
	s := c.create(map[string]any{"slug": "catatan-rapat"})
	if code, env := act(s.token, "delete", nil); code != 200 || data(env)["deleted"] != true {
		t.Fatalf("delete: %d %v", code, env)
	}
	if _, _, held := d.SlugHold("catatan-rapat"); !held {
		t.Fatal("name not held after delete")
	}
	s2 := c.create(map[string]any{"slug": "catatan-lain"})
	if code, env := act(s2.token, "release_slug", map[string]any{"slug": "catatan-rapat"}); code != 200 || data(env)["released"] != true {
		t.Fatalf("release_slug: %d %v", code, env)
	}
	if _, _, held := d.SlugHold("catatan-rapat"); held {
		t.Fatal("name still held after release")
	}
	// takedown: gone, and the name is held a year for everyone.
	tk := c.create(map[string]any{"slug": "konten-buruk"})
	if code, env := act(tk.token, "takedown", nil); code != 200 || data(env)["hold_days"].(float64) != 365 {
		t.Fatalf("takedown: %d %v", code, env)
	}
	if code, _, _ := c.do("GET", "/api/n/konten-buruk", nil, nil); code != 404 {
		t.Fatalf("after takedown: %d", code)
	}
	_, until, held := d.SlugHold("konten-buruk")
	if !held || until < time.Now().Unix()+364*86400 {
		t.Fatalf("takedown hold: %v %d", held, until)
	}
	if code, _, _ := c.do("POST", "/api/notes", map[string]any{"encrypted": true, "content": fakeCipher("x"), "mode": "read", "admin": key(), "write": key(), "slug": "konten-buruk"}, nil); code != 400 {
		t.Fatalf("held name reusable: %d", code)
	}
	// The names extension lists it as a takedown hold and can release it.
	_, env := call(t, srv, "imperium", "GET", "/hop/x/names", nil)
	x := data(env)
	if th := x["takedown_holds"].([]any); len(th) != 1 || th[0].(map[string]any)["slug"] != "konten-buruk" || x["blocked_count"].(float64) == 0 || len(x["reserved"].([]any)) == 0 {
		t.Fatalf("names: %v", x)
	}
	if code, _ := call(t, srv, "imperium", "POST", "/hop/x/names/holds/konten-buruk/actions", map[string]any{"action": "release"}); code != 200 {
		t.Fatalf("hold release: %d", code)
	}
	if code, _ := call(t, srv, "imperium", "POST", "/hop/x/names/holds/konten-buruk/actions", map[string]any{"action": "release"}); code != 404 {
		t.Fatalf("hold release twice: %d", code)
	}
	if code, _, _ := c.do("POST", "/api/notes", map[string]any{"encrypted": true, "content": fakeCipher("x"), "mode": "read", "admin": key(), "write": key(), "slug": "konten-buruk"}, nil); code != 201 {
		t.Fatalf("released name: %d", code)
	}
	// The other extensions answer.
	_, env = call(t, srv, "imperium", "GET", "/hop/x/files", nil)
	if x := data(env); x["on"] != true || x["count"].(float64) != 0 {
		t.Fatalf("files: %v", x)
	}
	_, env = call(t, srv, "imperium", "GET", "/hop/x/realtime", nil)
	if x := data(env); len(x["rooms"].([]any)) != 0 || x["nearby_rooms"].(float64) != 0 {
		t.Fatalf("realtime: %v", x)
	}
	_, env = call(t, srv, "imperium", "GET", "/hop/x/integrations", nil)
	if x := data(env); x["telegram"].(map[string]any)["on"] != false || x["ssh"].(map[string]any)["on"] != false || x["webhooks"].(map[string]any)["pending"].(float64) != 0 {
		t.Fatalf("integrations: %v", x)
	}
	// Audit has the actions, with the note as target.
	_, env = call(t, srv, "imperium", "GET", "/hop/audit?limit=3", nil)
	if l := list(env); len(l) != 3 || l[0].(map[string]any)["action"] != "hold.release" || env["page"].(map[string]any)["next"] == "" {
		t.Fatalf("audit: %v", env)
	}
}

func TestHopReportFlow(t *testing.T) {
	srv, _, d := hopApp(t, Config{})
	c := newClient(t, srv)
	n := c.create(map[string]any{"slug": "laporkan-aku"})
	// A reader reports the note from the page's menu.
	code, m, _ := c.do("POST", "/api/n/"+n.token+"/report", map[string]any{"reason": "spam", "note": "it sells pills"}, nil)
	if code != 201 || m["id"].(float64) != 1 {
		t.Fatalf("report: %d %v", code, m)
	}
	if code, m, _ := c.do("POST", "/api/n/"+n.token+"/report", map[string]any{"reason": "nope"}, nil); code != 400 || m["code"] != "err.bad_report" {
		t.Fatalf("bad reason: %d %v", code, m)
	}
	if code, m, _ := c.do("POST", "/api/n/"+n.token+"/report", map[string]any{"reason": "other", "note": strings.Repeat("x", 501)}, nil); code != 400 || m["code"] != "err.bad_report" {
		t.Fatalf("long note: %d %v", code, m)
	}
	if code, _, _ := c.do("POST", "/api/n/tidak-ada/report", map[string]any{"reason": "spam"}, nil); code != 404 {
		t.Fatalf("missing note: %d", code)
	}
	// 5 per hour per address.
	for i := 0; i < 4; i++ {
		if code, _, _ := c.do("POST", "/api/n/"+n.token+"/report", map[string]any{"reason": "abuse"}, nil); code != 201 {
			t.Fatalf("report %d: %d", i+2, code)
		}
	}
	if code, m, _ := c.do("POST", "/api/n/"+n.token+"/report", map[string]any{"reason": "abuse"}, nil); code != 429 || m["code"] != "err.report_limit" {
		t.Fatalf("limit: %d %v", code, m)
	}
	// The control center sees them, without the note's content.
	_, env := call(t, srv, "imperium", "GET", "/hop/reports?status=open", nil)
	rows := list(env)
	if len(rows) != 5 {
		t.Fatalf("reports: %v", env)
	}
	first := rows[4].(map[string]any)
	if first["reason"] != "spam" || first["note"] != "it sells pills" || first["token"] != "laporkan-aku" || first["encrypted"] != true || first["note_exists"] != true || first["title"] != "" {
		t.Fatalf("report row: %v", first)
	}
	if raw, _ := json.Marshal(env); strings.Contains(string(raw), fakeCipher("hello")) {
		t.Fatal("report list leaked note content")
	}
	// resolve, dismiss, and takedown through a report.
	if code, env := call(t, srv, "imperium", "POST", "/hop/reports/1/actions", map[string]any{"action": "resolve"}); code != 200 || data(env)["status"] != "resolved" {
		t.Fatalf("resolve: %d %v", code, env)
	}
	if code, env := call(t, srv, "imperium", "POST", "/hop/reports/2/actions", map[string]any{"action": "dismiss"}); code != 200 || data(env)["status"] != "dismissed" {
		t.Fatalf("dismiss: %d %v", code, env)
	}
	_, env = call(t, srv, "imperium", "GET", "/hop/reports?status=open", nil)
	if len(list(env)) != 3 {
		t.Fatalf("open after resolve: %v", env)
	}
	if code, env := call(t, srv, "imperium", "POST", "/hop/reports/3/actions", map[string]any{"action": "takedown", "reason": "spam"}); code != 200 || data(env)["deleted"] != true {
		t.Fatalf("takedown: %d %v", code, env)
	}
	if code, _, _ := c.do("GET", "/api/n/"+n.token, nil, nil); code != 404 {
		t.Fatalf("note after takedown: %d", code)
	}
	if _, _, held := d.SlugHold("laporkan-aku"); !held {
		t.Fatal("takedown via report did not hold the name")
	}
	// A takedown of a note that is already gone still resolves the report.
	if code, env := call(t, srv, "imperium", "POST", "/hop/reports/4/actions", map[string]any{"action": "takedown"}); code != 200 || data(env)["deleted"] != false || data(env)["status"] != "resolved" {
		t.Fatalf("takedown gone: %d %v", code, env)
	}
	if code, env := call(t, srv, "imperium", "POST", "/hop/reports/99/actions", map[string]any{"action": "resolve"}); code != 404 || env["error"] != "not_found" {
		t.Fatalf("missing report: %d %v", code, env)
	}
	if code, env := call(t, srv, "imperium", "POST", "/hop/reports/5/actions", map[string]any{"action": "escalate"}); code != 400 || env["error"] != "unknown_action" {
		t.Fatalf("unknown report action: %d %v", code, env)
	}
	_, env = call(t, srv, "imperium", "GET", "/hop/stats", nil)
	if k := data(env)["kpi"].(map[string]any); k["takedowns"].(float64) != 1 || k["reports_open"].(float64) != 1 {
		t.Fatalf("stats after reports: %v", k)
	}
}

func TestHopSettingsAndSearch(t *testing.T) {
	srv, _, d := hopApp(t, Config{})
	_, env := call(t, srv, "imperium", "GET", "/hop/settings", nil)
	var reg map[string]any
	for _, s := range list(env) {
		if m := s.(map[string]any); m["key"] == "registration" {
			reg = m
		} else if m["readonly"] != true {
			t.Fatalf("%v should be read-only", m["key"])
		}
	}
	if reg == nil || reg["value"] != true || reg["readonly"] != nil && reg["readonly"] != false {
		t.Fatalf("registration setting: %v", reg)
	}
	// Only registration may change, and it overrides the environment.
	if code, env := call(t, srv, "imperium", "PUT", "/hop/settings", map[string]any{"file_mb": 1}); code != 400 || env["error"] != "readonly_setting" {
		t.Fatalf("readonly: %d %v", code, env)
	}
	if code, env := call(t, srv, "imperium", "PUT", "/hop/settings", map[string]any{"registration": "no"}); code != 400 || env["error"] != "bad_value" {
		t.Fatalf("bad value: %d %v", code, env)
	}
	if code, env := call(t, srv, "imperium", "PUT", "/hop/settings", map[string]any{"registration": false}); code != 200 || data(env)["restart_required"] != false {
		t.Fatalf("put: %d %v", code, env)
	}
	c := newClient(t, srv)
	if code, _, _ := c.do("POST", "/api/register", map[string]any{"username": "baru", "auth": key()}, nil); code != 403 {
		t.Fatalf("register while closed: %d", code)
	}
	x := noRedirect(t, srv)
	if code, _ := ssoCallback(t, x, ticketFor("3", ssoStart(t, x, "/"))); code != http.StatusForbidden {
		t.Fatalf("sso sign-up while closed: %d", code)
	}
	if code, _, body := c.do("GET", "/register", nil, nil); code != 200 || !strings.Contains(body, "closed") {
		t.Fatalf("register page while closed: %d", code)
	}
	// The override survives a restart (it lives in the database).
	a2, err := New(Config{Registration: true, BaseURL: "https://note.test", SSOURL: "https://accounts.test", SSOSecret: testSecret, HopClients: []string{"imperium"}}, d)
	if err != nil {
		t.Fatal(err)
	}
	if a2.registration() {
		t.Fatal("override lost after restart")
	}
	call(t, srv, "imperium", "PUT", "/hop/settings", map[string]any{"registration": true})
	if code, _, _ := c.do("POST", "/api/register", map[string]any{"username": "baru", "auth": key()}, nil); code != 201 {
		t.Fatalf("register after reopening: %d", code)
	}

	// Search: exact token or name, public titles, users.
	u := ssoUser(t, srv, "8", "cari.aku@example.com", nil)
	enc := u.create(map[string]any{"slug": "nama-unik-sekali"})
	pub, _ := json.Marshal(map[string]string{"t": "Daftar Belanja", "b": "nama-unik-sekali"})
	u.create(map[string]any{"encrypted": false, "content": string(pub)})
	_, env = call(t, srv, "imperium", "GET", "/hop/search?q=nama-unik-sekali", nil)
	if l := list(env); len(l) != 1 || l[0].(map[string]any)["kind"] != "content" || l[0].(map[string]any)["url"] != "https://note.test/"+enc.token || l[0].(map[string]any)["label"] != "/nama-unik-sekali" {
		t.Fatalf("search token: %v", env)
	}
	_, env = call(t, srv, "imperium", "GET", "/hop/search?q=belanja", nil)
	if l := list(env); len(l) != 1 || l[0].(map[string]any)["label"] != "Daftar Belanja" {
		t.Fatalf("search title: %v", env)
	}
	_, env = call(t, srv, "imperium", "GET", "/hop/search?q=cari.aku", nil)
	if l := list(env); len(l) != 1 || l[0].(map[string]any)["kind"] != "user" || l[0].(map[string]any)["id"] != "8" || l[0].(map[string]any)["label"] != "@cari_aku" {
		t.Fatalf("search user: %v", env)
	}
	_, env = call(t, srv, "imperium", "GET", "/hop/search?q=", nil)
	if len(list(env)) != 0 {
		t.Fatalf("empty search: %v", env)
	}
	// The settings change was audited.
	_, env = call(t, srv, "imperium", "GET", "/hop/audit", nil)
	if l := list(env); len(l) != 2 || l[0].(map[string]any)["action"] != "settings.update" || l[0].(map[string]any)["target"] != "registration" {
		t.Fatalf("audit: %v", env)
	}
}
