package app

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Real-time collaboration. The server is a relay: every payload ("d") is
// encrypted in the browser with the note key, so it only sees message types,
// sizes and who is connected. Document state lives in the browsers (Yjs);
// the oldest writer in the room is the source for newcomers and the one who
// saves to the database.
//
// Client → server: {"t": "auth", "write": proof, "admin": token} first, then
// {"t": type, "to": peer id (optional), "d": ciphertext}.
// Types: "y" doc update (writers only), "need" ask for the doc state,
// "sync" reply to "need" (writers only), "aw" presence and cursor, "chat",
// "follow" presenter position.

const (
	liveMaxMsg   = 2 << 20 // a full document state can be big
	livePeers    = 50
	liveRate     = 60 // messages per second per connection, burst included
	liveRecheck  = 10 * time.Second
	liveWriteTTL = 10 * time.Second
)

type livePeer struct {
	id     int
	write  bool // may change the document
	admin  bool
	name   string
	send   chan []byte
	cancel context.CancelFunc
}

type liveRoom struct {
	mu    sync.Mutex
	peers map[int]*livePeer
}

type liveHub struct {
	mu    sync.Mutex
	seq   int
	rooms map[int64]*liveRoom
}

func (h *liveHub) join(note int64, p *livePeer) (*liveRoom, []*livePeer, bool) {
	h.mu.Lock()
	if h.rooms == nil {
		h.rooms = map[int64]*liveRoom{}
	}
	r := h.rooms[note]
	if r == nil {
		r = &liveRoom{peers: map[int]*livePeer{}}
		h.rooms[note] = r
	}
	h.seq++
	p.id = h.seq
	h.mu.Unlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.peers) >= livePeers {
		return nil, nil, false
	}
	others := r.list()
	r.peers[p.id] = p
	return r, others, true
}

func (h *liveHub) leave(note int64, r *liveRoom, p *livePeer) {
	r.mu.Lock()
	delete(r.peers, p.id)
	empty := len(r.peers) == 0
	r.mu.Unlock()
	if empty {
		h.mu.Lock()
		if h.rooms[note] == r {
			r.mu.Lock()
			if len(r.peers) == 0 {
				delete(h.rooms, note)
			}
			r.mu.Unlock()
		}
		h.mu.Unlock()
	}
}

// count is how many rooms are open, for stats.
func (h *liveHub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.rooms)
}

type liveRoomInfo struct {
	note  int64
	peers int
}

// snapshot lists the open rooms and how many people are in each, for the
// control center. Note ids only: it can't see who or what.
func (h *liveHub) snapshot() []liveRoomInfo {
	h.mu.Lock()
	rooms := make(map[int64]*liveRoom, len(h.rooms))
	for k, v := range h.rooms {
		rooms[k] = v
	}
	h.mu.Unlock()
	out := make([]liveRoomInfo, 0, len(rooms))
	for note, r := range rooms {
		r.mu.Lock()
		out = append(out, liveRoomInfo{note: note, peers: len(r.peers)})
		r.mu.Unlock()
	}
	sort.Slice(out, func(i, j int) bool { return out[i].note < out[j].note })
	return out
}

// list returns the peers in join order. Callers hold r.mu.
func (r *liveRoom) list() []*livePeer {
	out := make([]*livePeer, 0, len(r.peers))
	for _, p := range r.peers {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out
}

// deliver queues a message; a peer too slow to keep up is dropped rather
// than holding up the room.
func deliver(p *livePeer, msg []byte) {
	select {
	case p.send <- msg:
	default:
		p.cancel()
	}
}

func (r *liveRoom) broadcast(from int, msg []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, p := range r.peers {
		if id != from {
			deliver(p, msg)
		}
	}
}

type peerInfo struct {
	ID    int  `json:"id"`
	Write bool `json:"w"`
}

func info(ps []*livePeer) []peerInfo {
	out := make([]peerInfo, len(ps))
	for i, p := range ps {
		out[i] = peerInfo{p.id, p.write}
	}
	return out
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func (a *App) apiLive(w http.ResponseWriter, r *http.Request) {
	x, ok := a.access(w, r)
	if !ok {
		return
	}
	n := x.note
	// Self-destructing notes would leak their content to every open tab, and
	// frozen ones never change.
	if n.Frozen || n.MaxViews > 0 {
		apiErr(w, http.StatusForbidden, "err.live_off")
		return
	}
	// The server's read/write timeouts are meant for plain requests.
	rc := http.NewResponseController(w)
	rc.SetReadDeadline(time.Time{})
	rc.SetWriteDeadline(time.Time{})
	// Same-origin only: the default origin check rejects other sites.
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	c.SetReadLimit(liveMaxMsg)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	defer c.CloseNow()

	// First message proves the key (and maybe admin): headers can't be set on
	// a browser WebSocket, and URLs end up in logs.
	authCtx, stop := context.WithTimeout(ctx, 10*time.Second)
	_, raw, err := c.Read(authCtx)
	stop()
	var auth struct {
		T     string `json:"t"`
		Write string `json:"write"`
		Admin string `json:"admin"`
	}
	if err != nil || json.Unmarshal(raw, &auth) != nil || auth.T != "auth" {
		c.Close(websocket.StatusPolicyViolation, "auth")
		return
	}
	x.write = hashMatch(auth.Write, n.WriteHash)
	x.admin = hashMatch(auth.Admin, n.AdminHash)
	if !x.write || (!x.admin && runBeforeRead(n, time.Now(), x.ip) != nil) {
		c.Close(websocket.StatusPolicyViolation, "denied")
		return
	}
	canWrite := func() bool {
		return x.can(modeEdit) && (x.admin || runBeforeWrite(x.note, time.Now(), x.ip) == nil)
	}

	p := &livePeer{write: canWrite(), admin: x.admin, send: make(chan []byte, 256), cancel: cancel}
	room, others, ok := a.live.join(n.ID, p)
	if !ok {
		c.Close(websocket.StatusTryAgainLater, "full")
		return
	}
	defer func() {
		a.live.leave(n.ID, room, p)
		room.broadcast(p.id, mustJSON(map[string]any{"t": "leave", "id": p.id}))
	}()
	p.send <- mustJSON(map[string]any{"t": "hello", "id": p.id, "w": p.write, "peers": info(others)})
	room.broadcast(p.id, mustJSON(map[string]any{"t": "join", "id": p.id, "w": p.write}))

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-p.send:
				wctx, stop := context.WithTimeout(ctx, liveWriteTTL)
				err := c.Write(wctx, websocket.MessageText, msg)
				stop()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()

	// Pings keep proxies (nginx closes idle connections after 60 s) and
	// half-dead mobile connections in check.
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pctx, stop := context.WithTimeout(ctx, liveWriteTTL)
				err := c.Ping(pctx)
				stop()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()

	tokens, last, checked := float64(liveRate), time.Now(), time.Now()
	for {
		_, raw, err := c.Read(ctx)
		if err != nil {
			return
		}
		now := time.Now()
		tokens = min(liveRate, tokens+now.Sub(last).Seconds()*liveRate)
		last = now
		if tokens < 1 {
			continue // over the rate: drop
		}
		tokens--
		var m struct {
			T  string `json:"t"`
			To int    `json:"to"`
			D  string `json:"d"`
		}
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		// A scheduled lock or a freeze can start mid-session.
		if p.write && now.Sub(checked) > liveRecheck {
			checked = now
			if fresh, _, err := a.db.NoteByLink(r.PathValue("token")); err != nil || fresh.Frozen {
				return
			} else {
				x.note = fresh
				still := canWrite()
				room.mu.Lock()
				p.write = still
				room.mu.Unlock()
				if !still {
					deliver(p, mustJSON(map[string]any{"t": "ro"}))
				}
			}
		}
		out := mustJSON(map[string]any{"t": m.T, "from": p.id, "d": m.D})
		switch m.T {
		case "y":
			if p.write {
				room.broadcast(p.id, out)
			}
		case "aw", "chat", "follow":
			room.broadcast(p.id, out)
		case "need":
			// The oldest writer holds the state; for a writer asking, only one
			// that joined earlier counts, so two newcomers never wait on each
			// other. Nobody: a writer starts from the saved text, a reader waits
			// for a writer to join.
			room.mu.Lock()
			var src *livePeer
			for _, q := range room.list() {
				if q.id != p.id && q.write && (!p.write || q.id < p.id) {
					src = q
					break
				}
			}
			if src != nil {
				deliver(src, out)
			} else {
				deliver(p, mustJSON(map[string]any{"t": "nosync"}))
			}
			room.mu.Unlock()
		case "sync":
			if !p.write {
				continue
			}
			room.mu.Lock()
			if q := room.peers[m.To]; q != nil {
				deliver(q, out)
			}
			room.mu.Unlock()
		}
	}
}
