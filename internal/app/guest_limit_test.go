package app

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/internal/db"
)

func TestReservedProductNames(t *testing.T) {
	a := &App{}
	for _, n := range []string{"secret", "embargo", "send", "vault", "pay"} {
		if !a.reservedName(n) {
			t.Errorf("%s should be reserved", n)
		}
	}
}

func TestSitemapStaticPagesOnly(t *testing.T) {
	d, _ := db.Open(":memory:")
	a, _ := New(Config{Registration: true, BaseURL: "https://note.test"}, d)
	srv := httptest.NewServer(a.Handler())
	defer func() { srv.Close(); d.Close() }()
	c := newClient(t, srv)
	create, _, _ := c.do("POST", "/api/notes", map[string]any{"encrypted": false, "content": `{"t":"x","b":"x"}`, "mode": "read", "admin": key(), "write": key(), "slug": "my-secret-plan"}, nil)
	if create >= 300 {
		t.Fatalf("create: %d", create)
	}
	code, _, body := c.do("GET", "/sitemap.xml", nil, nil)
	if code != 200 || !strings.Contains(body, "<loc>https://note.test/about</loc>") || !strings.Contains(body, "<loc>https://note.test/changelog</loc>") {
		t.Fatalf("sitemap: %d %s", code, body)
	}
	if strings.Contains(body, "my-secret-plan") || strings.Contains(body, "/n/") || strings.Contains(body, "/@") {
		t.Fatalf("sitemap lists notes: %s", body)
	}
}

func TestGuestNoteKeys(t *testing.T) {
	d, _ := db.Open(":memory:")
	defer d.Close()
	a, _ := New(Config{GuestNotesPerIP: 1}, d)
	if !a.guestNote("203.0.113.1") || a.guestNote("203.0.113.1") || !a.guestNote("203.0.113.2") {
		t.Fatal("two addresses count separately")
	}
	if !a.guestNote("2001:db8:1:2::1") || a.guestNote("2001:db8:1:2:ffff::9") || !a.guestNote("2001:db8:1:3::1") {
		t.Fatal("IPv6 is grouped by /64")
	}
}

func TestSSHNotesCountAsGuest(t *testing.T) {
	a, d, _ := newAppDB(t)
	defer d.Close()
	a.cfg.GuestNotesPerIP = 1
	if out := a.sshRun("new", strings.NewReader("one"), "198.51.100.7"); strings.HasPrefix(out, "error:") {
		t.Fatalf("first: %q", out)
	}
	if out := a.sshRun("new", strings.NewReader("two"), "198.51.100.7"); !strings.HasPrefix(out, "error: too many notes") {
		t.Fatalf("second: %q", out)
	}
	if out := a.sshRun("new", strings.NewReader("three"), "198.51.100.8"); strings.HasPrefix(out, "error:") {
		t.Fatalf("other address: %q", out)
	}
}

func TestForwardedForOnlyWhenTrusted(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:5555"
	r.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := (&App{}).clientIP(r); got != "10.0.0.1" {
		t.Fatalf("untrusted: %s", got)
	}
	if got := (&App{cfg: Config{TrustProxy: true}}).clientIP(r); got != "203.0.113.9" {
		t.Fatalf("trusted: %s", got)
	}
}

func TestGuestNoteLimit(t *testing.T) {
	create := func(c *client) int {
		code, _, _ := c.do("POST", "/api/notes", map[string]any{"encrypted": false, "content": `{"t":"x","b":"x"}`, "mode": "read", "admin": key(), "write": key()}, nil)
		return code
	}
	run := func(limit int) (*client, func()) {
		d, _ := db.Open(":memory:")
		a, _ := New(Config{Registration: true, GuestNotesPerIP: limit}, d)
		srv := httptest.NewServer(a.Handler())
		return newClient(t, srv), func() { srv.Close(); d.Close() }
	}
	c, done := run(2)
	if create(c) >= 300 || create(c) >= 300 {
		t.Fatal("guest notes under the limit")
	}
	if code := create(c); code != 429 {
		t.Fatalf("third guest note: %d", code)
	}
	done()
	c, done = run(0)
	defer done()
	for i := 0; i < 5; i++ {
		if code := create(c); code >= 300 {
			t.Fatalf("limit 0 must not limit: %d", code)
		}
	}
}

func TestGuestNoteLimitSkipsSignedIn(t *testing.T) {
	d, _ := db.Open(":memory:")
	a, _ := New(Config{Registration: true, GuestNotesPerIP: 1}, d)
	srv := httptest.NewServer(a.Handler())
	defer func() { srv.Close(); d.Close() }()
	c := newClient(t, srv)
	if code, _, _ := c.do("POST", "/api/register", map[string]any{"username": "ivan", "auth": key()}, nil); code != 201 {
		t.Fatalf("register: %d", code)
	}
	for i := 0; i < 4; i++ {
		code, _, _ := c.do("POST", "/api/notes", map[string]any{"encrypted": false, "content": `{"t":"x","b":"x"}`, "mode": "read", "admin": key(), "write": key()}, nil)
		if code >= 300 {
			t.Fatalf("signed-in note %d: %d", i, code)
		}
	}
}
