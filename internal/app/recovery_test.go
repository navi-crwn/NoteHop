package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// The browser's part (sealing the vault key) is opaque here: wraps are just
// strings the server must store and hand back unchanged.
func wrapOf(s string) string { return strings.Repeat("w", 30) + s + strings.Repeat("z", 12) }

func codeSet(proofs []string) []map[string]string {
	out := make([]map[string]string, len(proofs))
	for i, p := range proofs {
		out[i] = map[string]string{"proof": p, "wrap": wrapOf(string(rune('a' + i)))}
	}
	return out
}

func TestRecoveryCodes(t *testing.T) {
	srv := newTestApp(t)
	c := newClient(t, srv)
	pw1 := key()
	if code, _, raw := c.do("POST", "/api/register", map[string]any{"username": "rani", "auth": pw1}, nil); code != 201 {
		t.Fatalf("register: %d %s", code, raw)
	}
	proofs := make([]string, recoveryCount)
	for i := range proofs {
		proofs[i] = key()
	}
	if code, m, _ := c.do("POST", "/api/me/recovery", map[string]any{"current": key(), "codes": codeSet(proofs)}, nil); code != 401 || m["code"] != "err.password_wrong" {
		t.Fatalf("wrong password must not set codes: %d %v", code, m)
	}
	if code, _, _ := c.do("POST", "/api/me/recovery", map[string]any{"current": pw1, "codes": codeSet(proofs[:7])}, nil); code != 400 {
		t.Fatalf("seven codes: %d", code)
	}
	dup := append([]string{}, proofs...)
	dup[1] = dup[0]
	if code, _, _ := c.do("POST", "/api/me/recovery", map[string]any{"current": pw1, "codes": codeSet(dup)}, nil); code != 400 {
		t.Fatalf("duplicate codes: %d", code)
	}
	if code, m, raw := c.do("POST", "/api/me/recovery", map[string]any{"current": pw1, "codes": codeSet(proofs)}, nil); code != 200 || m["left"] != float64(8) {
		t.Fatalf("set codes: %d %s", code, raw)
	}
	if _, m, _ := c.do("GET", "/api/me", nil, nil); m["recovery"].(map[string]any)["left"] != float64(8) || m["vault_wrap"] != "" {
		t.Fatalf("me: %v", m)
	}

	// A stranger with no session: unknown name and wrong code look the same.
	s := newClient(t, srv)
	_, wrongName, _ := s.do("POST", "/api/recover", map[string]any{"username": "nobody", "proof": proofs[0]}, nil)
	_, wrongCode, _ := s.do("POST", "/api/recover", map[string]any{"username": "rani", "proof": key()}, nil)
	if wrongName["code"] != "err.recover_wrong" || wrongCode["code"] != "err.recover_wrong" {
		t.Fatalf("wrong: %v / %v", wrongName, wrongCode)
	}
	code, m, raw := s.do("POST", "/api/recover", map[string]any{"username": "Rani", "proof": proofs[3]}, nil)
	if code != 200 || m["wrap"] != wrapOf("d") || m["ticket"] == "" || m["sso"] != false {
		t.Fatalf("recover: %d %s", code, raw)
	}
	ticket := m["ticket"].(string)
	pw2, wrap2 := key(), wrapOf("new-password")
	if code, _, _ := s.do("POST", "/api/recover/finish", map[string]any{"ticket": "nope", "next": pw2, "vault_wrap": wrap2}, nil); code != 400 {
		t.Fatalf("bad ticket: %d", code)
	}
	if code, m, raw := s.do("POST", "/api/recover/finish", map[string]any{"ticket": ticket, "next": pw2, "vault_wrap": wrap2}, nil); code != 200 || m["left"] != float64(7) {
		t.Fatalf("finish: %d %s", code, raw)
	}
	if code, _, _ := s.do("GET", "/api/me", nil, nil); code != 200 {
		t.Fatal("recovery signs this browser in")
	}
	if code, _, _ := c.do("GET", "/api/me", nil, nil); code != 401 {
		t.Fatal("recovery signs other browsers out")
	}
	if code, _, _ := s.do("POST", "/api/recover/finish", map[string]any{"ticket": ticket, "next": key(), "vault_wrap": wrap2}, nil); code != 400 {
		t.Fatal("a ticket works once")
	}
	if code, _, _ := s.do("POST", "/api/recover", map[string]any{"username": "rani", "proof": proofs[3]}, nil); code != 401 {
		t.Fatal("a used code is spent")
	}
	l := newClient(t, srv)
	if code, _, _ := l.do("POST", "/api/login", map[string]any{"username": "rani", "auth": pw1}, nil); code != 401 {
		t.Fatal("old password still works")
	}
	if code, m, _ := l.do("POST", "/api/login", map[string]any{"username": "rani", "auth": pw2}, nil); code != 200 || m["vault_wrap"] != wrap2 {
		t.Fatalf("new password: %d %v", code, m)
	}

	// Change the password: other sessions are signed out, this one stays.
	pw3, wrap3 := key(), wrapOf("third")
	if code, _, _ := s.do("POST", "/api/me/secret", map[string]any{"current": pw1, "next": pw3, "vault_wrap": wrap3}, nil); code != 401 {
		t.Fatal("change needs the current password")
	}
	if code, _, raw := s.do("POST", "/api/me/secret", map[string]any{"current": pw2, "next": pw3, "vault_wrap": wrap3}, nil); code != 200 {
		t.Fatalf("change: %d %s", code, raw)
	}
	if code, _, _ := s.do("GET", "/api/me", nil, nil); code != 200 {
		t.Fatal("changing keeps this session")
	}
	if code, _, _ := l.do("GET", "/api/me", nil, nil); code != 401 {
		t.Fatal("changing signs other sessions out")
	}
	if _, m, _ := s.do("GET", "/api/me", nil, nil); m["vault_wrap"] != wrap3 || m["recovery"].(map[string]any)["left"] != float64(7) {
		t.Fatalf("after change: %v", m)
	}

}

// Guessing is slowed per account, not only per address.
func TestRecoveryLimitPerAccount(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(Config{Registration: true, TrustProxy: true}, d)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() { srv.Close(); d.Close() })
	c := newClient(t, srv)
	c.do("POST", "/api/register", map[string]any{"username": "rani", "auth": key()}, nil)
	limited := 0
	for i := 0; i < 8; i++ {
		ip := map[string]string{"X-Forwarded-For": fmt.Sprintf("10.0.0.%d", i+1)} // a new address every time
		if code, _, _ := c.do("POST", "/api/recover", map[string]any{"username": "rani", "proof": key()}, ip); code == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited != 3 {
		t.Fatalf("5 guesses a minute per account, then 429: got %d refused of 8", limited)
	}
}

// Single sign-on accounts keep their password at the account server; their
// recovery codes bring back the vault phrase, and only while signed in.
func TestRecoverySSO(t *testing.T) {
	srv := newSSOApp(t, true)
	c := noRedirect(t, srv)
	state := ssoStart(t, c, "/dashboard")
	if code, _ := ssoCallback(t, c, ticketFor("42", state)); code != http.StatusSeeOther {
		t.Fatal("sso login")
	}
	check1 := key()
	if code, _, _ := c.do("POST", "/api/vault", map[string]any{"check": check1}, nil); code != 201 {
		t.Fatal("vault create")
	}
	proofs := make([]string, recoveryCount)
	for i := range proofs {
		proofs[i] = key()
	}
	if code, _, raw := c.do("POST", "/api/me/recovery", map[string]any{"current": check1, "codes": codeSet(proofs)}, nil); code != 200 {
		t.Fatalf("set: %d %s", code, raw)
	}
	_, me, _ := c.do("GET", "/api/me", nil, nil)
	name := me["username"].(string)
	if me["sso"] != true {
		t.Fatalf("me: %v", me)
	}
	stranger := newClient(t, srv)
	if code, m, _ := stranger.do("POST", "/api/recover", map[string]any{"username": name, "proof": proofs[0]}, nil); code != 400 || m["code"] != "err.recover_sso" {
		t.Fatalf("sso recovery without a session: %d %v", code, m)
	}
	code, m, raw := c.do("POST", "/api/recover", map[string]any{"proof": proofs[5]}, nil)
	if code != 200 || m["sso"] != true || m["wrap"] != wrapOf("f") {
		t.Fatalf("recover: %d %s", code, raw)
	}
	check2, wrap2 := key(), wrapOf("phrase-2")
	if code, _, raw := c.do("POST", "/api/recover/finish", map[string]any{"ticket": m["ticket"], "next": check2, "vault_wrap": wrap2}, nil); code != 200 {
		t.Fatalf("finish: %d %s", code, raw)
	}
	if code, _, _ := c.do("POST", "/api/vault", map[string]any{"check": check1}, nil); code != 401 {
		t.Fatal("old phrase still accepted")
	}
	if code, m, _ := c.do("POST", "/api/vault", map[string]any{"check": check2}, nil); code != 200 || m["vault_wrap"] != wrap2 {
		t.Fatalf("new phrase: %d %v", code, m)
	}
}
