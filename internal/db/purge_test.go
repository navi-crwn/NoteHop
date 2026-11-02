package db

import (
	"database/sql"
	"strconv"
	"testing"
	"time"
)

func newOwnedNote(t *testing.T, d *DB, owner int64, token string) *Note {
	t.Helper()
	n := &Note{OwnerID: sql.NullInt64{Int64: owner, Valid: true}, Content: `{"t":"","b":"x"}`, AdminHash: "a", WriteHash: "w"}
	if err := d.CreateNote(n, &Link{Token: token, Mode: "read"}); err != nil {
		t.Fatal(err)
	}
	return n
}

func addFile(t *testing.T, d *DB, id, scope string) {
	t.Helper()
	f := File{ID: id, Size: 4, Data: []byte("data"), Created: time.Now().Unix(), Expires: time.Now().Add(time.Hour).Unix(), Scope: scope}
	if ok, err := d.AddFile(f, 0); err != nil || !ok {
		t.Fatalf("add file: %v %v", ok, err)
	}
}

func hasFile(d *DB, id string) bool {
	_, err := d.GetFile(id)
	return err == nil
}

// An account's link names are held for it after the notes go. SQLite hands the
// id of a deleted last row to the next account, so the hold must not keep
// pointing at the number: a new account would inherit the old one's names.
func TestDeleteUserReleasesNameHolds(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	old, err := d.CreateUser("lama", "h")
	if err != nil {
		t.Fatal(err)
	}
	newOwnedNote(t, d, old.ID, "rapat-senin")
	if _, err := d.DeleteNotesByOwner(old.ID); err != nil {
		t.Fatal(err)
	}
	if owner, _, held := d.SlugHold("rapat-senin"); !held || owner.Int64 != old.ID {
		t.Fatalf("name should be held for the account: %v %v", owner, held)
	}
	if err := d.DeleteUser(old.ID); err != nil {
		t.Fatal(err)
	}
	fresh, err := d.CreateUser("baru", "h")
	if err != nil {
		t.Fatal(err)
	}
	owner, _, held := d.SlugHold("rapat-senin")
	if !held {
		t.Fatal("the name should stay reserved after the account is gone")
	}
	if owner.Valid && owner.Int64 == fresh.ID {
		t.Fatalf("a new account (id %d) inherited the deleted account's name hold", fresh.ID)
	}
}

// Attachments belong to their note: deleting the note (or its owner, or a
// takedown) removes them now, not when their 24 hours run out.
func TestDeleteNoteRemovesFiles(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	u, _ := d.CreateUser("lama", "h")
	scope := func(n *Note) string { return "note:" + strconv.FormatInt(n.ID, 10) }

	a := newOwnedNote(t, d, u.ID, "note-a")
	b := newOwnedNote(t, d, u.ID, "note-b")
	c := newOwnedNote(t, d, u.ID, "note-c")
	e := newOwnedNote(t, d, u.ID, "note-e")
	keep := newOwnedNote(t, d, u.ID, "note-keep")
	for i, n := range []*Note{a, b, c, e, keep} {
		addFile(t, d, "file-"+strconv.Itoa(i), scope(n))
	}
	addFile(t, d, "file-draft", "draft:abc")

	if err := d.DeleteNote(a.ID); err != nil {
		t.Fatal(err)
	}
	if err := d.Takedown(b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`UPDATE notes SET expires = ? WHERE id = ?`, time.Now().Unix()-1, e.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Sweep(); err != nil {
		t.Fatal(err)
	}
	for i, gone := range []bool{true, true, false, true, false} {
		if hasFile(d, "file-"+strconv.Itoa(i)) == gone {
			t.Errorf("file %d: gone=%v, want %v", i, !hasFile(d, "file-"+strconv.Itoa(i)), gone)
		}
	}
	if !hasFile(d, "file-draft") {
		t.Error("a draft's file belongs to no note yet and must stay")
	}

	// Purging an account takes the files of all its notes, and only theirs.
	other, _ := d.CreateUser("lain", "h")
	o := newOwnedNote(t, d, other.ID, "note-other")
	addFile(t, d, "file-other", scope(o))
	if _, err := d.DeleteNotesByOwner(u.ID); err != nil {
		t.Fatal(err)
	}
	if hasFile(d, "file-2") || hasFile(d, "file-4") {
		t.Error("files of a purged account's notes survived")
	}
	if !hasFile(d, "file-other") {
		t.Error("another account's file was removed")
	}
}
