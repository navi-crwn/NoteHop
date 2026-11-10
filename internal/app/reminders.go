package app

import (
	"log"
	"net/http"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// Reminder cards are the dashboard's nudges towards a task a member has not
// done yet (today: making recovery codes). The HOP family's hop-ui.js gives
// every [data-hop-remind] card a "Hide" button that posts its key here. A
// hidden card stays away for seven days and comes back if the task is still
// open; once the task is done the card is gone for good, whatever was hidden.
// Status lines (the vault being open or locked) are not reminders: they
// cannot be hidden.

const reminderQuiet = 7 * 24 * time.Hour

// remindRecovery: the account has no unused recovery code.
const remindRecovery = "recovery-codes"

var reminderKeys = map[string]bool{remindRecovery: true}

// reminderDue reports whether a card is shown now: its task is still open and
// the member has not hidden it in the last seven days.
func (a *App) reminderDue(u *db.User, key string, taskOpen bool, now time.Time) bool {
	if !taskOpen {
		return false
	}
	at, err := a.db.ReminderDismissedAt(u.ID, key)
	if err != nil {
		log.Printf("reminder %s: %v", key, err)
		return true
	}
	return at == 0 || now.Unix()-at >= int64(reminderQuiet/time.Second)
}

// apiDismissReminder: POST /api/me/reminders/dismiss {"key": "recovery-codes"}.
func (a *App) apiDismissReminder(w http.ResponseWriter, r *http.Request) {
	u := a.verifiedUser(r)
	if u == nil {
		apiErr(w, http.StatusUnauthorized, "err.not_signed_in")
		return
	}
	var q struct {
		Key string `json:"key"`
	}
	if !readJSON(w, r, &q, 256) {
		return
	}
	if !reminderKeys[q.Key] {
		apiErr(w, http.StatusBadRequest, "err.bad_request")
		return
	}
	if err := a.db.DismissReminder(u.ID, q.Key); err != nil {
		log.Printf("dismiss reminder: %v", err)
		apiErr(w, http.StatusInternalServerError, "err.save_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "again": time.Now().Add(reminderQuiet).Unix()})
}
