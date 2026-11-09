package app

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/web"
)

// Every question goes through NH.confirm / NH.prompt (app.js), which use the
// HOP family's shared dialogs and fall back to the browser's own only when
// hop-ui.js is not there. A bare confirm() or prompt() anywhere else would
// bring the browser's dialog back for good.
func TestNoNativeDialogsOutsideTheFallback(t *testing.T) {
	bare := regexp.MustCompile(`(^|[^.\w])(confirm|prompt|alert)\(`)
	files, err := fs.Glob(web.Static, "static/js/*.js")
	if err != nil || len(files) < 10 {
		t.Fatalf("scripts: %v %v", files, err)
	}
	for _, path := range files {
		b, _ := fs.ReadFile(web.Static, path)
		for i, line := range strings.Split(string(b), "\n") {
			if !bare.MatchString(line) {
				continue
			}
			// ai.js has a function of its own called prompt (the text sent to the model).
			if strings.HasSuffix(path, "/ai.js") && strings.Contains(line, "prompt(task") {
				continue
			}
			t.Errorf("%s:%d: a native dialog: %s", path, i+1, strings.TrimSpace(line))
		}
	}
	b, _ := fs.ReadFile(web.Static, "static/js/app.js")
	app := string(b)
	for _, want := range []string{"window.HopUI && HopUI.confirm", "window.confirm(", "window.HopUI && HopUI.prompt", "window.prompt("} {
		if !strings.Contains(app, want) {
			t.Errorf("app.js lacks %q (shared dialog first, the browser's own as the fallback)", want)
		}
	}
}
