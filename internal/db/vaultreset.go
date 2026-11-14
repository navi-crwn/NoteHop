package db

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// VaultResetCounts is what a vault reset takes (before it runs) or took (after).
type VaultResetCounts struct {
	Notes   int64 `json:"notes"`   // encrypted notes of the account (frozen copies included): they go
	Kept    int64 `json:"kept"`    // basic notes of the account: they stay, readable; only their sealed owner blob goes
	Files   int64 `json:"files"`   // attachments of those notes, and unused uploads of the account
	Links   int64 `json:"links"`   // share links of those notes (they stop working with the notes)
	Margins int64 `json:"margins"` // private margin notes
	Codes   int64 `json:"codes"`   // recovery codes
}

// VaultResetCounts counts what ResetVault would delete for this account.
func (d *DB) VaultResetCounts(userID int64) (VaultResetCounts, error) {
	return countVaultReset(d, userID)
}

func countVaultReset(q interface {
	QueryRow(query string, args ...any) *sql.Row
}, userID int64) (VaultResetCounts, error) {
	var c VaultResetCounts
	err := q.QueryRow(`SELECT
		(SELECT COUNT(*) FROM notes WHERE owner_id = ?1 AND encrypted = 1),
		(SELECT COUNT(*) FROM notes WHERE owner_id = ?1 AND encrypted = 0),
		(SELECT COUNT(*) FROM files WHERE scope IN (SELECT 'note:' || n.id FROM notes n WHERE n.owner_id = ?1 AND n.encrypted = 1)
			OR (owner_id = ?1 AND scope GLOB 'draft:*')),
		(SELECT COUNT(*) FROM links l JOIN notes n ON n.id = l.note_id WHERE n.owner_id = ?1 AND n.encrypted = 1),
		(SELECT COUNT(*) FROM margins WHERE user_id = ?1),
		(SELECT COUNT(*) FROM recovery_codes WHERE user_id = ?1)`, userID).
		Scan(&c.Notes, &c.Kept, &c.Files, &c.Links, &c.Margins, &c.Codes)
	return c, err
}

// ResetVault wipes everything an account sealed with a vault phrase it no longer knows, and lets it
// start again with a new one. Only for accounts that sign in through a HOP ID (the vault phrase is
// separate from the password there); an account whose vault key comes from its password cannot use it.
//
// It deletes, in ONE transaction (any failure leaves the account exactly as it was):
//   - every ENCRYPTED note of the account with its attachments, share links, entries, versions and the rest that
//     hangs on a note (the link names are held for the account, as after any deletion). BASIC notes stay: the server
//     reads them anyway, the owner can still delete them, and "Edit" in My notes gives them new keys (takeover.go);
//     only their sealed owner blob, which held keys that are gone, is emptied,
//   - the account's uploads that no note claimed,
//   - its private margin notes,
//   - its recovery codes (each one sealed the old vault key),
//   - the phrase check and the sealed vault key, so /vault asks for a new phrase, and the vault generation goes up,
//   - the reminder state, so "make recovery codes" comes back,
//   - every session except keep.
//
// It returns what went. The vault key itself was never on the server, so there is nothing else to wipe.
// The record of it (audit, when not nil) is written in the same transaction: no record, no reset.
func (d *DB) ResetVault(userID int64, keep string, audit *Audit) (VaultResetCounts, error) {
	tx, err := d.Begin()
	if err != nil {
		return VaultResetCounts{}, err
	}
	defer tx.Rollback()
	var sso string
	if err := tx.QueryRow(`SELECT COALESCE(sso_id, '') FROM users WHERE id = ?`, userID).Scan(&sso); err != nil {
		return VaultResetCounts{}, ErrNotFound
	}
	if sso == "" {
		return VaultResetCounts{}, errors.New("db: vault reset is for HOP ID accounts")
	}
	c, err := countVaultReset(tx, userID)
	if err != nil {
		return VaultResetCounts{}, err
	}
	steps := []func() error{
		func() error { return holdSlugs(tx, `n.owner_id = ? AND n.encrypted = 1`, userID) },
		func() error { return dropNoteFiles(tx, `n.owner_id = ? AND n.encrypted = 1`, userID) },
		func() error {
			_, err := tx.Exec(`DELETE FROM files WHERE owner_id = ? AND scope GLOB 'draft:*'`, userID)
			return err
		},
		func() error {
			_, err := tx.Exec(`DELETE FROM notes WHERE owner_id = ? AND encrypted = 1`, userID)
			return err
		},
		// a basic note stays (the server can read it anyway); the blob that held its old keys is unreadable now
		func() error {
			_, err := tx.Exec(`UPDATE notes SET owner_blob = '' WHERE owner_id = ? AND encrypted = 0`, userID)
			return err
		},
		func() error { _, err := tx.Exec(`DELETE FROM margins WHERE user_id = ?`, userID); return err },
		func() error { _, err := tx.Exec(`DELETE FROM recovery_codes WHERE user_id = ?`, userID); return err },
		func() error { _, err := tx.Exec(`DELETE FROM reminders WHERE user_id = ?`, userID); return err },
		func() error {
			_, err := tx.Exec(`UPDATE users SET vault_check = '', vault_wrap = '', vault_epoch = vault_epoch + 1 WHERE id = ?`, userID)
			return err
		},
		func() error {
			_, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ? AND token_hash != ?`, userID, keep)
			return err
		},
	}
	if audit != nil {
		steps = append(steps, func() error {
			detail, _ := json.Marshal(c)
			_, err := tx.Exec(`INSERT INTO hop_audit (at, actor, app, action, target, detail) VALUES (?, ?, ?, ?, ?, ?)`,
				now(), audit.Actor, audit.App, audit.Action, audit.Target, string(detail))
			return err
		})
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return VaultResetCounts{}, err
		}
	}
	return c, tx.Commit()
}

// SetNoteKeys swaps the admin and write hashes of a BASIC, live (not frozen) note owned by this account, and empties
// the sealed owner blob, which held the keys being replaced. ErrNotFound when no such note matches.
func (d *DB) SetNoteKeys(noteID, ownerID int64, adminHash, writeHash string) error {
	return deleted(d.Exec(`UPDATE notes SET admin_hash = ?, write_hash = ?, owner_blob = '', updated = updated
		WHERE id = ? AND owner_id = ? AND encrypted = 0 AND frozen = 0`, adminHash, writeHash, noteID, ownerID))
}
