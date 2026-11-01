// Package db is NoteHop's SQLite store. The server only ever holds what the
// browser hands it: ciphertext for encrypted notes, plus the metadata needed to
// enforce links, expiry and burn-after-reading.
package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("version conflict")
	ErrTaken    = errors.New("already taken")
	ErrLastLink = errors.New("a note needs at least one link")
)

type DB struct{ *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY,
	username TEXT NOT NULL UNIQUE,
	auth_hash TEXT NOT NULL,
	created INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
	token_hash TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	expires INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS notes (
	id INTEGER PRIMARY KEY,
	owner_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
	owner_blob TEXT NOT NULL DEFAULT '',
	encrypted INTEGER NOT NULL,
	password INTEGER NOT NULL DEFAULT 0,
	salt TEXT NOT NULL DEFAULT '',
	content TEXT NOT NULL,
	version INTEGER NOT NULL DEFAULT 1,
	checks TEXT NOT NULL DEFAULT '{}',
	admin_hash TEXT NOT NULL,
	write_hash TEXT NOT NULL,
	parent_id INTEGER REFERENCES notes(id) ON DELETE SET NULL,
	frozen INTEGER NOT NULL DEFAULT 0,
	max_views INTEGER NOT NULL DEFAULT 0,
	views INTEGER NOT NULL DEFAULT 0,
	expires INTEGER NOT NULL DEFAULT 0,
	created INTEGER NOT NULL,
	updated INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS notes_owner ON notes(owner_id);
CREATE INDEX IF NOT EXISTS notes_expires ON notes(expires) WHERE expires > 0;
CREATE INDEX IF NOT EXISTS notes_parent ON notes(parent_id);
CREATE TABLE IF NOT EXISTS links (
	token TEXT PRIMARY KEY,
	note_id INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
	mode TEXT NOT NULL,
	created INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS links_note ON links(note_id);
CREATE TABLE IF NOT EXISTS entries (
	id INTEGER PRIMARY KEY,
	note_id INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
	content TEXT NOT NULL,
	created INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS entries_note ON entries(note_id);
`

// Open opens (and migrates) the database. ":memory:" works for tests.
func Open(path string) (*DB, error) {
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	if path != ":memory:" {
		dsn += "&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	}
	sdb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection serialises writes, which keeps burn-after-reading and
	// optimistic version checks race-free without extra locking.
	sdb.SetMaxOpenConns(1)
	if _, err := sdb.Exec(schema); err != nil {
		sdb.Close()
		return nil, err
	}
	d := &DB{sdb}
	if err := d.migrate(); err != nil {
		sdb.Close()
		return nil, err
	}
	return d, nil
}

// migrations run in order after the base schema; the database remembers how
// many it has applied in PRAGMA user_version. Only ever append to this list:
// never edit or reorder an entry that has shipped. Parallel branches that both
// add one must renumber when they merge.
var migrations = []string{
	// 1: accounts that sign in through single sign-on, plus their vault phrase check.
	`ALTER TABLE users ADD COLUMN sso_id TEXT;
	ALTER TABLE users ADD COLUMN email TEXT NOT NULL DEFAULT '';
	ALTER TABLE users ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
	ALTER TABLE users ADD COLUMN avatar TEXT NOT NULL DEFAULT '';
	ALTER TABLE users ADD COLUMN vault_check TEXT NOT NULL DEFAULT '';
	CREATE UNIQUE INDEX users_sso ON users(sso_id) WHERE sso_id IS NOT NULL;`,
	// 2: optional per-note features as JSON (scheduled lock, ...), so each
	// feature doesn't need its own column. See NoteConfig in internal/app.
	`ALTER TABLE notes ADD COLUMN config TEXT NOT NULL DEFAULT '{}';`,
	// 3: read receipts, version history, suggestions, sign-off, polls,
	// decoys, private margins and partial-burn block keys (see extra.go).
	`ALTER TABLE notes ADD COLUMN last_open INTEGER NOT NULL DEFAULT 0;
	CREATE TABLE versions (id INTEGER PRIMARY KEY, note_id INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
		version INTEGER NOT NULL, content TEXT NOT NULL, created INTEGER NOT NULL);
	CREATE INDEX versions_note ON versions(note_id, version);
	CREATE TABLE suggestions (id INTEGER PRIMARY KEY, note_id INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
		content TEXT NOT NULL, base_version INTEGER NOT NULL, created INTEGER NOT NULL);
	CREATE INDEX suggestions_note ON suggestions(note_id);
	CREATE TABLE signatures (id INTEGER PRIMARY KEY, note_id INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
		name TEXT NOT NULL, version INTEGER NOT NULL, snapshot TEXT NOT NULL DEFAULT '', created INTEGER NOT NULL);
	CREATE INDEX signatures_note ON signatures(note_id);
	CREATE TABLE votes (note_id INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE, poll INTEGER NOT NULL,
		option INTEGER NOT NULL, count INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (note_id, poll, option));
	CREATE TABLE decoys (note_id INTEGER PRIMARY KEY REFERENCES notes(id) ON DELETE CASCADE, content TEXT NOT NULL,
		write_hash TEXT NOT NULL, opened INTEGER NOT NULL DEFAULT 0, last_opened INTEGER NOT NULL DEFAULT 0);
	CREATE TABLE margins (user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, ref TEXT NOT NULL,
		content TEXT NOT NULL, updated INTEGER NOT NULL, PRIMARY KEY (user_id, ref));
	CREATE TABLE burn_keys (note_id INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE, id TEXT NOT NULL,
		key TEXT NOT NULL, max_views INTEGER NOT NULL, views INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (note_id, id));`,
	// 4: temporary encrypted attachments (24 hours). Each file has its own
	// key, kept only inside the note text.
	`CREATE TABLE files (id TEXT PRIMARY KEY, size INTEGER NOT NULL, data BLOB NOT NULL,
		created INTEGER NOT NULL, expires INTEGER NOT NULL);
	CREATE INDEX files_expires ON files(expires);`,
	// 5: inboxes (webhook-to-note) and Telegram chats linked to them.
	`CREATE TABLE inboxes (token TEXT PRIMARY KEY, note_id INTEGER NOT NULL UNIQUE REFERENCES notes(id) ON DELETE CASCADE,
		pub TEXT NOT NULL, priv TEXT NOT NULL, created INTEGER NOT NULL);
	CREATE TABLE telegram_chats (chat_id INTEGER PRIMARY KEY, inbox TEXT NOT NULL REFERENCES inboxes(token) ON DELETE CASCADE,
		created INTEGER NOT NULL);`,
	// 6: link names of deleted notes are held for a while before anyone else
	// can take them (so an old link never opens a stranger's note).
	`CREATE TABLE slug_holds (slug TEXT PRIMARY KEY, owner_id INTEGER, until INTEGER NOT NULL);
	CREATE INDEX slug_holds_until ON slug_holds(until);`,
	// 7: one row per poll voter, so the server knows each voter's last choice
	// (instead of trusting the browser) and can limit votes per network.
	`CREATE TABLE poll_voters (note_id INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE, poll INTEGER NOT NULL,
		voter TEXT NOT NULL, option INTEGER NOT NULL, net TEXT NOT NULL, PRIMARY KEY (note_id, poll, voter));
	CREATE INDEX poll_voters_net ON poll_voters(note_id, poll, net);`,
	// 8: account recovery. vault_wrap is the vault key sealed with the key
	// from the password (or vault phrase), so the password can change without
	// changing the vault key; empty means the old way (the vault key is that
	// key). Each recovery code seals the vault key too; the server keeps only
	// SHA-256 of the code's proof and the sealed copy, never the code.
	`ALTER TABLE users ADD COLUMN vault_wrap TEXT NOT NULL DEFAULT '';
	CREATE TABLE recovery_codes (user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, slot INTEGER NOT NULL,
		proof_hash TEXT NOT NULL, wrap TEXT NOT NULL, used INTEGER NOT NULL DEFAULT 0, created INTEGER NOT NULL,
		PRIMARY KEY (user_id, slot));`,
	// 9: the control center's view of this app (HOP App Protocol). A site
	// role per account (admin or user, set from the account server's roles or
	// seeded from NOTEHOP_ADMINS), an account status (suspended accounts can't
	// sign in), an audit log of what the control center did, the ids of the
	// account server's events already handled (so retries are harmless), and
	// settings the control center may change at run time.
	`ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'user';
	ALTER TABLE users ADD COLUMN status TEXT NOT NULL DEFAULT 'active';
	CREATE TABLE hop_audit (id INTEGER PRIMARY KEY, at INTEGER NOT NULL, actor TEXT NOT NULL, app TEXT NOT NULL,
		action TEXT NOT NULL, target TEXT NOT NULL, detail TEXT NOT NULL DEFAULT '{}');
	CREATE TABLE hop_events_seen (id TEXT PRIMARY KEY, at INTEGER NOT NULL);
	CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated INTEGER NOT NULL);`,
	// 10: reports from readers ("Report this note"). No foreign key: a report
	// outlives the note it was about, so the record of a takedown stays.
	`CREATE TABLE reports (id INTEGER PRIMARY KEY, note_id INTEGER NOT NULL, token TEXT NOT NULL, reason TEXT NOT NULL,
		note TEXT NOT NULL DEFAULT '', ip_hash TEXT NOT NULL, created INTEGER NOT NULL, status TEXT NOT NULL DEFAULT 'open');
	CREATE INDEX reports_status ON reports(status, id);
	CREATE INDEX reports_ip ON reports(ip_hash, created);`,
	// 11: content warning classification (none, sensitive, adult) and creator note
	`ALTER TABLE notes ADD COLUMN content_warning TEXT NOT NULL DEFAULT 'none';
	ALTER TABLE notes ADD COLUMN warning_reason TEXT NOT NULL DEFAULT '';`,
	// 12: attachment budgets follow one draft, then one note (never a browser tab).
	`ALTER TABLE files ADD COLUMN scope TEXT NOT NULL DEFAULT '';
	CREATE INDEX files_scope ON files(scope, expires);
	CREATE TABLE file_drafts (scope TEXT PRIMARY KEY, note_id INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE);`,
}

// migrate applies pending migrations, each in its own transaction together
// with the version bump, so a crash never leaves one half applied.
func (d *DB) migrate() error {
	var v int
	if err := d.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	if v > len(migrations) {
		return fmt.Errorf("database is at schema version %d, newer than this binary (%d)", v, len(migrations))
	}
	for i := v; i < len(migrations); i++ {
		tx, err := d.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func now() int64 { return time.Now().Unix() }

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// ---------- notes ----------

type Note struct {
	UploadDraft    string // create-only capability hash; not note content or encryption key
	ID             int64
	OwnerID        sql.NullInt64
	OwnerBlob      string
	Encrypted      bool
	Password       bool
	Salt           string
	Content        string
	Version        int64
	Checks         string
	AdminHash      string
	WriteHash      string
	ParentID       sql.NullInt64
	Frozen         bool
	MaxViews       int64
	Views          int64
	Expires        int64
	Created        int64
	Updated        int64
	Config         string // JSON, decoded by the app layer
	LastOpen       int64  // last counted (non-admin) open
	ContentWarning string // 'none', 'sensitive', 'adult'
	WarningReason  string // optional creator note
}

func (n *Note) IsAdult() bool {
	return n.ContentWarning == "adult"
}

func (n *Note) IsSensitive() bool {
	return n.ContentWarning == "sensitive"
}

func (n *Note) HasWarning() bool {
	return n.IsAdult() || n.IsSensitive()
}

type Link struct {
	Token   string `json:"token"`
	NoteID  int64  `json:"-"`
	Mode    string `json:"mode"`
	Created int64  `json:"created"`
}

type Entry struct {
	ID      int64  `json:"id"`
	Content string `json:"content"`
	Created int64  `json:"created"`
}

const noteCols = `n.id, n.owner_id, n.owner_blob, n.encrypted, n.password, n.salt, n.content, n.version, n.checks,
	n.admin_hash, n.write_hash, n.parent_id, n.frozen, n.max_views, n.views, n.expires, n.created, n.updated, n.config, n.last_open,
	n.content_warning, n.warning_reason`

func scanNote(row interface{ Scan(...any) error }, extra ...any) (*Note, error) {
	n := &Note{}
	dest := append([]any{&n.ID, &n.OwnerID, &n.OwnerBlob, &n.Encrypted, &n.Password, &n.Salt, &n.Content, &n.Version, &n.Checks,
		&n.AdminHash, &n.WriteHash, &n.ParentID, &n.Frozen, &n.MaxViews, &n.Views, &n.Expires, &n.Created, &n.Updated, &n.Config, &n.LastOpen,
		&n.ContentWarning, &n.WarningReason}, extra...)
	if err := row.Scan(dest...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return n, nil
}

// CreateNote stores a note and its first link in one transaction.
func (d *DB) CreateNote(n *Note, link *Link) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	t := now()
	n.Created, n.Updated, n.Version = t, t, 1
	if n.Checks == "" {
		n.Checks = "{}"
	}
	if n.Config == "" {
		n.Config = "{}"
	}
	if n.ContentWarning == "" {
		n.ContentWarning = "none"
	}
	res, err := tx.Exec(`INSERT INTO notes (owner_id, owner_blob, encrypted, password, salt, content, version, checks,
		admin_hash, write_hash, parent_id, frozen, max_views, views, expires, created, updated, config, content_warning, warning_reason)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?)`,
		n.OwnerID, n.OwnerBlob, n.Encrypted, n.Password, n.Salt, n.Content, n.Version, n.Checks,
		n.AdminHash, n.WriteHash, n.ParentID, n.Frozen, n.MaxViews, n.Expires, n.Created, n.Updated, n.Config,
		n.ContentWarning, n.WarningReason)
	if err != nil {
		return err
	}
	n.ID, _ = res.LastInsertId()
	if n.UploadDraft != "" {
		if _, err := tx.Exec(`INSERT INTO file_drafts (scope, note_id) VALUES (?, ?)`, n.UploadDraft, n.ID); err != nil {
			if isUnique(err) {
				return ErrTaken
			}
			return err
		}
		if _, err := tx.Exec(`UPDATE files SET scope = ? WHERE scope = ?`, "note:"+strconv.FormatInt(n.ID, 10), n.UploadDraft); err != nil {
			return err
		}
	}
	link.NoteID, link.Created = n.ID, t
	if _, err := tx.Exec(`INSERT INTO links (token, note_id, mode, created) VALUES (?, ?, ?, ?)`,
		link.Token, link.NoteID, link.Mode, link.Created); err != nil {
		if isUnique(err) {
			return ErrTaken
		}
		return err
	}
	return tx.Commit()
}

// NoteByLink resolves a link token. Expired notes read as missing even before
// the janitor gets to them.
func (d *DB) NoteByLink(token string) (*Note, *Link, error) {
	l := &Link{Token: token}
	n, err := scanNote(d.QueryRow(`SELECT `+noteCols+`, l.mode, l.created FROM links l JOIN notes n ON n.id = l.note_id
		WHERE l.token = ? AND (n.expires = 0 OR n.expires > ?)`, token, now()), &l.Mode, &l.Created)
	if err != nil {
		return nil, nil, err
	}
	l.NoteID = n.ID
	return n, l, nil
}

// TakeView counts one read. For burn-after-reading notes it refuses once the
// limit is reached, so two readers can never both get the "last" view.
func (d *DB) TakeView(n *Note) (int64, error) {
	var views int64
	err := d.QueryRow(`UPDATE notes SET views = views + 1, last_open = ? WHERE id = ? AND (max_views = 0 OR views < max_views) RETURNING views`,
		now(), n.ID).Scan(&views)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return views, err
}

// UpdateContent replaces the body if nobody saved since baseVersion. Saving
// folds checklist ticks into the text, so the override map is cleared.
func (d *DB) UpdateContent(id, baseVersion int64, content string, ownerBlob *string) (int64, error) {
	tx, err := d.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var old string
	var oldVersion, updated int64
	if err := tx.QueryRow(`SELECT content, version, updated FROM notes WHERE id = ? AND frozen = 0`, id).Scan(&old, &oldVersion, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrConflict
		}
		return 0, err
	}
	if oldVersion != baseVersion {
		return 0, ErrConflict
	}
	q := `UPDATE notes SET content = ?, version = version + 1, checks = '{}', updated = ?`
	args := []any{content, now()}
	if ownerBlob != nil {
		q += `, owner_blob = ?`
		args = append(args, *ownerBlob)
	}
	q += ` WHERE id = ? AND version = ? RETURNING version`
	args = append(args, id, baseVersion)
	var v int64
	if err := tx.QueryRow(q, args...).Scan(&v); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrConflict
		}
		return 0, err
	}
	if err := keepVersion(tx, id, oldVersion, old, updated); err != nil {
		return 0, err
	}
	return v, tx.Commit()
}

// SetCheck records one checkbox state on top of the note's text.
func (d *DB) SetCheck(id int64, index int, checked bool) (map[string]bool, error) {
	tx, err := d.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var raw string
	if err := tx.QueryRow(`SELECT checks FROM notes WHERE id = ? AND frozen = 0`, id).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	m := map[string]bool{}
	_ = json.Unmarshal([]byte(raw), &m)
	m[strconv.Itoa(index)] = checked
	b, _ := json.Marshal(m)
	if _, err := tx.Exec(`UPDATE notes SET checks = ?, updated = ? WHERE id = ?`, string(b), now(), id); err != nil {
		return nil, err
	}
	return m, tx.Commit()
}

func (d *DB) DeleteNote(id int64) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := holdSlugs(tx, `n.id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM notes WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// Freeze copies the note as it is right now (text, ticks and appended entries)
// into a permanent, read-only snapshot reachable through a new read link.
func (d *DB) Freeze(n *Note, token string) (*Link, error) {
	tx, err := d.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	t := now()
	res, err := tx.Exec(`INSERT INTO notes (owner_id, owner_blob, encrypted, password, salt, content, version, checks,
		admin_hash, write_hash, parent_id, frozen, created, updated, content_warning, warning_reason)
		SELECT owner_id, owner_blob, encrypted, password, salt, content, version, checks, admin_hash, write_hash, id, 1, ?, ?, content_warning, warning_reason
		FROM notes WHERE id = ?`, t, t, n.ID)
	if err != nil {
		return nil, err
	}
	sid, _ := res.LastInsertId()
	if _, err := tx.Exec(`INSERT INTO entries (note_id, content, created) SELECT ?, content, created FROM entries WHERE note_id = ? ORDER BY id`,
		sid, n.ID); err != nil {
		return nil, err
	}
	l := &Link{Token: token, NoteID: sid, Mode: "read", Created: t}
	if _, err := tx.Exec(`INSERT INTO links (token, note_id, mode, created) VALUES (?, ?, ?, ?)`, l.Token, sid, l.Mode, t); err != nil {
		if isUnique(err) {
			return nil, ErrTaken
		}
		return nil, err
	}
	return l, tx.Commit()
}

// SetNoteWarning changes the content warning classification and optional note for moderation.
func (d *DB) SetNoteWarning(id int64, warning, reason string) error {
	_, err := d.Exec(`UPDATE notes SET content_warning = ?, warning_reason = ? WHERE id = ?`, warning, reason, id)
	return err
}

type Snapshot struct {
	Token   string `json:"token"`
	Created int64  `json:"created"`
}

func (d *DB) Snapshots(noteID int64) ([]Snapshot, error) {
	rows, err := d.Query(`SELECT (SELECT token FROM links WHERE note_id = n.id ORDER BY rowid LIMIT 1), n.created
		FROM notes n WHERE n.parent_id = ? ORDER BY n.id DESC`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Snapshot{}
	for rows.Next() {
		var s Snapshot
		var tok sql.NullString
		if err := rows.Scan(&tok, &s.Created); err != nil {
			return nil, err
		}
		if tok.Valid {
			s.Token = tok.String
			out = append(out, s)
		}
	}
	return out, rows.Err()
}

// ParentToken returns a link to the note a snapshot was frozen from, if it still exists.
func (d *DB) ParentToken(n *Note) string {
	if !n.ParentID.Valid {
		return ""
	}
	var tok string
	_ = d.QueryRow(`SELECT token FROM links WHERE note_id = ? ORDER BY rowid LIMIT 1`, n.ParentID.Int64).Scan(&tok)
	return tok
}

// ---------- links ----------

func (d *DB) Links(noteID int64) ([]Link, error) {
	rows, err := d.Query(`SELECT token, note_id, mode, created FROM links WHERE note_id = ? ORDER BY rowid`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Link{}
	for rows.Next() {
		var l Link
		if err := rows.Scan(&l.Token, &l.NoteID, &l.Mode, &l.Created); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (d *DB) AddLink(l *Link) error {
	l.Created = now()
	_, err := d.Exec(`INSERT INTO links (token, note_id, mode, created) VALUES (?, ?, ?, ?)`, l.Token, l.NoteID, l.Mode, l.Created)
	if isUnique(err) {
		return ErrTaken
	}
	return err
}

func (d *DB) DeleteLink(noteID int64, token string) error {
	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM links WHERE note_id = ?`, noteID).Scan(&count); err != nil {
		return err
	}
	if count <= 1 {
		return ErrLastLink
	}
	if err := holdSlugs(d, `l.note_id = ? AND l.token = ?`, noteID, token); err != nil {
		return err
	}
	res, err := d.Exec(`DELETE FROM links WHERE note_id = ? AND token = ?`, noteID, token)
	if err != nil {
		return err
	}
	if k, _ := res.RowsAffected(); k == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------- entries (append-only lines) ----------

func (d *DB) Entries(noteID int64) ([]Entry, error) {
	rows, err := d.Query(`SELECT id, content, created FROM entries WHERE note_id = ? ORDER BY id`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.Content, &e.Created); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (d *DB) CountEntries(noteID int64) (int, error) {
	var c int
	err := d.QueryRow(`SELECT COUNT(*) FROM entries WHERE note_id = ?`, noteID).Scan(&c)
	return c, err
}

func (d *DB) AddEntry(noteID int64, content string) (Entry, error) {
	e := Entry{Content: content, Created: now()}
	res, err := d.Exec(`INSERT INTO entries (note_id, content, created) VALUES (?, ?, ?)`, noteID, content, e.Created)
	if err != nil {
		return e, err
	}
	e.ID, _ = res.LastInsertId()
	_, err = d.Exec(`UPDATE notes SET updated = ? WHERE id = ?`, e.Created, noteID)
	return e, err
}

func (d *DB) DeleteEntry(noteID, id int64) error {
	res, err := d.Exec(`DELETE FROM entries WHERE note_id = ? AND id = ?`, noteID, id)
	if err != nil {
		return err
	}
	if k, _ := res.RowsAffected(); k == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------- users & sessions ----------

// User is a NoteHop account. Password accounts have an AuthHash; accounts
// that sign in through single sign-on have an SSOID instead and unlock their vault
// with a separate phrase, checked against VaultCheck.
type User struct {
	ID          int64
	Username    string
	AuthHash    string
	Created     int64
	SSOID       string
	Email       string
	DisplayName string
	Avatar      string
	VaultCheck  string
	VaultWrap   string // see migration 8
	Role        string // "admin" or "user" (migration 9)
	Status      string // "active" or "suspended"
}

// Account roles and statuses.
const (
	RoleAdmin       = "admin"
	RoleUser        = "user"
	StatusActive    = "active"
	StatusSuspended = "suspended"
)

const userCols = `u.id, u.username, u.auth_hash, u.created, COALESCE(u.sso_id, ''), u.email, u.display_name, u.avatar, u.vault_check, u.vault_wrap, u.role, u.status`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	u := &User{}
	err := row.Scan(&u.ID, &u.Username, &u.AuthHash, &u.Created, &u.SSOID, &u.Email, &u.DisplayName, &u.Avatar, &u.VaultCheck, &u.VaultWrap, &u.Role, &u.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (d *DB) CreateUser(username, authHash string) (*User, error) {
	u := &User{Username: username, AuthHash: authHash, Created: now(), Role: RoleUser, Status: StatusActive}
	res, err := d.Exec(`INSERT INTO users (username, auth_hash, created) VALUES (?, ?, ?)`, u.Username, u.AuthHash, u.Created)
	if isUnique(err) {
		return nil, ErrTaken
	}
	if err != nil {
		return nil, err
	}
	u.ID, _ = res.LastInsertId()
	return u, nil
}

// CreateSSOUser makes an account for a single sign-on user. It has no
// password (empty auth_hash never matches). ErrTaken means the username or the
// sign-on id is already in use.
func (d *DB) CreateSSOUser(u *User) error {
	u.Created = now()
	if u.Role == "" {
		u.Role = RoleUser
	}
	u.Status = StatusActive
	res, err := d.Exec(`INSERT INTO users (username, auth_hash, created, sso_id, email, display_name, avatar, role)
		VALUES (?, '', ?, ?, ?, ?, ?, ?)`, u.Username, u.Created, u.SSOID, u.Email, u.DisplayName, u.Avatar, u.Role)
	if isUnique(err) {
		return ErrTaken
	}
	if err != nil {
		return err
	}
	u.ID, _ = res.LastInsertId()
	return nil
}

// UpdateSSOProfile copies what the account server owns (email, name, photo).
func (d *DB) UpdateSSOProfile(u *User) error {
	_, err := d.Exec(`UPDATE users SET email = ?, display_name = ?, avatar = ? WHERE id = ?`, u.Email, u.DisplayName, u.Avatar, u.ID)
	return err
}

// SetVaultCheck stores the vault check once; ErrConflict if one is already set.
func (d *DB) SetVaultCheck(userID int64, hash string) error {
	res, err := d.Exec(`UPDATE users SET vault_check = ? WHERE id = ? AND vault_check = ''`, hash, userID)
	if err != nil {
		return err
	}
	if k, _ := res.RowsAffected(); k == 0 {
		return ErrConflict
	}
	return nil
}

func (d *DB) UserByName(username string) (*User, error) {
	return scanUser(d.QueryRow(`SELECT `+userCols+` FROM users u WHERE u.username = ?`, username))
}

func (d *DB) UserByID(id int64) (*User, error) {
	return scanUser(d.QueryRow(`SELECT `+userCols+` FROM users u WHERE u.id = ?`, id))
}

func (d *DB) UserBySSO(ssoID string) (*User, error) {
	return scanUser(d.QueryRow(`SELECT `+userCols+` FROM users u WHERE u.sso_id = ?`, ssoID))
}

func (d *DB) CreateSession(tokenHash string, userID int64, ttl time.Duration) error {
	_, err := d.Exec(`INSERT INTO sessions (token_hash, user_id, expires) VALUES (?, ?, ?)`, tokenHash, userID, time.Now().Add(ttl).Unix())
	return err
}

func (d *DB) UserBySession(tokenHash string) (*User, error) {
	return scanUser(d.QueryRow(`SELECT `+userCols+` FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires > ?`, tokenHash, now()))
}

func (d *DB) DeleteSession(tokenHash string) error {
	_, err := d.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// DeleteOtherSessions signs a user out everywhere except the session keep
// (empty: everywhere).
func (d *DB) DeleteOtherSessions(userID int64, keep string) error {
	_, err := d.Exec(`DELETE FROM sessions WHERE user_id = ? AND token_hash != ?`, userID, keep)
	return err
}

// SetSecret replaces how an account unlocks: the login hash (password
// accounts) or the vault phrase check (single sign-on), plus the sealed vault key.
func (d *DB) SetSecret(userID int64, sso bool, hash, vaultWrap string) error {
	col := "auth_hash"
	if sso {
		col = "vault_check"
	}
	_, err := d.Exec(`UPDATE users SET `+col+` = ?, vault_wrap = ? WHERE id = ?`, hash, vaultWrap, userID)
	return err
}

// RecoveryCode is one stored recovery code: SHA-256 of its proof and the
// vault key sealed with it. Used is when it was spent (0: still good).
type RecoveryCode struct {
	Slot      int
	ProofHash string
	Wrap      string
	Used      int64
	Created   int64
}

// SetRecoveryCodes replaces all of a user's recovery codes.
func (d *DB) SetRecoveryCodes(userID int64, codes []RecoveryCode) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM recovery_codes WHERE user_id = ?`, userID); err != nil {
		return err
	}
	t := now()
	for i, c := range codes {
		if _, err := tx.Exec(`INSERT INTO recovery_codes (user_id, slot, proof_hash, wrap, created) VALUES (?, ?, ?, ?, ?)`,
			userID, i, c.ProofHash, c.Wrap, t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) RecoveryCodes(userID int64) ([]RecoveryCode, error) {
	rows, err := d.Query(`SELECT slot, proof_hash, wrap, used, created FROM recovery_codes WHERE user_id = ? ORDER BY slot`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecoveryCode
	for rows.Next() {
		var c RecoveryCode
		if err := rows.Scan(&c.Slot, &c.ProofHash, &c.Wrap, &c.Used, &c.Created); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Recover spends a recovery code and sets the new secret in one go, and
// signs the account out everywhere. ErrConflict: the code was already spent.
func (d *DB) Recover(userID int64, slot int, sso bool, hash, vaultWrap string) error {
	col := "auth_hash"
	if sso {
		col = "vault_check"
	}
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE recovery_codes SET used = ? WHERE user_id = ? AND slot = ? AND used = 0`, now(), userID, slot)
	if err != nil {
		return err
	}
	if k, _ := res.RowsAffected(); k == 0 {
		return ErrConflict
	}
	if _, err := tx.Exec(`UPDATE users SET `+col+` = ?, vault_wrap = ? WHERE id = ?`, hash, vaultWrap, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

// OwnedNote is a dashboard row. Blob is the owner's vault-encrypted
// {title, key, admin} — the server can't read it.
type OwnedNote struct {
	Token    string `json:"token"`
	Mode     string `json:"mode"`
	Blob     string `json:"blob"`
	Frozen   bool   `json:"frozen"`
	Views    int64  `json:"views"`
	MaxViews int64  `json:"max_views"`
	Expires  int64  `json:"expires"`
	Created  int64  `json:"created"`
	Updated  int64  `json:"updated"`
	LastOpen int64  `json:"last_open"`
}

func (d *DB) NotesByOwner(userID int64) ([]OwnedNote, error) {
	rows, err := d.Query(`SELECT l.token, l.mode, n.owner_blob, n.frozen, n.views, n.max_views, n.expires, n.created, n.updated, n.last_open
		FROM notes n JOIN links l ON l.token = (SELECT token FROM links WHERE note_id = n.id ORDER BY rowid LIMIT 1)
		WHERE n.owner_id = ? AND (n.expires = 0 OR n.expires > ?) ORDER BY n.updated DESC LIMIT 500`, userID, now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OwnedNote{}
	for rows.Next() {
		var o OwnedNote
		if err := rows.Scan(&o.Token, &o.Mode, &o.Blob, &o.Frozen, &o.Views, &o.MaxViews, &o.Expires, &o.Created, &o.Updated, &o.LastOpen); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// Backup writes a consistent copy of the database to path while the server
// keeps running (VACUUM INTO also compacts it). path must not exist yet.
func (d *DB) Backup(path string) error {
	_, err := d.Exec(`VACUUM INTO ?`, path)
	return err
}

// SetConfig replaces a note's feature settings (JSON).
func (d *DB) SetConfig(id int64, config string) error {
	res, err := d.Exec(`UPDATE notes SET config = ? WHERE id = ?`, config, id)
	if err != nil {
		return err
	}
	if k, _ := res.RowsAffected(); k == 0 {
		return ErrNotFound
	}
	return nil
}

// Sweep removes expired notes, sessions and attachments.
func (d *DB) Sweep() (int64, error) {
	t := now()
	if err := holdSlugs(d, `n.expires > 0 AND n.expires <= ?`, t); err != nil {
		return 0, err
	}
	res, err := d.Exec(`DELETE FROM notes WHERE expires > 0 AND expires <= ?`, t)
	if err != nil {
		return 0, err
	}
	if _, err := d.Exec(`DELETE FROM slug_holds WHERE until <= ?`, t); err != nil {
		return 0, err
	}
	k, _ := res.RowsAffected()
	if _, err = d.Exec(`DELETE FROM sessions WHERE expires <= ?`, t); err != nil {
		return k, err
	}
	_, err = d.Exec(`DELETE FROM files WHERE expires <= ?`, t)
	return k, err
}
