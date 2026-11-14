package app

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// A basic note with a password is guarded by that password alone: the server can read the text, and the only thing
// between a stranger and it is the proof derived from the password. So wrong guesses are counted, and past a limit
// attempts are refused for a while. Rules (each one is tested):
//   - only BASIC notes: an encrypted note's proof also needs its 256-bit note key, so there is nothing to guess, and
//     answering "too many tries" there would give a decoy away and lock out the holder of the real password;
//   - only WRONG proofs count: an open editor sends its (right) proof every few seconds and never uses up anything;
//   - 10 wrong proofs a minute from one address for one note, and 200 an hour for the note from everybody. The hourly
//     limit does not shut out an address that already opened the note (it proved the password within the last six
//     hours), so a crowd of strangers cannot lock the people who have it; their own address limit still holds;
//   - a right proof from an address clears that address's wrong count, so someone who mistyped can still get in;
//   - the signed-in owner is never counted or refused (no other way in); the ADMIN KEY is not an exemption: whoever
//     holds only an admin link must know the password like anybody else;
//   - checking and counting happen under one lock, so concurrent guesses cannot slip past the limit;
//   - taking the note over (a new password) starts the counts again.
const (
	pwTriesPerMinute = 10  // wrong proofs from one address for one note
	pwTriesPerHour   = 200 // wrong proofs from everybody for one note
	pwTrustFor       = 6 * time.Hour
	pwSweepPerCall   = 20 // how many notes' old entries one call may tidy: the cost of a call stays small
)

type pwIP struct {
	minStart time.Time
	minN     int
	okAt     time.Time // last right proof from this address
}

type pwNote struct {
	hourStart time.Time
	hourN     int
	ips       map[string]*pwIP
}

// pwGuards keeps the counts per note.
type pwGuards struct {
	mu sync.Mutex
	m  map[int64]*pwNote
}

// attempt counts one proof and says whether the caller is refused (wait > 0, nothing counted then). match is whether the
// proof was right. A right proof refused only because of a limit is still refused.
func (g *pwGuards) attempt(root int64, ip string, match bool, now time.Time) (wait time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.m == nil {
		g.m = map[int64]*pwNote{}
	}
	n := g.m[root]
	if n == nil {
		n = &pwNote{ips: map[string]*pwIP{}}
		g.m[root] = n
	}
	if now.Sub(n.hourStart) >= time.Hour {
		n.hourStart, n.hourN = now, 0
	}
	e := n.ips[ip]
	if e == nil {
		e = &pwIP{}
		n.ips[ip] = e
	}
	if now.Sub(e.minStart) >= time.Minute {
		e.minStart, e.minN = now, 0
	}
	trusted := !e.okAt.IsZero() && now.Sub(e.okAt) < pwTrustFor
	switch {
	case e.minN >= pwTriesPerMinute:
		wait = time.Minute - now.Sub(e.minStart)
	case n.hourN >= pwTriesPerHour && !trusted:
		wait = time.Hour - now.Sub(n.hourStart)
	}
	if wait > 0 {
		g.sweep(now)
		return wait
	}
	if match {
		e.okAt, e.minN = now, 0
	} else {
		e.minN++
		n.hourN++
	}
	g.sweep(now)
	return 0
}

// sweep forgets what has expired, a few notes per call (the map's own iteration order is the cursor), so no call
// ever walks everything. Caller holds the lock.
func (g *pwGuards) sweep(now time.Time) {
	seen := 0
	for id, n := range g.m {
		if seen++; seen > pwSweepPerCall {
			return
		}
		idle := now.Sub(n.hourStart) >= time.Hour
		left := 0
		for ip, e := range n.ips {
			if left++; left > 50 {
				idle = false
				break
			}
			if now.Sub(e.minStart) >= time.Minute && (e.okAt.IsZero() || now.Sub(e.okAt) >= pwTrustFor) {
				delete(n.ips, ip)
			}
		}
		if idle && len(n.ips) == 0 {
			delete(g.m, id)
		}
	}
}

// forget starts a note's counts again (its password was just replaced).
func (g *pwGuards) forget(root int64) {
	g.mu.Lock()
	delete(g.m, root)
	g.mu.Unlock()
}

// writeProof compares the supplied write proof with the note's. For a basic note with a password it also counts wrong
// proofs and reports how long the caller is refused (0 = not). exempt is asked only when it matters.
func (a *App) writeProof(n *db.Note, supplied, ip string, exempt func() bool) (match bool, blocked time.Duration) {
	match = hashMatch(supplied, n.WriteHash)
	if !n.Password || n.Encrypted || supplied == "" {
		return match, 0
	}
	root := n.ID // a snapshot shares its note's password and its allowance
	if n.ParentID.Valid {
		root = n.ParentID.Int64
	}
	if !match && exempt() {
		return false, 0
	}
	wait := a.pwTries.attempt(root, ip, match, time.Now())
	if wait > 0 && match && exempt() {
		return true, 0
	}
	if wait > 0 {
		return false, wait
	}
	return match, 0
}

// ownerSession is true when the request carries the signed-in session of the note's owner.
func (a *App) ownerSession(r *http.Request, n *db.Note) bool {
	u := a.currentUser(r)
	return u != nil && n.OwnerID.Valid && n.OwnerID.Int64 == u.ID
}

// tooManyPasswordTries answers a request refused by writeProof.
func tooManyPasswordTries(w http.ResponseWriter, wait time.Duration) {
	w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
	apiErr(w, http.StatusTooManyRequests, "err.password_tries")
}
