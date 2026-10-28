package db

import (
	"database/sql"
	"errors"
	"strings"
)

// Queries for the control center (HOP App Protocol, internal/app/hop*.go):
// account roles and statuses, lists and counts, the audit log, handled
// events, run-time settings and reader reports. Like everything else here,
// none of it reads note content: encrypted notes are opaque to the server.

// ---------- accounts ----------

// SeedAdmins gives the admin role to the listed usernames, if those accounts
// exist. Called at start for NOTEHOP_ADMINS; the list grants nothing by
// itself, so registering a listed name later doesn't make anyone an admin.
func (d *DB) SeedAdmins(usernames []string) error {
	for _, name := range usernames {
		if _, err := d.Exec(`UPDATE users SET role = ? WHERE username = ?`, RoleAdmin, name); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) SetRole(userID int64, role string) error {
	return deleted(d.Exec(`UPDATE users SET role = ? WHERE id = ?`, role, userID))
}

func (d *DB) SetStatus(userID int64, status string) error {
	return deleted(d.Exec(`UPDATE users SET status = ? WHERE id = ?`, status, userID))
}

// SetUsername renames an account; ErrTaken when the name is in use.
func (d *DB) SetUsername(userID int64, username string) error {
	_, err := d.Exec(`UPDATE users SET username = ? WHERE id = ?`, username, userID)
	if isUnique(err) {
		return ErrTaken
	}
	return err
}

// ClearVaultCheck forgets the vault phrase check so the account sets a new
// phrase next time. The vault key itself was never here.
func (d *DB) ClearVaultCheck(userID int64) error {
	return deleted(d.Exec(`UPDATE users SET vault_check = '', vault_wrap = '' WHERE id = ?`, userID))
}

// DeleteNotesByOwner removes every note of an account (their link names are
// held as after any deletion) and returns how many went.
func (d *DB) DeleteNotesByOwner(userID int64) (int64, error) {
	tx, err := d.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if err := holdSlugs(tx, `n.owner_id = ?`, userID); err != nil {
		return 0, err
	}
	res, err := tx.Exec(`DELETE FROM notes WHERE owner_id = ?`, userID)
	if err != nil {
		return 0, err
	}
	k, _ := res.RowsAffected()
	return k, tx.Commit()
}

// DeleteUser removes the account row; sessions, margins and recovery codes
// go with it (foreign keys), notes must be deleted first or they stay as
// ownerless notes.
func (d *DB) DeleteUser(userID int64) error {
	return deleted(d.Exec(`DELETE FROM users WHERE id = ?`, userID))
}

// UserCounts is what the control center sees about one account.
type UserCounts struct {
	Notes, NotesPublic, Sessions int64
}

func (d *DB) UserCounts(userID int64) (UserCounts, error) {
	var c UserCounts
	err := d.QueryRow(`SELECT (SELECT COUNT(*) FROM notes WHERE owner_id = ?), (SELECT COUNT(*) FROM notes WHERE owner_id = ? AND encrypted = 0),
		(SELECT COUNT(*) FROM sessions WHERE user_id = ? AND expires > ?)`, userID, userID, userID, now()).Scan(&c.Notes, &c.NotesPublic, &c.Sessions)
	return c, err
}

// ListUsers pages through accounts by id, optionally only those whose
// username or email contains q.
func (d *DB) ListUsers(q string, cursor int64, limit int) ([]User, error) {
	rows, err := d.Query(`SELECT `+userCols+` FROM users u WHERE u.id > ? AND (? = '' OR u.username LIKE ? OR u.email LIKE ?)
		ORDER BY u.id LIMIT ?`, cursor, q, "%"+like(q)+"%", "%"+like(q)+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// like neutralises LIKE wildcards in user input: "%" and "\" become "_"
// (any one character), close enough for a contains-search without an
// ESCAPE clause.
func like(q string) string {
	return strings.NewReplacer("%", "_", "\\", "_").Replace(q)
}

// ---------- notes ----------

// NoteFilter narrows ListNotes. Zero values mean "any".
type NoteFilter struct {
	Token   string // link token or name starts with this
	Title   string // public notes whose text mentions this (title match is finished in the app layer)
	OwnerID int64
	Status  string // encrypted, public, expiring, frozen, burn
	Cursor  int64  // ids below this
	Limit   int
}

func (d *DB) ListNotes(f NoteFilter) ([]Note, error) {
	where, args := []string{`1 = 1`}, []any{}
	if f.Cursor > 0 {
		where, args = append(where, `n.id < ?`), append(args, f.Cursor)
	}
	if f.Token != "" {
		where, args = append(where, `EXISTS (SELECT 1 FROM links l WHERE l.note_id = n.id AND l.token LIKE ?)`), append(args, like(f.Token)+"%")
	}
	if f.Title != "" {
		where, args = append(where, `n.encrypted = 0 AND n.content LIKE ?`), append(args, "%"+like(f.Title)+"%")
	}
	if f.OwnerID > 0 {
		where, args = append(where, `n.owner_id = ?`), append(args, f.OwnerID)
	}
	switch f.Status {
	case "encrypted":
		where = append(where, `n.encrypted = 1`)
	case "public":
		where = append(where, `n.encrypted = 0`)
	case "expiring":
		where = append(where, `n.expires > 0`)
	case "frozen":
		where = append(where, `n.frozen = 1`)
	case "burn":
		where = append(where, `n.max_views > 0`)
	}
	rows, err := d.Query(`SELECT `+noteCols+` FROM notes n WHERE `+strings.Join(where, " AND ")+` ORDER BY n.id DESC LIMIT ?`, append(args, f.Limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Note{}
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

// NoteCounts are the extras of a note's detail view.
type NoteCounts struct {
	Entries, Versions, Suggestions, Reports int64
}

func (d *DB) NoteCounts(noteID int64) (NoteCounts, error) {
	var c NoteCounts
	err := d.QueryRow(`SELECT (SELECT COUNT(*) FROM entries WHERE note_id = ?), (SELECT COUNT(*) FROM versions WHERE note_id = ?),
		(SELECT COUNT(*) FROM suggestions WHERE note_id = ?), (SELECT COUNT(*) FROM reports WHERE note_id = ? AND status = 'open')`,
		noteID, noteID, noteID, noteID).Scan(&c.Entries, &c.Versions, &c.Suggestions, &c.Reports)
	return c, err
}

// SetFrozen makes a note read-only for everyone, its owner included.
func (d *DB) SetFrozen(noteID int64) error {
	return deleted(d.Exec(`UPDATE notes SET frozen = 1, updated = ? WHERE id = ?`, now(), noteID))
}

// SetExpires changes when a note disappears (0: never).
func (d *DB) SetExpires(noteID, expires int64) error {
	return deleted(d.Exec(`UPDATE notes SET expires = ? WHERE id = ?`, expires, noteID))
}

// FirstToken is a note's oldest link, the one shown as its address.
func (d *DB) FirstToken(noteID int64) string {
	var tok string
	_ = d.QueryRow(`SELECT token FROM links WHERE note_id = ? ORDER BY rowid LIMIT 1`, noteID).Scan(&tok)
	return tok
}

// PublicNotesMentioning finds public notes whose text contains q, for the
// title search; the app layer checks the title itself.
func (d *DB) PublicNotesMentioning(q string, limit int) ([]Note, error) {
	return d.ListNotes(NoteFilter{Title: q, Limit: limit})
}

// ---------- files and holds ----------

// FileInfo is an attachment without its bytes.
type FileInfo struct {
	ID      string `json:"id"`
	Size    int64  `json:"size"`
	Created int64  `json:"created"`
	Expires int64  `json:"expires"`
}

func (d *DB) ListFiles(cursor string, limit int) ([]FileInfo, error) {
	rows, err := d.Query(`SELECT id, size, created, expires FROM files WHERE expires > ? AND id > ? ORDER BY id LIMIT ?`, now(), cursor, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FileInfo{}
	for rows.Next() {
		var f FileInfo
		if err := rows.Scan(&f.ID, &f.Size, &f.Created, &f.Expires); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FileUsage is how many live attachments there are and their total size.
func (d *DB) FileUsage() (count, bytes int64, err error) {
	err = d.QueryRow(`SELECT COUNT(*), COALESCE(SUM(size), 0) FROM files WHERE expires > ?`, now()).Scan(&count, &bytes)
	return
}

// Hold is a link name kept back after its note went away.
type Hold struct {
	Slug    string
	OwnerID sql.NullInt64
	Until   int64
}

func (d *DB) Holds() ([]Hold, error) {
	rows, err := d.Query(`SELECT slug, owner_id, until FROM slug_holds WHERE until > ? ORDER BY until DESC LIMIT 1000`, now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Hold{}
	for rows.Next() {
		var h Hold
		if err := rows.Scan(&h.Slug, &h.OwnerID, &h.Until); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// ---------- counts for /hop/stats and /hop/metrics ----------

type Stats struct {
	Users, UsersSSO, Notes, NotesEncrypted, NotesPublic, NotesOwned, NotesBurn int64
	Files, FilesBytes, SlugHolds, Takedowns, Reports, TelegramChats, Inboxes   int64
}

func (d *DB) Stats() (Stats, error) {
	var s Stats
	t := now()
	err := d.QueryRow(`SELECT
		(SELECT COUNT(*) FROM users), (SELECT COUNT(*) FROM users WHERE sso_id IS NOT NULL),
		(SELECT COUNT(*) FROM notes), (SELECT COUNT(*) FROM notes WHERE encrypted = 1), (SELECT COUNT(*) FROM notes WHERE encrypted = 0),
		(SELECT COUNT(*) FROM notes WHERE owner_id IS NOT NULL), (SELECT COUNT(*) FROM notes WHERE max_views > 0),
		(SELECT COUNT(*) FROM files WHERE expires > ?), (SELECT COALESCE(SUM(size), 0) FROM files WHERE expires > ?),
		(SELECT COUNT(*) FROM slug_holds WHERE until > ?), (SELECT COUNT(*) FROM hop_audit WHERE action = 'takedown' OR (action = 'report.takedown' AND detail LIKE '%"deleted":true%')),
		(SELECT COUNT(*) FROM reports WHERE status = 'open'), (SELECT COUNT(*) FROM telegram_chats), (SELECT COUNT(*) FROM inboxes)`,
		t, t, t).Scan(&s.Users, &s.UsersSSO, &s.Notes, &s.NotesEncrypted, &s.NotesPublic, &s.NotesOwned, &s.NotesBurn,
		&s.Files, &s.FilesBytes, &s.SlugHolds, &s.Takedowns, &s.Reports, &s.TelegramChats, &s.Inboxes)
	return s, err
}

// DayCount is one point of a per-day series.
type DayCount struct {
	Day string
	N   int64
}

// NotesPerDay counts notes made per UTC day since `since` (unix seconds).
func (d *DB) NotesPerDay(since int64) ([]DayCount, error) {
	rows, err := d.Query(`SELECT date(created, 'unixepoch'), COUNT(*) FROM notes WHERE created >= ? GROUP BY 1 ORDER BY 1`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DayCount
	for rows.Next() {
		var c DayCount
		if err := rows.Scan(&c.Day, &c.N); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---------- audit, events, settings ----------

type Audit struct {
	ID     int64
	At     int64
	Actor  string
	App    string
	Action string
	Target string
	Detail string // JSON
}

func (d *DB) AddAudit(a Audit) error {
	if a.Detail == "" {
		a.Detail = "{}"
	}
	_, err := d.Exec(`INSERT INTO hop_audit (at, actor, app, action, target, detail) VALUES (?, ?, ?, ?, ?, ?)`,
		now(), a.Actor, a.App, a.Action, a.Target, a.Detail)
	return err
}

// Audits pages newest first; cursor is the id to continue below (0: start).
func (d *DB) Audits(cursor int64, limit int) ([]Audit, error) {
	if cursor <= 0 {
		cursor = 1 << 62
	}
	rows, err := d.Query(`SELECT id, at, actor, app, action, target, detail FROM hop_audit WHERE id < ? ORDER BY id DESC LIMIT ?`, cursor, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Audit{}
	for rows.Next() {
		var a Audit
		if err := rows.Scan(&a.ID, &a.At, &a.Actor, &a.App, &a.Action, &a.Target, &a.Detail); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// FirstEvent remembers an event id and reports whether it was new, so a
// retried webhook is answered without acting twice.
func (d *DB) FirstEvent(id string) (bool, error) {
	_, err := d.Exec(`INSERT INTO hop_events_seen (id, at) VALUES (?, ?)`, id, now())
	if isUnique(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// Keep the table small: ids older than a month are of no use to anyone.
	_, _ = d.Exec(`DELETE FROM hop_events_seen WHERE at < ?`, now()-30*86400)
	return true, nil
}

// ForgetEvent lets an event be delivered again after handling it failed.
func (d *DB) ForgetEvent(id string) {
	_, _ = d.Exec(`DELETE FROM hop_events_seen WHERE id = ?`, id)
}

// Setting returns a run-time setting, or ok=false when it was never set.
func (d *DB) Setting(key string) (string, bool) {
	var v string
	err := d.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	return v, err == nil
}

func (d *DB) SetSetting(key, value string) error {
	_, err := d.Exec(`INSERT INTO settings (key, value, updated) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated = excluded.updated`, key, value, now())
	return err
}

func (d *DB) DeleteSetting(key string) error {
	_, err := d.Exec(`DELETE FROM settings WHERE key = ?`, key)
	return err
}

// ---------- reports ----------

type Report struct {
	ID      int64
	NoteID  int64
	Token   string
	Reason  string
	Note    string // what the reporter wrote, never the note's content
	IPHash  string
	Created int64
	Status  string // open, resolved, dismissed
}

func (d *DB) AddReport(r *Report) error {
	r.Created, r.Status = now(), "open"
	res, err := d.Exec(`INSERT INTO reports (note_id, token, reason, note, ip_hash, created, status) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.NoteID, r.Token, r.Reason, r.Note, r.IPHash, r.Created, r.Status)
	if err != nil {
		return err
	}
	r.ID, _ = res.LastInsertId()
	return nil
}

// ReportsFrom counts reports one address sent since `since`, for the limit.
func (d *DB) ReportsFrom(ipHash string, since int64) (int, error) {
	var n int
	err := d.QueryRow(`SELECT COUNT(*) FROM reports WHERE ip_hash = ? AND created > ?`, ipHash, since).Scan(&n)
	return n, err
}

func (d *DB) Report(id int64) (*Report, error) {
	r := &Report{}
	err := d.QueryRow(`SELECT id, note_id, token, reason, note, ip_hash, created, status FROM reports WHERE id = ?`, id).
		Scan(&r.ID, &r.NoteID, &r.Token, &r.Reason, &r.Note, &r.IPHash, &r.Created, &r.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return r, err
}

// Reports pages newest first, all statuses when status is empty.
func (d *DB) Reports(status string, cursor int64, limit int) ([]Report, error) {
	if cursor <= 0 {
		cursor = 1 << 62
	}
	rows, err := d.Query(`SELECT id, note_id, token, reason, note, ip_hash, created, status FROM reports
		WHERE id < ? AND (? = '' OR status = ?) ORDER BY id DESC LIMIT ?`, cursor, status, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Report{}
	for rows.Next() {
		var r Report
		if err := rows.Scan(&r.ID, &r.NoteID, &r.Token, &r.Reason, &r.Note, &r.IPHash, &r.Created, &r.Status); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) SetReportStatus(id int64, status string) error {
	return deleted(d.Exec(`UPDATE reports SET status = ? WHERE id = ?`, status, id))
}
