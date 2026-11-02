//go:build unix

package db

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// A backup holds password hashes, session hashes and webhook keys: it must not
// be readable by other users, whatever umask the caller runs with.
func TestBackupPrivate(t *testing.T) {
	old := syscall.Umask(0o022)
	defer syscall.Umask(old)
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	out := filepath.Join(dir, "copy.db")
	if err := d.Backup(out); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("backup mode %v, want 0600", st.Mode().Perm())
	}
	// A failed backup leaves nothing behind.
	bad := filepath.Join(dir, "missing", "copy.db")
	if err := d.Backup(bad); err == nil {
		t.Fatal("backup into a missing folder must fail")
	}
	if _, err := os.Stat(bad); err == nil {
		t.Fatal("failed backup left a file")
	}
}
