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
// every attempt is refused for a while, the right one too. Only WRONG proofs count: an open editor sends its (right)
// proof every few seconds and must never use up the allowance. The note's owner (signed in) and its admin key are
// not counted or blocked: neither has another way in, and a stranger could otherwise lock them out.
const (
	pwTriesPerMinute = 10  // wrong proofs from one address for one note
	pwTriesPerHour   = 200 // wrong proofs from everybody for one note
)

type pwWindow struct {
	start time.Time
	n     int
}

// pwGuards is a fixed-window counter per key.
type pwGuards struct {
	mu sync.Mutex
	m  map[string]*pwWindow
}

// left is how long a key stays over its limit; 0 when it is not over.
func (g *pwGuards) left(key string, limit int, span time.Duration, now time.Time) time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()
	w := g.m[key]
	if w == nil || now.Sub(w.start) >= span || w.n < limit {
		return 0
	}
	return span - now.Sub(w.start)
}

func (g *pwGuards) fail(key string, span time.Duration, now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.m == nil {
		g.m = map[string]*pwWindow{}
	}
	if len(g.m) > 20000 { // forget what has expired so the map cannot grow without end
		for k, w := range g.m {
			if now.Sub(w.start) >= time.Hour {
				delete(g.m, k)
			}
		}
	}
	w := g.m[key]
	if w == nil || now.Sub(w.start) >= span {
		g.m[key] = &pwWindow{start: now, n: 1}
		return
	}
	w.n++
}

// writeProof compares the supplied write proof with the note's. For a note with a password it also counts wrong
// proofs and reports how long the caller is blocked (0 = not) once an address or the note as a whole is over its
// limit. exempt is asked only when it matters (a wrong proof, or a limit already reached), never on the normal path.
func (a *App) writeProof(n *db.Note, supplied, ip string, exempt func() bool) (match bool, blocked time.Duration) {
	match = hashMatch(supplied, n.WriteHash)
	if !n.Password || supplied == "" {
		return match, 0
	}
	root := n.ID // a snapshot shares its note's password and its allowance
	if n.ParentID.Valid {
		root = n.ParentID.Int64
	}
	kNote := "n" + strconv.FormatInt(root, 10)
	kIP := kNote + "|" + ip
	now := time.Now()
	left := a.pwTries.left(kIP, pwTriesPerMinute, time.Minute, now)
	if l := a.pwTries.left(kNote, pwTriesPerHour, time.Hour, now); l > left {
		left = l
	}
	if match && left == 0 {
		return true, 0
	}
	if exempt() {
		return match, 0
	}
	if left > 0 {
		return false, left
	}
	a.pwTries.fail(kIP, time.Minute, now)
	a.pwTries.fail(kNote, time.Hour, now)
	return false, 0
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
