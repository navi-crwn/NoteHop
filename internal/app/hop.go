package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
	"github.com/navi-crwn/NoteHop/internal/hop"
)

// The HOP App Protocol: /hop/* is how the control center reads and acts on
// this app (stats, users, content, reports, settings) and how the account
// server pushes account events (profile changes, suspensions, deletions).
// Every call is signed with the SSO secret by one of the apps in
// NOTEHOP_HOP_CLIENTS; there are no cookies and no CSRF here (middleware.go).
//
// Zero-knowledge holds on this side too: for an encrypted note the control
// center gets metadata and encrypted:true, never content; a title or preview
// is only ever filled for public notes.
//
// Replies: {ok:true, data, page?} or {ok:false, error, message}. Times are
// ISO-8601 UTC. Users are identified by their account-server id (sub) plus
// the local id. Everything that changes data is written to hop_audit.

const (
	hopMaxBody   = 1 << 20
	hopPageLimit = 50
	hopPageMax   = 200
)

// hopHandler gets the already-read and verified request body.
type hopHandler func(w http.ResponseWriter, r *http.Request, body []byte)

func (a *App) hopRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /hop/health", a.hop(a.hopHealth))
	m.HandleFunc("GET /hop/manifest", a.hop(a.hopManifest))
	m.HandleFunc("GET /hop/stats", a.hop(a.hopStats))
	m.HandleFunc("GET /hop/users", a.hop(a.hopUsers))
	m.HandleFunc("GET /hop/users/{id}", a.hop(a.hopUser))
	m.HandleFunc("POST /hop/users/{id}/actions", a.hop(a.hopUserAction))
	m.HandleFunc("POST /hop/profile", a.hop(a.hopProfile))
	m.HandleFunc("POST /hop/events", a.hop(a.hopEvent))
	m.HandleFunc("GET /hop/content", a.hop(a.hopContent))
	m.HandleFunc("GET /hop/content/note/{id}", a.hop(a.hopNote))
	m.HandleFunc("POST /hop/content/note/{id}/actions", a.hop(a.hopNoteAction))
	m.HandleFunc("GET /hop/content/file/{id}", a.hop(a.hopFile))
	m.HandleFunc("POST /hop/content/file/{id}/actions", a.hop(a.hopFileAction))
	m.HandleFunc("GET /hop/reports", a.hop(a.hopReports))
	m.HandleFunc("POST /hop/reports/{id}/actions", a.hop(a.hopReportAction))
	m.HandleFunc("GET /hop/settings", a.hop(a.hopSettings))
	m.HandleFunc("PUT /hop/settings", a.hop(a.hopPutSettings))
	m.HandleFunc("GET /hop/search", a.hop(a.hopSearch))
	m.HandleFunc("GET /hop/metrics", a.hop(a.hopMetrics))
	m.HandleFunc("GET /hop/audit", a.hop(a.hopAudit))
	m.HandleFunc("GET /hop/x/names", a.hop(a.hopNames))
	m.HandleFunc("POST /hop/x/names/holds/{slug}/actions", a.hop(a.hopHoldAction))
	m.HandleFunc("GET /hop/x/files", a.hop(a.hopFiles))
	m.HandleFunc("GET /hop/x/realtime", a.hop(a.hopRealtime))
	m.HandleFunc("GET /hop/x/integrations", a.hop(a.hopIntegrations))
	m.HandleFunc("/hop/", a.hop(func(w http.ResponseWriter, r *http.Request, _ []byte) {
		hopFail(w, http.StatusNotFound, "not_found", "no such endpoint")
	}))
}

// hop checks the signature before anything else runs. 404 not_configured
// while single sign-on is off, 401 for a bad, stale or replayed signature.
func (a *App) hop(fn hopHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.cfg.SSOEnabled() {
			hopFail(w, http.StatusNotFound, "not_configured", "single sign-on is not configured on this server")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, hopMaxBody))
		if err != nil {
			hopFail(w, http.StatusRequestEntityTooLarge, "too_large", "the request body is too large")
			return
		}
		app := r.Header.Get("X-HOP-App")
		if !slices.Contains(a.cfg.HopClients, app) {
			hopFail(w, http.StatusUnauthorized, "bad_signature", "unknown calling app")
			return
		}
		if err := a.nonces.Verify(r, body, app, []string{a.cfg.SSOSecret}, hop.MaxAge); err != nil {
			hopFail(w, http.StatusUnauthorized, err.Error(), "the request signature was refused")
			return
		}
		fn(w, r, body)
	}
}

// ---------- replies ----------

type hopPage struct {
	Next  string `json:"next,omitempty"`
	Total int    `json:"total,omitempty"`
}

func hopOK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": data})
}

func hopList(w http.ResponseWriter, data any, next string, total int) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": data, "page": hopPage{Next: next, Total: total}})
}

func hopFail(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": code, "message": msg})
}

// hopBody decodes a JSON body; an empty body is an empty object.
func hopBody(w http.ResponseWriter, body []byte, v any) bool {
	if len(body) == 0 {
		return true
	}
	if err := json.Unmarshal(body, v); err != nil {
		hopFail(w, http.StatusBadRequest, "bad_request", "the body is not valid JSON")
		return false
	}
	return true
}

func hopServerErr(w http.ResponseWriter, what string, err error) {
	log.Printf("hop %s: %v", what, err)
	hopFail(w, http.StatusInternalServerError, "server_error", "something went wrong on the server")
}

// iso formats unix seconds as ISO-8601 UTC; zero becomes null.
func iso(t int64) any {
	if t == 0 {
		return nil
	}
	return time.Unix(t, 0).UTC().Format(time.RFC3339)
}

// parseISO reads an ISO-8601 time; "" or null (an empty string here) is 0.
func parseISO(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0, err
	}
	return t.Unix(), nil
}

// hopPaging reads ?cursor and ?limit.
func hopPaging(r *http.Request) (cursor int64, limit int) {
	cursor, _ = strconv.ParseInt(r.URL.Query().Get("cursor"), 10, 64)
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = hopPageLimit
	}
	if limit > hopPageMax {
		limit = hopPageMax
	}
	return cursor, limit
}

// audit records what the caller did. detail must be JSON-friendly.
func (a *App) audit(r *http.Request, action, target string, detail map[string]any) {
	if detail == nil {
		detail = map[string]any{}
	}
	b, _ := json.Marshal(detail)
	if err := a.db.AddAudit(db.Audit{Actor: clip(r.Header.Get("X-HOP-Actor"), 120), App: r.Header.Get("X-HOP-App"),
		Action: action, Target: target, Detail: string(b)}); err != nil {
		log.Printf("hop audit: %v", err)
	}
}

// actionReq is the body of every .../actions call.
type actionReq struct {
	Action string         `json:"action"`
	Reason string         `json:"reason"`
	Params map[string]any `json:"params"`
}

func (q actionReq) str(key string) string {
	s, _ := q.Params[key].(string)
	return s
}

// ---------- health, manifest, stats, metrics ----------

func (a *App) hopHealth(w http.ResponseWriter, r *http.Request, _ []byte) {
	dbOK := "ok"
	var checks []map[string]any
	if err := a.db.Ping(); err != nil {
		dbOK = "error"
		checks = append(checks, map[string]any{"name": "db", "ok": false, "detail": err.Error()})
	} else {
		checks = append(checks, map[string]any{"name": "db", "ok": true, "detail": "sqlite"})
	}
	sso := "off"
	if a.cfg.SSOEnabled() {
		sso = "ok"
	}
	checks = append(checks, map[string]any{"name": "sso", "ok": sso == "ok", "detail": a.cfg.SSOURL})
	checks = append(checks, map[string]any{"name": "files", "ok": true, "detail": fmt.Sprintf("%d MB per file", a.cfg.FileMB)})
	hopOK(w, map[string]any{
		"app": "notehop", "version": a.cfg.Version, "uptime_s": int64(time.Since(a.started).Seconds()),
		"db": dbOK, "storage": "ok", "sso": sso, "checks": checks,
	})
}

func (a *App) hopManifest(w http.ResponseWriter, r *http.Request, _ []byte) {
	hopOK(w, map[string]any{
		"app": "notehop", "name": a.cfg.Site, "icon": "note", "base_url": a.cfg.BaseURL,
		"capabilities": []string{"admin_api", "events", "metrics", "zero_knowledge"},
		"content_types": []map[string]any{
			{"type": "note", "label": "Notes", "actions": []string{"delete", "takedown", "freeze", "set_expiry", "release_slug", "disable_inbox", "disable_watch"}},
			{"type": "file", "label": "Attachments", "actions": []string{"delete"}},
		},
		"extensions":      []string{"names", "files", "realtime", "integrations"},
		"settings_schema": a.settingsList(),
		"actions": map[string]any{
			"user":   []string{"revoke_sessions", "delete_notes", "purge", "set_role", "reset_vault"},
			"report": []string{"dismiss", "resolve", "takedown"},
		},
	})
}

func (a *App) hopStats(w http.ResponseWriter, r *http.Request, _ []byte) {
	s, err := a.db.Stats()
	if err != nil {
		hopServerErr(w, "stats", err)
		return
	}
	since := time.Now().UTC().AddDate(0, 0, -13)
	since = time.Date(since.Year(), since.Month(), since.Day(), 0, 0, 0, 0, time.UTC)
	days, err := a.db.NotesPerDay(since.Unix())
	if err != nil {
		hopServerErr(w, "stats", err)
		return
	}
	// Every one of the 14 days is present, zero when nothing happened.
	byDay := map[string]int64{}
	for _, d := range days {
		byDay[d.Day] = d.N
	}
	series := make([][2]any, 0, 14)
	for i := 0; i < 14; i++ {
		day := since.AddDate(0, 0, i).Format("2006-01-02")
		series = append(series, [2]any{day, byDay[day]})
	}
	hopOK(w, map[string]any{
		"kpi": map[string]any{
			"users": s.Users, "users_sso": s.UsersSSO, "notes": s.Notes, "notes_encrypted": s.NotesEncrypted, "notes_public": s.NotesPublic,
			"notes_owned": s.NotesOwned, "notes_burn": s.NotesBurn, "files": s.Files, "files_bytes": s.FilesBytes,
			"files_budget_bytes": int64(a.cfg.FilesTotalMB) << 20, "live_rooms": a.live.count(), "slug_holds": s.SlugHolds,
			"takedowns": s.Takedowns, "reports_open": s.Reports,
		},
		"series": map[string]any{"notes_created": series},
	})
}

func (a *App) hopMetrics(w http.ResponseWriter, r *http.Request, _ []byte) {
	s, err := a.db.Stats()
	if err != nil {
		hopServerErr(w, "metrics", err)
		return
	}
	var b strings.Builder
	line := func(name, help string, v int64) {
		fmt.Fprintf(&b, "# HELP notehop_%s %s\n# TYPE notehop_%s gauge\nnotehop_%s %d\n", name, help, name, name, v)
	}
	line("users", "Accounts.", s.Users)
	line("users_sso", "Accounts that sign in through the account server.", s.UsersSSO)
	line("notes", "Notes stored.", s.Notes)
	line("notes_encrypted", "Notes the server cannot read.", s.NotesEncrypted)
	line("notes_public", "Public notes.", s.NotesPublic)
	line("files", "Live attachments.", s.Files)
	line("files_bytes", "Bytes of live attachments.", s.FilesBytes)
	line("live_rooms", "Open live-collaboration rooms.", int64(a.live.count()))
	line("slug_holds", "Link names on hold.", s.SlugHolds)
	line("reports_open", "Open reader reports.", s.Reports)
	line("uptime_seconds", "Seconds since start.", int64(time.Since(a.started).Seconds()))
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	io.WriteString(w, b.String())
}

// ---------- settings ----------

// Settings the control center may change at run time; everything else is
// read-only and comes from the environment.
const settingRegistration = "registration"

// loadSettings mirrors the settings table into memory at start.
func (a *App) loadSettings() {
	if v, ok := a.db.Setting(settingRegistration); ok {
		a.settings.Store(settingRegistration, v == "true")
	}
}

// registration is whether sign-ups are open: the control center's setting
// when it set one, NOTEHOP_REGISTRATION otherwise.
func (a *App) registration() bool {
	if v, ok := a.settings.Load(settingRegistration); ok {
		return v.(bool)
	}
	return a.cfg.Registration
}

type hopSetting struct {
	Key         string `json:"key"`
	Type        string `json:"type"`
	Value       any    `json:"value"`
	Default     any    `json:"default"`
	Readonly    bool   `json:"readonly"`
	Description string `json:"description"`
}

func (a *App) settingsList() []hopSetting {
	ro := func(key, typ string, v any, desc string) hopSetting {
		return hopSetting{Key: key, Type: typ, Value: v, Default: v, Readonly: true, Description: desc}
	}
	return []hopSetting{
		{Key: settingRegistration, Type: "bool", Value: a.registration(), Default: a.cfg.Registration, Description: "Whether new accounts may be made (sign-up and first sign-on)."},
		ro("file_mb", "int", a.cfg.FileMB, "Largest attachment in MB; 0 turns attachments off (NOTEHOP_FILE_MB)."),
		ro("files_total_mb", "int", a.cfg.FilesTotalMB, "Disk budget for all live attachments in MB (NOTEHOP_FILES_TOTAL_MB)."),
		ro("files_ip_mb", "int", a.cfg.FilesPerIPMB, "Attachments one address may upload per day in MB (NOTEHOP_FILES_IP_MB)."),
		ro("ssh", "bool", a.cfg.SSHAddr != "", "SSH for public notes (NOTEHOP_SSH_ADDR)."),
		ro("telegram", "bool", a.cfg.TelegramToken != "", "Telegram bot (TELEGRAM_BOT_TOKEN)."),
		ro("private_hooks", "bool", a.cfg.PrivateHooks, "Watch webhooks may reach private addresses (NOTEHOP_PRIVATE_HOOKS)."),
		ro("ai", "string", a.cfg.aiMode(), "Writing help: on, device or off (NOTEHOP_AI)."),
		ro("rate_create_per_min", "int", 30, "Notes one address may create per minute."),
		ro("rate_write_per_min", "int", 120, "Changes one address may make per minute."),
		ro("rate_upload_per_min", "int", 20, "Uploads one address may start per minute."),
		ro("rate_report_per_hour", "int", reportsPerHour, "Reports one address may send per hour."),
	}
}

func (a *App) hopSettings(w http.ResponseWriter, r *http.Request, _ []byte) {
	hopOK(w, a.settingsList())
}

func (a *App) hopPutSettings(w http.ResponseWriter, r *http.Request, body []byte) {
	var q map[string]any
	if !hopBody(w, body, &q) {
		return
	}
	for k := range q {
		if k != settingRegistration {
			hopFail(w, http.StatusBadRequest, "readonly_setting", "only "+settingRegistration+" can be changed here; the rest comes from the environment")
			return
		}
	}
	if v, ok := q[settingRegistration]; ok {
		on, isBool := v.(bool)
		if !isBool {
			hopFail(w, http.StatusBadRequest, "bad_value", settingRegistration+" must be true or false")
			return
		}
		if err := a.db.SetSetting(settingRegistration, strconv.FormatBool(on)); err != nil {
			hopServerErr(w, "settings", err)
			return
		}
		a.settings.Store(settingRegistration, on)
		a.audit(r, "settings.update", settingRegistration, map[string]any{"value": on})
	}
	hopOK(w, map[string]any{"settings": a.settingsList(), "restart_required": false})
}

// ---------- search and audit ----------

func (a *App) hopSearch(w http.ResponseWriter, r *http.Request, _ []byte) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	out := []map[string]any{}
	if q == "" {
		hopOK(w, out)
		return
	}
	// An exact link token or name.
	if n, l, err := a.db.NoteByLink(q); err == nil {
		out = append(out, map[string]any{"kind": "content", "type": "note", "id": strconv.FormatInt(n.ID, 10),
			"label": a.noteLabel(n, l.Token), "url": a.noteURL(l.Token)})
	}
	// Public notes by title (never encrypted ones: there is no title to read).
	if notes, err := a.db.PublicNotesMentioning(q, 20); err == nil {
		for _, n := range notes {
			title := publicTitle(&n)
			if !strings.Contains(strings.ToLower(title), strings.ToLower(q)) {
				continue
			}
			tok := a.db.FirstToken(n.ID)
			out = append(out, map[string]any{"kind": "content", "type": "note", "id": strconv.FormatInt(n.ID, 10), "label": title, "url": a.noteURL(tok)})
		}
	}
	if users, err := a.db.ListUsers(strings.ToLower(q), 0, 20); err == nil {
		for _, u := range users {
			out = append(out, map[string]any{"kind": "user", "id": hopUserID(&u), "label": "@" + u.Username, "url": a.cfg.BaseURL + "/@" + u.Username})
		}
	}
	hopOK(w, out)
}

func (a *App) hopAudit(w http.ResponseWriter, r *http.Request, _ []byte) {
	cursor, limit := hopPaging(r)
	rows, err := a.db.Audits(cursor, limit)
	if err != nil {
		hopServerErr(w, "audit", err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, x := range rows {
		out = append(out, map[string]any{"id": x.ID, "at": iso(x.At), "actor": x.Actor, "app": x.App, "action": x.Action, "target": x.Target, "detail": json.RawMessage(x.Detail)})
	}
	next := ""
	if len(rows) == limit {
		next = strconv.FormatInt(rows[len(rows)-1].ID, 10)
	}
	hopList(w, out, next, 0)
}

// ---------- shared helpers ----------

// hopUserID is the account-server id when there is one, else "local:<id>",
// which the user endpoints accept too.
func hopUserID(u *db.User) string {
	if u.SSOID != "" {
		return u.SSOID
	}
	return "local:" + strconv.FormatInt(u.ID, 10)
}

// hopFindUser resolves a path id: an account-server id, or "local:<id>".
func (a *App) hopFindUser(id string) (*db.User, error) {
	if n, ok := strings.CutPrefix(id, "local:"); ok {
		k, err := strconv.ParseInt(n, 10, 64)
		if err != nil {
			return nil, db.ErrNotFound
		}
		return a.db.UserByID(k)
	}
	return a.db.UserBySSO(id)
}

func (a *App) noteURL(token string) string {
	if token == "" {
		return ""
	}
	return a.cfg.BaseURL + "/n/" + token
}

// publicTitle is a public note's title; "" for an encrypted note, whose
// content is opaque here.
func publicTitle(n *db.Note) string {
	if n.Encrypted {
		return ""
	}
	var c struct{ T string }
	_ = json.Unmarshal([]byte(n.Content), &c)
	return clip(c.T, 200)
}

// publicPreview is the first 200 characters of a public note's text.
func publicPreview(n *db.Note) string {
	if n.Encrypted {
		return ""
	}
	var c struct{ B string }
	_ = json.Unmarshal([]byte(n.Content), &c)
	r := []rune(strings.TrimSpace(c.B))
	if len(r) > 200 {
		r = r[:200]
	}
	return string(r)
}

func (a *App) noteLabel(n *db.Note, token string) string {
	if t := publicTitle(n); t != "" {
		return t
	}
	return "/n/" + token
}

var errHopUnknownAction = errors.New("unknown action")
