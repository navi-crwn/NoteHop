package app

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Notes live at /TOKEN (or /link-name) on the main site; the old /n/TOKEN
// redirects there. An optional short domain (NOTEHOP_SHARE_URL) only makes
// the links people hand out shorter: share.example/TOKEN redirects to the
// main site's /TOKEN, and anything else there goes to the same path on the
// main site. Nothing is served on it, so it holds no sessions or pages. The
// browser keeps the #fragment (key and rights) across both redirects.

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

// siteURL is the main site's address, for links printed outside a browser.
func (a *App) siteURL() string {
	if a.cfg.BaseURL != "" {
		return a.cfg.BaseURL
	}
	return "http://localhost"
}

// notePath is where a note lives on the main site. A link name that is also
// a site page (only admins can take those) keeps the long form.
func notePath(token string) string {
	if shortName(token) {
		return "/" + url.PathEscape(token)
	}
	return "/n/" + url.PathEscape(token)
}

// readerLink is the link handed to readers: on the short domain when there
// is one. ownerLink stays on the main site.
func (a *App) readerLink(token string) string {
	if s := a.shareURL(); s != "" && shortName(token) {
		return s + "/" + url.PathEscape(token)
	}
	return a.siteURL() + notePath(token)
}

func (a *App) ownerLink(token string) string { return a.siteURL() + notePath(token) }

// a token or link name, and not a site page (/new, /faq…: reserved names)
var shortRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{2,63}$`)

func shortName(s string) bool { return shortRe.MatchString(s) && !reservedNames[normName(s)] }

// shareGate sends everything on the short domain to the main site.
func (a *App) shareGate(next http.Handler) http.Handler {
	host := a.shareHost()
	if host == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Host, host) {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/healthz" {
			w.Write([]byte("ok"))
			return
		}
		p := r.URL.EscapedPath()
		if t, ok := strings.CutPrefix(r.URL.Path, "/n/"); ok && shortName(t) {
			p = "/" + url.PathEscape(t)
		}
		code := http.StatusMovedPermanently
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			code = http.StatusPermanentRedirect // keeps the method and body
		}
		http.Redirect(w, r, a.cfg.BaseURL+p+query(r), code)
	})
}

// oldNoteLink moves /n/TOKEN to /TOKEN; the browser keeps the #fragment.
func (a *App) oldNoteLink(w http.ResponseWriter, r *http.Request) {
	tok := r.PathValue("token")
	if !shortName(tok) {
		a.notePage(w, r, tok)
		return
	}
	http.Redirect(w, r, "/"+url.PathEscape(tok)+query(r), http.StatusMovedPermanently)
}

func query(r *http.Request) string {
	if r.URL.RawQuery == "" {
		return ""
	}
	return "?" + r.URL.RawQuery
}
