package app

import (
	"net/http/httptest"
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

func TestSitemapIs404(t *testing.T) {
	d, _ := db.Open(":memory:")
	a, _ := New(Config{Registration: true}, d)
	srv := httptest.NewServer(a.Handler())
	defer func() { srv.Close(); d.Close() }()
	c := newClient(t, srv)
	if code, _, _ := c.do("GET", "/sitemap.xml", nil, nil); code != 404 {
		t.Fatalf("sitemap.xml: %d", code)
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
