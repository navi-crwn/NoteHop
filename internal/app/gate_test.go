package app

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/internal/db"
)

func TestContentWarningGate(t *testing.T) {
	srv := newTestApp(t)
	c := newClient(t, srv)

	// 1. Create a note with adult content warning and reason
	n := c.create(map[string]any{
		"content_warning": "adult",
		"warning_reason":  "Contains adult themes",
	})

	// 2. Fetch metadata via GET /api/n/{token} without gate passed
	code, meta, _ := c.do("GET", "/api/n/"+n.token, nil, nil)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if meta["content_warning"] != "adult" {
		t.Fatalf("expected adult warning, got %v", meta["content_warning"])
	}
	if meta["warning_reason"] != "Contains adult themes" {
		t.Fatalf("expected reason, got %v", meta["warning_reason"])
	}
	if meta["gate_passed"] == true {
		t.Fatalf("expected gate_passed to be false")
	}

	// 3. Confirm gate via POST /gate/{token}
	form := url.Values{"remember": {"1"}}
	req, _ := http.NewRequest("POST", srv.URL+"/gate/"+n.token, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	// Cookie should now be set in client cookiejar
	codeAfter, metaAfter, _ := c.do("GET", "/api/n/"+n.token, nil, nil)
	if codeAfter != http.StatusOK {
		t.Fatalf("expected 200, got %d", codeAfter)
	}
	if metaAfter["gate_passed"] != true {
		t.Fatalf("expected gate_passed to be true after gate confirmation")
	}
}

func TestContentWarningModeration(t *testing.T) {
	srv, _, d := hopApp(t, Config{})

	n := &db.Note{
		Content:        `{"t":"Test Note","b":"Content"}`,
		Encrypted:      false,
		ContentWarning: "none",
	}
	l := &db.Link{Token: "modtok1", Mode: "read"}
	if err := d.CreateNote(n, l); err != nil {
		t.Fatal(err)
	}

	idStr := strconv.FormatInt(n.ID, 10)

	// Mark NSFW via HOP protocol
	code, env := call(t, srv, "imperium", "POST", "/hop/content/note/"+idStr+"/actions", map[string]any{
		"action": "mark_nsfw",
		"reason": "18+ Adult material",
	})
	if code != 200 {
		t.Fatalf("mark_nsfw: %d %v", code, env)
	}

	updated, err := d.NoteByID(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ContentWarning != "adult" || updated.WarningReason != "18+ Adult material" {
		t.Fatalf("expected adult, got %q / %q", updated.ContentWarning, updated.WarningReason)
	}

	// Mark sensitive
	code, env = call(t, srv, "imperium", "POST", "/hop/content/note/"+idStr+"/actions", map[string]any{
		"action": "mark_sensitive",
		"reason": "Graphic violence",
	})
	if code != 200 {
		t.Fatalf("mark_sensitive: %d %v", code, env)
	}

	updated, err = d.NoteByID(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ContentWarning != "sensitive" || updated.WarningReason != "Graphic violence" {
		t.Fatalf("expected sensitive, got %q / %q", updated.ContentWarning, updated.WarningReason)
	}

	// Unmark NSFW
	code, env = call(t, srv, "imperium", "POST", "/hop/content/note/"+idStr+"/actions", map[string]any{
		"action": "unmark_nsfw",
	})
	if code != 200 {
		t.Fatalf("unmark_nsfw: %d %v", code, env)
	}

	updated, err = d.NoteByID(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ContentWarning != "none" {
		t.Fatalf("expected none, got %q", updated.ContentWarning)
	}
}
