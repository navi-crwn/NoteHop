package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// Single sign-on with an account server you run (NOTEHOP_SSO_URL). That
// server owns the account (sign-up, password, social logins, 2FA, email
// verification and resets); NoteHop only receives a short-lived ticket
// signed with the shared secret and keeps a local account linked by the
// server's user id. Since NoteHop never sees that password, the vault key
// for the dashboard comes from a separate phrase (see apiVault).

const ssoCookie = "nh_sso"

type ssoTicket struct {
	Sub    string `json:"sub"`
	Aud    string `json:"aud"` // the client the ticket is for; checked when set
	Email  string `json:"email"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
	Admin  bool   `json:"admin"`
	State  string `json:"state"`
	Iat    int64  `json:"iat"`
	Exp    int64  `json:"exp"`
	Jti    string `json:"jti"`
}

var errBadTicket = errors.New("bad ticket")

// parseTicket checks the signature and lifetime of a sign-on ticket:
// b64u(json) "." b64u(HMAC-SHA256(secret, b64u(json))).
func parseTicket(raw, secret string, now time.Time) (*ssoTicket, error) {
	body, sig, ok := strings.Cut(raw, ".")
	if !ok || body == "" || sig == "" {
		return nil, errBadTicket
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(got, mac.Sum(nil)) {
		return nil, errBadTicket
	}
	js, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return nil, errBadTicket
	}
	var t ssoTicket
	if err := json.Unmarshal(js, &t); err != nil || t.Sub == "" || t.Jti == "" || t.State == "" {
		return nil, errBadTicket
	}
	n := now.Unix()
	if t.Exp < n || t.Iat > n+60 || t.Exp-t.Iat > 600 {
		return nil, errBadTicket
	}
	return &t, nil
}

// usedTickets remembers ticket ids until they expire, so each works once.
type usedTickets struct {
	sync.Mutex
	m map[string]int64
}

func (u *usedTickets) first(jti string, exp int64) bool {
	u.Lock()
	defer u.Unlock()
	now := time.Now().Unix()
	for k, e := range u.m {
		if e < now {
			delete(u.m, k)
		}
	}
	if _, seen := u.m[jti]; seen {
		return false
	}
	u.m[jti] = exp
	return true
}

// safeNext keeps post-login redirects on this site.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/dashboard"
	}
	return next
}

// ssoStart sends the visitor to the account server to sign in.
func (a *App) ssoStart(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.SSOEnabled() {
		a.notFound(w, r)
		return
	}
	state := randToken(32)
	http.SetCookie(w, &http.Cookie{Name: ssoCookie, Value: state + "|" + url.QueryEscape(safeNext(r.FormValue("next"))),
		Path: "/auth/", HttpOnly: true, Secure: a.cfg.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: 900})
	q := url.Values{"state": {state}, "return_to": {a.cfg.BaseURL + "/auth/sso/callback"}}
	path := "/sso/" + url.PathEscape(a.cfg.SSOClient)
	// A provider button (Google, say) skips the account server's login page.
	// Only configured ids are passed on; anything else gets the plain login.
	for _, p := range a.cfg.SSOProviders {
		if p.URL == r.FormValue("provider") {
			path += "/" + url.PathEscape(p.URL)
		}
	}
	http.Redirect(w, r, a.cfg.SSOURL+path+"?"+q.Encode(), http.StatusFound)
}

func (a *App) ssoCallback(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.SSOEnabled() {
		a.notFound(w, r)
		return
	}
	c, err := r.Cookie(ssoCookie)
	if err != nil {
		a.authFail(w, r, http.StatusBadRequest, "err.sso_cookie")
		return
	}
	state, next, _ := strings.Cut(c.Value, "|")
	next, _ = url.QueryUnescape(next)
	t, err := parseTicket(r.FormValue("ticket"), a.cfg.SSOSecret, time.Now())
	// A ticket the server made for another of its apps must not work here.
	if err != nil || !hmac.Equal([]byte(t.State), []byte(state)) || (t.Aud != "" && t.Aud != a.cfg.SSOClient) || !a.used.first(t.Jti, t.Exp) {
		a.authFail(w, r, http.StatusBadRequest, "err.sso_ticket")
		return
	}
	u, err := a.linkSSOUser(t)
	if errors.Is(err, errRegClosed) {
		a.authFail(w, r, http.StatusForbidden, "err.sso_closed")
		return
	}
	if err != nil {
		log.Printf("sso: %v", err)
		a.authFail(w, r, http.StatusInternalServerError, "err.sso_failed")
		return
	}
	a.startSession(w, u)
	http.SetCookie(w, &http.Cookie{Name: ssoCookie, Path: "/auth/", MaxAge: -1})
	// The vault page opens straight to next when this browser already holds
	// the vault key, and asks for the vault phrase otherwise.
	http.Redirect(w, r, "/vault?next="+url.QueryEscape(safeNext(next)), http.StatusSeeOther)
}

var errRegClosed = errors.New("registration closed")

// linkSSOUser finds the NoteHop account for a signed-on user, or makes one.
// Existing password accounts are not matched (they have no email); linking
// them is a later feature.
func (a *App) linkSSOUser(t *ssoTicket) (*db.User, error) {
	prof := db.User{SSOID: t.Sub, Email: clip(t.Email, 254), DisplayName: clip(t.Name, 60), Avatar: clip(t.Avatar, 500)}
	for try := 0; try < 6; try++ {
		u, err := a.db.UserBySSO(t.Sub)
		if err == nil {
			u.Email, u.DisplayName, u.Avatar = prof.Email, prof.DisplayName, prof.Avatar
			return u, a.db.UpdateSSOProfile(u)
		}
		if !errors.Is(err, db.ErrNotFound) {
			return nil, err
		}
		if !a.cfg.Registration {
			return nil, errRegClosed
		}
		u = &prof
		u.Username = ssoUsername(t.Email, try)
		if a.usernameProblem(u.Username) != "" {
			u.Username = ssoUsername("user@", try+1) // e-mail named after a reserved word
		}
		err = a.db.CreateSSOUser(u)
		if !errors.Is(err, db.ErrTaken) {
			return u, err
		}
		// Taken: either the username (try another) or the sign-on id was
		// linked by a parallel request (the next lookup finds it).
	}
	return nil, errors.New("no free username")
}

var notUserChar = regexp.MustCompile(`[^a-z0-9_]+`)

// ssoUsername makes a username from the email's local part, with a random
// suffix after the first attempt.
func ssoUsername(email string, try int) string {
	local, _, _ := strings.Cut(strings.ToLower(email), "@")
	base := strings.Trim(notUserChar.ReplaceAllString(local, "_"), "_")
	if len(base) > 24 {
		base = base[:24]
	}
	if len(base) < 3 {
		base = "user"
	}
	if try == 0 {
		return base
	}
	return base + "_" + strings.ToLower(randToken(5))
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}

// authFail shows a sign-in error page; key is a catalog key.
func (a *App) authFail(w http.ResponseWriter, r *http.Request, status int, key string) {
	l := a.locale(w, r)
	a.render(w, r, status, "error", pageData{Title: l.T("title.sign_in_failed"), Msg: l.T(key), NoIndex: true})
}

// ---------- vault phrase for single sign-on accounts ----------

// vaultPage unlocks (or, the first time, creates) the vault key of a single
// sign-on account. Password accounts get theirs from the password at login.
func (a *App) vaultPage(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login?next=/dashboard", http.StatusSeeOther)
		return
	}
	if u.SSOID == "" {
		http.Redirect(w, r, "/login?next="+url.QueryEscape(safeNext(r.URL.Query().Get("next"))), http.StatusSeeOther)
		return
	}
	a.render(w, r, http.StatusOK, "vault", pageData{Title: a.locale(w, r).T("title.vault"), User: u, NoIndex: true})
}

// apiVault receives HMAC(vault, "notehop/vault-check") from the browser, never
// the vault key itself. The first call stores it (hashed like a login secret);
// later calls only say whether the phrase matches, so a typo doesn't leave the
// dashboard silently locked.
func (a *App) apiVault(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if u == nil {
		apiErr(w, http.StatusUnauthorized, "err.not_signed_in")
		return
	}
	if u.SSOID == "" {
		apiErr(w, http.StatusBadRequest, "err.vault_password_account")
		return
	}
	var q struct {
		Check string `json:"check"`
	}
	if !readJSON(w, r, &q, 1024) {
		return
	}
	check, err := base64.RawURLEncoding.DecodeString(q.Check)
	if err != nil || len(check) != 32 {
		apiErr(w, http.StatusBadRequest, "err.bad_data")
		return
	}
	if u.VaultCheck == "" {
		err := a.db.SetVaultCheck(u.ID, hashAuth(check))
		if err == nil {
			writeJSON(w, http.StatusCreated, map[string]bool{"created": true})
			return
		}
		if !errors.Is(err, db.ErrConflict) {
			log.Printf("vault: %v", err)
			apiErr(w, http.StatusInternalServerError, "err.vault_save_failed")
			return
		}
		// Set by another tab meanwhile: check against that one.
		if u, err = a.db.UserBySSO(u.SSOID); err != nil {
			apiErr(w, http.StatusInternalServerError, "err.vault_check_failed")
			return
		}
	}
	if !checkAuth(check, u.VaultCheck) {
		apiErr(w, http.StatusUnauthorized, "err.vault_wrong")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"created": false, "vault_wrap": u.VaultWrap})
}
