package app

import (
	"net/http"
	"time"
)

// Scheduled lock: from a set time on, edit/append/check links stop accepting
// changes (a sign-up list closes, a form stops taking answers). The admin
// link keeps working, so the owner can still fix things or move the time.
var scheduledLock = feature{
	name: "scheduled-lock",
	beforeWrite: func(c hookCtx) *hookErr {
		if c.cfg.LockAt > 0 && c.now.Unix() >= c.cfg.LockAt {
			return &hookErr{http.StatusForbidden, "Catatan ini sudah dikunci sejak " + indoTime(c.cfg.LockAt) + ". Sekarang hanya bisa dibaca."}
		}
		return nil
	},
	meta: func(c hookCtx, m *metaResp) {
		m.LockAt = c.cfg.LockAt
		m.Locked = c.cfg.LockAt > 0 && c.now.Unix() >= c.cfg.LockAt
	},
}

var indoMonths = [...]string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}

// indoTime formats a time for messages, in UTC since the server doesn't know
// the reader's zone (the browser shows local times itself).
func indoTime(unix int64) string {
	t := time.Unix(unix, 0).UTC()
	return t.Format("2") + " " + indoMonths[t.Month()-1] + " " + t.Format("2006 15.04") + " UTC"
}
