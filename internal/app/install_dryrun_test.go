package app

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

// The installer says "nothing below is executed" on a dry run, so it must leave no folder or file behind.
func TestInstallDryRunCreatesNothing(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not installed")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	for _, run := range []string{"systemd", "docker"} {
		t.Run(run, func(t *testing.T) {
			dir, home := t.TempDir(), t.TempDir()
			for _, name := range []string{"install.sh", "go.mod"} {
				b, err := os.ReadFile(filepath.Join("..", "..", name))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command("bash", "install.sh", "-y", "--no-star", "--dry-run", "--run", run, "--data", filepath.Join(dir, "data"), "--port", "8695")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "HOME="+home)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("install.sh: %v\n%s", err, out)
			}
			var got []string
			filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
				if path != dir {
					got = append(got, d.Name())
				}
				return err
			})
			if !slices.Equal(got, []string{"go.mod", "install.sh"}) {
				t.Errorf("a dry run left things behind: %v", got)
			}
		})
	}
}
