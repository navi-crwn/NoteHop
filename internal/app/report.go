package app

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// "Report this note": a reader tells the site operator a note is illegal,
// abusive, spam or infringing. The report carries the link token and the
// reader's words, never the note's content (the server can't read an
// encrypted note anyway). Reports are reviewed through the control center
// (/hop/reports) and limited per address so the queue can't be flooded.

// settingReportKey names the secret behind reportHash. It is not in settingsList.
const settingReportKey = "report_key"

const (
	reportsPerHour = 5
	reportMaxNote  = 500
)

// A note's key and owner token live in the link's #fragment (#k=…, #a=…). A
// reporter who pastes the full link would hand them to whoever reads the queue,
// and the control center forwards report text on. The report already carries
// the link token, so the key part is dropped before the text is stored. The
// separators may be percent-encoded (%23k=, %23k%3D, also encoded twice), and a
// pasted key may stand alone (k=…), but "task=…" is not one.
var keyFragment = regexp.MustCompile(`(?i)(^|[^a-z0-9_]|%(?:25)*(?:23|26|3f|3b))(k|a)(?:=|%(?:25)*3d)[A-Za-z0-9_%-]{8,}`)

func scrubKeyFragments(s string) string { return keyFragment.ReplaceAllString(s, "${1}${2}=[removed]") }

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
	q.Note = scrubKeyFragments(strings.TrimSpace(q.Note))
	if !reportReasons[q.Reason] || len([]rune(q.Note)) > reportMaxNote {
		apiErr(w, http.StatusBadRequest, "err.bad_report")
		return
	}
	ipHash := a.reportHash(a.clientIP(r))
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

// reportHash is what a report stores of the reporter's address: enough to count
// reports per address, not enough to find the address. The IPv4 space is small,
// so a plain hash could be reversed by trying every address; this one is keyed
// with a secret made on first use and kept in the database's settings table,
// which the control center neither lists nor changes.
func (a *App) reportHash(ip string) string {
	a.reportKeyOnce.Do(func() {
		if v, ok := a.db.Setting(settingReportKey); ok {
			if b, err := hex.DecodeString(v); err == nil && len(b) == 32 {
				a.reportKey = b
				return
			}
		}
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		if err := a.db.SetSetting(settingReportKey, hex.EncodeToString(b)); err != nil {
			log.Printf("report: keeping the address key in memory only: %v", err)
		}
		a.reportKey = b
	})
	m := hmac.New(sha256.New, a.reportKey)
	m.Write([]byte("report/" + ip))
	return hex.EncodeToString(m.Sum(nil))[:32]
}
