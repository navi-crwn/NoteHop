package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func signTicket(t ssoTicket, secret string) string {
	js, _ := json.Marshal(t)
	body := base64.RawURLEncoding.EncodeToString(js)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func TestParseTicket(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ok := ssoTicket{Sub: "7", Email: "a@b.c", State: "s", Jti: "j", Iat: now.Unix(), Exp: now.Unix() + 120}
	if _, err := parseTicket(signTicket(ok, testSecret), testSecret, now); err != nil {
		t.Fatalf("valid ticket: %v", err)
	}
	bad := map[string]string{
		"wrong secret": signTicket(ok, strings.Repeat("x", 32)),
		"no signature": strings.Split(signTicket(ok, testSecret), ".")[0],
		"garbage":      "a.b",
	}
	expired, long, future, noState := ok, ok, ok, ok
	expired.Exp = now.Unix() - 1
	long.Iat, long.Exp = now.Unix()-10, now.Unix()+700
	future.Iat = now.Unix() + 120
	noState.State = ""
	bad["expired"] = signTicket(expired, testSecret)
	bad["too long-lived"] = signTicket(long, testSecret)
	bad["issued in the future"] = signTicket(future, testSecret)
	bad["no state"] = signTicket(noState, testSecret)
	tampered := signTicket(ok, testSecret)
	tampered = "x" + tampered[1:]
	bad["tampered"] = tampered
	for name, raw := range bad {
		if _, err := parseTicket(raw, testSecret, now); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestSafeNext(t *testing.T) {
	for in, want := range map[string]string{
		"/n/abc": "/n/abc", "": "/dashboard", "//evil.com": "/dashboard",
		"/\\evil.com": "/dashboard", "https://evil.com": "/dashboard",
	} {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
}

func newSSOApp(t *testing.T, reg bool) *httptest.Server {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(Config{Registration: reg, BaseURL: "https://note.test", SSOURL: "https://accounts.test", SSOSecret: testSecret, SSOName: "Accounts", SSOClient: "notehop",
		SSOProviders: []Link{{Name: "Google", URL: "google"}}}, d)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() { srv.Close(); d.Close() })
	return srv
}

// ssoLogin runs /auth/sso and returns the state the account server would receive.
func ssoStart(t *testing.T, c *client, next string) string {
	t.Helper()
	res, err := c.hc.Get(c.srv.URL + "/auth/sso?next=" + url.QueryEscape(next))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	loc, _ := url.Parse(res.Header.Get("Location"))
	if res.StatusCode != http.StatusFound || loc.Host != "accounts.test" || loc.Path != "/sso/notehop" ||
		loc.Query().Get("return_to") != "https://note.test/auth/sso/callback" {
		t.Fatalf("start: %d %s", res.StatusCode, loc)
	}
	return loc.Query().Get("state")
}

func ssoCallback(t *testing.T, c *client, ticket string) (int, string) {
	t.Helper()
	res, err := c.hc.Get(c.srv.URL + "/auth/sso/callback?ticket=" + url.QueryEscape(ticket))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode, res.Header.Get("Location")
}

func noRedirect(t *testing.T, srv *httptest.Server) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, srv: srv, hc: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}}
}

func ticketFor(sub, state string) string {
	n := time.Now().Unix()
	return signTicket(ssoTicket{Sub: sub, Email: "Budi.Santoso@example.com", Name: "Budi", State: state,
		Jti: randToken(16), Iat: n, Exp: n + 120}, testSecret)
}

// A ticket the account server made for another of its apps is refused; one
// for this app (or without aud, from older servers) is accepted.
func TestSSOAudience(t *testing.T) {
	srv := newSSOApp(t, true)
	c := noRedirect(t, srv)
	n := time.Now().Unix()
	state := ssoStart(t, c, "/dashboard")
	other := signTicket(ssoTicket{Sub: "9", Aud: "otherapp", Email: "a@example.com", State: state, Jti: randToken(16), Iat: n, Exp: n + 120}, testSecret)
	if code, _ := ssoCallback(t, c, other); code != http.StatusBadRequest {
		t.Fatalf("ticket for another app: %d", code)
	}
	state = ssoStart(t, c, "/dashboard")
	mine := signTicket(ssoTicket{Sub: "9", Aud: "notehop", Email: "a@example.com", State: state, Jti: randToken(16), Iat: n, Exp: n + 120}, testSecret)
	if code, loc := ssoCallback(t, c, mine); code != http.StatusSeeOther || loc != "/vault?next=%2Fdashboard" {
		t.Fatalf("ticket for this app: %d %s", code, loc)
	}
}

func TestSSOLoginAndVault(t *testing.T) {
	srv := newSSOApp(t, true)
	c := noRedirect(t, srv)

	if code, _, _ := c.do("GET", "/api/me", nil, nil); code != 401 {
		t.Fatalf("me before login: %d", code)
	}
	state := ssoStart(t, c, "/n/abc")
	tk := ticketFor("42", state)
	code, loc := ssoCallback(t, c, tk)
	if code != http.StatusSeeOther || loc != "/vault?next=%2Fn%2Fabc" {
		t.Fatalf("callback: %d %q", code, loc)
	}
	code, m, _ := c.do("GET", "/api/me", nil, nil)
	if code != 200 || m["username"] != "budi_santoso" {
		t.Fatalf("me: %d %v", code, m)
	}
	// A ticket works once.
	ssoStart(t, c, "/")
	if code, _ := ssoCallback(t, c, tk); code != http.StatusBadRequest {
		t.Fatalf("replay: %d", code)
	}
	// State must match this browser's cookie.
	ssoStart(t, c, "/")
	if code, _ := ssoCallback(t, c, ticketFor("42", "other-state")); code != http.StatusBadRequest {
		t.Fatalf("wrong state: %d", code)
	}
	// Without the cookie (another browser) it fails too.
	other := noRedirect(t, srv)
	if code, _ := ssoCallback(t, other, ticketFor("42", state)); code != http.StatusBadRequest {
		t.Fatalf("no cookie: %d", code)
	}

	if code, _, _ := c.do("GET", "/vault", nil, nil); code != 200 {
		t.Fatalf("vault page: %d", code)
	}
	check, wrong := key(), key()
	if code, m, _ := c.do("POST", "/api/vault", map[string]string{"check": check}, nil); code != 201 || m["created"] != true {
		t.Fatalf("vault create: %d %v", code, m)
	}
	if code, m, _ := c.do("POST", "/api/vault", map[string]string{"check": check}, nil); code != 200 || m["created"] != false {
		t.Fatalf("vault unlock: %d %v", code, m)
	}
	if code, _, _ := c.do("POST", "/api/vault", map[string]string{"check": wrong}, nil); code != 401 {
		t.Fatalf("wrong phrase: %d", code)
	}
	if code, _, _ := c.do("POST", "/api/vault", map[string]string{"check": "short"}, nil); code != 400 {
		t.Fatalf("bad check: %d", code)
	}

	// Signing in again (new device) reaches the same account, and a second
	// A sign-on user with the same email local part gets another username.
	d := noRedirect(t, srv)
	if code, _ := ssoCallback(t, d, ticketFor("42", ssoStart(t, d, "/"))); code != http.StatusSeeOther {
		t.Fatalf("second device: %d", code)
	}
	if _, m, _ := d.do("GET", "/api/me", nil, nil); m["username"] != "budi_santoso" {
		t.Fatalf("second device user: %v", m)
	}
	e := noRedirect(t, srv)
	ssoCallback(t, e, ticketFor("43", ssoStart(t, e, "/")))
	if _, m, _ := e.do("GET", "/api/me", nil, nil); !strings.HasPrefix(m["username"].(string), "budi_santoso_") {
		t.Fatalf("second user: %v", m)
	}
	// SSO accounts have no password, so password login never works for them.
	if code, _, _ := e.do("POST", "/api/login", map[string]string{"username": "budi_santoso", "auth": key()}, nil); code != 401 {
		t.Fatalf("password login on SSO account: %d", code)
	}
}

func TestSSOOff(t *testing.T) {
	c := noRedirect(t, newTestApp(t))
	if code, _, _ := c.do("GET", "/auth/sso", nil, nil); code != 404 {
		t.Fatalf("start without config: %d", code)
	}
	if code, _, _ := c.do("GET", "/auth/sso/callback?ticket=x", nil, nil); code != 404 {
		t.Fatalf("callback without config: %d", code)
	}
	if code, _, body := c.do("GET", "/login", nil, nil); code != 200 || strings.Contains(body, "/auth/sso") {
		t.Fatalf("login page shows SSO while off: %d", code)
	}
}

func TestSSORegistrationClosed(t *testing.T) {
	c := noRedirect(t, newSSOApp(t, false))
	if code, _ := ssoCallback(t, c, ticketFor("9", ssoStart(t, c, "/"))); code != http.StatusForbidden {
		t.Fatalf("new account while registration closed: %d", code)
	}
}

func TestVaultNeedsSSOAccount(t *testing.T) {
	c := noRedirect(t, newTestApp(t))
	if code, _, _ := c.do("POST", "/api/vault", map[string]string{"check": key()}, nil); code != 401 {
		t.Fatalf("anonymous: %d", code)
	}
	c.do("POST", "/api/register", map[string]string{"username": "pw_user", "auth": key()}, nil)
	if code, _, _ := c.do("POST", "/api/vault", map[string]string{"check": key()}, nil); code != 400 {
		t.Fatalf("password account: %d", code)
	}
	if code, _, _ := c.do("GET", "/vault", nil, nil); code != http.StatusSeeOther {
		t.Fatalf("vault page for password account: %d", code)
	}
}

// A provider button goes to /sso/<client>/<id>, but only for configured ids,
// and the login page shows it above the account server's own button.
func TestSSOProvider(t *testing.T) {
	srv := newSSOApp(t, true)
	c := noRedirect(t, srv)
	for provider, want := range map[string]string{"google": "/sso/notehop/google", "evil/../x": "/sso/notehop", "": "/sso/notehop"} {
		res, err := c.hc.Get(srv.URL + "/auth/sso?next=%2Fdashboard&provider=" + url.QueryEscape(provider))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		loc, _ := url.Parse(res.Header.Get("Location"))
		if res.StatusCode != http.StatusFound || loc.Path != want || loc.Query().Get("state") == "" {
			t.Errorf("provider %q: %d %s, want path %s", provider, res.StatusCode, loc, want)
		}
	}
	for _, page := range []string{"/login", "/register"} {
		res, err := c.hc.Get(srv.URL + page)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		html := string(b)
		g, own := strings.Index(html, "/auth/sso?provider=google&amp;next="), strings.Index(html, `href="/auth/sso?next=`)
		if g < 0 || own < 0 || g > own || !strings.Contains(html, "Continue with Google") || !strings.Contains(html, "handled by Accounts") {
			t.Errorf("%s: provider button missing or after the account button", page)
		}
	}
}

func TestSSOChainedLogout(t *testing.T) {
	srv := newSSOApp(t, true)
	c := noRedirect(t, srv)
	state := ssoStart(t, c, "/")
	if code, _ := ssoCallback(t, c, ticketFor("42", state)); code != http.StatusSeeOther {
		t.Fatalf("login: %d", code)
	}
	for _, tc := range []struct{ next, want string }{
		{"https://bio.accounts.test/x", "https://bio.accounts.test/x"},
		{"https://accounts.test/", "https://accounts.test/"},
		{"https://test/", "/"},
		{"https://evil.example/", "/"},
		{"http://test/", "/"},
	} {
		res, err := c.hc.Get(srv.URL + "/auth/sso/logout?next=" + url.QueryEscape(tc.next))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != tc.want {
			t.Fatalf("next=%q: %d %q, want %q", tc.next, res.StatusCode, res.Header.Get("Location"), tc.want)
		}
	}
	if code, _, _ := c.do("GET", "/api/me", nil, nil); code != 401 {
		t.Fatalf("still signed in after logout: %d", code)
	}
}
