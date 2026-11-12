package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/internal/db"
)

func seoGet(t *testing.T, srv *httptest.Server, path, ua string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", srv.URL+path, nil)
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res, string(b)
}

// A note is never indexable, whatever the kind of note and the kind of response: the page, the
// password or code prompt, the old link, raw, mock, the API, an attachment, the frames, errors.
func TestEveryNoteResponseIsNoindex(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(Config{Registration: true, FileMB: 1, FilesTotalMB: 2}, d)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() { srv.Close(); d.Close() })
	c := newClient(t, srv)
	basicN := basic(c, map[string]any{"slug": "seo-basic", "content": `{"t":"Judul","b":"isi"}`})
	enc := c.create(nil)
	pw := c.create(map[string]any{"password": true, "salt": key()})
	burn := c.create(map[string]any{"max_views": 1})
	code, up := upload(t, srv, bytes.Repeat([]byte{7}, 1000))
	if code != 201 {
		t.Fatalf("upload: %d", code)
	}
	paths := []string{"/api/files/" + up["id"].(string), "/api/files/AAAAAAAAAAAAAAAAAAAA",
		"/render/mermaid", "/run", "/in/nobody", "/gate/nobody", "/@helena", "/hop/x", "/auth/sso"}
	for _, n := range []note{enc, pw, burn} {
		paths = append(paths, "/"+n.token, "/n/"+n.token, "/raw/"+n.token, "/mock/"+n.token, "/mock/"+n.token+"/a.json", "/api/n/"+n.token)
	}
	paths = append(paths, "/"+basicN.token, "/seo-basic", "/n/"+basicN.token, "/raw/"+basicN.token, "/mock/"+basicN.token, "/api/n/"+basicN.token,
		"/nope-nope-nope", "/n/nope-nope-nope", "/raw/nope-nope-nope", "/mock/nope-nope-nope", "/api/n/nope-nope-nope")
	for _, ua := range []string{"", "Googlebot/2.1", "WhatsApp/2.23"} {
		for _, p := range paths {
			res, _ := seoGet(t, srv, p, ua)
			if tag := res.Header.Get("X-Robots-Tag"); tag != "noindex, nofollow, noarchive" {
				t.Errorf("ua %q %s (%d): X-Robots-Tag %q", ua, p, res.StatusCode, tag)
			}
		}
	}
	// HTML note pages and error pages also say it in the page itself.
	for _, base := range []string{"/" + enc.token, "/" + pw.token, "/" + basicN.token, "/n/" + enc.token, "/nope-nope-nope", "/login", "/register"} {
		for _, p := range []string{base, base + "?lang=de"} {
			res, body := seoGet(t, srv, p, "")
			if tag := res.Header.Get("X-Robots-Tag"); tag != "noindex, nofollow, noarchive" {
				t.Errorf("%s: X-Robots-Tag %q", p, tag)
			}
			if !strings.Contains(body, `name="robots" content="noindex, nofollow"`) || strings.Contains(body, `rel="canonical"`) || strings.Contains(body, "hreflang") || strings.Contains(body, "og:url") {
				t.Errorf("%s: want robots meta and no canonical, og:url or hreflang", p)
			}
		}
	}
}

// Public pages follow the family rule (KONTRAK-SEO-BAHASA): the canonical is the configured base URL plus the
// path, plus ?lang=<code> only when the request carried a supported one; og:url equals it; ten alternates.
func TestPublicPagesCanonicalAndShareTags(t *testing.T) {
	d, _ := db.Open(":memory:")
	a, err := New(Config{BaseURL: "https://note.example.org"}, d)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() { srv.Close(); d.Close() })
	suffix := map[string]string{"": "", "?lang=de": "?lang=de", "?lang=de&utm_source=x": "?lang=de", "?utm_source=x&lang=ja": "?lang=ja",
		"?lang=pt": "?lang=pt-BR", "?lang=en": "?lang=en", "?lang=xx": "", "?lang=ar": "", "?utm_source=x": ""}
	codes := []string{"en", "zh-CN", "de", "id", "ja", "pt-BR", "es", "fr", "ru"}
	titles := map[string]string{}
	for _, p := range indexablePages {
		for q, sfx := range suffix {
			for _, host := range []string{"", "other.example.net"} {
				req, _ := http.NewRequest("GET", srv.URL+p+q, nil)
				req.Header.Set("User-Agent", "Googlebot/2.1")
				if host != "" {
					req.Host = host
				}
				res, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				raw, _ := io.ReadAll(res.Body)
				res.Body.Close()
				body := string(raw)
				if res.StatusCode != 200 {
					t.Errorf("%s%s: %d", p, q, res.StatusCode)
					continue
				}
				if tag := res.Header.Get("X-Robots-Tag"); tag != "" {
					t.Errorf("%s%s: indexable page carries X-Robots-Tag %q", p, q, tag)
				}
				page := "https://note.example.org" + p
				want := page + sfx
				for _, s := range []string{`<link rel="canonical" href="` + want + `">`, `<meta property="og:url" content="` + want + `">`,
					`<meta property="og:image" content="https://note.example.org/static/img/og.png">`, `<meta name="twitter:card" content="summary_large_image">`,
					`<link rel="alternate" hreflang="x-default" href="` + page + `">`} {
					if !strings.Contains(body, s) {
						t.Errorf("%s%s (host %q): lacks %s", p, q, host, s)
					}
				}
				for _, c := range codes {
					if !strings.Contains(body, `<link rel="alternate" hreflang="`+c+`" href="`+page+`?lang=`+c+`">`) {
						t.Errorf("%s%s: no hreflang %s", p, q, c)
					}
				}
				if n := strings.Count(body, `rel="alternate" hreflang=`); n != 10 || strings.Contains(body, `content="noindex`) {
					t.Errorf("%s%s: %d alternates (want 10) or noindex present", p, q, n)
				}
				if got := strings.Contains(body, `application/ld+json`); got != (p == "/") {
					t.Errorf("%s%s: JSON-LD present=%v", p, q, got)
				}
				if q == "" && host == "" {
					titles[p] = regexp.MustCompile(`<title>(.*?)</title>`).FindStringSubmatch(body)[1]
				}
			}
		}
	}
	if titles["/secret"] == titles["/new"] || titles["/embargo"] == titles["/new"] || titles["/secret"] == titles["/embargo"] {
		t.Errorf("/secret and /embargo need titles of their own: %q %q %q", titles["/secret"], titles["/embargo"], titles["/new"])
	}
	// The home page's JSON-LD parses and names the site, in an escaped data block.
	_, home := seoGet(t, srv, "/", "")
	m := regexp.MustCompile(`(?s)<script type="application/ld\+json">(.*?)</script>`).FindStringSubmatch(home)
	if m == nil {
		t.Fatal("no JSON-LD on the home page")
	}
	var ld map[string]any
	if err := json.Unmarshal([]byte(m[1]), &ld); err != nil {
		t.Fatalf("JSON-LD does not parse: %v\n%s", err, m[1])
	}
	if ld["@type"] != "WebApplication" || ld["url"] != "https://note.example.org/" || ld["image"] != "https://note.example.org/static/img/og.png" {
		t.Errorf("JSON-LD: %v", ld)
	}
}

// Account pages are noindex; the sitemap lists every public page and nothing else; robots.txt points to it.
func TestSitemapAndAccountPages(t *testing.T) {
	srv := newTestApp(t)
	for _, p := range []string{"/login", "/register", "/dashboard", "/security", "/vault", "/recover", "/offline", "/status"} {
		res, body := seoGet(t, srv, p, "")
		if tag := res.Header.Get("X-Robots-Tag"); !strings.Contains(tag, "noindex") {
			t.Errorf("%s: X-Robots-Tag %q", p, tag)
		}
		if !strings.Contains(body, `name="robots" content="noindex, nofollow"`) {
			t.Errorf("%s: no robots meta", p)
		}
	}
	res, sm := seoGet(t, srv, "/sitemap.xml", "")
	if tag := res.Header.Get("X-Robots-Tag"); tag != "" {
		t.Errorf("sitemap X-Robots-Tag %q", tag)
	}
	locs := regexp.MustCompile(`<loc>[^<]*?(/[^<]*)</loc>`).FindAllStringSubmatch(sm, -1)
	if len(locs) != len(indexablePages) {
		t.Errorf("sitemap has %d urls, want %d", len(locs), len(indexablePages))
	}
	if n := strings.Count(sm, `<xhtml:link rel="alternate" hreflang=`); n != 10*len(indexablePages) {
		t.Errorf("sitemap has %d alternates, want %d", n, 10*len(indexablePages))
	}
	if !strings.Contains(sm, `<loc>`+"http://"+strings.TrimPrefix(srv.URL, "http://")+`/secret</loc><xhtml:link rel="alternate" hreflang="en" href="`+srv.URL+`/secret?lang=en"/>`) || !strings.Contains(sm, `hreflang="x-default" href="`+srv.URL+`/secret"/>`) {
		t.Errorf("sitemap entry for /secret lacks its alternates")
	}
	for _, p := range []string{"/", "/new", "/secret", "/embargo", "/about", "/features", "/faq", "/nearby", "/diff", "/changelog", "/contact", "/terms", "/privacy"} {
		if !regexp.MustCompile(`<loc>[^<]*` + regexp.QuoteMeta(p) + `</loc>`).MatchString(sm) {
			t.Errorf("sitemap lacks %s", p)
		}
	}
	for _, p := range []string{"/dashboard", "/login", "/register", "/api", "/n/", "/raw", "/security", "/vault", "/@"} {
		if strings.Contains(sm, p) {
			t.Errorf("sitemap must not list %s", p)
		}
	}
	_, rb := seoGet(t, srv, "/robots.txt", "")
	if !strings.Contains(rb, "Sitemap: ") || !strings.Contains(rb, "Disallow: /raw/") {
		t.Errorf("robots.txt: %s", rb)
	}
}
