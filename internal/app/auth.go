package app

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// Accounts never send their password. The browser stretches it with PBKDF2
// into two halves: a vault key that stays local (it unlocks the note keys
// saved in the dashboard) and an auth secret that comes here and is hashed
// again with argon2id. So the server can log you in but can't open your notes.

const (
	sessionCookie = "nh_session"
	sessionTTL    = 30 * 24 * time.Hour
	argonMem      = 19 * 1024
	argonTime     = 2
	argonPar      = 1
)

var userRe = regexp.MustCompile(`^[a-z0-9_]{3,32}$`)

func hashAuth(secret []byte) string {
	salt := make([]byte, 16)
	rand.Read(salt)
	key := argon2.IDKey(secret, salt, argonTime, argonMem, argonPar, 32)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMem, argonTime, argonPar,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

func checkAuth(secret []byte, hash string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[5])
	if err1 != nil || err2 != nil {
		return false
	}
	got := argon2.IDKey(secret, salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// dummyHash keeps login timing the same whether or not the username exists.
var dummyHash = hashAuth([]byte("notehop-dummy"))

type authReq struct {
	Username string `json:"username"`
	Auth     string `json:"auth"`
}

func (q *authReq) parse() ([]byte, error) {
	q.Username = strings.ToLower(strings.TrimSpace(q.Username))
	if !userRe.MatchString(q.Username) {
		return nil, errors.New("err.bad_username")
	}
	secret, err := base64.RawURLEncoding.DecodeString(q.Auth)
	if err != nil || len(secret) != 32 {
		return nil, errors.New("err.bad_auth")
	}
	return secret, nil
}

func (a *App) apiRegister(w http.ResponseWriter, r *http.Request) {
	if !a.registration() {
		apiErr(w, http.StatusForbidden, "err.registration_closed")
		return
	}
	var q authReq
	if !readJSON(w, r, &q, 1024) {
		return
	}
	secret, err := q.parse()
	if err != nil {
		apiErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if key := a.usernameProblem(q.Username); key != "" {
		apiErr(w, http.StatusBadRequest, key)
		return
	}
	u, err := a.db.CreateUser(q.Username, hashAuth(secret))
	if errors.Is(err, db.ErrTaken) {
		apiErr(w, http.StatusConflict, "err.username_taken")
		return
	}
	if err != nil {
		log.Printf("register: %v", err)
		apiErr(w, http.StatusInternalServerError, "err.register_failed")
		return
	}
	a.startSession(w, r, u)
	writeJSON(w, http.StatusCreated, map[string]string{"username": u.Username})
}

func (a *App) apiLogin(w http.ResponseWriter, r *http.Request) {
	var q authReq
	if !readJSON(w, r, &q, 1024) {
		return
	}
	secret, err := q.parse()
	if err != nil {
		apiErr(w, http.StatusBadRequest, err.Error())
		return
	}
	u, err := a.db.UserByName(q.Username)
	if err != nil {
		checkAuth(secret, dummyHash)
		apiErr(w, http.StatusUnauthorized, "err.bad_login")
		return
	}
	if !checkAuth(secret, u.AuthHash) {
		apiErr(w, http.StatusUnauthorized, "err.bad_login")
		return
	}
	if u.Status == db.StatusSuspended {
		apiErr(w, http.StatusForbidden, "err.account_suspended")
		return
	}
	a.startSession(w, r, u)
	// vault_wrap: the vault key sealed with the key from the password, once
	// the password has been changed or reset (empty before; see recovery.go).
	writeJSON(w, http.StatusOK, map[string]string{"username": u.Username, "vault_wrap": u.VaultWrap})
}

// startSession signs u in at this browser. Whatever session the browser held before
// belongs to whoever was signed in then, so that row ends here: a new sign-in never
// leaves the old account's session behind.
func (a *App) startSession(w http.ResponseWriter, r *http.Request, u *db.User) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		_ = a.db.DeleteSession(hashHex(c.Value))
	}
	tok := randToken(40)
	if err := a.db.CreateSession(hashHex(tok), u.ID, sessionTTL); err != nil {
		log.Printf("session: %v", err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true, Secure: a.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode, MaxAge: int(sessionTTL.Seconds()),
	})
}

// endSession drops this browser's session row and cookie, nothing else.
func (a *App) endSession(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = a.db.DeleteSession(hashHex(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
		Secure: a.cfg.SecureCookies, SameSite: http.SameSiteLaxMode})
}

func (a *App) apiLogout(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	a.endSession(w, r)
	out := map[string]any{"ok": true}
	if u != nil && u.SSOID != "" {
		// A HOP ID account signs out of the whole family and comes back here.
		if next := a.hubLogoutURL(); next != "" {
			out["next"] = next
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// currentUser is the account behind this browser's session cookie, whether or not the
// account server has vouched for it lately. Use it where the session only decides
// what a note's owner may do (so text being written is never refused halfway); use
// verifiedUser for anything that shows or changes the account itself.
func (a *App) currentUser(r *http.Request) *db.User {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil
	}
	u, err := a.db.UserBySession(hashHex(c.Value))
	if err != nil || u.Status == db.StatusSuspended {
		return nil
	}
	return u
}

// recheckAfter is how long the account server's word is trusted. A HOP ID session older
// than this is not shown as an account and does not open the account pages until the
// browser has been through the account server again, which costs nothing while the same
// HOP ID is still signed in there and swaps the account if it is not. A var so tests can move it.
var recheckAfter = 5 * time.Minute

// sessionState is the account behind the cookie (nil when there is none) and whether it can be
// taken at its word: a password account always, a HOP ID account while the account server
// vouched for it recently. A live but not fresh session is "stale": worth one silent trip.
func (a *App) sessionState(r *http.Request) (*db.User, bool) {
	u := a.currentUser(r)
	if u == nil {
		return nil, false
	}
	if u.SSOID == "" || !a.cfg.SSOEnabled() {
		return u, true
	}
	c, _ := r.Cookie(sessionCookie)
	at, err := a.db.SessionVerifiedAt(hashHex(c.Value))
	return u, err == nil && time.Since(at) <= recheckAfter
}

// verifiedUser is the account to show and to act as: the session's, but only while the
// account server has vouched for it recently. A stale session may belong to an account that
// is no longer the one signed in there (switched without signing out), so it counts as
// nobody until the next trip through the account server.
func (a *App) verifiedUser(r *http.Request) *db.User {
	u, fresh := a.sessionState(r)
	if !fresh {
		return nil
	}
	return u
}

// accountPage is the gate of a page that belongs to a signed-in account. It returns the
// verified account, or has already answered: a stale HOP ID session goes through the account
// server once, quietly (prompt=none), and comes back; nobody goes to sign in. Never a loop:
// the way back carries _rc=1, and a session still unconfirmed then gets a message instead.
func (a *App) accountPage(w http.ResponseWriter, r *http.Request) *db.User {
	u, fresh := a.sessionState(r)
	if u != nil && fresh {
		return u
	}
	w.Header().Set("Cache-Control", "no-store")
	here := r.URL.Path
	switch {
	case u == nil:
		http.Redirect(w, r, "/login?next="+url.QueryEscape(here), http.StatusSeeOther)
	case !a.hubUp():
		// no way to re-check while the account server is down: say so rather than send the browser there
		a.authFail(w, r, http.StatusServiceUnavailable, "err.sso_hub_down")
	case r.URL.Query().Get("_rc") != "":
		a.authFail(w, r, http.StatusBadRequest, "err.sso_recheck")
	default:
		http.Redirect(w, r, "/auth/sso?prompt=none&next="+url.QueryEscape(here), http.StatusFound)
	}
	return nil
}

func (a *App) apiMe(w http.ResponseWriter, r *http.Request) {
	u := a.verifiedUser(r)
	if u == nil {
		apiErr(w, http.StatusUnauthorized, "err.not_signed_in")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"username": u.Username, "sso": u.SSOID != "", "vault_wrap": u.VaultWrap,
		"recovery": a.recoveryStatus(u)})
}

func (a *App) apiMyNotes(w http.ResponseWriter, r *http.Request) {
	u := a.verifiedUser(r)
	if u == nil {
		apiErr(w, http.StatusUnauthorized, "err.not_signed_in")
		return
	}
	notes, err := a.db.NotesByOwner(u.ID)
	if err != nil {
		log.Printf("my notes: %v", err)
		apiErr(w, http.StatusInternalServerError, "err.load_notes_failed")
		return
	}
	// A basic note is named from its own text, which the server can read
	// anyway; an encrypted note's title lives only in the vault blob.
	for i := range notes {
		notes[i].Title = publicTitle(&db.Note{Encrypted: notes[i].Encrypted, Content: notes[i].Content})
		notes[i].Content = ""
	}
	writeJSON(w, http.StatusOK, map[string]any{"notes": notes})
}
