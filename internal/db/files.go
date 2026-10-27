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
