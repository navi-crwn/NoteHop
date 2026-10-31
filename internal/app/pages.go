package app

import (
	"context"
	"net/http"
	"time"
)

// backPages are the inner pages that show a "Back to NoteHop" link (U-06).
var backPages = map[string]bool{
	"about": true, "features": true, "faq": true, "contact": true, "terms": true, "privacy": true,
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
