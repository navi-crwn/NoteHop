package app

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// Recovery codes and changing the password (or, for single sign-on accounts,
// the vault phrase). The vault key never comes here. The browser seals it
// with each recovery code and with the key from the password; the server keeps
// those sealed copies (users.vault_wrap, recovery_codes.wrap) and SHA-256 of
// each code's proof (HKDF of the code, see NHC.recoveryKeys). So it can tell a
// right code from a wrong one and hand back that code's sealed copy, but it
// can't open any of them. Without a code, nobody can reset a password: that
// is the price of a server that can't read your notes.

const recoveryCount = 8

var wrapRe = regexp.MustCompile(`^[A-Za-z0-9_-]{40,300}$`)

func secret32(s string) ([]byte, bool) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	return b, err == nil && len(b) == 32
}

// proves checks the account's current secret: the login secret for password
// accounts, the vault phrase check for single sign-on ones.
func proves(u *db.User, current string) bool {
	b, ok := secret32(current)
	hash := u.AuthHash
	if u.SSOID != "" {
		hash = u.VaultCheck
	}
	if !ok || hash == "" {
		checkAuth([]byte("x"), dummyHash) // same work either way
		return false
	}
	return checkAuth(b, hash)
}

func proofHash(proof []byte) string { s := sha256.Sum256(proof); return hex.EncodeToString(s[:]) }

// recoveryTickets lets the second step of a recovery (the new password) use
// the code checked in the first step, once, within ten minutes.
type recoveryTickets struct {
	sync.Mutex
	m map[string]recoveryTicket
}

type recoveryTicket struct {
	user int64
	slot int
	exp  time.Time
}

func (t *recoveryTickets) put(user int64, slot int) string {
	t.Lock()
	defer t.Unlock()
	for k, v := range t.m {
		if time.Now().After(v.exp) {
			delete(t.m, k)
		}
	}
	tok := randToken(32)
	t.m[tok] = recoveryTicket{user: user, slot: slot, exp: time.Now().Add(10 * time.Minute)}
	return tok
}

func (t *recoveryTickets) take(tok string) (recoveryTicket, bool) {
	t.Lock()
	defer t.Unlock()
	v, ok := t.m[tok]
	delete(t.m, tok)
	return v, ok && time.Now().Before(v.exp)
}

type recoveryStatus struct {
	Total   int   `json:"total"`
	Left    int   `json:"left"`
	Created int64 `json:"created"`
}

func (a *App) recoveryStatus(u *db.User) recoveryStatus {
	codes, err := a.db.RecoveryCodes(u.ID)
	if err != nil {
		log.Printf("recovery codes: %v", err)
	}
	var s recoveryStatus
	for _, c := range codes {
		s.Total++
		if c.Used == 0 {
			s.Left++
		}
		s.Created = c.Created
	}
	return s
}

func (a *App) securityPage(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login?next=/security", http.StatusSeeOther)
		return
	}
	a.render(w, r, http.StatusOK, "security", pageData{Title: a.locale(w, r).T("sec.title"), User: u, NoIndex: true})
}

// apiSetRecovery stores a new set of recovery codes (the old ones stop
// working). It asks for the current secret, so a stolen session cookie can't
// swap in codes of its own.
func (a *App) apiSetRecovery(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if u == nil {
		apiErr(w, http.StatusUnauthorized, "err.not_signed_in")
		return
	}
	var q struct {
		Current string `json:"current"`
		Codes   []struct {
			Proof string `json:"proof"`
			Wrap  string `json:"wrap"`
		} `json:"codes"`
	}
	if !readJSON(w, r, &q, 8<<10) {
		return
	}
	if len(q.Codes) != recoveryCount {
		apiErr(w, http.StatusBadRequest, "err.bad_data")
		return
	}
	codes := make([]db.RecoveryCode, len(q.Codes))
	seen := map[string]bool{}
	for i, c := range q.Codes {
		p, ok := secret32(c.Proof)
		if !ok || !wrapRe.MatchString(c.Wrap) || seen[c.Proof] {
			apiErr(w, http.StatusBadRequest, "err.bad_data")
			return
		}
		seen[c.Proof] = true
		codes[i] = db.RecoveryCode{ProofHash: proofHash(p), Wrap: c.Wrap}
	}
	if !proves(u, q.Current) {
		apiErr(w, http.StatusUnauthorized, secretWrong(u))
		return
	}
	if err := a.db.SetRecoveryCodes(u.ID, codes); err != nil {
		log.Printf("set recovery: %v", err)
		apiErr(w, http.StatusInternalServerError, "err.recovery_failed")
		return
	}
	writeJSON(w, http.StatusOK, a.recoveryStatus(u))
}

func secretWrong(u *db.User) string {
	if u.SSOID != "" {
		return "err.vault_wrong"
	}
	return "err.password_wrong"
}

// apiChangeSecret sets a new password (or vault phrase). The vault key stays
// the same, sealed again with the new one; other devices are signed out.
func (a *App) apiChangeSecret(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if u == nil {
		apiErr(w, http.StatusUnauthorized, "err.not_signed_in")
		return
	}
	var q struct {
		Current   string `json:"current"`
		Next      string `json:"next"`
		VaultWrap string `json:"vault_wrap"`
	}
	if !readJSON(w, r, &q, 2048) {
		return
	}
	next, ok := secret32(q.Next)
	if !ok || !wrapRe.MatchString(q.VaultWrap) {
		apiErr(w, http.StatusBadRequest, "err.bad_data")
		return
	}
	if !proves(u, q.Current) {
		apiErr(w, http.StatusUnauthorized, secretWrong(u))
		return
	}
	if err := a.db.SetSecret(u.ID, u.SSOID != "", hashAuth(next), q.VaultWrap); err != nil {
		log.Printf("change secret: %v", err)
		apiErr(w, http.StatusInternalServerError, "err.recovery_failed")
		return
	}
	keep := ""
	if c, err := r.Cookie(sessionCookie); err == nil {
		keep = hashHex(c.Value)
	}
	if err := a.db.DeleteOtherSessions(u.ID, keep); err != nil {
		log.Printf("sign out others: %v", err)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// apiRecover checks a recovery code (step 1). Password accounts give their
// username; single sign-on accounts are signed in already and only lost their
// vault phrase. The answer is that code's sealed vault key and a ticket for
// step 2. A wrong code and an unknown username get the same answer.
func (a *App) apiRecover(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Username string `json:"username"`
		Proof    string `json:"proof"`
	}
	if !readJSON(w, r, &q, 1024) {
		return
	}
	q.Username = strings.ToLower(strings.TrimSpace(q.Username))
	proof, ok := secret32(q.Proof)
	if !ok {
		apiErr(w, http.StatusBadRequest, "err.bad_data")
		return
	}
	var u *db.User
	if me := a.currentUser(r); me != nil && me.SSOID != "" && (q.Username == "" || q.Username == me.Username) {
		u = me
	} else if userRe.MatchString(q.Username) {
		if found, err := a.db.UserByName(q.Username); err == nil && found.SSOID == "" {
			u = found
		} else if err == nil {
			apiErr(w, http.StatusBadRequest, "err.recover_sso", "name", a.cfg.SSOName)
			return
		}
	} else {
		apiErr(w, http.StatusBadRequest, "err.bad_username")
		return
	}
	// Online guessing: on top of the per-address limit, a few tries a minute per account.
	who := q.Username
	if u != nil {
		who = u.Username
	}
	if !a.limit.allow("r"+who, 5) {
		w.Header().Set("Retry-After", "60")
		apiErr(w, http.StatusTooManyRequests, "err.rate_limited")
		return
	}
	var codes []db.RecoveryCode
	if u != nil {
		codes, _ = a.db.RecoveryCodes(u.ID)
	}
	want, slot := proofHash(proof), -1
	var wrap string
	for _, c := range codes {
		if c.Used == 0 && subtle.ConstantTimeCompare([]byte(c.ProofHash), []byte(want)) == 1 {
			slot, wrap = c.Slot, c.Wrap
		}
	}
	if slot < 0 {
		apiErr(w, http.StatusUnauthorized, "err.recover_wrong")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"username": u.Username, "sso": u.SSOID != "", "wrap": wrap, "ticket": a.tickets.put(u.ID, slot)})
}

// apiRecoverFinish sets the new password (or vault phrase) with the ticket
// from step 1. The code is spent, every device is signed out, and this one
// is signed in again.
func (a *App) apiRecoverFinish(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Ticket    string `json:"ticket"`
		Next      string `json:"next"`
		VaultWrap string `json:"vault_wrap"`
	}
	if !readJSON(w, r, &q, 2048) {
		return
	}
	next, ok := secret32(q.Next)
	if !ok || !wrapRe.MatchString(q.VaultWrap) {
		apiErr(w, http.StatusBadRequest, "err.bad_data")
		return
	}
	t, ok := a.tickets.take(q.Ticket)
	if !ok {
		apiErr(w, http.StatusBadRequest, "err.recover_expired")
		return
	}
	u, err := a.db.UserByID(t.user)
	if err != nil {
		apiErr(w, http.StatusBadRequest, "err.recover_expired")
		return
	}
	err = a.db.Recover(u.ID, t.slot, u.SSOID != "", hashAuth(next), q.VaultWrap)
	if errors.Is(err, db.ErrConflict) {
		apiErr(w, http.StatusConflict, "err.recover_used")
		return
	}
	if err != nil {
		log.Printf("recover: %v", err)
		apiErr(w, http.StatusInternalServerError, "err.recovery_failed")
		return
	}
	a.startSession(w, u)
	writeJSON(w, http.StatusOK, map[string]any{"username": u.Username, "left": a.recoveryStatus(u).Left})
}
