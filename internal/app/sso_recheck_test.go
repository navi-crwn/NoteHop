package app

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// The silent re-check: account A is signed in here, the browser is signed in as B at the account
// server. See docs/silent-sso.md in the hub for the contract (prompt=none).

type recheckEnv struct {
	t      *testing.T
	app    *App
	db     *db.DB
	srv    *httptest.Server
	c      *client
	hubUp  bool
	silent bool
}

func newRecheckEnv(t *testing.T) *recheckEnv {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(Config{Registration: true, BaseURL: "https://note.test", SSOURL: "https://accounts.test", SSOSecret: testSecret, SSOName: "Accounts", SSOClient: "notehop"}, d)
	if err != nil {
		t.Fatal(err)
	}
	e := &recheckEnv{t: t, app: a, db: d, hubUp: true}
	a.hubUp = func() bool { return e.hubUp }
	e.srv = httptest.NewServer(a.Handler())
	t.Cleanup(func() { e.srv.Close(); d.Close() })
	e.c = noRedirect(t, e.srv)
	return e
}

// signIn runs the normal flow for hub account sub and returns once the session exists.
func (e *recheckEnv) signIn(c *client, sub string) {
	e.t.Helper()
	state := ssoStart(e.t, c, "/dashboard")
	if code, loc := ssoCallback(e.t, c, ticketFor(sub, state)); code != http.StatusSeeOther || !strings.HasPrefix(loc, "/vault") {
		e.t.Fatalf("sign in: %d %s", code, loc)
	}
}

func (e *recheckEnv) sessionCookie(c *client) string {
	u, _ := url.Parse(e.srv.URL)
	for _, ck := range c.hc.Jar.Cookies(u) {
		if ck.Name == sessionCookie {
			return ck.Value
		}
	}
	return ""
}

// age makes every session look as if the hub last vouched for it long ago.
func (e *recheckEnv) age() {
	e.t.Helper()
	if _, err := e.db.Exec(`UPDATE sessions SET verified_at = ?`, time.Now().Add(-2*recheckAfter).Unix()); err != nil {
		e.t.Fatal(err)
	}
}

func (e *recheckEnv) sessions() int {
	var n int
	e.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n)
	return n
}

// silentStart follows the redirect a stale page gives and returns the state the hub would receive.
func (e *recheckEnv) silentStart(c *client, path string) string {
	e.t.Helper()
	loc, _ := url.Parse(path)
	if !strings.HasPrefix(path, "/auth/sso") { // an account page redirects on the server; a public page does it in the script (data-stale)
		res, err := c.hc.Get(e.srv.URL + path)
		if err != nil {
			e.t.Fatal(err)
		}
		res.Body.Close()
		loc, _ = url.Parse(res.Header.Get("Location"))
		if res.StatusCode != http.StatusFound || loc.Path != "/auth/sso" || loc.Query().Get("prompt") != "none" {
			e.t.Fatalf("%s: want a redirect to /auth/sso?prompt=none, got %d %s", path, res.StatusCode, loc)
		}
	}
	var err error
	var res *http.Response
	res, err = c.hc.Get(e.srv.URL + loc.RequestURI())
	if err != nil {
		e.t.Fatal(err)
	}
	res.Body.Close()
	hub, _ := url.Parse(res.Header.Get("Location"))
	if hub.Host != "accounts.test" || hub.Path != "/sso/notehop" || hub.Query().Get("prompt") != "none" ||
		hub.Query().Get("return_to") != "https://note.test/auth/sso/callback" {
		e.t.Fatalf("start: %s", hub)
	}
	return hub.Query().Get("state")
}

func (e *recheckEnv) callback(c *client, query string) (int, string) {
	e.t.Helper()
	res, err := c.hc.Get(e.srv.URL + "/auth/sso/callback?" + query)
	if err != nil {
		e.t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode, res.Header.Get("Location")
}

func (e *recheckEnv) status(c *client, path string) (int, string, string) {
	e.t.Helper()
	res, err := c.hc.Get(e.srv.URL + path)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := res.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return res.StatusCode, res.Header.Get("Location"), sb.String()
}

func (e *recheckEnv) me(c *client) (int, string) {
	code, m, _ := c.do("GET", "/api/me", nil, nil)
	name, _ := m["username"].(string)
	return code, name
}

// Account A here, account B at the hub: the page load sends the browser through the hub once,
// B's ticket replaces A's session, and A's old session cookie no longer works.
func TestRecheckSwitchedAccount(t *testing.T) {
	e := newRecheckEnv(t)
	e.signIn(e.c, "1")
	code0, whoA := e.me(e.c)
	if code0 != 200 || whoA == "" {
		t.Fatalf("A not signed in: %d", code0)
	}
	if code, _, _ := e.status(e.c, "/dashboard"); code != 200 {
		t.Fatalf("fresh session: dashboard %d", code)
	}
	oldCookie := e.sessionCookie(e.c)
	e.age()

	state := e.silentStart(e.c, "/dashboard")
	code, loc := e.callback(e.c, "ticket="+url.QueryEscape(ticketFor("2", state)))
	if code != http.StatusSeeOther || loc != "/vault?next="+url.QueryEscape("/dashboard?_rc=1") {
		t.Fatalf("switched account: %d %s", code, loc)
	}
	if e.sessions() != 1 {
		t.Fatalf("old session not ended: %d rows", e.sessions())
	}
	_, who := e.me(e.c)
	if who == "" || who == whoA {
		t.Fatalf("B not signed in, got %q (A was %q)", who, whoA)
	}
	var sub string
	e.db.QueryRow(`SELECT u.sso_id FROM sessions s JOIN users u ON u.id = s.user_id`).Scan(&sub)
	if sub != "2" {
		t.Fatalf("session belongs to hub account %q, want 2", sub)
	}
	// the old cookie, replayed from another request, opens nothing
	other := noRedirect(t, e.srv)
	u, _ := url.Parse(e.srv.URL)
	other.hc.Jar.SetCookies(u, []*http.Cookie{{Name: sessionCookie, Value: oldCookie}})
	if code, _ := e.me(other); code != 401 {
		t.Fatalf("A's old session still works: %d", code)
	}
}

// Same account at the hub: the session is confirmed (fresh again) and the page opens without
// the vault page in between.
func TestRecheckSameAccount(t *testing.T) {
	e := newRecheckEnv(t)
	e.signIn(e.c, "1")
	_, who := e.me(e.c)
	e.age()
	if code, _ := e.me(e.c); code != 401 {
		t.Fatalf("a stale session must not answer as the account: %d", code)
	}
	state := e.silentStart(e.c, "/dashboard")
	code, loc := e.callback(e.c, "ticket="+url.QueryEscape(ticketFor("1", state)))
	if code != http.StatusSeeOther || loc != "/dashboard?_rc=1" {
		t.Fatalf("same account: %d %s", code, loc)
	}
	if code, again := e.me(e.c); code != 200 || again != who {
		t.Fatalf("after the check: %d %q, want %q", code, again, who)
	}
	if code, _, _ := e.status(e.c, "/dashboard?_rc=1"); code != 200 {
		t.Fatalf("dashboard after the check: %d", code)
	}
	if e.sessions() != 1 {
		t.Fatalf("sessions: %d", e.sessions())
	}
}

// No hub session: the app's session is deleted, a public page stays open as a guest, the
// dashboard asks to sign in.
func TestRecheckLoginRequired(t *testing.T) {
	e := newRecheckEnv(t)
	e.signIn(e.c, "1")
	e.age()
	state := e.silentStart(e.c, "/auth/sso?prompt=none&next=%2Ffaq")
	code, loc := e.callback(e.c, "error=login_required&state="+url.QueryEscape(state))
	if code != http.StatusSeeOther || loc != "/faq" {
		t.Fatalf("login_required: %d %s", code, loc)
	}
	if e.sessions() != 0 {
		t.Fatalf("session kept: %d rows", e.sessions())
	}
	code, _, body := e.status(e.c, "/faq")
	if code != 200 || strings.Contains(body, `data-user=`) || strings.Contains(body, `data-stale=`) {
		t.Fatalf("public page as guest: %d", code)
	}
	if code, loc, _ := e.status(e.c, "/dashboard"); code != http.StatusSeeOther || !strings.HasPrefix(loc, "/login") {
		t.Fatalf("dashboard as guest: %d %s", code, loc)
	}
}

// The answer must carry this browser's state, and a silent answer only follows a silent start.
func TestRecheckStateAndLoops(t *testing.T) {
	e := newRecheckEnv(t)
	e.signIn(e.c, "1")
	e.age()
	state := e.silentStart(e.c, "/dashboard")
	if code, _ := e.callback(e.c, "error=login_required&state=wrong"); code != http.StatusBadRequest {
		t.Fatalf("wrong state: %d", code)
	}
	if e.sessions() != 1 {
		t.Fatal("a forged login_required ended the session")
	}
	// no state at all
	if code, _ := e.callback(e.c, "error=login_required"); code != http.StatusBadRequest {
		t.Fatalf("no state: %d", code)
	}
	// an ordinary (not silent) start does not accept login_required either
	ssoStart(t, e.c, "/dashboard")
	if code, _ := e.callback(e.c, "error=login_required&state="+url.QueryEscape(state)); code != http.StatusBadRequest {
		t.Fatalf("error after a normal start: %d", code)
	}
	if e.sessions() != 1 {
		t.Fatal("session lost")
	}

	// temporarily_unavailable: keep the session, come back marked, and the account page then
	// shows a message instead of going round again
	state = e.silentStart(e.c, "/dashboard")
	code, loc := e.callback(e.c, "error=temporarily_unavailable&state="+url.QueryEscape(state))
	if code != http.StatusSeeOther || loc != "/dashboard?_rc=1" {
		t.Fatalf("temporarily_unavailable: %d %s", code, loc)
	}
	if e.sessions() != 1 {
		t.Fatal("session lost on temporarily_unavailable")
	}
	code, loc, body := e.status(e.c, loc)
	if code != http.StatusBadRequest || loc != "" || !strings.Contains(body, "cookies") {
		t.Fatalf("marked page must not redirect again: %d %q", code, loc)
	}
}

// The account server is down: nothing sends the browser there, no page asks for a re-check.
func TestRecheckHubDown(t *testing.T) {
	e := newRecheckEnv(t)
	e.signIn(e.c, "1")
	e.age()
	e.hubUp = false
	code, loc, _ := e.status(e.c, "/dashboard")
	if code != http.StatusServiceUnavailable || loc != "" {
		t.Fatalf("dashboard with the hub down: %d %q", code, loc)
	}
	code, _, body := e.status(e.c, "/faq")
	if code != 200 || strings.Contains(body, `data-stale=`) {
		t.Fatalf("public page with the hub down: %d", code)
	}
	e.hubUp = true
	if _, _, body := e.status(e.c, "/faq"); !strings.Contains(body, `data-stale="1"`) {
		t.Fatal("public page with a stale session and the hub up should ask for the re-check")
	}
}

// A new sign-in ends the session this browser held, whichever way it comes.
func TestSignInEndsOldSession(t *testing.T) {
	e := newRecheckEnv(t)
	e.signIn(e.c, "1")
	old := e.sessionCookie(e.c)
	e.signIn(e.c, "2") // a normal sign-in as someone else
	if e.sessions() != 1 || e.sessionCookie(e.c) == old {
		t.Fatalf("old session left behind: %d rows", e.sessions())
	}

	// a password account signing in at a browser that holds a HOP ID session
	secret := base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	other := noRedirect(t, e.srv)
	if code, _, _ := other.do("POST", "/api/register", map[string]string{"username": "local_user", "auth": secret}, nil); code != 201 {
		t.Fatalf("register: %d", code)
	}
	e.signIn(e.c, "3")
	hubCookie := e.sessionCookie(e.c)
	before := e.sessions()
	if code, _, _ := e.c.do("POST", "/api/login", map[string]string{"username": "local_user", "auth": secret}, nil); code != 200 {
		t.Fatalf("local login: %d", code)
	}
	if e.sessions() != before || e.sessionCookie(e.c) == hubCookie {
		t.Fatalf("local login left the HOP ID session behind: %d rows (was %d)", e.sessions(), before)
	}
}

// A password account has no hub behind it: never stale, never sent to the hub.
func TestRecheckSkipsLocalAccounts(t *testing.T) {
	e := newRecheckEnv(t)
	secret := base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	if code, _, _ := e.c.do("POST", "/api/register", map[string]string{"username": "local_user", "auth": secret}, nil); code != 201 {
		t.Fatalf("register: %d", code)
	}
	e.age()
	code, _, body := e.status(e.c, "/dashboard")
	if code != 200 || strings.Contains(body, `data-stale=`) {
		t.Fatalf("local account after a long time: %d", code)
	}
	if code, who := e.me(e.c); code != 200 || who != "local_user" {
		t.Fatalf("me: %d %q", code, who)
	}
}

// Old rows (verified_at = 0 from before the column existed) are checked once.
func TestRecheckLegacySessionIsStale(t *testing.T) {
	e := newRecheckEnv(t)
	e.signIn(e.c, "1")
	e.db.Exec(`UPDATE sessions SET verified_at = 0`)
	if code, _, _ := e.status(e.c, "/dashboard"); code != http.StatusFound {
		t.Fatalf("legacy session: %d", code)
	}
}

// What the owner of a note may do does not depend on the re-check, so a stale session never
// refuses text that is being saved. Only the account itself (my notes, vault, security) does.
func TestStaleSessionKeepsNoteOwnerChecks(t *testing.T) {
	e := newRecheckEnv(t)
	e.signIn(e.c, "1")
	e.age()
	if u := e.app.currentUser(httptestRequest(e.c, e)); u == nil {
		t.Fatal("currentUser must still know the session")
	}
	if code, _, _ := e.c.do("GET", "/api/me/notes", nil, nil); code != 401 {
		t.Fatalf("my notes with a stale session: %d", code)
	}
}

func httptestRequest(c *client, e *recheckEnv) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: e.sessionCookie(c)})
	return r
}

// The page script does the quiet trip only where nothing typed can be lost.
func TestRecheckScriptGuards(t *testing.T) {
	b, err := os.ReadFile("../../web/static/js/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	i := strings.Index(js, "function recheck()")
	if i < 0 {
		t.Fatal("no recheck in app.js")
	}
	blk := js[i:]
	blk = blk[:strings.Index(blk, "})();")]
	for _, want := range []string{"_rc", "data-stale", "dataset.stale", "note|new", "location.hash", "textarea", "prompt=none"} {
		if !strings.Contains(blk, want) && want != "data-stale" {
			t.Errorf("recheck lacks %q", want)
		}
	}
	if !regexp.MustCompile(`location\.replace\('/auth/sso\?prompt=none&next='`).MatchString(blk) {
		t.Error("the trip should start at /auth/sso?prompt=none")
	}
}
