package app

import (
	"context"
	"io"
	"net/http"
	"time"
)

// backPages are the inner pages that show a "Back to NoteHop" link (U-06).
var backPages = map[string]bool{
	"changelog": true,
	"about":     true, "features": true, "faq": true, "contact": true, "terms": true, "privacy": true,
	"status": true, "diff": true, "nearby": true, "security": true, "recover": true, "offline": true,
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

// robots keeps crawlers out of the machine paths. Notes themselves are not
// listed here on purpose: a crawler that may not fetch a note address never
// sees its "noindex" header, and an address linked from elsewhere could then
// still be listed. Every note response carries X-Robots-Tag and a robots meta
// tag instead, and no note is in any sitemap or public list.
func (a *App) robots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	io.WriteString(w, "User-agent: *\nDisallow: /api/\nDisallow: /raw/\nDisallow: /mock/\nDisallow: /in/\nDisallow: /gate/\nDisallow: /n/\nDisallow: /auth/\nDisallow: /hop/\nAllow: /\n")
}
