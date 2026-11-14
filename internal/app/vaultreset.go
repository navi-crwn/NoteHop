package app

import (
	"log"
	"net/http"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// Reset vault: for a HOP ID account that forgot its vault phrase and has no recovery code. NoteHop never held
// the vault key, so the only way to a new phrase is to give up what the old one sealed: every note of the
// account (with its attachments and share links), its margin notes and its recovery codes. The page says all
// of that and asks for a word to be typed; this file is the server half: who may, what is checked, what is
// written down. The deleting itself is one transaction in db.ResetVault.
//
// Who may: a signed-in HOP ID account whose session the account server vouched for in the last
// few minutes, strictly (resetGate). The vault phrase cannot be asked for (that is what was forgotten), so this is
// the proof of ownership available today. Asking the account server for a fresh sign-in first (prompt=login or
// max_age) is not supported by the hub yet; see the report.

// resetWord is typed to confirm. One fixed word on every screen and in every language, checked here.
const resetWord = "RESET"

// resetGate answers who is asking, or has already answered. The proof is strict: the account server must have
// vouched for THIS session within recheckAfter. verifiedUser would also pass a stale session while the hub is
// unreachable, while a (forgeable) cooldown cookie is set, or when single sign-on is switched off; that leniency
// keeps pages open, but it must not unlock something that cannot be undone.
func (a *App) resetGate(w http.ResponseWriter, r *http.Request) *db.User {
	u := a.currentUser(r)
	if u == nil {
		apiErr(w, http.StatusUnauthorized, "err.not_signed_in")
		return nil
	}
	if !a.freshlyVouched(r, u) {
		apiErr(w, http.StatusUnauthorized, "err.reset_recheck")
		return nil
	}
	if u.SSOID == "" {
		// A password account's vault key comes from the password: there is no separate phrase to forget.
		apiErr(w, http.StatusBadRequest, "err.reset_hopid_only")
		return nil
	}
	// The vault phrase cannot be asked for, so the person must have just signed in at the account server for real:
	// the page sends them there (prompt=login) and back. A hub that cannot stamp the sign-in leaves no way through.
	if !a.freshlyLoggedIn(r, u) {
		apiErr(w, http.StatusUnauthorized, "err.reset_login")
		return nil
	}
	return u
}

// apiResetPreview: GET /api/me/vault/reset says what a reset would delete, for the confirmation screen.
func (a *App) apiResetPreview(w http.ResponseWriter, r *http.Request) {
	u := a.resetGate(w, r)
	if u == nil {
		return
	}
	c, err := a.db.VaultResetCounts(u.ID)
	if err != nil {
		log.Printf("vault reset preview: %v", err)
		apiErr(w, http.StatusInternalServerError, "err.reset_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"word": resetWord, "counts": c})
}

// apiResetVault: POST /api/me/vault/reset {"confirm": "RESET"}.
func (a *App) apiResetVault(w http.ResponseWriter, r *http.Request) {
	u := a.resetGate(w, r)
	if u == nil {
		return
	}
	// A few tries a minute per account, on top of the per-address limit.
	if !a.limit.allow("v"+u.Username, 5) {
		w.Header().Set("Retry-After", "60")
		apiErr(w, http.StatusTooManyRequests, "err.rate_limited")
		return
	}
	var q struct {
		Confirm string `json:"confirm"`
	}
	if !readJSON(w, r, &q, 256) {
		return
	}
	if q.Confirm != resetWord {
		apiErr(w, http.StatusBadRequest, "err.reset_confirm")
		return
	}
	keep := ""
	if c, err := r.Cookie(sessionCookie); err == nil {
		keep = hashHex(c.Value)
	}
	c, err := a.db.ResetVault(u.ID, keep, &db.Audit{Actor: "user:" + hopUserID(u), App: "notehop", Action: "user.vault_reset_self", Target: hopUserID(u)})
	if err != nil {
		log.Printf("vault reset: account %d: %v", u.ID, err)
		apiErr(w, http.StatusInternalServerError, "err.reset_failed")
		return
	}
	a.dropGoneRooms()
	// The account's own record of it was written inside the reset's transaction, next to what the control center
	// writes. No key or phrase is ever received here, so there is none to leave out of the log.
	log.Printf("vault reset: account %d, %d notes, %d files", u.ID, c.Notes, c.Files)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "counts": c, "next": "/vault"})
}
