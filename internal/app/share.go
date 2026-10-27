package app

import (
	"net/http"
	"net/url"
	"strings"
)

// A separate share domain keeps reader links apart from the main site: if
// the short domain gets blocklisted for something a stranger shared, login,
// dashboard and editor keep working, and session cookies (host-only) never
// reach it. The share host serves notes, raw text, their API and static
// files; everything else redirects to the main site.

func (a *App) shareURL() string {
	if a.cfg.ShareURL == "" || a.cfg.BaseURL == "" {
		return ""
	}
	return a.cfg.ShareURL
}

func (a *App) shareHost() string {
	u, err := url.Parse(a.shareURL())
	if err != nil {
		return ""
	}
	return u.Host
}

func onShareHost(path string) bool {
	for _, p := range []string{"/n/", "/raw/", "/api/n/", "/static/"} {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return path == "/healthz"
}

func (a *App) shareGate(next http.Handler) http.Handler {
	host := a.shareHost()
	if host == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.Host, host) {
			if !onShareHost(r.URL.Path) {
				http.Redirect(w, r, a.cfg.BaseURL+r.URL.RequestURI(), http.StatusFound)
				return
			}
			// No accounts here, even if a cookie somehow arrived.
			r.Header.Del("Cookie")
		}
		next.ServeHTTP(w, r)
	})
}
