package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// A database made by the first release (no user_version, old users table)
// must upgrade in place and keep its rows.
func TestMigrateOldDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT NOT NULL UNIQUE, auth_hash TEXT NOT NULL, created INTEGER NOT NULL);
		INSERT INTO users (username, auth_hash, created) VALUES ('lama', 'h', 1);`); err != nil {
		t.Fatal(err)
	}
	old.Close()

	for i := 0; i < 2; i++ { // second open must be a no-op
		d, err := Open(path)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		var v int
		d.QueryRow(`PRAGMA user_version`).Scan(&v)
		if v != len(migrations) {
			t.Fatalf("user_version = %d, want %d", v, len(migrations))
		}
		u, err := d.UserByName("lama")
		if err != nil || u.SSOID != "" || u.AuthHash != "h" {
			t.Fatalf("old user: %+v %v", u, err)
		}
		d.Close()
	}
}

func TestMigrateRefusesNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	d.Exec(`PRAGMA user_version = 999`)
	d.Close()
	if _, err := Open(path); err == nil {
		t.Fatal("want error for a schema newer than the binary")
	}
}

func TestSSOUser(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	u := &User{Username: "budi", SSOID: "42", Email: "budi@example.com"}
	if err := d.CreateSSOUser(u); err != nil {
		t.Fatal(err)
	}
	if err := d.CreateSSOUser(&User{Username: "budi2", SSOID: "42"}); err != ErrTaken {
		t.Fatalf("duplicate sso id: %v", err)
	}
	if err := d.SetVaultCheck(u.ID, "x"); err != nil {
		t.Fatal(err)
	}
	if err := d.SetVaultCheck(u.ID, "y"); err != ErrConflict {
		t.Fatalf("second vault check: %v", err)
	}
	got, err := d.UserBySSO("42")
	if err != nil || got.VaultCheck != "x" || got.Email != "budi@example.com" {
		t.Fatalf("by sso id: %+v %v", got, err)
	}
}

func TestBackup(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.CreateUser("salin", "h"); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "copy.db")
	if err := d.Backup(out); err != nil {
		t.Fatal(err)
	}
	c, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.UserByName("salin"); err != nil {
		t.Fatalf("user missing from backup: %v", err)
	}
	if err := d.Backup(out); err == nil {
		t.Fatal("backup over an existing file must fail")
	}
}
