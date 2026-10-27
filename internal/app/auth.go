package app

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
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
	if !a.cfg.Registration {
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
	a.startSession(w, u)
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
	a.startSession(w, u)
	// vault_wrap: the vault key sealed with the key from the password, once
	// the password has been changed or reset (empty before; see recovery.go).
	writeJSON(w, http.StatusOK, map[string]string{"username": u.Username, "vault_wrap": u.VaultWrap})
}

func (a *App) startSession(w http.ResponseWriter, u *db.User) {
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

func (a *App) apiLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = a.db.DeleteSession(hashHex(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
		Secure: a.cfg.SecureCookies, SameSite: http.SameSiteLaxMode})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) currentUser(r *http.Request) *db.User {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil
	}
	u, err := a.db.UserBySession(hashHex(c.Value))
	if err != nil {
		return nil
	}
	return u
}

func (a *App) apiMe(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if u == nil {
		apiErr(w, http.StatusUnauthorized, "err.not_signed_in")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"username": u.Username, "sso": u.SSOID != "", "vault_wrap": u.VaultWrap,
		"recovery": a.recoveryStatus(u)})
}

func (a *App) apiMyNotes(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
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
	writeJSON(w, http.StatusOK, map[string]any{"notes": notes})
}
