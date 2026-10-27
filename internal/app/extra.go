package app

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// Handlers for the optional note features: history, suggestions, sign-off,
// polls, decoys, partial burn, claiming and private margins. As everywhere,
// the server stores what the browser sends and checks permissions; content
// stays ciphertext for encrypted notes.

const (
	maxSuggestions = 200
	maxSignatures  = 1000
	maxPolls       = 50
	maxPollOptions = 20
	maxMargin      = 64 << 10
)

// ---------- dead man's switch ----------

// checkIn pushes a dead man's switch forward whenever the admin shows up.
func (a *App) checkIn(x access) {
	cfg := parseConfig(x.note.Config)
	if cfg.DMSDays <= 0 {
		return
	}
	cfg.OpenAt = time.Now().Unix() + int64(cfg.DMSDays)*86400
	x.note.Config = cfg.String()
	if err := a.db.SetConfig(x.note.ID, x.note.Config); err != nil {
		log.Printf("check in: %v", err)
	}
}

// ---------- decoy ----------

// openDecoy serves the decoy when the X-Write proof matches the decoy
// password. It counts views like a real open (so burn limits behave the
// same) and records the opening for the admin. Reports whether it answered.
func (a *App) openDecoy(w http.ResponseWriter, r *http.Request, x access, poll bool) bool {
	d, err := a.db.Decoy(x.note.ID)
	if err != nil || !hashMatch(r.Header.Get("X-Write"), d.WriteHash) {
		return false
	}
	n := x.note
	if e := runBeforeRead(n, time.Now(), x.ip); e != nil {
		apiErr(w, e.status, e.key, e.kv...)
		return true
	}
	burnable := n.MaxViews > 0
	if poll && burnable {
		apiErr(w, http.StatusForbidden, "err.burn_no_poll")
		return true
	}
	if !poll {
		views, err := a.db.TakeView(n)
		if err != nil {
			apiErr(w, http.StatusNotFound, "err.burned")
			return true
		}
		n.Views = views
		if err := a.db.DecoyOpened(n.ID); err != nil {
			log.Printf("decoy opened: %v", err)
		}
	}
	m := a.meta(r, x)
	m.Mode = modeRead // whoever holds the decoy password must not find a working editor
	resp := openResp{metaResp: m, Content: d.Content, Checks: json.RawMessage("{}"), Entries: []db.Entry{}}
	if burnable && n.Views >= n.MaxViews {
		resp.Burned = true
		if err := a.db.DeleteNote(n.ID); err != nil {
			log.Printf("burn: %v", err)
		}
	}
	writeJSON(w, http.StatusOK, resp)
	return true
}

func (a *App) apiSetDecoy(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok || !a.requireAdmin(w, x) {
		return
	}
	if !x.note.Password {
		apiErr(w, http.StatusBadRequest, "err.decoy_needs_password")
		return
	}
	var q struct {
		Content string `json:"content"`
		Write   string `json:"write"`
	}
	if !readJSON(w, r, &q, maxContent+8<<10) {
		return
	}
	if !keyRe.MatchString(q.Write) || !validContent(true, q.Content, maxContent, "t", "b") || hashHex(q.Write) == x.note.WriteHash {
		apiErr(w, http.StatusBadRequest, "err.bad_decoy")
		return
	}
	if err := a.db.SetDecoy(x.note.ID, q.Content, hashHex(q.Write)); err != nil {
		apiErr(w, http.StatusInternalServerError, "err.save_failed")
		return
	}
	writeJSON(w, http.StatusOK, a.meta(r, x))
}

func (a *App) apiDeleteDecoy(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok || !a.requireAdmin(w, x) {
		return
	}
	if err := a.db.DeleteDecoy(x.note.ID); err != nil {
		apiErr(w, http.StatusInternalServerError, "err.delete_failed")
		return
	}
	writeJSON(w, http.StatusOK, a.meta(r, x))
}

// ---------- version history ----------

// History is for those who can change the note: the admin, or edit links.
func (a *App) canHistory(w http.ResponseWriter, x access) bool {
	if x.admin || (x.link.Mode == modeEdit && x.write) {
		return true
	}
	apiErr(w, http.StatusForbidden, "err.not_allowed")
	return false
}

func (a *App) apiVersions(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok || !a.canHistory(w, x) {
		return
	}
	v, err := a.db.Versions(x.note.ID)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "err.read_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": v, "current": x.note.Version})
}

func (a *App) apiVersion(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok || !a.canHistory(w, x) {
		return
	}
	ver, _ := strconv.ParseInt(r.PathValue("v"), 10, 64)
	v, err := a.db.VersionContent(x.note.ID, ver)
	if err != nil {
		apiErr(w, http.StatusNotFound, "err.not_found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// ---------- suggestions ----------

func (a *App) apiSuggest(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok || !a.writable(w, x, modeSuggest, modeEdit) {
		return
	}
	var q struct {
		Content     string `json:"content"`
		BaseVersion int64  `json:"base_version"`
	}
	if !readJSON(w, r, &q, maxContent+8<<10) {
		return
	}
	if !validContent(x.note.Encrypted, q.Content, maxContent, "t", "b") {
		apiErr(w, http.StatusBadRequest, "err.bad_content")
		return
	}
	if c, _ := a.db.CountSuggestions(x.note.ID); c >= maxSuggestions {
		apiErr(w, http.StatusForbidden, "err.full")
		return
	}
	s, err := a.db.AddSuggestion(x.note.ID, q.Content, q.BaseVersion)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "err.save_failed")
		return
	}
	runAfterUpdate(x.note, time.Now())
	writeJSON(w, http.StatusCreated, map[string]any{"id": s.ID})
}

func (a *App) apiSuggestions(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok || !a.requireAdmin(w, x) {
		return
	}
	list, err := a.db.Suggestions(x.note.ID)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "err.read_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"suggestions": list})
}

func (a *App) apiDeleteSuggestion(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok || !a.requireAdmin(w, x) {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := a.db.DeleteSuggestion(x.note.ID, id); err != nil {
		apiErr(w, http.StatusNotFound, "err.not_found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------- sign-off ----------

// apiSign records a reader's signature on the current version. Each signed
// version gets a frozen snapshot, so what was signed can't change later.
func (a *App) apiSign(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok {
		return
	}
	n := x.note
	switch {
	case !parseConfig(n.Config).SignOff:
		apiErr(w, http.StatusForbidden, "err.sign_off_off")
		return
	case n.Frozen:
		apiErr(w, http.StatusForbidden, "err.frozen")
		return
	case !x.write:
		apiErr(w, http.StatusForbidden, "err.bad_key")
		return
	}
	if !x.admin {
		if e := runBeforeWrite(n, time.Now(), x.ip); e != nil {
			apiErr(w, e.status, e.key, e.kv...)
			return
		}
	}
	var q struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &q, 4096) {
		return
	}
	if !validContent(n.Encrypted, q.Name, 2048, "n") {
		apiErr(w, http.StatusBadRequest, "err.bad_data")
		return
	}
	if list, _ := a.db.Signatures(n.ID); len(list) >= maxSignatures {
		apiErr(w, http.StatusForbidden, "err.full")
		return
	}
	snap := a.db.SnapshotOfVersion(n.ID, n.Version)
	if snap == "" {
		l, err := a.db.Freeze(n, randToken(10))
		if err != nil {
			log.Printf("sign snapshot: %v", err)
			apiErr(w, http.StatusInternalServerError, "err.freeze_failed")
			return
		}
		snap = l.Token
	}
	s := &db.Signature{Name: q.Name, Version: n.Version, Snapshot: snap}
	if err := a.db.AddSignature(n.ID, s); err != nil {
		apiErr(w, http.StatusInternalServerError, "err.save_failed")
		return
	}
	runAfterUpdate(n, time.Now())
	writeJSON(w, http.StatusCreated, s)
}

// ---------- polls ----------

func (a *App) apiVote(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok {
		return
	}
	n := x.note
	switch {
	case n.Frozen:
		apiErr(w, http.StatusForbidden, "err.frozen")
		return
	case !x.write:
		apiErr(w, http.StatusForbidden, "err.bad_key")
		return
	}
	if !x.admin {
		if e := runBeforeWrite(n, time.Now(), x.ip); e != nil {
			apiErr(w, e.status, e.key, e.kv...)
			return
		}
	}
	var q struct {
		Poll   int  `json:"poll"`
		Option int  `json:"option"`
		Prev   *int `json:"prev"`
	}
	if !readJSON(w, r, &q, 256) {
		return
	}
	prev := -1
	if q.Prev != nil {
		prev = *q.Prev
	}
	if q.Poll < 0 || q.Poll >= maxPolls || q.Option < -1 || q.Option >= maxPollOptions || prev < -1 || prev >= maxPollOptions || (q.Option < 0 && prev < 0) {
		apiErr(w, http.StatusBadRequest, "err.bad_vote")
		return
	}
	votes, err := a.db.Vote(n.ID, q.Poll, q.Option, prev)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "err.save_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"votes": votes})
}

// ---------- claiming ----------

// apiClaim puts a note made before signing in into the signed-in account.
// Needs the admin link (proof of ownership) and a vault-sealed blob.
func (a *App) apiClaim(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok || !a.requireAdmin(w, x) {
		return
	}
	u := a.currentUser(r)
	if u == nil {
		apiErr(w, http.StatusUnauthorized, "err.not_signed_in")
		return
	}
	var q struct {
		OwnerBlob string `json:"owner_blob"`
	}
	if !readJSON(w, r, &q, maxOwnerBlob+256) {
		return
	}
	if q.OwnerBlob == "" || len(q.OwnerBlob) > maxOwnerBlob {
		apiErr(w, http.StatusBadRequest, "err.owner_blob")
		return
	}
	if err := a.db.Claim(x.note.ID, u.ID, q.OwnerBlob); err != nil {
		apiErr(w, http.StatusConflict, "err.already_owned")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------- partial burn ----------

func (a *App) apiAddBurn(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok || !a.writable(w, x, modeEdit) {
		return
	}
	if !x.note.Encrypted {
		apiErr(w, http.StatusBadRequest, "err.burn_block_needs_e2e")
		return
	}
	var q burnReq
	if !readJSON(w, r, &q, 512) {
		return
	}
	if !q.valid() {
		apiErr(w, http.StatusBadRequest, "err.bad_burn_block")
		return
	}
	if list, _ := a.db.BurnKeys(x.note.ID); len(list) >= maxBurnBlocks {
		apiErr(w, http.StatusForbidden, "err.full")
		return
	}
	if err := a.db.AddBurnKey(x.note.ID, db.BurnKey{ID: q.ID, Key: q.Key, MaxViews: q.MaxViews}); err != nil {
		if errors.Is(err, db.ErrTaken) {
			apiErr(w, http.StatusConflict, "err.bad_burn_block")
			return
		}
		apiErr(w, http.StatusInternalServerError, "err.save_failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

// apiRevealBurn hands out a block key, counting a view unless it's the
// admin. Needs X-Write: only holders of the note key can ask.
func (a *App) apiRevealBurn(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok {
		return
	}
	if !x.write {
		apiErr(w, http.StatusForbidden, "err.bad_key")
		return
	}
	if !x.admin {
		if e := runBeforeRead(x.note, time.Now(), x.ip); e != nil {
			apiErr(w, e.status, e.key, e.kv...)
			return
		}
	}
	key, left, err := a.db.RevealBurnKey(x.note.ID, r.PathValue("id"), !x.admin)
	if err != nil {
		apiErr(w, http.StatusNotFound, "err.block_burned")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"key": key, "left": left})
}

// ---------- private margins ----------

var refRe = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (a *App) apiMargin(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if u == nil {
		apiErr(w, http.StatusUnauthorized, "err.not_signed_in")
		return
	}
	ref := r.PathValue("ref")
	if !refRe.MatchString(ref) {
		apiErr(w, http.StatusBadRequest, "err.bad_data")
		return
	}
	if r.Method == http.MethodGet {
		c, err := a.db.Margin(u.ID, ref)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, "err.read_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"content": c})
		return
	}
	var q struct {
		Content string `json:"content"`
	}
	if !readJSON(w, r, &q, maxMargin+256) {
		return
	}
	if q.Content != "" && (len(q.Content) > maxMargin || !b64Re.MatchString(q.Content)) {
		apiErr(w, http.StatusBadRequest, "err.bad_data")
		return
	}
	if err := a.db.SetMargin(u.ID, ref, q.Content); err != nil {
		apiErr(w, http.StatusInternalServerError, "err.save_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
