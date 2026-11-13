package db

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// An account with everything a vault protects, and a second account that must stay exactly as it was.
type vaultFixture struct {
	d          *DB
	a, b       *User
	keepA      string // the session of A that survives the reset
	aNotes     []*Note
	bNote      *Note
	frozenLink *Link
}

func seedVault(t *testing.T) *vaultFixture {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	mk := func(name, sso string) *User {
		u := &User{Username: name, SSOID: sso, Email: name + "@example.test"}
		if err := d.CreateSSOUser(u); err != nil {
			t.Fatal(err)
		}
		if err := d.SetVaultCheck(u.ID, "check-"+name); err != nil {
			t.Fatal(err)
		}
		if err := d.SetSecret(u.ID, true, "check-"+name, "wrap-"+name); err != nil {
			t.Fatal(err)
		}
		if err := d.SetRecoveryCodes(u.ID, []RecoveryCode{{Slot: 0, ProofHash: "p0" + name, Wrap: "w0"}, {Slot: 1, ProofHash: "p1" + name, Wrap: "w1"}}); err != nil {
			t.Fatal(err)
		}
		if err := d.SetMargin(u.ID, "ref-"+name, "private words"); err != nil {
			t.Fatal(err)
		}
		if err := d.DismissReminder(u.ID, "recovery-codes"); err != nil {
			t.Fatal(err)
		}
		return u
	}
	f := &vaultFixture{d: d, a: mk("helena", "sso-helena"), b: mk("budi", "sso-budi")}
	for _, tok := range []string{"helena-one", "helena-two"} {
		n := newOwnedNote(t, d, f.a.ID, tok)
		f.aNotes = append(f.aNotes, n)
	}
	// a second share link for the first note, a frozen copy (it keeps the owner), an attachment of a note,
	// and an upload that no note has claimed yet
	if err := d.AddLink(&Link{Token: "extra-LINK-a", NoteID: f.aNotes[0].ID, Mode: "read"}); err != nil {
		t.Fatal(err)
	}
	fl, err := d.Freeze(f.aNotes[0], "frozen-copy-a")
	if err != nil {
		t.Fatal(err)
	}
	f.frozenLink = fl
	addFile(t, d, "fileA-note", "note:"+itoa(f.aNotes[0].ID))
	addFile(t, d, "fileA-draft", "draft:abc")
	if _, err := d.Exec(`UPDATE files SET owner_id = ? WHERE id = 'fileA-draft'`, f.a.ID); err != nil {
		t.Fatal(err)
	}
	f.bNote = newOwnedNote(t, d, f.b.ID, "budi-one")
	addFile(t, d, "fileB-note", "note:"+itoa(f.bNote.ID))
	addFile(t, d, "fileB-draft", "draft:budi")
	if _, err := d.Exec(`UPDATE files SET owner_id = ? WHERE id = 'fileB-draft'`, f.b.ID); err != nil {
		t.Fatal(err)
	}
	for _, s := range []struct {
		hash string
		u    *User
	}{{"sess-a-keep", f.a}, {"sess-a-other", f.a}, {"sess-b", f.b}} {
		if err := d.CreateSession(s.hash, s.u.ID, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	f.keepA = "sess-a-keep"
	return f
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func (f *vaultFixture) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := f.d.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The reset takes every trace of what the old phrase sealed, from every table the account has rows in,
// and leaves another account untouched.
func TestResetVaultDeletesEverythingOfTheAccountAndNothingElse(t *testing.T) {
	f := seedVault(t)
	before, err := f.d.VaultResetCounts(f.a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Notes != 3 || before.Links != 4 || before.Files != 2 || before.Margins != 1 || before.Codes != 2 {
		t.Fatalf("what the screen will say it takes: %+v (want 3 notes incl. the frozen copy, 4 links, 2 files, 1 margin, 2 codes)", before)
	}
	got, err := f.d.ResetVault(f.a.ID, f.keepA)
	if err != nil {
		t.Fatal(err)
	}
	if got != before {
		t.Fatalf("it took %+v but said it would take %+v", got, before)
	}

	for table, q := range map[string]string{
		"notes":          `SELECT COUNT(*) FROM notes WHERE owner_id = ?`,
		"links":          `SELECT COUNT(*) FROM links WHERE token IN ('helena-one','helena-two','extra-LINK-a','frozen-copy-a')`,
		"margins":        `SELECT COUNT(*) FROM margins WHERE user_id = ?`,
		"recovery_codes": `SELECT COUNT(*) FROM recovery_codes WHERE user_id = ?`,
		"reminders":      `SELECT COUNT(*) FROM reminders WHERE user_id = ?`,
		"files":          `SELECT COUNT(*) FROM files WHERE id IN ('fileA-note','fileA-draft')`,
	} {
		var n int
		if strings.Contains(q, "?") {
			n = f.count(t, q, f.a.ID)
		} else {
			n = f.count(t, q)
		}
		if n != 0 {
			t.Errorf("%s: %d rows of the account are left", table, n)
		}
	}
	// the old links are gone like any deleted note's: nothing opens, and the names are held for the account
	for _, tok := range []string{"helena-one", "helena-two", "extra-LINK-a", "frozen-copy-a"} {
		if _, _, err := f.d.NoteByLink(tok); err == nil {
			t.Errorf("link %s still opens a note", tok)
		}
	}
	if owner, _, held := f.d.SlugHold("helena-one"); !held || owner.Int64 != f.a.ID {
		t.Errorf("the link name was not held for the account: %v %v", owner, held)
	}
	u, err := f.d.UserByID(f.a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if u.VaultCheck != "" || u.VaultWrap != "" {
		t.Errorf("the unlock secrets are still there: check=%q wrap=%q", u.VaultCheck, u.VaultWrap)
	}
	if u.SSOID != "sso-helena" {
		t.Errorf("the account itself changed: %q", u.SSOID)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM sessions WHERE user_id = ?`, f.a.ID); n != 1 {
		t.Errorf("sessions left: %d, want only the one used to reset", n)
	}
	if _, err := f.d.SessionVerifiedAt(f.keepA); err != nil {
		t.Errorf("the session used to reset was ended: %v", err)
	}

	// the other account is exactly as before
	if n := f.count(t, `SELECT COUNT(*) FROM notes WHERE owner_id = ?`, f.b.ID); n != 1 {
		t.Errorf("other account's notes: %d", n)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM files WHERE id IN ('fileB-note','fileB-draft')`); n != 2 {
		t.Errorf("other account's files: %d", n)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM margins WHERE user_id = ?`, f.b.ID); n != 1 {
		t.Errorf("other account's margins: %d", n)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM recovery_codes WHERE user_id = ?`, f.b.ID); n != 2 {
		t.Errorf("other account's recovery codes: %d", n)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM sessions WHERE user_id = ?`, f.b.ID); n != 1 {
		t.Errorf("other account's sessions: %d", n)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM reminders WHERE user_id = ?`, f.b.ID); n != 1 {
		t.Errorf("other account's reminders: %d", n)
	}
	ub, _ := f.d.UserByID(f.b.ID)
	if ub.VaultCheck != "check-budi" || ub.VaultWrap != "wrap-budi" {
		t.Errorf("other account's unlock secrets changed: %q %q", ub.VaultCheck, ub.VaultWrap)
	}
	if _, _, err := f.d.NoteByLink("budi-one"); err != nil {
		t.Errorf("other account's note no longer opens: %v", err)
	}
}

// One transaction: a failure in the middle (here a delete that is refused after the notes are already gone)
// leaves the account exactly as it was.
func TestResetVaultIsOneTransaction(t *testing.T) {
	f := seedVault(t)
	if _, err := f.d.Exec(`CREATE TRIGGER refuse_codes BEFORE DELETE ON recovery_codes BEGIN SELECT RAISE(ABORT, 'refused'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.d.ResetVault(f.a.ID, f.keepA); err == nil {
		t.Fatal("the reset reported success although a step failed")
	}
	after, err := f.d.VaultResetCounts(f.a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Notes != 3 || after.Files != 2 || after.Links != 4 || after.Margins != 1 || after.Codes != 2 {
		t.Fatalf("a failed reset changed the account: %+v", after)
	}
	u, _ := f.d.UserByID(f.a.ID)
	if u.VaultCheck != "check-helena" || u.VaultWrap != "wrap-helena" {
		t.Fatalf("a failed reset changed the unlock secrets: %q %q", u.VaultCheck, u.VaultWrap)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM sessions WHERE user_id = ?`, f.a.ID); n != 2 {
		t.Fatalf("a failed reset ended sessions: %d", n)
	}
	if _, _, held := f.d.SlugHold("helena-one"); held {
		t.Fatal("a failed reset held the link names")
	}
}

// An account whose vault key comes from its password cannot use it, and nothing is deleted.
func TestResetVaultRefusesPasswordAccounts(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	u, err := d.CreateUser("lokal", "hash")
	if err != nil {
		t.Fatal(err)
	}
	newOwnedNote(t, d, u.ID, "lokal-note")
	if _, err := d.ResetVault(u.ID, ""); err == nil {
		t.Fatal("a password account was reset")
	}
	if _, _, err := d.NoteByLink("lokal-note"); err != nil {
		t.Fatalf("a refused reset deleted the note: %v", err)
	}
	if _, err := d.ResetVault(987654, ""); err != ErrNotFound {
		t.Fatalf("unknown account: %v", err)
	}
}
