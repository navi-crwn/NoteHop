package app

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/navi-crwn/NoteHop/internal/db"
)

func newTestApp(t *testing.T) *httptest.Server {
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
	return srv
}

func key() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// fakeCipher stands in for browser ciphertext: any base64url of 38+ chars.
func fakeCipher(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte("iv-iv-iv-iv-" + s + "-tag-tag-tag-tag"))
}

type client struct {
	t   *testing.T
	srv *httptest.Server
	hc  *http.Client
}

func newClient(t *testing.T, srv *httptest.Server) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, srv: srv, hc: &http.Client{Jar: jar}}
}

func (c *client) do(method, path string, body any, hdr map[string]string) (int, map[string]any, string) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.srv.URL+path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return res.StatusCode, m, string(raw)
}

type note struct {
	token, admin, write string
}

func (c *client) create(extra map[string]any) note {
	c.t.Helper()
	n := note{admin: key(), write: key()}
	body := map[string]any{"encrypted": true, "content": fakeCipher("hello"), "mode": "read", "admin": n.admin, "write": n.write}
	for k, v := range extra {
		body[k] = v
	}
	code, m, raw := c.do("POST", "/api/notes", body, nil)
	if code != http.StatusCreated {
		c.t.Fatalf("create: %d %s", code, raw)
	}
	n.token = m["token"].(string)
	return n
}

func TestCreateAndOpen(t *testing.T) {
	c := newClient(t, newTestApp(t))
	n := c.create(nil)
	code, m, _ := c.do("GET", "/api/n/"+n.token, nil, nil)
	if code != 200 || m["mode"] != "read" || m["encrypted"] != true || m["admin"] != false {
		t.Fatalf("meta: %d %v", code, m)
	}
	if _, ok := m["content"]; ok {
		t.Fatal("meta must not include content")
	}
	code, m, _ = c.do("POST", "/api/n/"+n.token+"/open", nil, nil)
	if code != 200 || m["content"] != fakeCipher("hello") || m["views"].(float64) != 1 {
		t.Fatalf("open: %d %v", code, m)
	}
	code, m, _ = c.do("GET", "/api/n/"+n.token, nil, map[string]string{"X-Admin": n.admin})
	if m["admin"] != true || len(m["links"].([]any)) != 1 {
		t.Fatalf("admin meta: %v", m)
	}
	if code, _, _ := c.do("GET", "/n/"+n.token, nil, nil); code != 200 {
		t.Fatalf("page: %d", code)
	}
	if code, _, _ := c.do("GET", "/n/nope-nope", nil, nil); code != 404 {
		t.Fatalf("missing page: %d", code)
	}
}

func TestCreateValidation(t *testing.T) {
	c := newClient(t, newTestApp(t))
	bad := []map[string]any{
		{"mode": "owner", "encrypted": true, "content": fakeCipher("x"), "admin": key(), "write": key()},
		{"mode": "read", "encrypted": true, "content": "not base64!", "admin": key(), "write": key()},
		{"mode": "read", "encrypted": false, "content": "{}", "admin": key(), "write": key()},
		{"mode": "read", "encrypted": false, "password": true, "salt": key(), "content": `{"t":"","b":"x"}`, "admin": key(), "write": key()},
		{"mode": "read", "encrypted": true, "content": fakeCipher("x"), "admin": "short", "write": key()},
		{"mode": "read", "encrypted": true, "content": fakeCipher("x"), "admin": key(), "write": key(), "slug": "A!"},
		{"mode": "read", "encrypted": true, "content": fakeCipher("x"), "admin": key(), "write": key(), "max_views": 5000},
	}
	for i, b := range bad {
		if code, _, raw := c.do("POST", "/api/notes", b, nil); code != 400 {
			t.Errorf("case %d: want 400, got %d %s", i, code, raw)
		}
	}
	c.create(map[string]any{"slug": "rapat-senin"})
	if code, _, _ := c.do("POST", "/api/notes", map[string]any{"mode": "read", "encrypted": true, "content": fakeCipher("x"),
		"admin": key(), "write": key(), "slug": "rapat-senin"}, nil); code != 409 {
		t.Fatalf("duplicate slug: %d", code)
	}
}

func TestModesAndWriteKey(t *testing.T) {
	c := newClient(t, newTestApp(t))
	n := c.create(map[string]any{"mode": "read", "content": fakeCipher("v1")})
	w := map[string]string{"X-Write": n.write}

	// read link: no edits, entries or checks, even with the key
	if code, _, _ := c.do("PUT", "/api/n/"+n.token, map[string]any{"content": fakeCipher("v2"), "version": 1}, w); code != 403 {
		t.Fatalf("read link save: %d", code)
	}
	if code, _, _ := c.do("POST", "/api/n/"+n.token+"/entries", map[string]any{"content": fakeCipher("e")}, w); code != 403 {
		t.Fatalf("read link append: %d", code)
	}

	// admin adds an append link and a check link
	adm := map[string]string{"X-Admin": n.admin, "X-Write": n.write}
	_, app, _ := c.do("POST", "/api/n/"+n.token+"/links", map[string]any{"mode": "append"}, adm)
	_, chk, _ := c.do("POST", "/api/n/"+n.token+"/links", map[string]any{"mode": "check", "slug": "belanja"}, adm)
	appTok := app["token"].(string)

	// append needs the key proof
	if code, _, _ := c.do("POST", "/api/n/"+appTok+"/entries", map[string]any{"content": fakeCipher("e")}, nil); code != 403 {
		t.Fatalf("append without key: %d", code)
	}
	if code, _, _ := c.do("POST", "/api/n/"+appTok+"/entries", map[string]any{"content": fakeCipher("e1")}, w); code != 201 {
		t.Fatalf("append: %d", code)
	}
	if code, _, _ := c.do("PUT", "/api/n/"+appTok, map[string]any{"content": fakeCipher("v2"), "version": 1}, w); code != 403 {
		t.Fatalf("append link save: %d", code)
	}
	// append link can't delete entries
	if code, _, _ := c.do("DELETE", "/api/n/"+appTok+"/entries/1", nil, w); code != 403 {
		t.Fatalf("append link delete entry: %d", code)
	}

	// check link ticks boxes only
	if code, m, _ := c.do("POST", "/api/n/belanja/checks", map[string]any{"index": 2, "checked": true}, w); code != 200 || m["checks"].(map[string]any)["2"] != true {
		t.Fatalf("check: %d %v", code, m)
	}
	if code, _, _ := c.do("POST", "/api/n/"+appTok+"/checks", map[string]any{"index": 0, "checked": true}, w); code != 403 {
		t.Fatalf("append link check: %d", code)
	}
	_ = chk

	// admin edits; save clears check overrides
	code, m, _ := c.do("PUT", "/api/n/"+n.token, map[string]any{"content": fakeCipher("v2"), "version": 1}, adm)
	if code != 200 || m["version"].(float64) != 2 {
		t.Fatalf("admin save: %d %v", code, m)
	}
	_, m, _ = c.do("POST", "/api/n/"+n.token+"/open", map[string]any{}, adm)
	if m["content"] != fakeCipher("v2") || len(m["checks"].(map[string]any)) != 0 || len(m["entries"].([]any)) != 1 {
		t.Fatalf("after save: %v", m)
	}
	if m["views"].(float64) != 0 {
		t.Fatalf("admin opens must not count: %v", m["views"])
	}
	// admin removes the leaked append link; others keep working
	if code, _, _ := c.do("DELETE", "/api/n/"+n.token+"/links/"+appTok, nil, adm); code != 200 {
		t.Fatalf("unlink: %d", code)
	}
	if code, _, _ := c.do("GET", "/api/n/"+appTok, nil, nil); code != 404 {
		t.Fatalf("dead link: %d", code)
	}
	if code, _, _ := c.do("GET", "/api/n/belanja", nil, nil); code != 200 {
		t.Fatalf("other link: %d", code)
	}
}

func TestEditConflict(t *testing.T) {
	c := newClient(t, newTestApp(t))
	n := c.create(map[string]any{"mode": "edit"})
	w := map[string]string{"X-Write": n.write}
	if code, _, _ := c.do("PUT", "/api/n/"+n.token, map[string]any{"content": fakeCipher("a"), "version": 1}, w); code != 200 {
		t.Fatalf("first save: %d", code)
	}
	if code, _, _ := c.do("PUT", "/api/n/"+n.token, map[string]any{"content": fakeCipher("b"), "version": 1}, w); code != 409 {
		t.Fatalf("stale save: %d", code)
	}
	if code, _, _ := c.do("PUT", "/api/n/"+n.token, map[string]any{"content": fakeCipher("b"), "version": 2}, map[string]string{"X-Write": key()}); code != 403 {
		t.Fatalf("wrong key: %d", code)
	}
}

func TestBurnAfterReading(t *testing.T) {
	c := newClient(t, newTestApp(t))
	n := c.create(map[string]any{"max_views": 1})
	// meta and the HTML shell don't spend the view (link previews)
	c.do("GET", "/api/n/"+n.token, nil, nil)
	c.do("GET", "/n/"+n.token, nil, nil)
	if code, _, _ := c.do("POST", "/api/n/"+n.token+"/open", map[string]any{"poll": true}, nil); code != 403 {
		t.Fatalf("poll on burn note: %d", code)
	}
	code, m, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, nil)
	if code != 200 || m["burned"] != true || m["content"] == "" {
		t.Fatalf("burn open: %d %v", code, m)
	}
	if code, _, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, nil); code != 404 {
		t.Fatalf("second open: %d", code)
	}
}

func TestBurnConcurrent(t *testing.T) {
	c := newClient(t, newTestApp(t))
	n := c.create(map[string]any{"max_views": 3})
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if code, _, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, nil); code == 200 {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 3 {
		t.Fatalf("want exactly 3 reads, got %d", ok)
	}
}

func TestPasswordNeedsProof(t *testing.T) {
	c := newClient(t, newTestApp(t))
	n := c.create(map[string]any{"password": true, "salt": key()[:22], "max_views": 1})
	// wrong password (wrong write proof) is refused without burning the note
	for i := 0; i < 3; i++ {
		if code, _, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, map[string]string{"X-Write": key()}); code != 403 {
			t.Fatalf("wrong pw: %d", code)
		}
	}
	if code, m, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, map[string]string{"X-Write": n.write}); code != 200 || m["burned"] != true {
		t.Fatalf("right pw: %d %v", code, m)
	}
}

func TestFreeze(t *testing.T) {
	c := newClient(t, newTestApp(t))
	n := c.create(map[string]any{"mode": "edit", "content": fakeCipher("v1")})
	adm := map[string]string{"X-Admin": n.admin, "X-Write": n.write}
	c.do("POST", "/api/n/"+n.token+"/entries", map[string]any{"content": fakeCipher("line")}, adm)
	if code, _, _ := c.do("POST", "/api/n/"+n.token+"/freeze", nil, map[string]string{"X-Write": n.write}); code != 403 {
		t.Fatalf("freeze without admin: %d", code)
	}
	code, s, _ := c.do("POST", "/api/n/"+n.token+"/freeze", nil, adm)
	if code != 201 || s["mode"] != "read" {
		t.Fatalf("freeze: %d %v", code, s)
	}
	snap := s["token"].(string)
	c.do("PUT", "/api/n/"+n.token, map[string]any{"content": fakeCipher("v2"), "version": 1}, adm)

	_, m, _ := c.do("POST", "/api/n/"+snap+"/open", nil, nil)
	if m["content"] != fakeCipher("v1") || m["frozen"] != true || len(m["entries"].([]any)) != 1 {
		t.Fatalf("snapshot content: %v", m)
	}
	// nobody, admin included, can change a snapshot
	if code, _, _ := c.do("PUT", "/api/n/"+snap, map[string]any{"content": fakeCipher("x"), "version": 1}, adm); code != 403 {
		t.Fatalf("edit snapshot: %d", code)
	}
	if code, _, _ := c.do("POST", "/api/n/"+snap+"/links", map[string]any{"mode": "edit"}, adm); code != 400 {
		t.Fatalf("edit link on snapshot: %d", code)
	}
	_, m, _ = c.do("GET", "/api/n/"+n.token, nil, adm)
	if len(m["snapshots"].([]any)) != 1 {
		t.Fatalf("snapshots listed: %v", m)
	}
}

func TestRaw(t *testing.T) {
	c := newClient(t, newTestApp(t))
	content, _ := json.Marshal(map[string]string{"t": "Cfg", "b": "- [ ] a\n- [ ] b\n```\n- [ ] code\n```"})
	pub := c.create(map[string]any{"encrypted": false, "content": string(content), "mode": "check"})
	c.do("POST", "/api/n/"+pub.token+"/checks", map[string]any{"index": 1, "checked": true}, map[string]string{"X-Write": pub.write})
	c.do("POST", "/api/n/"+pub.token+"/entries", map[string]any{"content": `{"n":"x","b":"tambahan"}`}, map[string]string{"X-Write": pub.write, "X-Admin": pub.admin})
	code, _, raw := c.do("GET", "/raw/"+pub.token, nil, nil)
	want := "- [ ] a\n- [x] b\n```\n- [ ] code\n```\n\ntambahan\n"
	if code != 200 || raw != want {
		t.Fatalf("raw: %d %q", code, raw)
	}
	enc := c.create(nil)
	if code, _, raw := c.do("GET", "/raw/"+enc.token, nil, nil); code != 403 || !strings.Contains(raw, "encrypted") {
		t.Fatalf("raw encrypted: %d %s", code, raw)
	}
}

func TestDeleteAndOwnership(t *testing.T) {
	srv := newTestApp(t)
	c := newClient(t, srv)
	if code, _, _ := c.do("POST", "/api/register", map[string]any{"username": "ivan", "auth": key()}, nil); code != 201 {
		t.Fatalf("register: %d", code)
	}
	n := c.create(map[string]any{"owner_blob": "opaque-vault-blob"})
	_, m, _ := c.do("GET", "/api/me/notes", nil, nil)
	notes := m["notes"].([]any)
	if len(notes) != 1 || notes[0].(map[string]any)["blob"] != "opaque-vault-blob" {
		t.Fatalf("my notes: %v", m)
	}
	stranger := newClient(t, srv)
	if code, _, _ := stranger.do("DELETE", "/api/n/"+n.token, nil, nil); code != 403 {
		t.Fatalf("stranger delete: %d", code)
	}
	// the owner's session alone is enough
	if code, _, _ := c.do("DELETE", "/api/n/"+n.token, nil, nil); code != 200 {
		t.Fatalf("owner delete: %d", code)
	}
}

func TestAuth(t *testing.T) {
	srv := newTestApp(t)
	c := newClient(t, srv)
	auth := key()
	if code, _, _ := c.do("POST", "/api/register", map[string]any{"username": "Bad Name", "auth": auth}, nil); code != 400 {
		t.Fatalf("bad username: %d", code)
	}
	c.do("POST", "/api/register", map[string]any{"username": "ivan", "auth": auth}, nil)
	if code, _, _ := c.do("POST", "/api/register", map[string]any{"username": "ivan", "auth": auth}, nil); code != 409 {
		t.Fatalf("duplicate: %d", code)
	}
	c2 := newClient(t, srv)
	if code, _, _ := c2.do("POST", "/api/login", map[string]any{"username": "ivan", "auth": key()}, nil); code != 401 {
		t.Fatalf("wrong auth: %d", code)
	}
	if code, _, _ := c2.do("POST", "/api/login", map[string]any{"username": "ivan", "auth": auth}, nil); code != 200 {
		t.Fatalf("login: %d", code)
	}
	if code, m, _ := c2.do("GET", "/api/me", nil, nil); code != 200 || m["username"] != "ivan" {
		t.Fatalf("me: %d %v", code, m)
	}
	c2.do("POST", "/api/logout", map[string]any{}, nil)
	if code, _, _ := c2.do("GET", "/api/me", nil, nil); code != 401 {
		t.Fatalf("after logout: %d", code)
	}
}

func TestCrossSiteRejected(t *testing.T) {
	c := newClient(t, newTestApp(t))
	code, _, _ := c.do("POST", "/api/notes", map[string]any{}, map[string]string{"Origin": "https://evil.example"})
	if code != 403 {
		t.Fatalf("cross-site: %d", code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	srv := newTestApp(t)
	res, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if !strings.Contains(res.Header.Get("Content-Security-Policy"), "script-src 'self'") || res.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("headers: %v", res.Header)
	}
}
