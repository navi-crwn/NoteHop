package app

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

const basicBody = `{"t":"Rapat","b":"agenda"}`

func basic(c *client, extra map[string]any) note {
	body := map[string]any{"encrypted": false, "content": basicBody}
	for k, v := range extra {
		body[k] = v
	}
	return c.create(body)
}

// Only a basic note has a link name of its own; an encrypted note keeps the long link.
func TestSlugOnlyForBasicNotes(t *testing.T) {
	c := newClient(t, newTestApp(t))
	code, m, _ := c.do("POST", "/api/notes", map[string]any{"mode": "read", "encrypted": true, "content": fakeCipher("x"), "admin": key(), "write": key(), "slug": "catatan-saya"}, nil)
	if code != 400 || m["code"] != "err.slug_basic_only" {
		t.Fatalf("encrypted with a name: %d %v", code, m)
	}
	n := basic(c, map[string]any{"slug": "catatan-saya"})
	if n.token != "catatan-saya" {
		t.Fatalf("basic name: %q", n.token)
	}
	e := c.create(nil)
	adm := map[string]string{"X-Admin": e.admin, "X-Write": e.write}
	code, m, _ = c.do("POST", "/api/n/"+e.token+"/links", map[string]any{"mode": "read", "slug": "nama-lain"}, adm)
	if code != 400 || m["code"] != "err.slug_basic_only" {
		t.Fatalf("name on an encrypted note's new link: %d %v", code, m)
	}
	if code, _, _ := c.do("POST", "/api/n/"+e.token+"/links", map[string]any{"mode": "read"}, adm); code != 201 {
		t.Fatalf("random link on an encrypted note: %d", code)
	}
}

func TestRenameLink(t *testing.T) {
	srv, _, d := hopApp(t, Config{})
	c := newClient(t, srv)
	n := basic(c, map[string]any{"slug": "nama-lama"})
	adm := map[string]string{"X-Admin": n.admin, "X-Write": n.write}
	other := basic(c, map[string]any{"slug": "sudah-dipakai"})

	for _, tc := range []struct {
		slug, want string
		code       int
	}{
		{"nama-lama", "err.rename_same", 400},
		{"A!", "err.bad_slug", 400},
		{"ab", "err.bad_slug", 400},
		{other.token, "err.slug_taken", 409},
		{"support-team", "err.slug_reserved", 400},
	} {
		code, m, _ := c.do("PUT", "/api/n/"+n.token+"/links/nama-lama", map[string]any{"slug": tc.slug}, adm)
		if code != tc.code || m["code"] != tc.want {
			t.Errorf("%q: %d %v, want %d %s", tc.slug, code, m, tc.code, tc.want)
		}
	}
	// Without the owner key nothing changes.
	if code, _, _ := c.do("PUT", "/api/n/nama-lama/links/nama-lama", map[string]any{"slug": "nama-baru"}, nil); code != 403 {
		t.Fatalf("rename without the owner key: %d", code)
	}
	code, m, _ := c.do("PUT", "/api/n/nama-lama/links/nama-lama", map[string]any{"slug": "Nama-Baru"}, adm)
	if code != 200 || m["token"] != "nama-baru" || m["mode"] != "read" {
		t.Fatalf("rename: %d %v", code, m)
	}
	if code, _, _ := c.do("GET", "/api/n/nama-baru", nil, nil); code != 200 {
		t.Fatalf("new name: %d", code)
	}
	if code, _, _ := c.do("GET", "/api/n/nama-lama", nil, nil); code != 404 {
		t.Fatalf("old name still works: %d", code)
	}
	if _, _, held := d.SlugHold("nama-lama"); !held {
		t.Fatal("old name is not held for the owner")
	}

	// A link that does not exist, and an encrypted note.
	if code, m, _ := c.do("PUT", "/api/n/nama-baru/links/tidak-ada", map[string]any{"slug": "nama-ketiga"}, map[string]string{"X-Admin": n.admin, "X-Write": n.write}); code != 404 || m["code"] != "err.link_missing" {
		t.Fatalf("missing link: %d %v", code, m)
	}
	e := c.create(nil)
	if code, m, _ := c.do("PUT", "/api/n/"+e.token+"/links/"+e.token, map[string]any{"slug": "nama-enkripsi"}, map[string]string{"X-Admin": e.admin, "X-Write": e.write}); code != 400 || m["code"] != "err.slug_basic_only" {
		t.Fatalf("rename on an encrypted note: %d %v", code, m)
	}
}

func TestConvertNote(t *testing.T) {
	c := newClient(t, newTestApp(t))
	n := basic(c, map[string]any{"slug": "dari-basic"})
	adm := map[string]string{"X-Admin": n.admin, "X-Write": n.write}
	url := "/api/n/" + n.token + "/convert"

	// Not the owner, not a valid target, same type, bad content.
	if code, _, _ := c.do("POST", url, map[string]any{"to": "encrypted", "content": fakeCipher("x"), "version": 1}, nil); code != 403 {
		t.Fatalf("convert without the owner key: %d", code)
	}
	for _, b := range []struct {
		body map[string]any
		want string
	}{
		{map[string]any{"to": "other", "content": basicBody, "version": 1}, "err.convert_bad"},
		{map[string]any{"to": "basic", "content": basicBody, "version": 1}, "err.convert_same"},
		{map[string]any{"to": "encrypted", "content": "not cipher!", "version": 1}, "err.bad_content"},
	} {
		if code, m, _ := c.do("POST", url, b.body, adm); code != 400 || m["code"] != b.want {
			t.Errorf("%v: %d %v", b.body, code, m)
		}
	}
	// A stale version is refused.
	if code, m, _ := c.do("POST", url, map[string]any{"to": "encrypted", "content": fakeCipher("x"), "version": 7}, adm); code != 409 || m["code"] != "err.conflict" {
		t.Fatalf("stale version: %d %v", code, m)
	}

	// basic -> encrypted: the content is the new ciphertext, the links keep working, the history goes.
	c.do("PUT", "/api/n/"+n.token, map[string]any{"content": `{"t":"Rapat","b":"agenda dua"}`, "version": 1}, adm)
	if code, m, _ := c.do("POST", url, map[string]any{"to": "encrypted", "content": fakeCipher("sealed"), "version": 2}, adm); code != 200 || m["encrypted"] != true || m["version"].(float64) != 3 {
		t.Fatalf("to encrypted: %d %v", code, m)
	}
	if _, m, _ := c.do("GET", "/api/n/"+n.token, nil, adm); m["encrypted"] != true {
		t.Fatalf("meta after: %v", m)
	}
	if _, m, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, nil); m["content"] != fakeCipher("sealed") {
		t.Fatalf("open after: %v", m)
	}
	if _, m, _ := c.do("GET", "/api/n/"+n.token+"/versions", nil, adm); len(asList(m["versions"])) != 0 {
		t.Fatalf("history kept in the old form: %v", m)
	}
	// ...and a name can no longer be chosen or changed on it.
	if code, m, _ := c.do("PUT", "/api/n/"+n.token+"/links/"+n.token, map[string]any{"slug": "nama-baru"}, adm); code != 400 || m["code"] != "err.slug_basic_only" {
		t.Fatalf("rename after: %d %v", code, m)
	}

	// encrypted -> basic, with a password note: the password goes with it.
	if code, m, _ := c.do("POST", url, map[string]any{"to": "basic", "content": basicBody, "version": 3}, adm); code != 200 || m["encrypted"] != false {
		t.Fatalf("to basic: %d %v", code, m)
	}
	if _, m, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, nil); m["content"] != basicBody || m["encrypted"] != false {
		t.Fatalf("open basic: %v", m)
	}
}

func TestConvertBlockedByExtras(t *testing.T) {
	c := newClient(t, newTestApp(t))
	n := c.create(map[string]any{"mode": "append"})
	adm := map[string]string{"X-Admin": n.admin, "X-Write": n.write}
	if code, _, _ := c.do("POST", "/api/n/"+n.token+"/entries", map[string]any{"content": fakeCipher("e1")}, adm); code != 201 {
		t.Fatalf("entry: %d", code)
	}
	code, m, _ := c.do("POST", "/api/n/"+n.token+"/convert", map[string]any{"to": "basic", "content": basicBody, "version": 1}, adm)
	if code != 409 || m["code"] != "err.convert_blocked" {
		t.Fatalf("blocked: %d %v", code, m)
	}
	// A snapshot is read-only, so it cannot change type either.
	s := c.create(nil)
	sadm := map[string]string{"X-Admin": s.admin, "X-Write": s.write}
	_, fr, _ := c.do("POST", "/api/n/"+s.token+"/freeze", nil, sadm)
	if code, m, _ := c.do("POST", "/api/n/"+fr["token"].(string)+"/convert", map[string]any{"to": "basic", "content": basicBody, "version": 1}, sadm); code == 200 {
		t.Fatalf("snapshot converted: %d %v", code, m)
	}
}

// No note, basic or encrypted, may be indexed: the header, the meta tag, a
// robots.txt that keeps crawlers off the machine paths, and no title or text in a preview.
func TestNotesAreNeverIndexed(t *testing.T) {
	srv := newTestApp(t)
	c := newClient(t, srv)
	b := basic(c, map[string]any{"slug": "rahasia-umum", "content": `{"t":"Judul Pribadi","b":"isi pribadi"}`})
	e := c.create(nil)

	get := func(path, ua string) (*http.Response, string) {
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		if ua != "" {
			req.Header.Set("User-Agent", ua)
		}
		res, err := c.hc.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		return res, string(raw)
	}
	for _, path := range []string{"/" + b.token, "/" + e.token, "/n/" + e.token, "/raw/" + b.token, "/api/n/" + b.token, "/api/n/" + e.token, "/mock/" + b.token, "/nope-nope-nope"} {
		res, _ := get(path, "")
		if tag := res.Header.Get("X-Robots-Tag"); !strings.Contains(tag, "noindex") || !strings.Contains(tag, "noarchive") {
			t.Errorf("%s: X-Robots-Tag %q", path, tag)
		}
	}
	for _, path := range []string{"/" + b.token, "/" + e.token} {
		if _, body := get(path, ""); !strings.Contains(body, `name="robots" content="noindex, nofollow"`) {
			t.Errorf("%s: no robots meta tag", path)
		}
	}
	// Site pages stay indexable.
	for _, path := range []string{"/", "/about", "/faq"} {
		if res, _ := get(path, ""); strings.Contains(res.Header.Get("X-Robots-Tag"), "noindex") {
			t.Errorf("%s should be indexable", path)
		}
	}
	// A link preview carries neither the title nor the text of a basic note.
	for _, ua := range []string{"WhatsApp/2.23", "Twitterbot/1.0", "facebookexternalhit/1.1", "Googlebot/2.1"} {
		_, body := get("/"+b.token, ua)
		if strings.Contains(body, "Judul Pribadi") || strings.Contains(body, "isi pribadi") {
			t.Errorf("%s: preview exposes the note", ua)
		}
	}
	res, robots := get("/robots.txt", "")
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/plain") {
		t.Fatalf("robots.txt: %d %q", res.StatusCode, res.Header.Get("Content-Type"))
	}
	for _, want := range []string{"User-agent: *", "Disallow: /api/", "Disallow: /raw/", "Disallow: /mock/", "Disallow: /in/"} {
		if !strings.Contains(robots, want) {
			t.Errorf("robots.txt lacks %q", want)
		}
	}
	if strings.Contains(robots, "Sitemap") {
		t.Error("robots.txt points to a sitemap")
	}
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}
