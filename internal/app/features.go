package app

import (
	"context"
	"encoding/json"
	"log"
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

// hookErr is a refusal shown to the user: status + Indonesian message.
type hookErr struct {
	status int
	msg    string
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
var features = []feature{scheduledLock}

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
