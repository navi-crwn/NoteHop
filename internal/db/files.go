package db

import "database/sql"

// File is an attachment: AES-GCM ciphertext the browser made with a key the
// server never sees.
type File struct {
	ID      string
	Size    int64
	Data    []byte
	Created int64
	Expires int64
}

// AddFile stores an attachment unless that would take the total over limit
// bytes (0 = no limit). Checked in the same transaction as the insert, so
// parallel uploads can't overshoot it together.
func (d *DB) AddFile(f File, limit int64) (bool, error) {
	tx, err := d.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if limit > 0 {
		var used int64
		if err := tx.QueryRow(`SELECT COALESCE(SUM(size), 0) FROM files WHERE expires > ?`, now()).Scan(&used); err != nil {
			return false, err
		}
		if used+f.Size > limit {
			return false, nil
		}
	}
	if _, err := tx.Exec(`INSERT INTO files (id, size, data, created, expires) VALUES (?, ?, ?, ?, ?)`,
		f.ID, f.Size, f.Data, f.Created, f.Expires); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// GetFile returns an attachment that has not expired yet.
func (d *DB) GetFile(id string) (*File, error) {
	f := &File{ID: id}
	err := d.QueryRow(`SELECT size, data, created, expires FROM files WHERE id = ? AND expires > ?`, id, now()).
		Scan(&f.Size, &f.Data, &f.Created, &f.Expires)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return f, err
}

// Inbox lets outside services add entries to a note (webhook-to-note). Pub
// is the note's P-256 public key; Priv is its private key sealed by the
// browser with the note key, kept here only so every reader can fetch it.
type Inbox struct {
	Token  string
	NoteID int64
	Pub    string
	Priv   string
}

func (d *DB) SetInbox(in Inbox) error {
	_, err := d.Exec(`INSERT INTO inboxes (token, note_id, pub, priv, created) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(note_id) DO UPDATE SET token = excluded.token, pub = excluded.pub, priv = excluded.priv, created = excluded.created`,
		in.Token, in.NoteID, in.Pub, in.Priv, now())
	return err
}

func (d *DB) DeleteInbox(noteID int64) error {
	_, err := d.Exec(`DELETE FROM inboxes WHERE note_id = ?`, noteID)
	return err
}

func (d *DB) InboxOf(noteID int64) (*Inbox, error) {
	in := &Inbox{NoteID: noteID}
	err := d.QueryRow(`SELECT token, pub, priv FROM inboxes WHERE note_id = ?`, noteID).Scan(&in.Token, &in.Pub, &in.Priv)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return in, err
}

func (d *DB) InboxByToken(token string) (*Inbox, error) {
	in := &Inbox{Token: token}
	err := d.QueryRow(`SELECT note_id, pub, priv FROM inboxes WHERE token = ?`, token).Scan(&in.NoteID, &in.Pub, &in.Priv)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return in, err
}

// Telegram chats send their messages to one inbox each.
func (d *DB) LinkTelegram(chatID int64, inbox string) error {
	_, err := d.Exec(`INSERT INTO telegram_chats (chat_id, inbox, created) VALUES (?, ?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET inbox = excluded.inbox`, chatID, inbox, now())
	return err
}

func (d *DB) UnlinkTelegram(chatID int64) error {
	_, err := d.Exec(`DELETE FROM telegram_chats WHERE chat_id = ?`, chatID)
	return err
}

func (d *DB) TelegramInbox(chatID int64) (string, error) {
	var t string
	err := d.QueryRow(`SELECT inbox FROM telegram_chats WHERE chat_id = ?`, chatID).Scan(&t)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	return t, err
}

// NoteByID is for server-side jobs that start from a note id.
func (d *DB) NoteByID(id int64) (*Note, error) {
	return scanNote(d.QueryRow(`SELECT `+noteCols+` FROM notes n WHERE n.id = ?`, id))
}

// ProfileNotes are a user's public notes marked "on my profile".
func (d *DB) ProfileNotes(userID int64) ([]*Note, error) {
	rows, err := d.Query(`SELECT `+noteCols+` FROM notes n WHERE n.owner_id = ? AND n.encrypted = 0
		AND n.config LIKE '%"profile":true%' AND (n.expires = 0 OR n.expires > ?) ORDER BY n.updated DESC LIMIT 200`, userID, now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// SlugHoldDays is how long a link name stays reserved after its note or link
// is gone. Only the same account may take it back in that time.
const SlugHoldDays = 90

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// holdSlugs reserves the names of the links matching where (over links l
// joined with notes n). Random tokens almost always contain capitals, so
// only lowercase names, the custom ones, are held.
func holdSlugs(e execer, where string, args ...any) error {
	_, err := e.Exec(`INSERT OR REPLACE INTO slug_holds (slug, owner_id, until)
		SELECT l.token, n.owner_id, ? FROM links l JOIN notes n ON n.id = l.note_id
		WHERE `+where+` AND l.token NOT GLOB '*[A-Z]*'`, append([]any{now() + SlugHoldDays*86400}, args...)...)
	return err
}

// SlugHold reports whether a name is on hold, and for which account (if any).
func (d *DB) SlugHold(slug string) (owner sql.NullInt64, until int64, held bool) {
	err := d.QueryRow(`SELECT owner_id, until FROM slug_holds WHERE slug = ? AND until > ?`, slug, now()).Scan(&owner, &until)
	return owner, until, err == nil
}

// ReleaseSlug drops a hold once the name is in use again.
func (d *DB) ReleaseSlug(slug string) {
	d.Exec(`DELETE FROM slug_holds WHERE slug = ?`, slug)
}

// Takedown removes a reported note. Its link names are held for a year for
// everyone, its owner included, so the same content can't come straight back
// under the same address.
func (d *DB) Takedown(noteID int64) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT OR REPLACE INTO slug_holds (slug, owner_id, until)
		SELECT token, NULL, ? FROM links WHERE note_id = ? AND token NOT GLOB '*[A-Z]*'`, now()+365*86400, noteID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM notes WHERE id = ?`, noteID); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteFile removes an attachment before its 24 hours are up.
func (d *DB) DeleteFile(id string) (bool, error) {
	res, err := d.Exec(`DELETE FROM files WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	k, _ := res.RowsAffected()
	return k > 0, nil
}
