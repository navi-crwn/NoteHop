// Package app is NoteHop's HTTP layer: HTML pages, the JSON API and the raw
// endpoint. All note content arrives already encrypted (unless the author
// chose a public note), so handlers deal in opaque strings plus permissions.
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
	"github.com/navi-crwn/NoteHop/web"
)

type Config struct {
	Site          string
	BaseURL       string
	SecureCookies bool
	TrustProxy    bool
	Registration  bool
	SSOURL     string // "Masuk dengan SSO"; empty turns it off
	SSOSecret  string // shared with SSO's NOTEHOP_SSO_SECRET
}

// SSOEnabled reports whether sign-in with SSO is configured. It needs the
// public base URL too, because SSO only sends tickets back to an exact
// callback address.
func (c Config) SSOEnabled() bool {
	return c.SSOURL != "" && len(c.SSOSecret) >= 32 && c.BaseURL != ""
}

type App struct {
	cfg   Config
	db    *db.DB
	pages map[string]*template.Template
	asset string
	limit *limiter
	mux   *http.ServeMux
	used  usedTickets
}

func New(cfg Config, d *db.DB) (*App, error) {
	if cfg.Site == "" {
		cfg.Site = "NoteHop"
	}
	a := &App{cfg: cfg, db: d, limit: newLimiter(), used: usedTickets{m: map[string]int64{}}}
	if err := a.loadTemplates(); err != nil {
		return nil, err
	}
	a.asset = assetVersion()
	a.routes()
	return a, nil
}

func (a *App) Handler() http.Handler { return a.secure(a.mux) }

// Janitor deletes expired notes and sessions until ctx ends.
func (a *App) Janitor(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		if n, err := a.db.Sweep(); err != nil {
			log.Printf("janitor: %v", err)
		} else if n > 0 {
			log.Printf("janitor: removed %d expired note(s)", n)
		}
		a.limit.gc()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (a *App) routes() {
	m := http.NewServeMux()
	static, _ := fs.Sub(web.Static, "static")
	files := http.FileServer(http.FS(static))
	m.Handle("GET /static/", http.StripPrefix("/static/", cacheForever(files)))
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })

	m.HandleFunc("GET /{$}", a.page("new", "", "Catatan baru"))
	m.HandleFunc("GET /about", a.page("about", "Cara kerja", "Bagaimana NoteHop menjaga catatanmu tetap privat"))
	m.HandleFunc("GET /login", a.page("login", "Masuk", ""))
	m.HandleFunc("GET /register", a.page("register", "Daftar", ""))
	m.HandleFunc("GET /dashboard", a.dashboardPage)
	m.HandleFunc("GET /vault", a.vaultPage)
	m.HandleFunc("GET /auth/sso", a.ssoStart)
	m.HandleFunc("GET /auth/sso/callback", a.ssoCallback)
	m.HandleFunc("GET /n/{token}", a.notePage)
	m.HandleFunc("GET /raw/{token}", a.raw)

	m.HandleFunc("POST /api/notes", a.apiCreate)
	m.HandleFunc("GET /api/n/{token}", a.apiMeta)
	m.HandleFunc("POST /api/n/{token}/open", a.apiOpen)
	m.HandleFunc("PUT /api/n/{token}", a.apiSave)
	m.HandleFunc("DELETE /api/n/{token}", a.apiDelete)
	m.HandleFunc("POST /api/n/{token}/entries", a.apiAddEntry)
	m.HandleFunc("DELETE /api/n/{token}/entries/{id}", a.apiDeleteEntry)
	m.HandleFunc("POST /api/n/{token}/checks", a.apiCheck)
	m.HandleFunc("POST /api/n/{token}/freeze", a.apiFreeze)
	m.HandleFunc("POST /api/n/{token}/links", a.apiAddLink)
	m.HandleFunc("DELETE /api/n/{token}/links/{link}", a.apiDeleteLink)

	m.HandleFunc("POST /api/register", a.apiRegister)
	m.HandleFunc("POST /api/login", a.apiLogin)
	m.HandleFunc("POST /api/logout", a.apiLogout)
	m.HandleFunc("GET /api/me", a.apiMe)
	m.HandleFunc("GET /api/me/notes", a.apiMyNotes)
	m.HandleFunc("POST /api/vault", a.apiVault)

	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { a.notFound(w, r) })
	a.mux = m
}

// ---------- pages ----------

type pageData struct {
	Site    string
	Title   string
	Desc    string
	Path    string
	Asset   string
	User    *db.User
	Reg     bool
	NoIndex bool
	Token   string
	Mode    string
	SSO     bool   // show "Masuk dengan SSO"
	Next    string // where to go after signing in
	Msg     string // error page text; empty means "note not found"
	Origin  string // scheme://host for absolute OpenGraph URLs
}

func (a *App) loadTemplates() error {
	a.pages = map[string]*template.Template{}
	for _, name := range []string{"new", "note", "about", "login", "register", "dashboard", "vault", "error"} {
		t, err := template.New(name).ParseFS(web.Templates, "templates/base.html", "templates/icons.html", "templates/"+name+".html")
		if err != nil {
			return err
		}
		a.pages[name] = t
	}
	return nil
}

func (a *App) render(w http.ResponseWriter, r *http.Request, status int, name string, d pageData) {
	d.Site, d.Path, d.Asset, d.Reg = a.cfg.Site, r.URL.Path, a.asset, a.cfg.Registration
	d.SSO = a.cfg.SSOEnabled()
	d.Origin = a.origin(r)
	if d.Next == "" {
		d.Next = safeNext(r.URL.Query().Get("next"))
	}
	if d.User == nil {
		d.User = a.currentUser(r)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := a.pages[name].ExecuteTemplate(w, "base", d); err != nil {
		log.Printf("render %s: %v", name, err)
	}
}

func (a *App) page(name, title, desc string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.render(w, r, http.StatusOK, name, pageData{Title: title, Desc: desc})
	}
}

func (a *App) dashboardPage(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login?next=/dashboard", http.StatusSeeOther)
		return
	}
	a.render(w, r, http.StatusOK, "dashboard", pageData{Title: "Catatanku", User: u, NoIndex: true})
}

func (a *App) notePage(w http.ResponseWriter, r *http.Request) {
	tok := r.PathValue("token")
	n, l, err := a.db.NoteByLink(tok)
	if err != nil {
		a.notFound(w, r)
		return
	}
	title, desc := previewText(n)
	// Chat apps only want the preview card; they get it without the app.
	if isPreviewBot(r.UserAgent()) {
		a.preview(w, r, title, desc)
		return
	}
	// The page is only a shell: content is fetched (and, for burn notes,
	// counted) by script after the reader confirms, so link previews from
	// chat apps never use up a view.
	a.render(w, r, http.StatusOK, "note", pageData{Title: title, Desc: desc, Token: tok, Mode: l.Mode, NoIndex: true})
}

// origin is the public scheme://host, for absolute URLs in OpenGraph tags.
func (a *App) origin(r *http.Request) string {
	if a.cfg.BaseURL != "" {
		return a.cfg.BaseURL
	}
	scheme := "http"
	if r.TLS != nil || (a.cfg.TrustProxy && r.Header.Get("X-Forwarded-Proto") == "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (a *App) notFound(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		apiErr(w, http.StatusNotFound, "Tidak ditemukan.")
		return
	}
	a.render(w, r, http.StatusNotFound, "error", pageData{Title: "Tidak ditemukan", NoIndex: true})
}

func cacheForever(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=300")
		}
		h.ServeHTTP(w, r)
	})
}

// assetVersion fingerprints the embedded static files for cache busting.
func assetVersion() string {
	h := sha256.New()
	_ = fs.WalkDir(web.Static, "static", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := fs.ReadFile(web.Static, p)
			h.Write([]byte(p))
			h.Write(b)
		}
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))[:10]
}
