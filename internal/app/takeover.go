package app

import (
	"errors"
	"log"
	"net/http"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// POST /api/me/notes/{token}/takeover: the signed-in OWNER of a BASIC note gets new editing keys for it.
//
// Why it exists. A basic note's owner link (the admin key and the write key) lives in the vault blob only; the
// server keeps just their hashes. Someone without the vault open (or who reset it) could read and delete their
// basic notes but not edit them. Here the browser makes a new admin key and write key and the server swaps the two
// hashes. Older owner links of that note stop working: the page asks first and says so.
//
// What it may touch, and why that is narrow:
//   - only notes the session's account owns, and only BASIC notes (the server can read them anyway). An encrypted
//     note's keys never come near the server, so it is refused here whatever the caller says;
//   - not a frozen snapshot (read-only for everyone);
//   - for a HOP ID account the account server must have vouched for this very session recently, strictly
//     (freshlyVouched: no cooldown cookie, no "hub is down", no "sign-on is off" shortcut); the request must be
//     same-origin like every change, and there is a per-account limit.
//
// It adds no way to act on a note without being its owner, and it does not turn the session cookie into an
// admin key: later edits still send the keys the browser holds.
func (a *App) apiTakeover(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if u == nil {
		apiErr(w, http.StatusUnauthorized, "err.not_signed_in")
		return
	}
	if !a.freshlyVouched(r, u) {
		apiErr(w, http.StatusUnauthorized, "err.recheck")
		return
	}
	if !a.limit.allow("t"+u.Username, 20) {
		w.Header().Set("Retry-After", "60")
		apiErr(w, http.StatusTooManyRequests, "err.rate_limited")
		return
	}
	n, _, err := a.db.NoteByLink(r.PathValue("token"))
	if err != nil {
		if !errors.Is(err, db.ErrNotFound) {
			log.Printf("takeover lookup: %v", err)
		}
		apiErr(w, http.StatusNotFound, "err.note_missing")
		return
	}
	switch {
	case !n.OwnerID.Valid || n.OwnerID.Int64 != u.ID:
		apiErr(w, http.StatusForbidden, "err.owner_only_delete") // not this account's note: same answer as deleting
		return
	case n.Encrypted:
		apiErr(w, http.StatusBadRequest, "err.takeover_basic_only")
		return
	case n.Frozen:
		apiErr(w, http.StatusForbidden, "err.frozen")
		return
	}
	var q struct {
		Admin string `json:"admin"`
		Write string `json:"write"`
	}
	if !readJSON(w, r, &q, 512) {
		return
	}
	if !keyRe.MatchString(q.Admin) || !keyRe.MatchString(q.Write) || q.Admin == q.Write {
		apiErr(w, http.StatusBadRequest, "err.bad_data")
		return
	}
	if err := a.db.SetNoteKeys(n.ID, u.ID, hashHex(q.Admin), hashHex(q.Write)); err != nil {
		log.Printf("takeover: note %d: %v", n.ID, err)
		apiErr(w, http.StatusInternalServerError, "err.takeover_failed")
		return
	}
	// Whoever was editing live with the old keys is cut off; the owner's page reconnects with the new ones.
	a.live.closePeers(n.ID, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
