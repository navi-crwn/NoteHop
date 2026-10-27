package db

import (
	"database/sql"
	"errors"
	"strconv"
)

// Tables for the optional note features. Like notes, every content column
// holds what the browser sent: ciphertext for encrypted notes.

// ---------- version history ----------

const (
	maxVersions    = 100
	versionQuietly = 5 * 60 // seconds: edits closer together than this are one version
)

// keepVersion stores the text a save is replacing, unless the note was
// already saved moments ago: autosave runs every few seconds while typing,
// and history should hold the drafts people pause on, not every keystroke.
func keepVersion(tx *sql.Tx, noteID, version int64, content string, updated int64) error {
	var last int64
	_ = tx.QueryRow(`SELECT COALESCE(MAX(created), 0) FROM versions WHERE note_id = ?`, noteID).Scan(&last)
	if last != 0 && updated-last < versionQuietly {
		return nil
	}
	if _, err := tx.Exec(`INSERT INTO versions (note_id, version, content, created) VALUES (?, ?, ?, ?)`, noteID, version, content, updated); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM versions WHERE note_id = ? AND id NOT IN
		(SELECT id FROM versions WHERE note_id = ? ORDER BY version DESC LIMIT ?)`, noteID, noteID, maxVersions)
	return err
}

type Version struct {
	Version int64  `json:"version"`
	Created int64  `json:"created"`
	Size    int    `json:"size"`
	Content string `json:"content,omitempty"`
}

func (d *DB) Versions(noteID int64) ([]Version, error) {
	rows, err := d.Query(`SELECT version, created, length(content) FROM versions WHERE note_id = ? ORDER BY version DESC`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Version{}
	for rows.Next() {
		var v Version
		if err := rows.Scan(&v.Version, &v.Created, &v.Size); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (d *DB) VersionContent(noteID, version int64) (*Version, error) {
	v := &Version{Version: version}
	err := d.QueryRow(`SELECT created, content FROM versions WHERE note_id = ? AND version = ?`, noteID, version).Scan(&v.Created, &v.Content)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	v.Size = len(v.Content)
	return v, err
}

// ---------- suggestions ----------

type Suggestion struct {
	ID          int64  `json:"id"`
	Content     string `json:"content"`
	BaseVersion int64  `json:"base_version"`
	Created     int64  `json:"created"`
}

func (d *DB) AddSuggestion(noteID int64, content string, base int64) (Suggestion, error) {
	s := Suggestion{Content: content, BaseVersion: base, Created: now()}
	res, err := d.Exec(`INSERT INTO suggestions (note_id, content, base_version, created) VALUES (?, ?, ?, ?)`, noteID, content, base, s.Created)
	if err != nil {
		return s, err
	}
	s.ID, _ = res.LastInsertId()
	return s, nil
}

func (d *DB) Suggestions(noteID int64) ([]Suggestion, error) {
	rows, err := d.Query(`SELECT id, content, base_version, created FROM suggestions WHERE note_id = ? ORDER BY id`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Suggestion{}
	for rows.Next() {
		var s Suggestion
		if err := rows.Scan(&s.ID, &s.Content, &s.BaseVersion, &s.Created); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (d *DB) CountSuggestions(noteID int64) (int, error) {
	var n int
	err := d.QueryRow(`SELECT COUNT(*) FROM suggestions WHERE note_id = ?`, noteID).Scan(&n)
	return n, err
}

func (d *DB) DeleteSuggestion(noteID, id int64) error {
	return deleted(d.Exec(`DELETE FROM suggestions WHERE note_id = ? AND id = ?`, noteID, id))
}

func deleted(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if k, _ := res.RowsAffected(); k == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------- sign-off ----------

type Signature struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Version  int64  `json:"version"`
	Snapshot string `json:"snapshot"`
	Created  int64  `json:"created"`
}

func (d *DB) AddSignature(noteID int64, s *Signature) error {
	s.Created = now()
	res, err := d.Exec(`INSERT INTO signatures (note_id, name, version, snapshot, created) VALUES (?, ?, ?, ?, ?)`,
		noteID, s.Name, s.Version, s.Snapshot, s.Created)
	if err != nil {
		return err
	}
	s.ID, _ = res.LastInsertId()
	return nil
}

func (d *DB) Signatures(noteID int64) ([]Signature, error) {
	rows, err := d.Query(`SELECT id, name, version, snapshot, created FROM signatures WHERE note_id = ? ORDER BY id`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Signature{}
	for rows.Next() {
		var s Signature
		if err := rows.Scan(&s.ID, &s.Name, &s.Version, &s.Snapshot, &s.Created); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SnapshotOfVersion finds a frozen copy made from noteID at version, so many
// signatures of the same text share one snapshot.
func (d *DB) SnapshotOfVersion(noteID, version int64) string {
	var tok string
	_ = d.QueryRow(`SELECT l.token FROM notes n JOIN links l ON l.note_id = n.id
		WHERE n.parent_id = ? AND n.frozen = 1 AND n.version = ? ORDER BY n.id DESC LIMIT 1`, noteID, version).Scan(&tok)
	return tok
}

// ---------- polls ----------

// Vote moves one vote: adds to option, and takes it back from prev (>= 0)
// when the voter changes their mind.
func (d *DB) Vote(noteID int64, poll, option, prev int) (map[string]map[string]int64, error) {
	tx, err := d.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if option >= 0 {
		if _, err := tx.Exec(`INSERT INTO votes (note_id, poll, option, count) VALUES (?, ?, ?, 1)
			ON CONFLICT (note_id, poll, option) DO UPDATE SET count = count + 1`, noteID, poll, option); err != nil {
			return nil, err
		}
	}
	if prev >= 0 && prev != option {
		if _, err := tx.Exec(`UPDATE votes SET count = MAX(count - 1, 0) WHERE note_id = ? AND poll = ? AND option = ?`, noteID, poll, prev); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d.Votes(noteID)
}

// Votes is {poll: {option: count}}.
func (d *DB) Votes(noteID int64) (map[string]map[string]int64, error) {
	rows, err := d.Query(`SELECT poll, option, count FROM votes WHERE note_id = ?`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]int64{}
	for rows.Next() {
		var p, o int
		var c int64
		if err := rows.Scan(&p, &o, &c); err != nil {
			return nil, err
		}
		k := strconv.Itoa(p)
		if out[k] == nil {
			out[k] = map[string]int64{}
		}
		out[k][strconv.Itoa(o)] = c
	}
	return out, rows.Err()
}

// ---------- decoy ----------

type Decoy struct {
	Content    string
	WriteHash  string
	Opened     int64
	LastOpened int64
}

func (d *DB) SetDecoy(noteID int64, content, writeHash string) error {
	_, err := d.Exec(`INSERT INTO decoys (note_id, content, write_hash) VALUES (?, ?, ?)
		ON CONFLICT (note_id) DO UPDATE SET content = excluded.content, write_hash = excluded.write_hash`, noteID, content, writeHash)
	return err
}

func (d *DB) DeleteDecoy(noteID int64) error {
	_, err := d.Exec(`DELETE FROM decoys WHERE note_id = ?`, noteID)
	return err
}

func (d *DB) Decoy(noteID int64) (*Decoy, error) {
	x := &Decoy{}
	err := d.QueryRow(`SELECT content, write_hash, opened, last_opened FROM decoys WHERE note_id = ?`, noteID).Scan(&x.Content, &x.WriteHash, &x.Opened, &x.LastOpened)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return x, err
}

func (d *DB) DecoyOpened(noteID int64) error {
	_, err := d.Exec(`UPDATE decoys SET opened = opened + 1, last_opened = ? WHERE note_id = ?`, now(), noteID)
	return err
}

// ---------- private margins ----------

// A margin is a user's own note beside someone else's, sealed with their
// vault key. ref is a hash of the note link, so the server can't tell which
// note it belongs to without already knowing the link.
func (d *DB) Margin(userID int64, ref string) (string, error) {
	var c string
	err := d.QueryRow(`SELECT content FROM margins WHERE user_id = ? AND ref = ?`, userID, ref).Scan(&c)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return c, err
}

func (d *DB) SetMargin(userID int64, ref, content string) error {
	if content == "" {
		_, err := d.Exec(`DELETE FROM margins WHERE user_id = ? AND ref = ?`, userID, ref)
		return err
	}
	_, err := d.Exec(`INSERT INTO margins (user_id, ref, content, updated) VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id, ref) DO UPDATE SET content = excluded.content, updated = excluded.updated`, userID, ref, content, now())
	return err
}

// ---------- partial burn ----------

// A burn block's text sits in the note, encrypted with its own key; the
// server keeps only that key and hands it out max_views times. The block
// text lives inside the note's ciphertext, so the server can't use the key.
type BurnKey struct {
	ID       string `json:"id"`
	Key      string `json:"-"`
	MaxViews int64  `json:"max_views"`
	Views    int64  `json:"views"`
}

func (d *DB) AddBurnKey(noteID int64, b BurnKey) error {
	_, err := d.Exec(`INSERT INTO burn_keys (note_id, id, key, max_views) VALUES (?, ?, ?, ?)`, noteID, b.ID, b.Key, b.MaxViews)
	if isUnique(err) {
		return ErrTaken
	}
	return err
}

// RevealBurnKey counts one view and returns the key; the key is deleted
// once its views are used up. ErrNotFound means it's gone.
func (d *DB) RevealBurnKey(noteID int64, id string, count bool) (key string, left int64, err error) {
	tx, err := d.Begin()
	if err != nil {
		return "", 0, err
	}
	defer tx.Rollback()
	var b BurnKey
	if err := tx.QueryRow(`SELECT key, max_views, views FROM burn_keys WHERE note_id = ? AND id = ?`, noteID, id).Scan(&b.Key, &b.MaxViews, &b.Views); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", 0, ErrNotFound
		}
		return "", 0, err
	}
	if count {
		b.Views++
		if b.Views >= b.MaxViews {
			_, err = tx.Exec(`DELETE FROM burn_keys WHERE note_id = ? AND id = ?`, noteID, id)
		} else {
			_, err = tx.Exec(`UPDATE burn_keys SET views = ? WHERE note_id = ? AND id = ?`, b.Views, noteID, id)
		}
		if err != nil {
			return "", 0, err
		}
	}
	return b.Key, b.MaxViews - b.Views, tx.Commit()
}

func (d *DB) BurnKeys(noteID int64) (map[string]BurnKey, error) {
	rows, err := d.Query(`SELECT id, max_views, views FROM burn_keys WHERE note_id = ?`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]BurnKey{}
	for rows.Next() {
		var b BurnKey
		if err := rows.Scan(&b.ID, &b.MaxViews, &b.Views); err != nil {
			return nil, err
		}
		out[b.ID] = b
	}
	return out, rows.Err()
}

// ---------- claiming and configs ----------

// Claim gives an ownerless note to a user (notes made before signing in).
func (d *DB) Claim(noteID, userID int64, blob string) error {
	return deleted(d.Exec(`UPDATE notes SET owner_id = ?, owner_blob = ? WHERE id = ? AND owner_id IS NULL`, userID, blob, noteID))
}

// NotesWithConfig lists notes whose config mentions key, for worker jobs.
func (d *DB) NotesWithConfig(key string) ([]Note, error) {
	rows, err := d.Query(`SELECT `+noteCols+` FROM notes n WHERE n.config LIKE ?`, `%"`+key+`"%`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

// ResetChecks clears checklist ticks (recurring checklists).
func (d *DB) ResetChecks(noteID int64) error {
	_, err := d.Exec(`UPDATE notes SET checks = '{}', updated = ? WHERE id = ?`, now(), noteID)
	return err
}
