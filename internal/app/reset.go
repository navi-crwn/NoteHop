package app

import (
	"context"
	"log"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// Recurring checklists: ticks made through checklist links are cleared every
// day, week or month (a daily routine, a weekly shopping list). Ticks live
// outside the encrypted text, so the server can reset them without reading
// anything. Boxes ticked inside the text itself (by editing) stay.
var recurringReset = feature{
	name: "recurring-reset",
	meta: func(c hookCtx, m *metaResp) { m.Reset, m.ResetAt = c.cfg.Reset, c.cfg.ResetAt },
	every: func(ctx context.Context, d *db.DB, now time.Time) error {
		notes, err := d.NotesWithConfig("reset_at")
		if err != nil {
			return err
		}
		for _, n := range notes {
			cfg := parseConfig(n.Config)
			if cfg.Reset == "" || cfg.ResetAt == 0 || cfg.ResetAt > now.Unix() || n.Frozen {
				continue
			}
			if err := d.ResetChecks(n.ID); err != nil {
				return err
			}
			cfg.ResetAt = nextReset(cfg.Reset, time.Unix(cfg.ResetAt, 0))
			for cfg.ResetAt <= now.Unix() { // the server was down for a while
				cfg.ResetAt = nextReset(cfg.Reset, time.Unix(cfg.ResetAt, 0))
			}
			if err := d.SetConfig(n.ID, cfg.String()); err != nil {
				return err
			}
			log.Printf("reset checklist of note %d", n.ID)
		}
		return nil
	},
}

func nextReset(every string, from time.Time) int64 {
	switch every {
	case "weekly":
		return from.AddDate(0, 0, 7).Unix()
	case "monthly":
		return from.AddDate(0, 1, 0).Unix()
	}
	return from.AddDate(0, 0, 1).Unix()
}
