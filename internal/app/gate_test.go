package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/navi-crwn/NoteHop/internal/db"
)

func TestGateFormOriginProtection(t *testing.T) {
	for _, tc := range []struct {
		name, site, origin string
		allowed            bool
	}{
		{"same-origin opaque form", "same-origin", "null", true},
		{"cross-site form", "cross-site", "https://elsewhere.example", false},
		{"same-site sibling", "same-site", "https://sibling.example", false},
		{"opaque without metadata", "", "null", false},
		{"legacy same origin", "", "http://localhost", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "http://localhost/gate/test", nil)
			r.Header.Set("Sec-Fetch-Site", tc.site)
			r.Header.Set("Origin", tc.origin)
			if sameOrigin(r) != tc.allowed {
				t.Fatalf("origin decision: expected %v", tc.allowed)
			}
		})
	}
}

func TestGateProtectsVersionAndLiveSync(t *testing.T) {
	c := newClient(t, newTestApp(t))
	n := c.create(map[string]any{"mode": "edit", "content_warning": "sensitive"})
	w := map[string]string{"X-Write": n.write}
	if code, _, _ := c.do("PUT", "/api/n/"+n.token, map[string]any{"content": fakeCipher("second version"), "version": 1}, w); code != 200 {
		t.Fatalf("save: %d", code)
	}
	if code, _, _ := c.do("GET", "/api/n/"+n.token+"/versions/1", nil, w); code != 403 {
		t.Fatalf("history bypass: %d", code)
	}
	if _, hello := dialLive(t, c, n.token, n.write, ""); hello != nil {
		t.Fatalf("live bypass: %v", hello)
	}
	confirmNoteGate(t, c, n.token, false)
	if code, m, _ := c.do("GET", "/api/n/"+n.token+"/versions/1", nil, w); code != 200 || m["content"] != fakeCipher("hello") {
		t.Fatalf("confirmed history: %d %v", code, m)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(c.srv.URL, "http")+"/api/n/"+n.token+"/live", &websocket.DialOptions{HTTPClient: c.hc})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	auth, _ := json.Marshal(map[string]string{"t": "auth", "write": n.write})
	if err := conn.Write(ctx, websocket.MessageText, auth); err != nil {
		t.Fatal(err)
	}
	_, reply, err := conn.Read(ctx)
	if err != nil || !strings.Contains(string(reply), `"hello"`) {
		t.Fatalf("confirmed live: %s %v", reply, err)
	}
}

func TestGateProtectsPlaintextAndBurnViews(t *testing.T) {
	for _, endpoint := range []string{"raw", "open"} {
		t.Run(endpoint, func(t *testing.T) {
			c := newClient(t, newTestApp(t))
			n := c.create(map[string]any{"encrypted": false, "content": `{"t":"Private title","b":"gated body"}`, "content_warning": "sensitive", "max_views": 1})
			method, path := "GET", "/raw/"+n.token
			if endpoint == "open" {
				method, path = "POST", "/api/n/"+n.token+"/open"
			}
			if code, _, body := c.do(method, path, nil, nil); code != 403 || strings.Contains(body, "gated body") {
				t.Fatalf("before confirmation: %d %s", code, body)
			}
			if code, meta, _ := c.do("GET", "/api/n/"+n.token, nil, nil); code != 200 || meta["views"] != float64(0) {
				t.Fatalf("gate consumed a view: %d %v", code, meta)
			}
			confirmNoteGate(t, c, n.token, false)
			if code, _, body := c.do(method, path, nil, nil); code != 200 || !strings.Contains(body, "gated body") {
				t.Fatalf("after confirmation: %d %s", code, body)
			}
			if code, _, _ := c.do("GET", "/api/n/"+n.token, nil, nil); code != 404 {
				t.Fatalf("burn note remains after reading: %d", code)
			}
		})
	}
}

func confirmNoteGate(t *testing.T, c *client, token string, remember bool) {
	t.Helper()
	form := url.Values{}
	if remember {
		form.Set("remember", "1")
	}
	req, _ := http.NewRequest("POST", c.srv.URL+"/gate/"+token, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := c.hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("confirmation: %d %s", res.StatusCode, b)
	}
	for _, cookie := range res.Cookies() {
		if cookie.Name != gateCookieName(token) {
			continue
		}
		wantAge := 0
		if remember {
			wantAge = 86400
		}
		if cookie.MaxAge != wantAge || cookie.Domain != "" || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
			t.Fatalf("unexpected cookie attributes: %+v", cookie)
		}
		return
	}
	t.Fatal("missing gate cookie")
}

func TestGateNoJSAndCookieOptIn(t *testing.T) {
	for _, warning := range []string{"adult", "sensitive"} {
		for _, remember := range []bool{false, true} {
			c := newClient(t, newTestApp(t))
			n := c.create(map[string]any{"content_warning": warning})
			_, _, page := c.do("GET", "/"+n.token, nil, nil)
			start := strings.Index(page, "<noscript>")
			end := strings.Index(page, "</noscript>")
			if start < 0 || end < start || !strings.Contains(page[start:end], `action="/gate/`+n.token+`"`) {
				t.Fatal("missing no-JS confirmation form")
			}
			if warning == "sensitive" && strings.Contains(page[start:end], "18+") {
				t.Fatal("sensitive gate asserts adult age")
			}
			if code, _, _ := c.do("POST", "/gate/"+n.token, nil, map[string]string{"Origin": "https://elsewhere.example"}); code != 403 {
				t.Fatalf("cross-origin gate accepted: %d", code)
			}
			confirmNoteGate(t, c, n.token, remember)
		}
	}
}

func TestGateProtectsMockProfileAndSSH(t *testing.T) {
	a, _, c := newAppDB(t)
	c.do("POST", "/api/register", map[string]any{"username": "gateowner", "auth": key()}, nil)
	n := c.create(map[string]any{"encrypted": false, "content": `{"t":"Hidden title","b":"{\"private\":true}"}`, "content_warning": "adult"})
	c.do("POST", "/api/n/"+n.token+"/config", map[string]any{"profile": true}, map[string]string{"X-Admin": n.admin})
	if code, _, body := c.do("GET", "/@gateowner", nil, nil); code != 404 || strings.Contains(body, "Hidden title") {
		t.Fatalf("profile leaked warning note: %d", code)
	}
	if code, _, body := c.do("GET", "/mock/"+n.token, nil, nil); code != 403 || strings.Contains(body, `"private"`) {
		t.Fatalf("mock leaked warning note: %d %s", code, body)
	}
	if body := a.sshGet(n.token); strings.Contains(body, "Hidden title") || !strings.Contains(body, "content warning") {
		t.Fatalf("SSH bypass: %s", body)
	}
	confirmNoteGate(t, c, n.token, true)
	if code, _, body := c.do("GET", "/mock/"+n.token, nil, nil); code != 200 || !strings.Contains(body, `"private"`) {
		t.Fatalf("confirmed mock: %d %s", code, body)
	}
}

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
	openCode, _, _ := c.do("POST", "/api/n/"+n.token+"/open", map[string]any{}, nil)
	if openCode != http.StatusForbidden {
		t.Errorf("open before gate: got %d, want 403", openCode)
	}
	rawCode, _, _ := c.do("GET", "/raw/"+n.token, nil, nil)
	if rawCode != http.StatusForbidden {
		t.Errorf("raw before gate: got %d, want 403", rawCode)
	}
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
