package app

import (
	"net/http/httptest"
	"testing"

	"github.com/navi-crwn/NoteHop/internal/db"
)

func TestNameRules(t *testing.T) {
	if !blockedName("dasar-kontol") || !blockedName("fuck") || !blockedName("f-u-c-k") || blockedName("class-notes") || blockedName("assassin-plan") {
		t.Fatal("blocklist matching")
	}
	a := &App{cfg: Config{Reserved: []string{"promo-lebaran"}}}
	for _, n := range []string{"faq", "admin", "notehop-promo", "admin_panel", "promo-lebaran", "official-news"} {
		if !a.reservedName(n) {
			t.Errorf("%s should be reserved", n)
		}
	}
	if a.reservedName("rapat-senin") {
		t.Error("ordinary name reserved")
	}
}

func TestSlugRules(t *testing.T) {
	d, _ := db.Open(":memory:")
	a, _ := New(Config{Registration: true, Admins: []string{"navi"}}, d)
	srv := httptest.NewServer(a.Handler())
	defer func() { srv.Close(); d.Close() }()
	c := newClient(t, srv)
	try := func(c *client, slug string) (int, string) {
		t.Helper()
		code, m, _ := c.do("POST", "/api/notes", map[string]any{"encrypted": false, "content": `{"t":"x","b":"x"}`, "mode": "read", "admin": key(), "write": key(), "slug": slug}, nil)
		s, _ := m["code"].(string)
		return code, s
	}
	for slug, want := range map[string]string{
		"rapat":         "err.slug_short",
		"support-team":  "err.slug_reserved",
		"notehop-promo": "err.slug_reserved",
		"rapat--senin":  "err.slug_format",
		"rapat-senin-":  "err.slug_format",
		"kontol-banget": "err.name_blocked",
	} {
		if _, got := try(c, slug); got != want {
			t.Errorf("%s: got %q, want %q", slug, got, want)
		}
	}
	// Usernames: reserved and blocked names are refused, the admin may take one.
	if code, m, _ := c.do("POST", "/api/register", map[string]any{"username": "support", "auth": key()}, nil); code != 400 || m["code"] != "err.username_reserved" {
		t.Fatalf("reserved username: %d %v", code, m)
	}
	if code, _, _ := c.do("POST", "/api/register", map[string]any{"username": "budi", "auth": key()}, nil); code != 201 {
		t.Fatalf("register: %d", code)
	}
	// Held after deletion: a stranger can't take it, the owner can.
	n := c.create(map[string]any{"slug": "rapat-senin"})
	if code, _, _ := c.do("DELETE", "/api/n/"+n.token, nil, map[string]string{"X-Admin": n.admin}); code != 200 {
		t.Fatalf("delete: %d", code)
	}
	stranger := newClient(t, srv)
	if _, got := try(stranger, "rapat-senin"); got != "err.slug_held" {
		t.Fatalf("stranger took a held name: %q", got)
	}
	if code, _ := try(c, "rapat-senin"); code != 201 {
		t.Fatalf("owner couldn't take the name back: %d", code)
	}
	// Short and reserved names are fine for a site admin. Being listed in
	// NOTEHOP_ADMINS grants nothing by itself: the role is seeded at a start
	// (SeedAdmins) once the account exists, so a squatter who registers the
	// name first gets no privileges.
	adm := newClient(t, srv)
	if code, _, _ := adm.do("POST", "/api/register", map[string]any{"username": "navi", "auth": key()}, nil); code != 201 {
		t.Fatalf("admin register: %d", code)
	}
	if code, got := try(adm, "eid"); code != 400 || got != "err.slug_short" {
		t.Fatalf("listed name before seeding should be an ordinary user: %d %s", code, got)
	}
	if err := a.SeedAdmins(); err != nil {
		t.Fatal(err)
	}
	if code, got := try(adm, "eid"); code != 201 {
		t.Fatalf("admin short name: %d %s", code, got)
	}
	if code, got := try(adm, "faq-notes"); code != 201 {
		t.Fatalf("admin reserved name: %d %s", code, got)
	}
	// Holds end and expired holds are swept.
	d.Exec(`UPDATE slug_holds SET until = 1`)
	d.Sweep()
	var k int
	d.QueryRow(`SELECT COUNT(*) FROM slug_holds`).Scan(&k)
	if k != 0 {
		t.Fatalf("holds left: %d", k)
	}
}
