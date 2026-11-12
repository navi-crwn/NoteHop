package app

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/navi-crwn/NoteHop/internal/i18n"
)

// backPages are the inner pages that show a "Back to NoteHop" link (U-06).
var backPages = map[string]bool{
	"changelog": true,
	"about":     true, "features": true, "faq": true, "contact": true, "terms": true, "privacy": true,
	"status": true, "diff": true, "nearby": true, "security": true, "recover": true, "offline": true,
}

// dashPages get the signed-in dashboard chrome (sidebar with this app's menu
// and the HOP family, settings-type top bar) when someone is signed in.
var dashPages = map[string]bool{"dashboard": true, "vault": true, "new": true, "security": true}

// miniFootPages are the pages people work on: one line of footer instead of
// the five columns, which stay on the pages that present NoteHop.
var miniFootPages = map[string]bool{
	"new": true, "note": true, "diff": true, "nearby": true, "login": true, "register": true,
	"vault": true, "security": true, "recover": true, "profile": true, "offline": true, "error": true,
}

// liveStatus is what /status shows: answered by this server, right now.
type liveStatus struct {
	Version string
	Since   string
	DBOK    bool
}

var processStart = time.Now()

func (a *App) statusPage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	l := a.locale(w, r)
	s := &liveStatus{Version: a.cfg.Version, Since: processStart.UTC().Format("2006-01-02 15:04 UTC"), DBOK: a.db.PingContext(ctx) == nil}
	if s.Version == "" {
		s.Version = a.asset
	}
	w.Header().Set("Cache-Control", "no-store")
	code := http.StatusOK
	if !s.DBOK {
		code = http.StatusServiceUnavailable
	}
	a.render(w, r, code, "status", pageData{Title: l.T("status.title"), Status: s})
}

// indexablePages are the only addresses a search engine may list: the pages
// that present NoteHop. The same list is the sitemap, decides which pages get a
// canonical link, and is the exception to the "noindex" every other response
// carries. A note (/TOKEN, /link-name, /n/, /raw/, /mock/), a profile, an
// account page, a file, the API and any route added later are not in it.
var indexablePages = []string{"/", "/new", "/secret", "/embargo", "/about", "/features", "/faq", "/nearby", "/diff", "/changelog", "/contact", "/terms", "/privacy"}

var indexablePage = func() map[string]bool {
	m := make(map[string]bool, len(indexablePages))
	for _, p := range indexablePages {
		m[p] = true
	}
	return m
}()

// noIndex is the X-Robots-Tag of everything that is not an indexable page.
const noIndex = "noindex, nofollow, noarchive"

// crawlable reports whether a path is answered without X-Robots-Tag: an
// indexable page, a file such a page is built from (styles, scripts, images,
// text catalogs) or one of the files crawlers ask for by name.
func crawlable(p string) bool {
	switch p {
	case "/robots.txt", "/sitemap.xml", "/manifest.webmanifest", "/sw.js":
		return true
	}
	return indexablePage[p] || strings.HasPrefix(p, "/static/") || strings.HasPrefix(p, "/i18n/")
}

// canonical is the one address a search engine should list for an indexable page: the public base
// URL and the path, never the query string. Language comes from a cookie (or ?lang=, which only sets
// that cookie), so the nine languages share one address and the pages carry no hreflang.
func (a *App) canonical(r *http.Request) string {
	return strings.TrimRight(a.origin(r), "/") + r.URL.Path
}

// webApp is the home page's structured data (schema.org WebApplication). It is
// a JSON data block, not a script: the content security policy has no say over it.
func (a *App) webApp(r *http.Request, l i18n.Locale) map[string]any {
	return map[string]any{
		"@context":            "https://schema.org",
		"@type":               "WebApplication",
		"name":                a.cfg.Site,
		"url":                 strings.TrimRight(a.origin(r), "/") + "/",
		"image":               strings.TrimRight(a.origin(r), "/") + "/static/img/og.png",
		"inLanguage":          []string{"en", "zh-CN", "de", "id", "ja", "pt-BR", "es", "fr", "ru"},
		"description":         l.T("meta.desc_new"),
		"applicationCategory": "UtilitiesApplication",
		"operatingSystem":     "Any",
		"offers":              map[string]any{"@type": "Offer", "price": "0", "priceCurrency": "USD"},
		"publisher":           map[string]any{"@type": "Organization", "name": "HEL.ink", "url": "https://hel.ink"},
	}
}

// sitemap lists the indexable pages, each once. No note, profile or link name
// is ever listed.
func (a *App) sitemap(w http.ResponseWriter, r *http.Request) {
	base := strings.TrimRight(a.origin(r), "/")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+"\n"+`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`+"\n")
	for _, p := range indexablePages {
		io.WriteString(w, "<url><loc>"+base+p+"</loc></url>\n")
	}
	io.WriteString(w, "</urlset>\n")
}

// robots keeps crawlers out of the machine paths. Notes themselves are not
// listed here on purpose: a crawler that may not fetch a note address never
// sees its "noindex" header, and an address linked from elsewhere could then
// still be listed. Every note response carries X-Robots-Tag and a robots meta
// tag instead, and no note is in the sitemap (static pages only) or any public list.
func (a *App) robots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	io.WriteString(w, "User-agent: *\nDisallow: /api/\nDisallow: /raw/\nDisallow: /mock/\nDisallow: /in/\nDisallow: /gate/\nDisallow: /n/\nDisallow: /auth/\nDisallow: /hop/\nAllow: /\nSitemap: "+strings.TrimRight(a.origin(r), "/")+"/sitemap.xml\n")
}
