package app

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// A separate share domain keeps reader links apart from the main site: if
// the short domain gets blocklisted for something a stranger shared, login,
// dashboard and editor keep working, and session cookies (host-only) never
// reach it. The share host serves notes, raw text, their API and what a
// note page loads (static files, translations, the sandboxed frames);
// everything else redirects to the main site.
//
// Reader links there are short: share.example/TOKEN. The old long form
// share.example/n/TOKEN redirects to it, and the main site's /n/TOKEN keeps
// working (the browser moves readers over, see note.js: who is a reader is
// only known from the #fragment, which never reaches the server).

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

// readerLink is the link handed to readers (no owner key): short on the
// share domain, /n/TOKEN on the main site otherwise.
func (a *App) readerLink(token string) string {
	if s := a.shareURL(); s != "" {
		return s + "/" + url.PathEscape(token)
	}
	return a.siteURL() + "/n/" + url.PathEscape(token)
}

// siteURL is the main site's address, for links printed outside a browser.
func (a *App) siteURL() string {
	if a.cfg.BaseURL != "" {
		return a.cfg.BaseURL
	}
	return "http://localhost"
}

var (
	sharePrefixes = []string{"/n/", "/raw/", "/mock/", "/api/n/", "/api/files", "/api/margins/", "/static/", "/i18n/"}
	shareExact    = map[string]bool{"/healthz": true, "/run": true, "/render/mermaid": true, "/sw.js": true, "/manifest.webmanifest": true, "/offline": true}
	// a link name or token as the whole path: share.example/TOKEN
	shortRe = regexp.MustCompile(`^/[A-Za-z0-9][A-Za-z0-9-]{2,63}$`)
)

func onShareHost(path string) bool {
	if shareExact[path] {
		return true
	}
	for _, p := range sharePrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func (a *App) shareGate(next http.Handler) http.Handler {
	host := a.shareHost()
	if host == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.Host, host) {
			p := r.URL.Path
			switch {
			case strings.HasPrefix(p, "/n/") && shortRe.MatchString(p[2:]) && !reservedNames[normName(p[3:])] && (r.Method == http.MethodGet || r.Method == http.MethodHead):
				// The long form moves to the short one; the browser keeps the #fragment.
				http.Redirect(w, r, p[2:]+query(r), http.StatusMovedPermanently)
				return
			case !onShareHost(p) && shortRe.MatchString(p) && !reservedNames[normName(p[1:])] && (r.Method == http.MethodGet || r.Method == http.MethodHead):
				// share.example/TOKEN is the note; site pages (/new, /faq…) are reserved names and go to the main site.
				r.URL.Path, r.URL.RawPath = "/n"+p, ""
			case !onShareHost(p):
				http.Redirect(w, r, a.cfg.BaseURL+r.URL.RequestURI(), http.StatusFound)
				return
			}
			// No accounts here, even if a cookie somehow arrived.
			r.Header.Del("Cookie")
		}
		next.ServeHTTP(w, r)
	})
}

func query(r *http.Request) string {
	if r.URL.RawQuery == "" {
		return ""
	}
	return "?" + r.URL.RawQuery
}
