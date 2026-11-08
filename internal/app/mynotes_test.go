package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/internal/hop"
)

// notesCall is one signed GET /api/v1/me/notes as app, acting for actor.
func notesCall(t *testing.T, srv *httptest.Server, app, secret, actor, path string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest("GET", srv.URL+path, nil)
	req.Header = hop.Headers(app, secret, actor, "GET", path, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var env map[string]any
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("%s: not JSON: %s", path, raw)
	}
	if res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control: %q", res.Header.Get("Cache-Control"))
	}
	return res.StatusCode, env
}

func noteItems(t *testing.T, env map[string]any) []map[string]any {
	t.Helper()
	d, _ := env["data"].(map[string]any)
	raw, _ := d["items"].([]any)
	out := []map[string]any{}
	for _, r := range raw {
		out = append(out, r.(map[string]any))
	}
	return out
}

func publicNote(title string) string {
	b, _ := json.Marshal(map[string]string{"t": title, "b": "body of " + title})
	return string(b)
}

func TestMyNotesForApp(t *testing.T) {
	srv, _, _ := hopApp(t, Config{NotesClients: []string{"biohop"}})
	ana := ssoUser(t, srv, "41", "ana@example.com", nil)
	budi := ssoUser(t, srv, "42", "budi@example.com", nil)

	readA := ana.create(map[string]any{"encrypted": false, "content": publicNote("Menu warung"), "mode": "read"})
	readB := ana.create(map[string]any{"encrypted": false, "content": publicNote("Jadwal kelas"), "mode": "read"})
	ana.create(map[string]any{"encrypted": false, "content": publicNote("Edit only"), "mode": "edit"})            // edit link only: never on a public page
	ana.create(map[string]any{"content": fakeCipher("secret body")})                                              // encrypted: never listed
	ana.create(map[string]any{"encrypted": false, "content": publicNote("Burn"), "mode": "read", "max_views": 1}) // burn after reading
	budi.create(map[string]any{"encrypted": false, "content": publicNote("Menu Budi"), "mode": "read"})

	// Only a listed app with a good signature.
	if code, env := notesCall(t, srv, "imperium", testSecret, "41", "/api/v1/me/notes"); code != 401 || env["error"] != "bad_signature" {
		t.Fatalf("unlisted app: %d %v", code, env)
	}
	if code, _ := notesCall(t, srv, "biohop", strings.Repeat("x", 40), "41", "/api/v1/me/notes"); code != 401 {
		t.Fatalf("wrong secret: %d", code)
	}
	if code, env := notesCall(t, srv, "biohop", testSecret, "", "/api/v1/me/notes"); code != 400 || env["error"] != "no_actor" {
		t.Fatalf("no actor: %d %v", code, env)
	}
	if code, env := notesCall(t, srv, "biohop", testSecret, "local:1", "/api/v1/me/notes"); code != 400 || env["error"] != "no_actor" {
		t.Fatalf("local id as actor: %d %v", code, env)
	}
	if code, env := notesCall(t, srv, "biohop", testSecret, "999", "/api/v1/me/notes"); code != 404 || env["error"] != "no_account" {
		t.Fatalf("unknown member: %d %v", code, env)
	}
	// A browser with a session gets nothing: this route is server to server only.
	if code, _, _ := ana.do("GET", "/api/v1/me/notes", nil, nil); code != 401 {
		t.Fatalf("session caller: %d", code)
	}

	code, env := notesCall(t, srv, "biohop", testSecret, "41", "/api/v1/me/notes")
	if code != 200 || env["ok"] != true {
		t.Fatalf("list: %d %v", code, env)
	}
	items := noteItems(t, env)
	if len(items) != 2 || items[0]["title"] != "Jadwal kelas" || items[1]["title"] != "Menu warung" {
		t.Fatalf("items: %v", items)
	}
	if items[0]["url"] != "https://note.test/"+readB.token || items[1]["id"] != readA.token {
		t.Fatalf("urls: %v", items)
	}
	raw, _ := json.Marshal(env)
	for _, leak := range []string{"Edit only", "Burn", "Menu Budi", "secret body", "#", "admin", "write"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("%q must not be in %s", leak, raw)
		}
	}

	// Search by title words, case-insensitive; pages by cursor.
	_, env = notesCall(t, srv, "biohop", testSecret, "41", "/api/v1/me/notes?q=WARUNG")
	if it := noteItems(t, env); len(it) != 1 || it[0]["title"] != "Menu warung" {
		t.Fatalf("q: %v", env)
	}
	_, env = notesCall(t, srv, "biohop", testSecret, "41", "/api/v1/me/notes?limit=1")
	first := noteItems(t, env)
	next := env["data"].(map[string]any)["next"].(string)
	if len(first) != 1 || next == "" {
		t.Fatalf("page 1: %v", env)
	}
	if _, err := strconv.ParseInt(next, 10, 64); err != nil {
		t.Fatalf("next: %q", next)
	}
	_, env = notesCall(t, srv, "biohop", testSecret, "41", "/api/v1/me/notes?limit=1&cursor="+next)
	second := noteItems(t, env)
	if len(second) != 1 || second[0]["id"] == first[0]["id"] || env["data"].(map[string]any)["next"] != "" {
		t.Fatalf("page 2: %v", env)
	}

	// Budi sees only his own.
	_, env = notesCall(t, srv, "biohop", testSecret, "42", "/api/v1/me/notes")
	if it := noteItems(t, env); len(it) != 1 || it[0]["title"] != "Menu Budi" {
		t.Fatalf("budi: %v", env)
	}
}

func TestMyNotesOwnSecretAndOff(t *testing.T) {
	own := strings.Repeat("b", 40)
	srv, _, _ := hopApp(t, Config{NotesClients: []string{"biohop"}, NotesSecrets: map[string]string{"biohop": own}})
	ssoUser(t, srv, "41", "ana@example.com", nil)
	if code, _ := notesCall(t, srv, "biohop", testSecret, "41", "/api/v1/me/notes"); code != 401 {
		t.Fatalf("shared secret when the app has its own: %d", code)
	}
	if code, _ := notesCall(t, srv, "biohop", own, "41", "/api/v1/me/notes"); code != 200 {
		t.Fatalf("own secret: %d", code)
	}

	off, _, _ := hopApp(t, Config{})
	if code, env := notesCall(t, off, "biohop", testSecret, "41", "/api/v1/me/notes"); code != 401 || env["error"] != "bad_signature" {
		t.Fatalf("no notes clients configured: %d %v", code, env)
	}
}

func TestMyNotesPickerSecret(t *testing.T) {
	picker := strings.Repeat("p", 40)
	own := strings.Repeat("b", 40)
	srv, _, _ := hopApp(t, Config{NotesClients: []string{"biohop"}, NotesSecrets: map[string]string{"biohop": own}, PickerSecret: picker})
	ssoUser(t, srv, "41", "ana@example.com", nil)
	if code, _ := notesCall(t, srv, "biohop", picker, "41", "/api/v1/me/notes"); code != 200 {
		t.Fatalf("picker secret: %d", code)
	}
	if code, _ := notesCall(t, srv, "biohop", own, "41", "/api/v1/me/notes"); code != 200 {
		t.Fatalf("the app's own secret keeps working during the move: %d", code)
	}
	if code, _ := notesCall(t, srv, "biohop", testSecret, "41", "/api/v1/me/notes"); code != 401 {
		t.Fatalf("the shared SSO secret is not accepted when the app has its own: %d", code)
	}

	short, _, _ := hopApp(t, Config{NotesClients: []string{"biohop"}, PickerSecret: "short"})
	ssoUser(t, short, "41", "ana@example.com", nil)
	if code, _ := notesCall(t, short, "biohop", "short", "41", "/api/v1/me/notes"); code != 401 {
		t.Fatalf("a short picker secret must not verify anything: %d", code)
	}
}
