package app

import (
	"errors"
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

// GET /api/v1/me/notes: a member's notes that another HOP app may link to
// from a public page, for the note picker in BioHop's editor. Only a HOP app
// named in NOTEHOP_NOTES_CLIENTS may call it, server to server, signed with
// the HOP App Protocol (its own HOP_SSO_SECRET_<APP>, or the SSO secret) and
// with X-HOP-Actor set to the member's HOP ID. The browser never calls it.
//
// What is listed, and why:
//   - public (not encrypted, no password) notes only. An encrypted note opens
//     only with the key in the URL fragment, and that key never reaches this
//     server; a link without it would show visitors nothing, so encrypted
//     notes are left out rather than offered keyless.
//   - only through a read-only link: a note shared only with an edit, append,
//     check or suggest link is left out, because putting that link on a
//     public page would hand those rights to every visitor.
//   - no content warning, no burn after reading (max_views), not expired,
//     and nothing a feature holds back before reading (runBeforeRead): the
//     same rules as the public profile page.
//
// Reply: {ok:true, data:{items:[{id, title, url, updated}], next}}. id is the
// read link's token, url its public address. ?q= matches words of the title,
// ?limit= 1..50 (20), ?cursor= the previous page's next. Errors use the
// /hop/* envelope: not_configured (404), bad_signature, stale_request,
// replay (401), no_actor (400), no_account (404), account_suspended (403).

const (
	notesPageLimit = 20
	notesPageMax   = 50
	notesScanMax   = 400 // notes read from the database per call before filtering
)

func (a *App) apiMyNotesForApp(w http.ResponseWriter, r *http.Request) {
	u := a.notesCaller(w, r)
	if u == nil {
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = notesPageLimit
	}
	limit = min(limit, notesPageMax)
	cursor, _ := strconv.ParseInt(q.Get("cursor"), 10, 64)
	search := strings.ToLower(strings.TrimSpace(clipRunes(q.Get("q"), 100)))

	items := []map[string]any{}
	next := ""
	now := time.Now()
	scanned := 0
	for len(items) <= limit && scanned < notesScanMax {
		rows, err := a.db.ListNotes(db.NoteFilter{OwnerID: u.ID, Status: "public", Title: search, Cursor: cursor, Limit: 100})
		if err != nil {
			log.Printf("me/notes: %v", err)
			hopFail(w, http.StatusInternalServerError, "server_error", "something went wrong on the server")
			return
		}
		for i := range rows {
			n := &rows[i]
			cursor = n.ID
			scanned++
			if !pickable(n, now) {
				continue
			}
			title := publicTitle(n)
			if search != "" && !strings.Contains(strings.ToLower(title), search) {
				continue
			}
			token := a.readToken(n.ID)
			if token == "" {
				continue
			}
			if len(items) == limit {
				next = strconv.FormatInt(prevID(items), 10)
				break
			}
			items = append(items, map[string]any{"id": token, "title": title, "url": a.noteURL(token), "updated": iso(n.Updated), "_id": n.ID})
		}
		if next != "" || len(rows) < 100 {
			break
		}
	}
	for _, it := range items {
		delete(it, "_id")
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": map[string]any{"items": items, "next": next}})
}

// prevID is the note id of the last item listed, the cursor for the next page.
func prevID(items []map[string]any) int64 {
	id, _ := items[len(items)-1]["_id"].(int64)
	return id
}

// pickable reports whether a note may be put on a public page by link.
func pickable(n *db.Note, now time.Time) bool {
	if n.Encrypted || n.Password || n.HasWarning() || n.MaxViews > 0 {
		return false
	}
	if n.Expires > 0 && n.Expires <= now.Unix() {
		return false
	}
	return runBeforeRead(n, now, "") == nil
}

// readToken is the note's first read-only link, or "" when it has none.
func (a *App) readToken(noteID int64) string {
	links, err := a.db.Links(noteID)
	if err != nil {
		return ""
	}
	for _, l := range links {
		if l.Mode == modeRead {
			return l.Token
		}
	}
	return ""
}

// notesCaller verifies the signed call and returns the member it acts for;
// nil means an error has been written.
func (a *App) notesCaller(w http.ResponseWriter, r *http.Request) *db.User {
	if !a.cfg.SSOEnabled() {
		hopFail(w, http.StatusNotFound, "not_configured", "single sign-on is not configured on this server")
		return nil
	}
	app := r.Header.Get("X-HOP-App")
	if !slices.Contains(a.cfg.NotesClients, app) {
		hopFail(w, http.StatusUnauthorized, "bad_signature", "unknown calling app")
		return nil
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, hopMaxBody))
	if err != nil {
		hopFail(w, http.StatusRequestEntityTooLarge, "too_large", "the request body is too large")
		return nil
	}
	if err := a.nonces.Verify(r, body, app, a.notesSecrets(app), hop.MaxAge); err != nil {
		hopFail(w, http.StatusUnauthorized, err.Error(), "the request signature was refused")
		return nil
	}
	actor := clip(r.Header.Get("X-HOP-Actor"), 100)
	if actor == "" || actor == "system" || strings.HasPrefix(actor, "local:") {
		hopFail(w, http.StatusBadRequest, "no_actor", "X-HOP-Actor must be the member's HOP ID")
		return nil
	}
	u, err := a.db.UserBySSO(actor)
	switch {
	case errors.Is(err, db.ErrNotFound):
		hopFail(w, http.StatusNotFound, "no_account", "this member has not signed in to NoteHop yet")
		return nil
	case err != nil:
		log.Printf("me/notes user: %v", err)
		hopFail(w, http.StatusInternalServerError, "server_error", "something went wrong on the server")
		return nil
	case u.Status != "" && u.Status != "active":
		hopFail(w, http.StatusForbidden, "account_suspended", "this account is suspended")
		return nil
	}
	return u
}

// notesSecrets verify a notes client: its own HOP_SSO_SECRET_<APP> when the
// family keeps one secret per app, else the SSO secret.
func (a *App) notesSecrets(app string) []string {
	if own, ok := a.cfg.NotesSecrets[app]; ok && len(own) >= 32 {
		return []string{own}
	}
	return []string{a.cfg.SSOSecret}
}
