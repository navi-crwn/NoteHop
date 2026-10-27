// Package app is NoteHop's HTTP layer: HTML pages, the JSON API and the raw
// endpoint. All note content arrives already encrypted (unless the author
// chose a public note), so handlers deal in opaque strings plus permissions.
package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
	"github.com/navi-crwn/NoteHop/internal/i18n"
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
	ShareURL      string // optional second domain for share links; needs BaseURL
	FileMB        int    // largest attachment in MB; 0 turns attachments off
	FilesTotalMB  int    // disk budget for all live attachments; 0 = no limit
	PrivateHooks  bool   // let Watch webhooks reach private/loopback addresses (and plain http)
	TelegramToken string // bot token from @BotFather; empty turns the bot off
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
	live  liveHub
	near  nearbyHub
	watch watcher
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

func (a *App) Handler() http.Handler { return a.secure(a.shareGate(a.mux)) }

// Janitor is the background worker: once a minute it deletes expired notes
// and sessions and runs the jobs of note features, until ctx ends.
func (a *App) Janitor(ctx context.Context) {
	go a.runWatch(ctx)
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		if n, err := a.db.Sweep(); err != nil {
			log.Printf("janitor: %v", err)
		} else if n > 0 {
			log.Printf("janitor: removed %d expired note(s)", n)
		}
		a.limit.gc()
		a.runJobs(ctx, time.Now())
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

	m.HandleFunc("GET /{$}", a.page("new", "", "meta.desc_new"))
	m.HandleFunc("GET /about", a.page("about", "title.about", "meta.desc_about"))
	m.HandleFunc("GET /login", a.page("login", "title.login", ""))
	m.HandleFunc("GET /register", a.page("register", "title.register", ""))
	m.HandleFunc("GET /diff", a.page("diff", "diffpage.title", "diffpage.lead"))
	m.HandleFunc("GET /nearby", a.page("nearby", "nearby.title", "nearby.lead"))
	m.HandleFunc("GET /offline", a.page("offline", "offline.title", "offline.lead"))
	m.HandleFunc("GET /sw.js", a.serviceWorker)
	m.HandleFunc("GET /manifest.webmanifest", a.manifest)
	m.HandleFunc("GET /api/nearby", a.apiNearby)
	m.HandleFunc("GET /i18n/{lang}", a.catalogJS)
	m.HandleFunc("GET /render/mermaid", mermaidFrame)
	m.HandleFunc("GET /dashboard", a.dashboardPage)
	m.HandleFunc("GET /vault", a.vaultPage)
	m.HandleFunc("GET /auth/sso", a.ssoStart)
	m.HandleFunc("GET /auth/sso/callback", a.ssoCallback)
	m.HandleFunc("GET /n/{token}", a.notePage)
	m.HandleFunc("GET /raw/{token}", a.raw)
	m.HandleFunc("GET /mock/{token}", a.mock)
	m.HandleFunc("GET /mock/{token}/{path...}", a.mock)
	m.HandleFunc("OPTIONS /mock/{token}/{path...}", a.mock)
	m.HandleFunc("GET /{at}", a.profile)

	m.HandleFunc("POST /api/notes", a.apiCreate)
	m.HandleFunc("GET /api/n/{token}", a.apiMeta)
	m.HandleFunc("POST /api/n/{token}/open", a.apiOpen)
	m.HandleFunc("PUT /api/n/{token}", a.apiSave)
	m.HandleFunc("DELETE /api/n/{token}", a.apiDelete)
	m.HandleFunc("POST /api/n/{token}/entries", a.apiAddEntry)
	m.HandleFunc("DELETE /api/n/{token}/entries/{id}", a.apiDeleteEntry)
	m.HandleFunc("POST /api/n/{token}/checks", a.apiCheck)
	m.HandleFunc("POST /api/n/{token}/freeze", a.apiFreeze)
	m.HandleFunc("POST /api/n/{token}/config", a.apiConfig)
	m.HandleFunc("GET /api/n/{token}/versions", a.apiVersions)
	m.HandleFunc("GET /api/n/{token}/versions/{v}", a.apiVersion)
	m.HandleFunc("POST /api/n/{token}/suggestions", a.apiSuggest)
	m.HandleFunc("GET /api/n/{token}/suggestions", a.apiSuggestions)
	m.HandleFunc("DELETE /api/n/{token}/suggestions/{id}", a.apiDeleteSuggestion)
	m.HandleFunc("POST /api/n/{token}/signatures", a.apiSign)
	m.HandleFunc("POST /api/n/{token}/votes", a.apiVote)
	m.HandleFunc("POST /api/n/{token}/claim", a.apiClaim)
	m.HandleFunc("PUT /api/n/{token}/decoy", a.apiSetDecoy)
	m.HandleFunc("DELETE /api/n/{token}/decoy", a.apiDeleteDecoy)
	m.HandleFunc("POST /api/n/{token}/burns", a.apiAddBurn)
	m.HandleFunc("POST /api/n/{token}/burns/{id}/reveal", a.apiRevealBurn)
	m.HandleFunc("GET /api/n/{token}/live", a.apiLive)
	m.HandleFunc("POST /api/n/{token}/inbox", a.apiInboxOn)
	m.HandleFunc("DELETE /api/n/{token}/inbox", a.apiInboxOff)
	m.HandleFunc("POST /in/{inbox}", a.apiInbox)
	m.HandleFunc("POST /api/telegram/{secret}", a.apiTelegram)
	m.HandleFunc("POST /api/files", a.apiUpload)
	m.HandleFunc("GET /api/files/{id}", a.apiFile)
	m.HandleFunc("GET /api/margins/{ref}", a.apiMargin)
	m.HandleFunc("PUT /api/margins/{ref}", a.apiMargin)
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
	Main    string // main site, for admin links
	Share   string // share domain, for links handed to readers (may be empty)
	L       i18n.Locale
	Langs   []i18n.Lang
	Clipper template.URL // bookmarklet for the about page
	Profile []profileNote
}

func (a *App) loadTemplates() error {
	a.pages = map[string]*template.Template{}
	for _, name := range []string{"new", "note", "about", "login", "register", "dashboard", "vault", "diff", "nearby", "profile", "offline", "error"} {
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
	d.L, d.Langs = a.locale(w, r), i18n.Langs
	d.SSO = a.cfg.SSOEnabled()
	d.Origin = a.origin(r)
	d.Main, d.Share = a.cfg.BaseURL, a.shareURL()
	d.Clipper = clipper(d.Origin)
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

// page serves a static page; title and desc are catalog keys.
func (a *App) page(name, title, desc string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := a.locale(w, r)
		d := pageData{}
		if title != "" {
			d.Title = l.T(title)
		}
		if desc != "" {
			d.Desc = l.T(desc)
		}
		a.render(w, r, http.StatusOK, name, d)
	}
}

func (a *App) dashboardPage(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login?next=/dashboard", http.StatusSeeOther)
		return
	}
	a.render(w, r, http.StatusOK, "dashboard", pageData{Title: a.locale(w, r).T("title.dashboard"), User: u, NoIndex: true})
}

func (a *App) notePage(w http.ResponseWriter, r *http.Request) {
	tok := r.PathValue("token")
	n, l, err := a.db.NoteByLink(tok)
	if err != nil {
		a.notFound(w, r)
		return
	}
	title, desc := previewText(n, a.locale(w, r))
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
		apiErr(w, http.StatusNotFound, "err.not_found")
		return
	}
	a.render(w, r, http.StatusNotFound, "error", pageData{Title: a.locale(w, r).T("title.not_found"), NoIndex: true})
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

// assetVersion fingerprints the embedded static files and catalogs for
// cache busting.
func assetVersion() string {
	h := sha256.New()
	for _, src := range []struct {
		fsys fs.FS
		dir  string
	}{{web.Static, "static"}, {web.I18n, "i18n"}} {
		_ = fs.WalkDir(src.fsys, src.dir, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				b, _ := fs.ReadFile(src.fsys, p)
				h.Write([]byte(p))
				h.Write(b)
			}
			return nil
		})
	}
	return hex.EncodeToString(h.Sum(nil))[:10]
}

// ---------- language ----------

const langCookie = "nh_lang"

// locale is the reader's language: ?lang= (remembered in a cookie), then the
// cookie, then English. Browsers' Accept-Language is deliberately ignored so
// the site is the same for everyone until they choose.
func (a *App) locale(w http.ResponseWriter, r *http.Request) i18n.Locale {
	if q := r.URL.Query().Get("lang"); i18n.Supported(q) {
		http.SetCookie(w, &http.Cookie{Name: langCookie, Value: q, Path: "/", MaxAge: 365 * 24 * 3600,
			Secure: a.cfg.SecureCookies, SameSite: http.SameSiteLaxMode})
		return i18n.Locale{Lang: i18n.Get(q)}
	}
	code := i18n.Default
	if c, err := r.Cookie(langCookie); err == nil && i18n.Supported(c.Value) {
		code = c.Value
	}
	return i18n.Locale{Lang: i18n.Get(code)}
}

// catalogJS hands the browser scripts their text: GET /i18n/en.js sets
// window.NH_I18N. A script file (not inline) because of the CSP.
func (a *App) catalogJS(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSuffix(r.PathValue("lang"), ".js")
	if !i18n.Supported(code) {
		a.notFound(w, r)
		return
	}
	b, _ := json.Marshal(i18n.Catalog(code))
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	if r.URL.RawQuery != "" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	fmt.Fprintf(w, "window.NH_I18N=%s;window.NH_LANG=%q;\n", b, code)
}

// clipper is the web clipper bookmarklet: it opens a new note with the page
// title, address and selected text in the #fragment, which browsers never
// send to the server.
func clipper(origin string) template.URL {
	js := `javascript:(function(){var d={t:document.title,u:location.href,s:String(getSelection())};` +
		`window.open(` + strconv.Quote(origin+"/#clip=") + `+encodeURIComponent(JSON.stringify(d)))})()`
	return template.URL(js)
}

// mermaidFrame is the sandboxed page that draws diagrams. It gets its own
// CSP: Mermaid needs inline styles, which the note page forbids. It's framed
// with sandbox="allow-scripts" (an opaque origin), so it can't touch the note
// page, cookies or storage, and it has no network access beyond our scripts.
func mermaidFrame(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'unsafe-inline'; img-src data:; font-src 'self'; frame-ancestors 'self'")
	h.Set("X-Frame-Options", "SAMEORIGIN")
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "public, max-age=3600")
	io.WriteString(w, `<!doctype html><meta charset="utf-8"><title>Mermaid</title>`+
		`<script src="/static/vendor/mermaid.min.js"></script><script src="/static/js/mermaid-frame.js"></script>`)
}

// serviceWorker serves the worker from the site root (so it may control every
// page) with this build's asset version, which renames its cache.
func (a *App) serviceWorker(w http.ResponseWriter, r *http.Request) {
	b, err := fs.ReadFile(web.Static, "static/js/sw.js")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(bytes.ReplaceAll(b, []byte("__VERSION__"), []byte(a.asset)))
}

func (a *App) manifest(w http.ResponseWriter, r *http.Request) {
	b, err := fs.ReadFile(web.Static, "static/manifest.webmanifest")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(b)
}
