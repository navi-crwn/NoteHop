package app

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/internal/db"
	"github.com/navi-crwn/NoteHop/web"
)

func TestAvatarURL(t *testing.T) {
	hub := &App{cfg: Config{SSOURL: "https://hub.hel.ink"}}
	none := &App{}
	long := "https://hub.hel.ink/" + strings.Repeat("a", 500)
	for _, tc := range []struct {
		name string
		a    *App
		in   string
		want string
	}{
		{"empty", hub, "", ""},
		{"absolute stays", hub, "https://cdn.example.com/a.png?v=2", "https://cdn.example.com/a.png?v=2"},
		{"absolute http stays", hub, "http://localhost:3000/a.png", "http://localhost:3000/a.png"},
		{"root path goes to the hub", hub, "/storage/avatars/a.png", "https://hub.hel.ink/storage/avatars/a.png"},
		{"bare path goes to the hub", hub, "storage/avatars/a.png", "https://hub.hel.ink/storage/avatars/a.png"},
		{"scheme-less host", hub, "//hel.ink/a.png", "https://hel.ink/a.png"},
		{"spaces trimmed", hub, "  /a.png ", "https://hub.hel.ink/a.png"},
		{"relative without a hub is useless", none, "/storage/a.png", ""},
		{"javascript", hub, "javascript:alert(1)", ""},
		{"data", hub, "data:image/png;base64,AAAA", ""},
		{"credentials", hub, "https://user:pw@hub.hel.ink/a.png", ""},
		{"too long is dropped, not cut", hub, long, ""},
		{"garbage", hub, "http://[::1", ""},
	} {
		if got := tc.a.avatarURL(tc.in); got != tc.want {
			t.Errorf("%s: avatarURL(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

// The avatar the hub sends (ticket or profile push) is kept as an address
// that works from any host, and /hop/users reports it that way.
func TestAvatarFromHubIsAbsolute(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	a, err := New(Config{Registration: true, SSOURL: "https://hub.hel.ink", BaseURL: "https://note.hel.ink"}, d)
	if err != nil {
		t.Fatal(err)
	}
	u, err := a.linkSSOUser(&ssoTicket{Sub: "hop-1", Email: "a@example.com", Name: "A", Username: "avauser", Avatar: "/storage/avatars/a.png"})
	if err != nil {
		t.Fatal(err)
	}
	if u.Avatar != "https://hub.hel.ink/storage/avatars/a.png" {
		t.Fatalf("ticket avatar: %q", u.Avatar)
	}
	if _, err := a.applyProfile(hopProfile{Sub: "hop-1", AvatarURL: "//hel.ink/hop-assets/a.png"}); err != nil {
		t.Fatal(err)
	}
	got, err := d.UserBySSO("hop-1")
	if err != nil || got.Avatar != "https://hel.ink/hop-assets/a.png" {
		t.Fatalf("profile avatar: %q %v", got.Avatar, err)
	}
	// A row stored before this fix is reported absolute too.
	got.Avatar = "/old/relative.png"
	if v := a.hopUserView(got); v["avatar"] != "https://hub.hel.ink/old/relative.png" {
		t.Fatalf("stored avatar in /hop/users: %v", v["avatar"])
	}
}

// Link previews and OG tags name the image by the site's public address, not
// by the host the request happened to arrive on, and the note page never
// carries the content.
func TestOGImageIsAbsoluteOnThePublicHost(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	a, err := New(Config{Registration: true, BaseURL: "https://note.hel.ink"}, d)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	c := newClient(t, srv)
	n := c.create(map[string]any{})
	const want = `content="https://note.hel.ink/static/img/og.png"`
	for name, ua := range map[string]string{"bot": "Twitterbot/1.0", "browser": "Mozilla/5.0 Chrome/126"} {
		req, _ := http.NewRequest("GET", srv.URL+"/"+n.token, nil)
		req.Header.Set("User-Agent", ua)
		res, err := c.hc.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		page := string(b)
		if !strings.Contains(page, want) || !strings.Contains(page, `name="robots" content="noindex, nofollow"`) || strings.Contains(page, fakeCipher("hello")) {
			t.Fatalf("%s page: og image or noindex missing:\n%s", name, page)
		}
	}
}

var staticRef = regexp.MustCompile(`["'(=]\s*(/static/[A-Za-z0-9_./-]+)`)

// Every /static/ file a template, a script or the manifest names exists in the
// embedded files, so a rename can't leave a page asking for a 404.
func TestStaticReferencesExist(t *testing.T) {
	sub, err := fs.Sub(web.Static, "static")
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	check := func(from, ref string) {
		seen++
		if _, err := fs.Stat(sub, strings.TrimPrefix(ref, "/static/")); err != nil {
			t.Errorf("%s names %s, which is not in web/static", from, ref)
		}
	}
	for _, root := range []struct {
		fsys fs.FS
		dir  string
	}{{web.Templates, "templates"}, {web.Static, "static"}} {
		_ = fs.WalkDir(root.fsys, root.dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || strings.Contains(p, "/vendor/") || strings.HasSuffix(p, ".webp") || strings.HasSuffix(p, ".png") {
				return nil
			}
			b, _ := fs.ReadFile(root.fsys, p)
			for _, m := range staticRef.FindAllStringSubmatch(string(b), -1) {
				if !strings.HasSuffix(m[1], "/") && !strings.Contains(m[1], "{{") {
					check(p, m[1])
				}
			}
			return nil
		})
	}
	var m struct {
		Icons []struct {
			Src string `json:"src"`
		} `json:"icons"`
	}
	b, _ := fs.ReadFile(sub, "manifest.webmanifest")
	if err := json.Unmarshal(b, &m); err != nil || len(m.Icons) == 0 {
		t.Fatalf("manifest: %v", err)
	}
	for _, ic := range m.Icons {
		check("manifest.webmanifest", ic.Src)
	}
	if seen < 30 {
		t.Fatalf("only %d /static/ references found: the pattern no longer matches the templates", seen)
	}
	// The demo pictures are named by a template loop.
	for _, g := range []string{"share", "private", "together", "blocks", "dev", "web"} {
		for _, th := range []string{"light", "dark"} {
			check("home tour", "/static/img/demos/"+g+"-"+th+".webp")
		}
	}
}

// The worker must not keep the plain text of a note (burn notes included).
func TestServiceWorkerKeepsNoPlainText(t *testing.T) {
	srv := newTestApp(t)
	res, err := http.Get(srv.URL + "/sw.js")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	js := string(b)
	if !strings.Contains(js, "NEVER_KEEP = /^\\/(auth|logout|raw|mock|in)(\\/|$)/") || !strings.Contains(js, "NEVER_KEEP.test(url.pathname)") {
		t.Fatalf("sw.js should skip /raw, /mock, /in when caching pages:\n%s", js)
	}
	if strings.Count(js, "res.status === 200") != 2 {
		t.Fatal("sw.js should cache only whole (200) responses of pages and static files")
	}
}
