package app

import (
	"net/http"
	"time"
)

// Time capsule: the server holds the ciphertext back until a set time, so
// even a reader with the full link and key can't open the note early. The
// admin link can always open it (to check or fix it).
var timeCapsule = feature{
	name: "time-capsule",
	beforeRead: func(c hookCtx) *hookErr {
		if c.cfg.OpenAt > 0 && c.now.Unix() < c.cfg.OpenAt {
			// 425 Too Early: the browser keeps counting down instead of giving up.
			return &hookErr{http.StatusTooEarly, "err.capsule_closed", []any{"time", time.Unix(c.cfg.OpenAt, 0)}}
		}
		return nil
	},
	// Writing before the opening time would let an edit link read nothing yet
	// change everything; keep it simple: no changes until it opens.
	beforeWrite: func(c hookCtx) *hookErr {
		if c.cfg.OpenAt > 0 && c.now.Unix() < c.cfg.OpenAt {
			return &hookErr{http.StatusTooEarly, "err.capsule_no_write", []any{"time", time.Unix(c.cfg.OpenAt, 0)}}
		}
		return nil
	},
	meta: func(c hookCtx, m *metaResp) {
		if c.cfg.OpenAt > 0 && c.now.Unix() < c.cfg.OpenAt {
			m.OpenAt = c.cfg.OpenAt
		}
	},
}
