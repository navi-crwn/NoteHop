package app

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// A note is either basic or encrypted.
//
//   - Basic: the server can read it. It may have a link name the owner chooses
//     and can rename (note.example/my-name), because nothing secret rides in the link.
//   - Encrypted: locked in the browser; the key is in the link after #k= and
//     never reaches the server. It keeps the long link; no short name.
//
// Only the owner (the holder of the owner link) can change a note from one to the
// other, in either direction. The browser does the encrypting and decrypting
// and sends the new content; the server only swaps it in.

// apiRenameLink gives one link of a basic note a new name.
func (a *App) apiRenameLink(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok || !a.requireAdmin(w, x) {
		return
	}
	var q struct {
		Slug string `json:"slug"`
	}
	if !readJSON(w, r, &q, 512) {
		return
	}
	from := r.PathValue("link")
	to := strings.ToLower(strings.TrimSpace(q.Slug))
	switch {
	case x.note.Encrypted:
		apiErr(w, http.StatusBadRequest, "err.slug_basic_only")
		return
	case to == "" || !slugRe.MatchString(to):
		apiErr(w, http.StatusBadRequest, "err.bad_slug")
		return
	case to == from:
		apiErr(w, http.StatusBadRequest, "err.rename_same")
		return
	}
	if key, kv := a.slugProblem(r, to); key != "" {
		apiErr(w, http.StatusBadRequest, key, kv...)
		return
	}
	switch err := a.db.RenameLink(x.note.ID, from, to); {
	case errors.Is(err, db.ErrTaken):
		apiErr(w, http.StatusConflict, "err.slug_taken")
	case errors.Is(err, db.ErrNotFound):
		apiErr(w, http.StatusNotFound, "err.link_missing")
	case err != nil:
		log.Printf("rename link: %v", err)
		apiErr(w, http.StatusInternalServerError, "err.link_failed")
	default:
		a.db.ReleaseSlug(to) // its owner took it back
		links, _ := a.db.Links(x.note.ID)
		for _, l := range links {
			if l.Token == to {
				writeJSON(w, http.StatusOK, l)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// apiConvert swaps a note between basic and encrypted.
func (a *App) apiConvert(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok || !a.requireAdmin(w, x) {
		return
	}
	var q struct {
		To      string `json:"to"` // "basic" or "encrypted"
		Content string `json:"content"`
		Version int64  `json:"version"`
	}
	if !readJSON(w, r, &q, maxContent+16<<10) {
		return
	}
	var toEnc bool
	switch q.To {
	case "encrypted":
		toEnc = true
	case "basic":
	default:
		apiErr(w, http.StatusBadRequest, "err.convert_bad")
		return
	}
	switch {
	case x.note.Frozen:
		apiErr(w, http.StatusBadRequest, "err.snapshot_read_only")
		return
	case toEnc == x.note.Encrypted:
		apiErr(w, http.StatusBadRequest, "err.convert_same")
		return
	case !validContent(toEnc, q.Content, maxContent, "t", "b"):
		apiErr(w, http.StatusBadRequest, "err.bad_content")
		return
	}
	blockers, err := a.db.ConvertBlockers(x.note.ID)
	if err != nil {
		log.Printf("convert: %v", err)
		apiErr(w, http.StatusInternalServerError, "err.save_failed")
		return
	}
	if len(blockers) > 0 {
		apiErr(w, http.StatusConflict, "err.convert_blocked", "what", strings.Join(blockers, ", "))
		return
	}
	v, err := a.db.ConvertNote(x.note.ID, q.Version, toEnc, q.Content)
	switch {
	case errors.Is(err, db.ErrConflict):
		apiErr(w, http.StatusConflict, "err.conflict")
	case err != nil:
		log.Printf("convert: %v", err)
		apiErr(w, http.StatusInternalServerError, "err.save_failed")
	default:
		writeJSON(w, http.StatusOK, map[string]any{"encrypted": toEnc, "version": v})
	}
}
