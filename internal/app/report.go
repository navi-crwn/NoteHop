package app

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// "Report this note": a reader tells the site operator a note is illegal,
// abusive, spam or infringing. The report carries the link token and the
// reader's words, never the note's content (the server can't read an
// encrypted note anyway). Reports are reviewed through the control center
// (/hop/reports) and limited per address so the queue can't be flooded.

const (
	reportsPerHour = 5
	reportMaxNote  = 500
)

var reportReasons = map[string]bool{"illegal": true, "abuse": true, "spam": true, "copyright": true, "other": true}

func (a *App) apiReport(w http.ResponseWriter, r *http.Request) {
	n, l, err := a.db.NoteByLink(r.PathValue("token"))
	if err != nil {
		apiErr(w, http.StatusNotFound, "err.note_missing")
		return
	}
	var q struct {
		Reason string `json:"reason"`
		Note   string `json:"note"`
	}
	if !readJSON(w, r, &q, 4096) {
		return
	}
	q.Reason = strings.ToLower(strings.TrimSpace(q.Reason))
	q.Note = strings.TrimSpace(q.Note)
	if !reportReasons[q.Reason] || len([]rune(q.Note)) > reportMaxNote {
		apiErr(w, http.StatusBadRequest, "err.bad_report")
		return
	}
	ipHash := hashHex("report/" + a.clientIP(r))[:32]
	if k, err := a.db.ReportsFrom(ipHash, time.Now().Add(-time.Hour).Unix()); err != nil || k >= reportsPerHour {
		w.Header().Set("Retry-After", "3600")
		apiErr(w, http.StatusTooManyRequests, "err.report_limit")
		return
	}
	rep := &db.Report{NoteID: n.ID, Token: l.Token, Reason: q.Reason, Note: q.Note, IPHash: ipHash}
	if err := a.db.AddReport(rep); err != nil {
		log.Printf("report: %v", err)
		apiErr(w, http.StatusInternalServerError, "err.report_failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": rep.ID})
}
