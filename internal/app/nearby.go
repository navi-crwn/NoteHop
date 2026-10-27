package app

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Nearby devices (/nearby): browsers behind the same public IP (same Wi-Fi,
// same office) find each other here, then talk directly over WebRTC for the
// shared pad and file transfers. The server only relays the WebRTC
// handshake; pad text and files never pass through it. Rooms live in
// memory and are keyed by the address, which is never stored.

const (
	nearbyPeers  = 20
	nearbyMaxMsg = 64 << 10
)

type nearbyPeer struct {
	id     string
	name   string
	device string
	send   chan []byte
	cancel context.CancelFunc
}

type nearbyHub struct {
	mu    sync.Mutex
	rooms map[string]map[string]*nearbyPeer
}

// nearbyRoom groups IPv4 by address and IPv6 by /64 (one home network).
func nearbyRoom(ip string) string {
	a := net.ParseIP(ip)
	if a == nil {
		return ip
	}
	if v4 := a.To4(); v4 != nil {
		return v4.String()
	}
	return a.Mask(net.CIDRMask(64, 128)).String()
}

func (h *nearbyHub) join(room string, p *nearbyPeer) ([]*nearbyPeer, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms == nil {
		h.rooms = map[string]map[string]*nearbyPeer{}
	}
	r := h.rooms[room]
	if r == nil {
		r = map[string]*nearbyPeer{}
		h.rooms[room] = r
	}
	if len(r) >= nearbyPeers {
		return nil, false
	}
	others := make([]*nearbyPeer, 0, len(r))
	for _, q := range r {
		others = append(others, q)
	}
	r[p.id] = p
	return others, true
}

func (h *nearbyHub) leave(room string, p *nearbyPeer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.rooms[room], p.id)
	if len(h.rooms[room]) == 0 {
		delete(h.rooms, room)
	}
}

// each runs fn for every peer in the room except one; callers must not block.
func (h *nearbyHub) each(room, except string, fn func(*nearbyPeer)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, q := range h.rooms[room] {
		if id != except {
			fn(q)
		}
	}
}

func nearbyDeliver(p *nearbyPeer, msg []byte) {
	select {
	case p.send <- msg:
	default:
		p.cancel()
	}
}

type nearbyInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Device string `json:"device"`
}

func (a *App) apiNearby(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	rc.SetReadDeadline(time.Time{})
	rc.SetWriteDeadline(time.Time{})
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	c.SetReadLimit(nearbyMaxMsg)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	defer c.CloseNow()

	room := nearbyRoom(a.clientIP(r))
	p := &nearbyPeer{id: randToken(12), send: make(chan []byte, 128), cancel: cancel}
	others, ok := a.near.join(room, p)
	if !ok {
		c.Close(websocket.StatusTryAgainLater, "full")
		return
	}
	defer func() {
		a.near.leave(room, p)
		a.near.each(room, p.id, func(q *nearbyPeer) { nearbyDeliver(q, mustJSON(map[string]any{"t": "leave", "id": p.id})) })
	}()
	list := make([]nearbyInfo, 0, len(others))
	a.near.mu.Lock() // names are written under the hub lock
	for _, q := range others {
		list = append(list, nearbyInfo{q.id, q.name, q.device})
	}
	a.near.mu.Unlock()
	p.send <- mustJSON(map[string]any{"t": "hello", "id": p.id, "peers": list})

	go func() {
		ping := time.NewTicker(30 * time.Second)
		defer ping.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ping.C:
				pctx, stop := context.WithTimeout(ctx, liveWriteTTL)
				err := c.Ping(pctx)
				stop()
				if err != nil {
					cancel()
					return
				}
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

	tokens, last := float64(liveRate), time.Now()
	for {
		_, raw, err := c.Read(ctx)
		if err != nil {
			return
		}
		now := time.Now()
		tokens = min(liveRate, tokens+now.Sub(last).Seconds()*liveRate)
		last = now
		if tokens < 1 {
			continue
		}
		tokens--
		var m struct {
			T      string          `json:"t"`
			To     string          `json:"to"`
			Name   string          `json:"name"`
			Device string          `json:"device"`
			D      json.RawMessage `json:"d"`
		}
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		switch m.T {
		case "hi": // name and device type, shown to the others
			a.near.mu.Lock()
			p.name, p.device = clipRunes(m.Name, 40), clipRunes(m.Device, 20)
			info := nearbyInfo{p.id, p.name, p.device}
			a.near.mu.Unlock()
			out := mustJSON(map[string]any{"t": "peer", "peer": info})
			a.near.each(room, p.id, func(q *nearbyPeer) { nearbyDeliver(q, out) })
		case "sig": // WebRTC offer/answer/candidate for one peer
			out := mustJSON(map[string]any{"t": "sig", "from": p.id, "d": m.D})
			a.near.each(room, p.id, func(q *nearbyPeer) {
				if q.id == m.To {
					nearbyDeliver(q, out)
				}
			})
		}
	}
}
