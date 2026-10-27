package app

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// Optional per-note features live in one JSON column (notes.config) and plug
// into the request path through hooks, so adding one is a new field in
// NoteConfig plus a feature value; notes.go doesn't change.
//
//   beforeRead  runs before content is handed out (time capsule, ...)
//   beforeWrite runs before any change by a non-admin link (scheduled lock, ...)
//   afterUpdate runs after content changed (webhooks, history, ...)
//   meta        copies what the browser needs to show into the metadata
//   every       runs from the janitor once a minute (cleanup, timed resets, ...)
//
// Every field here is metadata the server can see. Nothing that depends on the
// note's plaintext belongs here: the server never has it.

type NoteConfig struct {
	LockAt int64 `json:"lock_at,omitempty"` // unix time; after it, only the admin can change the note
	OpenAt int64 `json:"open_at,omitempty"` // unix time; before it, only the admin can read the note
}

func parseConfig(s string) NoteConfig {
	var c NoteConfig
	if s != "" && s != "{}" {
		if err := json.Unmarshal([]byte(s), &c); err != nil {
			log.Printf("note config: %v", err)
		}
	}
	return c
}

func (c NoteConfig) String() string {
	b, _ := json.Marshal(c)
	return string(b)
}

// hookErr is a refusal shown to the user: status, catalog key and the
// placeholder values for it (see apiErr).
type hookErr struct {
	status int
	key    string
	kv     []any
}

type hookCtx struct {
	note *db.Note
	cfg  NoteConfig
	now  time.Time
}

type feature struct {
	name        string
	beforeRead  func(hookCtx) *hookErr
	beforeWrite func(hookCtx) *hookErr
	afterUpdate func(hookCtx)
	meta        func(hookCtx, *metaResp)
	every       func(context.Context, *db.DB, time.Time) error
}

// features is the registry; order matters only for which refusal wins.
var features = []feature{timeCapsule, scheduledLock}

func runBeforeRead(n *db.Note, now time.Time) *hookErr {
	c := hookCtx{note: n, cfg: parseConfig(n.Config), now: now}
	for _, f := range features {
		if f.beforeRead != nil {
			if e := f.beforeRead(c); e != nil {
				return e
			}
		}
	}
	return nil
}

func runBeforeWrite(n *db.Note, now time.Time) *hookErr {
	c := hookCtx{note: n, cfg: parseConfig(n.Config), now: now}
	for _, f := range features {
		if f.beforeWrite != nil {
			if e := f.beforeWrite(c); e != nil {
				return e
			}
		}
	}
	return nil
}

func runAfterUpdate(n *db.Note, now time.Time) {
	c := hookCtx{note: n, cfg: parseConfig(n.Config), now: now}
	for _, f := range features {
		if f.afterUpdate != nil {
			f.afterUpdate(c)
		}
	}
}

func fillMeta(n *db.Note, m *metaResp, now time.Time) {
	c := hookCtx{note: n, cfg: parseConfig(n.Config), now: now}
	for _, f := range features {
		if f.meta != nil {
			f.meta(c, m)
		}
	}
}

// runJobs is the background worker's per-minute pass over feature jobs.
func (a *App) runJobs(ctx context.Context, now time.Time) {
	for _, f := range features {
		if f.every == nil {
			continue
		}
		if err := f.every(ctx, a.db, now); err != nil {
			log.Printf("job %s: %v", f.name, err)
		}
	}
}

// apiConfig lets the admin change a note's optional features.
func (a *App) apiConfig(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok || !a.requireAdmin(w, x) {
		return
	}
	if x.note.Frozen {
		apiErr(w, http.StatusForbidden, "err.frozen")
		return
	}
	var q struct {
		LockAt *int64 `json:"lock_at"`
		OpenAt *int64 `json:"open_at"`
	}
	if !readJSON(w, r, &q, 1024) {
		return
	}
	cfg, old := parseConfig(x.note.Config), parseConfig(x.note.Config)
	if q.LockAt != nil {
		cfg.LockAt = *q.LockAt
	}
	if q.OpenAt != nil {
		cfg.OpenAt = *q.OpenAt
	}
	// Times already in effect (a lock that has passed) aren't re-validated
	// as "must be in the future"; only the ones being set now are.
	check := cfg
	if q.LockAt == nil && old.LockAt <= time.Now().Unix() {
		check.LockAt = 0
	}
	if q.OpenAt == nil && old.OpenAt <= time.Now().Unix() {
		check.OpenAt = 0
	}
	if msg := checkTimes(check, x.note.Expires, time.Now().Unix()); msg != "" {
		apiErr(w, http.StatusBadRequest, msg)
		return
	}
	x.note.Config = cfg.String()
	if err := a.db.SetConfig(x.note.ID, x.note.Config); err != nil {
		apiErr(w, http.StatusInternalServerError, "err.config_failed")
		return
	}
	writeJSON(w, http.StatusOK, a.meta(r, x))
}

// checkTimes keeps timed features from contradicting each other or the
// note's expiry. Zero means "not set".
func checkTimes(c NoteConfig, expires, now int64) string {
	inRange := func(t int64) bool { return t > now && time.Duration(t-now)*time.Second <= maxExpiry }
	switch {
	case c.OpenAt != 0 && !inRange(c.OpenAt):
		return "err.open_at_range"
	case c.LockAt != 0 && !inRange(c.LockAt):
		return "err.lock_at_range"
	case c.OpenAt != 0 && c.LockAt != 0 && c.LockAt <= c.OpenAt:
		return "err.lock_before_open"
	case c.OpenAt != 0 && expires != 0 && expires <= c.OpenAt:
		return "err.expires_before_open"
	case c.LockAt != 0 && expires != 0 && expires <= c.LockAt:
		return "err.expires_before_lock"
	}
	return ""
}
