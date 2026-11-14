package app

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

// A vault reset needs a sign-in the account server stamped (auth_time) a few minutes ago, believed only from the
// signed ticket. Three cases with a stand-in hub: no stamp (an older hub), a stamp that is too old, a fresh one; and
// the ordinary sign-in, which never carries the proof.

// reloginStart begins a sign-in asked for with prompt=login and returns the state the hub is given, after checking
// that it asked for max_age=300.
func reloginStart(t *testing.T, c *client) string {
	t.Helper()
	res, err := c.hc.Get(c.srv.URL + "/auth/sso?prompt=login&next=" + url.QueryEscape("/security?reset=1"))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	loc, _ := url.Parse(res.Header.Get("Location"))
	if res.StatusCode != http.StatusFound || loc.Host != "accounts.test" || loc.Path != "/sso/notehop" {
		t.Fatalf("start: %d %s", res.StatusCode, loc)
	}
	if loc.Query().Get("max_age") != "300" || loc.Query().Get("prompt") != "" {
		t.Fatalf("the hub must be asked for max_age=300 (and not for a silent check): %s", loc.RawQuery)
	}
	return loc.Query().Get("state")
}

func ticketWithAuthTime(sub, state string, authTime int64) string {
	n := time.Now().Unix()
	return signTicket(ssoTicket{Sub: sub, Email: "helena@example.test", Name: "Helena", State: state, Jti: randToken(16), Iat: n, Exp: n + 120, V: 2, AuthTime: authTime}, testSecret)
}

func TestVaultResetNeedsASignInTheHubStamped(t *testing.T) {
	now := time.Now().Unix()
	body := map[string]string{"confirm": "RESET"}

	// the ordinary sign-in (also with a fresh stamp in the ticket) proves nothing: the flag the page sets is what counts
	e := newRecheckEnv(t)
	c := noRedirect(t, e.srv)
	state := ssoStart(t, c, "/dashboard")
	if code, _ := ssoCallback(t, c, ticketWithAuthTime("9101", state, now)); code != http.StatusSeeOther {
		t.Fatalf("ordinary sign-in: %d", code)
	}
	if code, m, _ := c.do("GET", resetPath, nil, nil); code != http.StatusForbidden || m["code"] != "err.reset_login" {
		t.Fatalf("ordinary sign-in, preview: %d %v", code, m)
	}
	if code, m, _ := c.do("POST", resetPath, body, nil); code != http.StatusForbidden || m["code"] != "err.reset_login" {
		t.Fatalf("ordinary sign-in, reset: %d %v", code, m)
	}

	for name, at := range map[string]int64{"auth_time 0 (a hub that sends none)": 0, "a stamp three hours old": now - 3*3600, "a stamp in the far future": now + 3600} {
		e := newRecheckEnv(t)
		c := noRedirect(t, e.srv)
		state := reloginStart(t, c)
		code, _ := ssoCallback(t, c, ticketWithAuthTime("9102", state, at))
		if code != http.StatusForbidden {
			t.Errorf("%s: want 403, got %d", name, code)
		}
		if code, _, _ := c.do("GET", "/api/me", nil, nil); code != http.StatusUnauthorized {
			t.Errorf("%s: a session was made anyway: %d", name, code)
		}
		if e.sessions() != 0 {
			t.Errorf("%s: %d session rows", name, e.sessions())
		}
	}

	// a fresh stamp: signed in, and the reset runs; nothing else changes who may reset
	e = newRecheckEnv(t)
	c = noRedirect(t, e.srv)
	state = reloginStart(t, c)
	if code, loc := ssoCallback(t, c, ticketWithAuthTime("9103", state, now-30)); code != http.StatusSeeOther || loc != "/security?reset=1" {
		t.Fatalf("fresh sign-in: %d %s", code, loc)
	}
	u, _ := e.db.UserBySSO("9103")
	c.create(nil)
	if code, m, raw := c.do("GET", resetPath, nil, nil); code != http.StatusOK {
		t.Fatalf("fresh sign-in, preview: %d %v %s", code, m, raw)
	}
	// the stamp ages out: after the window the same session is refused again, nothing deleted
	if _, err := e.db.Exec(`UPDATE sessions SET login_at = ?`, now-int64((reLoginWindow+time.Minute).Seconds())); err != nil {
		t.Fatal(err)
	}
	if code, m, _ := c.do("POST", resetPath, body, nil); code != http.StatusForbidden || m["code"] != "err.reset_login" {
		t.Fatalf("an old stamp: %d %v", code, m)
	}
	if got := e.count(`SELECT COUNT(*) FROM notes WHERE owner_id = ? AND encrypted = 1`, u.ID); got != 1 {
		t.Fatalf("a refused reset deleted notes: %d left", got)
	}
	if _, err := e.db.Exec(`UPDATE sessions SET login_at = ?`, time.Now().Unix()-20); err != nil {
		t.Fatal(err)
	}
	if code, _, raw := c.do("POST", resetPath, body, nil); code != http.StatusOK {
		t.Fatalf("fresh stamp, reset: %d %s", code, raw)
	}
	if got := e.count(`SELECT COUNT(*) FROM notes WHERE owner_id = ? AND encrypted = 1`, u.ID); got != 0 {
		t.Fatalf("the reset left %d encrypted notes", got)
	}
}

// A password account has no account server: its reset stays refused as before (HOP ID only), not asked to sign in.
func TestPasswordAccountsAreNotAskedToSignInAtTheHub(t *testing.T) {
	e := newRecheckEnv(t)
	c := newClient(t, e.srv)
	if code, _, raw := c.do("POST", "/api/register", map[string]string{"username": "lokal_h", "auth": key()}, nil); code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("register: %d %s", code, raw)
	}
	if code, m, _ := c.do("POST", resetPath, map[string]string{"confirm": "RESET"}, nil); code != http.StatusBadRequest || m["code"] != "err.reset_hopid_only" {
		t.Fatalf("password account: %d %v", code, m)
	}
}
