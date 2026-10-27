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
			return &hookErr{http.StatusForbidden, "err.locked", []any{"time", time.Unix(c.cfg.LockAt, 0)}}
		}
		return nil
	},
	meta: func(c hookCtx, m *metaResp) {
		m.LockAt = c.cfg.LockAt
		m.Locked = c.cfg.LockAt > 0 && c.now.Unix() >= c.cfg.LockAt
	},
}
