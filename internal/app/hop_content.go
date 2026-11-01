package app

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// Content, reports and the extensions (names, files, realtime,
// integrations) for the control center. Notes are described by their
// metadata; the title and a preview exist only for public notes, because an
// encrypted note is ciphertext to this server.

// hopNoteRow is one note as the control center lists it.
func (a *App) hopNoteRow(n *db.Note) map[string]any {
	links, _ := a.db.Links(n.ID)
	ls := make([]map[string]string, 0, len(links))
	first := ""
	for _, l := range links {
		if first == "" {
			first = l.Token
		}
		ls = append(ls, map[string]string{"token": l.Token, "mode": l.Mode})
	}
	cfg := parseConfig(n.Config)
	var features []string
	for name, on := range map[string]bool{
		"lock_at": cfg.LockAt != 0, "open_at": cfg.OpenAt != 0, "sign_off": cfg.SignOff, "ip_allow": len(cfg.IPAllow) > 0,
		"reset": cfg.Reset != "", "dms": cfg.DMSDays > 0, "watermark": cfg.Watermark, "watch": cfg.Watch != "", "profile": cfg.Profile,
	} {
		if on {
			features = append(features, name)
		}
	}
	if n.IsAdult() {
		features = append(features, "adult", "nsfw")
	} else if n.IsSensitive() {
		features = append(features, "sensitive")
	}
	sort.Strings(features)
	if features == nil {
		features = []string{}
	}
	status := "public"
	switch {
	case n.Frozen:
		status = "frozen"
	case n.MaxViews > 0:
		status = "burn"
	case n.Encrypted:
		status = "encrypted"
	}
	row := map[string]any{
		"id": strconv.FormatInt(n.ID, 10), "type": "note", "title": publicTitle(n), "owner": nil, "status": status, "encrypted": n.Encrypted,
		"password": n.Password, "size": len(n.Content), "views": n.Views, "max_views": n.MaxViews, "expires": iso(n.Expires),
		"created": iso(n.Created), "updated": iso(n.Updated), "last_open": iso(n.LastOpen), "links": ls, "features": features,
		"url": a.noteURL(first), "preview": publicPreview(n), "flags": features,
		"content_warning": n.ContentWarning, "warning_reason": n.WarningReason,
	}
	if n.OwnerID.Valid {
		if u, err := a.db.UserByID(n.OwnerID.Int64); err == nil {
			row["owner"] = map[string]any{"sub": u.SSOID, "id": hopUserID(u), "username": u.Username}
		}
	}
	return row
}

func (a *App) hopContent(w http.ResponseWriter, r *http.Request, _ []byte) {
	q := r.URL.Query()
	cursor, limit := hopPaging(r)
	if q.Get("type") == "file" {
		files, err := a.db.ListFiles(q.Get("cursor"), limit)
		if err != nil {
			hopServerErr(w, "files", err)
			return
		}
		out := make([]map[string]any, 0, len(files))
		for _, f := range files {
			out = append(out, map[string]any{"id": f.ID, "type": "file", "size": f.Size, "created": iso(f.Created), "expires": iso(f.Expires), "encrypted": true})
		}
		next := ""
		if len(files) == limit {
			next = files[len(files)-1].ID
		}
		hopList(w, out, next, 0)
		return
	}
	f := db.NoteFilter{Cursor: cursor, Limit: limit, Status: q.Get("status")}
	if owner := q.Get("owner"); owner != "" {
		u, err := a.hopFindUser(owner)
		if err != nil {
			u, err = a.db.UserByName(strings.ToLower(owner))
		}
		if err != nil {
			hopList(w, []any{}, "", 0)
			return
		}
		f.OwnerID = u.ID
	}
	search := strings.TrimSpace(q.Get("q"))
	// q matches the start of a link name, or words in a public note's title.
	// Two passes keep the SQL simple: by token first, then by title.
	var notes []db.Note
	seen := map[int64]bool{}
	if search != "" {
		byToken := f
		byToken.Token = search
		rows, err := a.db.ListNotes(byToken)
		if err != nil {
			hopServerErr(w, "content", err)
			return
		}
		for _, n := range rows {
			seen[n.ID] = true
			notes = append(notes, n)
		}
		byTitle := f
		byTitle.Title = search
		rows, err = a.db.ListNotes(byTitle)
		if err != nil {
			hopServerErr(w, "content", err)
			return
		}
		for _, n := range rows {
			if !seen[n.ID] && strings.Contains(strings.ToLower(publicTitle(&n)), strings.ToLower(search)) {
				notes = append(notes, n)
			}
		}
		sort.Slice(notes, func(i, j int) bool { return notes[i].ID > notes[j].ID })
		if len(notes) > limit {
			notes = notes[:limit]
		}
	} else {
		var err error
		if notes, err = a.db.ListNotes(f); err != nil {
			hopServerErr(w, "content", err)
			return
		}
	}
	out := make([]map[string]any, 0, len(notes))
	for i := range notes {
		out = append(out, a.hopNoteRow(&notes[i]))
	}
	next := ""
	if len(notes) == limit {
		next = strconv.FormatInt(notes[len(notes)-1].ID, 10)
	}
	hopList(w, out, next, 0)
}

func (a *App) hopNoteByPath(w http.ResponseWriter, r *http.Request) (*db.Note, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		hopFail(w, http.StatusNotFound, "not_found", "no note with that id")
		return nil, false
	}
	n, err := a.db.NoteByID(id)
	if errors.Is(err, db.ErrNotFound) {
		hopFail(w, http.StatusNotFound, "not_found", "no note with that id")
		return nil, false
	}
	if err != nil {
		hopServerErr(w, "note", err)
		return nil, false
	}
	return n, true
}

func (a *App) hopNote(w http.ResponseWriter, r *http.Request, _ []byte) {
	n, ok := a.hopNoteByPath(w, r)
	if !ok {
		return
	}
	row := a.hopNoteRow(n)
	c, _ := a.db.NoteCounts(n.ID)
	cfg := parseConfig(n.Config)
	_, inboxErr := a.db.InboxOf(n.ID)
	row["entries"], row["versions"], row["suggestions"], row["reports_open"] = c.Entries, c.Versions, c.Suggestions, c.Reports
	row["inbox"], row["watch"] = inboxErr == nil, cfg.Watch != ""
	row["snapshots"], _ = a.db.Snapshots(n.ID)
	row["parent"] = a.db.ParentToken(n)
	row["actions"] = []string{"delete", "takedown", "freeze", "set_expiry", "release_slug", "disable_inbox", "disable_watch", "mark_nsfw", "unmark_nsfw", "mark_sensitive"}
	hopOK(w, row)
}

func (a *App) hopNoteAction(w http.ResponseWriter, r *http.Request, body []byte) {
	n, ok := a.hopNoteByPath(w, r)
	if !ok {
		return
	}
	var q actionReq
	if !hopBody(w, body, &q) {
		return
	}
	res, err := a.noteAction(n, q)
	if errors.Is(err, errHopUnknownAction) {
		hopFail(w, http.StatusBadRequest, "unknown_action", "that action does not exist for notes")
		return
	}
	if err != nil {
		if strings.HasPrefix(err.Error(), "bad_") {
			hopFail(w, http.StatusBadRequest, err.Error(), "the action's parameters are not valid")
			return
		}
		hopServerErr(w, "note action "+q.Action, err)
		return
	}
	a.audit(r, q.Action, "note:"+strconv.FormatInt(n.ID, 10), map[string]any{"reason": clip(q.Reason, 500), "params": q.Params, "result": res})
	hopOK(w, res)
}

func (a *App) noteAction(n *db.Note, q actionReq) (map[string]any, error) {
	id := strconv.FormatInt(n.ID, 10)
	switch q.Action {
	case "delete":
		return map[string]any{"id": id, "deleted": true}, a.db.DeleteNote(n.ID)
	case "takedown":
		// Like the CLI: the note goes, its link names are held for a year.
		return map[string]any{"id": id, "deleted": true, "hold_days": 365}, a.db.Takedown(n.ID)
	case "freeze":
		return map[string]any{"id": id, "frozen": true}, a.db.SetFrozen(n.ID)
	case "set_expiry":
		exp, err := parseISO(q.str("expires"))
		if err != nil || (exp != 0 && exp <= time.Now().Unix()) {
			return nil, errors.New("bad_expires")
		}
		return map[string]any{"id": id, "expires": iso(exp)}, a.db.SetExpires(n.ID, exp)
	case "release_slug":
		slug := strings.ToLower(strings.TrimSpace(q.str("slug")))
		if slug == "" {
			return nil, errors.New("bad_slug")
		}
		a.db.ReleaseSlug(slug)
		return map[string]any{"slug": slug, "released": true}, nil
	case "disable_inbox":
		return map[string]any{"id": id, "inbox": false}, a.db.DeleteInbox(n.ID)
	case "disable_watch":
		cfg := parseConfig(n.Config)
		cfg.Watch, cfg.WatchKey = "", ""
		return map[string]any{"id": id, "watch": false}, a.db.SetConfig(n.ID, cfg.String())
	case "mark_nsfw":
		return map[string]any{"id": id, "content_warning": "adult", "warning_reason": q.Reason}, a.db.SetNoteWarning(n.ID, "adult", q.Reason)
	case "unmark_nsfw":
		return map[string]any{"id": id, "content_warning": "none", "warning_reason": ""}, a.db.SetNoteWarning(n.ID, "none", "")
	case "mark_sensitive":
		return map[string]any{"id": id, "content_warning": "sensitive", "warning_reason": q.Reason}, a.db.SetNoteWarning(n.ID, "sensitive", q.Reason)
	}
	return nil, errHopUnknownAction
}

func (a *App) hopFile(w http.ResponseWriter, r *http.Request, _ []byte) {
	id := r.PathValue("id")
	f, err := a.db.GetFile(id)
	if !fileIDRe.MatchString(id) || errors.Is(err, db.ErrNotFound) {
		hopFail(w, http.StatusNotFound, "not_found", "no attachment with that id")
		return
	}
	if err != nil {
		hopServerErr(w, "file", err)
		return
	}
	// Ciphertext only; the key lives in the note's text, which the server can't read.
	hopOK(w, map[string]any{"id": f.ID, "type": "file", "size": f.Size, "created": iso(f.Created), "expires": iso(f.Expires), "encrypted": true,
		"url": a.cfg.BaseURL + "/api/files/" + f.ID, "actions": []string{"delete"}})
}

func (a *App) hopFileAction(w http.ResponseWriter, r *http.Request, body []byte) {
	id := r.PathValue("id")
	var q actionReq
	if !hopBody(w, body, &q) {
		return
	}
	if q.Action != "delete" {
		hopFail(w, http.StatusBadRequest, "unknown_action", "attachments can only be deleted")
		return
	}
	if !fileIDRe.MatchString(id) {
		hopFail(w, http.StatusNotFound, "not_found", "no attachment with that id")
		return
	}
	ok, err := a.db.DeleteFile(id)
	if err != nil {
		hopServerErr(w, "file delete", err)
		return
	}
	if !ok {
		hopFail(w, http.StatusNotFound, "not_found", "no attachment with that id")
		return
	}
	a.audit(r, "file.delete", "file:"+id, map[string]any{"reason": clip(q.Reason, 500)})
	hopOK(w, map[string]any{"id": id, "deleted": true})
}

// ---------- reports ----------

func (a *App) hopReportRow(x *db.Report) map[string]any {
	row := map[string]any{"id": x.ID, "note_id": strconv.FormatInt(x.NoteID, 10), "token": x.Token, "reason": x.Reason, "note": x.Note,
		"created": iso(x.Created), "status": x.Status, "url": a.noteURL(x.Token), "note_exists": false}
	if n, err := a.db.NoteByID(x.NoteID); err == nil {
		row["note_exists"], row["encrypted"], row["title"] = true, n.Encrypted, publicTitle(n)
	}
	return row
}

func (a *App) hopReports(w http.ResponseWriter, r *http.Request, _ []byte) {
	cursor, limit := hopPaging(r)
	rows, err := a.db.Reports(r.URL.Query().Get("status"), cursor, limit)
	if err != nil {
		hopServerErr(w, "reports", err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for i := range rows {
		out = append(out, a.hopReportRow(&rows[i]))
	}
	next := ""
	if len(rows) == limit {
		next = strconv.FormatInt(rows[len(rows)-1].ID, 10)
	}
	hopList(w, out, next, 0)
}

func (a *App) hopReportAction(w http.ResponseWriter, r *http.Request, body []byte) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	x, err := a.db.Report(id)
	if errors.Is(err, db.ErrNotFound) {
		hopFail(w, http.StatusNotFound, "not_found", "no report with that id")
		return
	}
	if err != nil {
		hopServerErr(w, "report", err)
		return
	}
	var q actionReq
	if !hopBody(w, body, &q) {
		return
	}
	status := ""
	res := map[string]any{"id": x.ID}
	switch q.Action {
	case "dismiss":
		status = "dismissed"
	case "resolve":
		status = "resolved"
	case "takedown":
		status = "resolved"
		if n, err := a.db.NoteByID(x.NoteID); err == nil {
			if err := a.db.Takedown(n.ID); err != nil {
				hopServerErr(w, "report takedown", err)
				return
			}
			res["deleted"], res["hold_days"] = true, 365
		} else {
			res["deleted"] = false // already gone
		}
	default:
		hopFail(w, http.StatusBadRequest, "unknown_action", "a report can be dismissed, resolved or taken down")
		return
	}
	if err := a.db.SetReportStatus(x.ID, status); err != nil {
		hopServerErr(w, "report status", err)
		return
	}
	res["status"] = status
	a.audit(r, "report."+q.Action, "report:"+strconv.FormatInt(x.ID, 10), map[string]any{"reason": clip(q.Reason, 500), "note_id": x.NoteID, "result": res})
	hopOK(w, res)
}

// ---------- extensions ----------

func (a *App) hopNames(w http.ResponseWriter, r *http.Request, _ []byte) {
	reserved := make([]string, 0, len(reservedNames)+len(reservedPrefixes)+len(a.cfg.Reserved))
	for n := range reservedNames {
		reserved = append(reserved, n)
	}
	for _, p := range reservedPrefixes {
		reserved = append(reserved, p+"*")
	}
	reserved = append(reserved, a.cfg.Reserved...)
	sort.Strings(reserved)
	holds, err := a.db.Holds()
	if err != nil {
		hopServerErr(w, "holds", err)
		return
	}
	// A takedown holds a name for a year, an ordinary deletion for 90 days:
	// the longer ones are the takedowns.
	limit := time.Now().Unix() + db.SlugHoldDays*86400 + 86400
	regular, takedowns := []map[string]any{}, []map[string]any{}
	for _, h := range holds {
		row := map[string]any{"slug": h.Slug, "owner": nil, "until": iso(h.Until)}
		if h.OwnerID.Valid {
			if u, err := a.db.UserByID(h.OwnerID.Int64); err == nil {
				row["owner"] = map[string]any{"sub": u.SSOID, "id": hopUserID(u), "username": u.Username}
			}
		}
		if h.Until > limit {
			takedowns = append(takedowns, row)
		} else {
			regular = append(regular, row)
		}
	}
	hopOK(w, map[string]any{"reserved": reserved, "blocked_count": len(blockedWords), "min_slug": minSlug, "hold_days": db.SlugHoldDays,
		"holds": regular, "takedown_holds": takedowns})
}

func (a *App) hopHoldAction(w http.ResponseWriter, r *http.Request, body []byte) {
	slug := strings.ToLower(r.PathValue("slug"))
	var q actionReq
	if !hopBody(w, body, &q) {
		return
	}
	if q.Action != "release" {
		hopFail(w, http.StatusBadRequest, "unknown_action", "a hold can only be released")
		return
	}
	if _, _, held := a.db.SlugHold(slug); !held {
		hopFail(w, http.StatusNotFound, "not_found", "that name is not on hold")
		return
	}
	a.db.ReleaseSlug(slug)
	a.audit(r, "hold.release", "slug:"+slug, map[string]any{"reason": clip(q.Reason, 500)})
	hopOK(w, map[string]any{"slug": slug, "released": true})
}

func (a *App) hopFiles(w http.ResponseWriter, r *http.Request, _ []byte) {
	count, bytes, err := a.db.FileUsage()
	if err != nil {
		hopServerErr(w, "files", err)
		return
	}
	hopOK(w, map[string]any{"on": a.cfg.FileMB > 0, "count": count, "bytes": bytes, "budget_bytes": int64(a.cfg.FilesTotalMB) << 20,
		"file_mb": a.cfg.FileMB, "per_ip_mb": a.cfg.FilesPerIPMB, "ttl_hours": int(fileTTL.Hours()),
		"note_limit_bytes": 5 << 20, "uploads_available": a.cfg.FileMB > 0 && bytes < int64(a.cfg.FilesTotalMB)<<20 && a.storageAvailable(int64(a.cfg.FileMB)<<20)})
}

func (a *App) hopRealtime(w http.ResponseWriter, r *http.Request, _ []byte) {
	rooms := a.live.snapshot()
	out := make([]map[string]any, 0, len(rooms))
	for _, x := range rooms {
		out = append(out, map[string]any{"note_id": strconv.FormatInt(x.note, 10), "peers": x.peers})
	}
	hopOK(w, map[string]any{"rooms": out, "nearby_rooms": a.near.count()})
}

func (a *App) hopIntegrations(w http.ResponseWriter, r *http.Request, _ []byte) {
	s, err := a.db.Stats()
	if err != nil {
		hopServerErr(w, "integrations", err)
		return
	}
	hopOK(w, map[string]any{
		"telegram": map[string]any{"on": a.cfg.TelegramToken != "", "chats": s.TelegramChats},
		"ssh":      map[string]any{"on": a.cfg.SSHAddr != "", "addr": a.cfg.SSHAddr},
		"webhooks": map[string]any{"pending": a.watch.pending(), "private_hooks": a.cfg.PrivateHooks},
		"inboxes":  map[string]any{"count": s.Inboxes},
	})
}
