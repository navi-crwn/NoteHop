package app

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/web"
)

// The share dialog renames a link with Save and Cancel, and "Add link" only
// creates; the new-note page chooses between basic and encrypted.
func TestBasicAndEncryptedInTheUI(t *testing.T) {
	read := func(fsys fs.FS, path string) string {
		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		return string(b)
	}
	note, newPage := read(web.Templates, "templates/note.html"), read(web.Templates, "templates/new.html")
	noteJS, newJS := read(web.Static, "static/js/note.js"), read(web.Static, "static/js/new.js")
	for _, want := range []string{`id="add-link"`, `id="kind-switch"`, `id="kind-box"`} {
		if !strings.Contains(note, want) {
			t.Errorf("note.html lacks %s", want)
		}
	}
	for _, want := range []string{"function renameRow", "share.rename_save", "share.rename_cancel", "'/convert'", "convert.confirm_enc", "convert.confirm_basic"} {
		if !strings.Contains(noteJS, want) {
			t.Errorf("note.js lacks %s", want)
		}
	}
	// Renaming goes through its own request, not the create form.
	if !strings.Contains(noteJS, "api('PUT', '/links/'") {
		t.Error("rename must use PUT /links/{link}")
	}
	for _, want := range []string{`name="kind" value="encrypted"`, `name="kind" value="basic"`, `id="basic-slug"`} {
		if !strings.Contains(newPage, want) {
			t.Errorf("new.html lacks %s", want)
		}
	}
	if strings.Contains(newPage, `data-feature="slug"`) {
		t.Error("the link name is a basic-note field, not an advanced switch")
	}
	if !strings.Contains(newJS, "encrypted ? '' : slug.value.trim()") {
		t.Error("an encrypted note must never send a link name")
	}
}
