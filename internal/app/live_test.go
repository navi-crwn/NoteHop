package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type wsPeer struct {
	t *testing.T
	c *websocket.Conn
}

func dialLive(t *testing.T, c *client, token, write, admin string) (*wsPeer, map[string]any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(c.srv.URL, "http")+"/api/n/"+token+"/live", nil)
	if err != nil {
		t.Fatal(err)
	}
	p := &wsPeer{t, conn}
	t.Cleanup(func() { conn.CloseNow() })
	p.send(map[string]any{"t": "auth", "write": write, "admin": admin})
	return p, p.recv()
}

func (p *wsPeer) send(v any) {
	b, _ := json.Marshal(v)
	if err := p.c.Write(context.Background(), websocket.MessageText, b); err != nil {
		p.t.Fatal(err)
	}
}

func (p *wsPeer) recv() map[string]any {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, b, err := p.c.Read(ctx)
	if err != nil {
		return nil
	}
	var m map[string]any
	json.Unmarshal(b, &m)
	return m
}

// next skips presence noise until a message of type t arrives (or nothing).
func (p *wsPeer) next(t string) map[string]any {
	for {
		m := p.recv()
		if m == nil || m["t"] == t {
			return m
		}
	}
}

func TestLive(t *testing.T) {
	_, _, c := newAppDB(t)
	n := c.create(map[string]any{"mode": "edit"})
	adm := map[string]string{"X-Admin": n.admin}
	_, rl, _ := c.do("POST", "/api/n/"+n.token+"/links", map[string]any{"mode": "read"}, adm)
	read := rl["token"].(string)

	// Wrong key: closed without a hello.
	if _, hello := dialLive(t, c, n.token, key(), ""); hello != nil {
		t.Fatalf("bad key got %v", hello)
	}

	a, ha := dialLive(t, c, n.token, n.write, "")
	if ha["t"] != "hello" || ha["w"] != true {
		t.Fatalf("hello: %v", ha)
	}
	a.send(map[string]any{"t": "need"})
	if m := a.next("nosync"); m == nil {
		t.Fatal("first writer should start from the saved text")
	}

	b, hb := dialLive(t, c, n.token, n.write, "")
	if len(hb["peers"].([]any)) != 1 {
		t.Fatalf("peers: %v", hb)
	}
	b.send(map[string]any{"t": "need"})
	m := a.next("need")
	if m == nil {
		t.Fatal("need not forwarded to the older writer")
	}
	a.send(map[string]any{"t": "sync", "to": m["from"], "d": "STATE"})
	if m := b.next("sync"); m == nil || m["d"] != "STATE" {
		t.Fatalf("sync: %v", m)
	}
	b.send(map[string]any{"t": "y", "d": "U1"})
	if m := a.next("y"); m == nil || m["d"] != "U1" {
		t.Fatalf("update: %v", m)
	}

	// A reader gets updates but can't send them, and can't fake a sync.
	r, hr := dialLive(t, c, read, n.write, "")
	if hr["w"] != false {
		t.Fatalf("reader hello: %v", hr)
	}
	r.send(map[string]any{"t": "y", "d": "EVIL"})
	r.send(map[string]any{"t": "chat", "d": "hi"})
	if m := a.next("chat"); m == nil || m["d"] != "hi" {
		t.Fatalf("chat: %v", m)
	}
	r.send(map[string]any{"t": "need"})
	m = a.next("need")
	if m == nil {
		t.Fatal("reader's need should reach the oldest writer")
	}
	r.send(map[string]any{"t": "sync", "to": ha["id"], "d": "EVIL"})
	b.send(map[string]any{"t": "y", "d": "U2"})
	if m := a.next("y"); m == nil || m["d"] != "U2" {
		t.Fatalf("reader update leaked through: %v", m)
	}
	if m := r.next("y"); m == nil || m["d"] != "U2" {
		t.Fatalf("reader should receive updates: %v", m)
	}

	// Self-destructing notes have no live room.
	burn := c.create(map[string]any{"max_views": 1})
	code, _, _ := c.do("GET", "/api/n/"+burn.token+"/live", nil, nil)
	if code != 403 {
		t.Fatalf("burn note live: %d", code)
	}
}

func TestNearby(t *testing.T) {
	_, _, c := newAppDB(t)
	dial := func() *wsPeer {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(c.srv.URL, "http")+"/api/nearby?code=123456", nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.CloseNow() })
		return &wsPeer{t, conn}
	}
	x := dial()
	hx := x.next("hello")
	y := dial()
	hy := y.next("hello")
	if ps := hy["peers"].([]any); len(ps) != 1 || ps[0].(map[string]any)["id"] != hx["id"] {
		t.Fatalf("peers: %v", hy)
	}
	y.send(map[string]any{"t": "hi", "name": "Laptop", "device": "computer"})
	if m := x.next("peer"); m == nil || m["peer"].(map[string]any)["name"] != "Laptop" {
		t.Fatalf("hi: %v", m)
	}
	y.send(map[string]any{"t": "sig", "to": hx["id"], "d": map[string]any{"sdp": "x"}})
	if m := x.next("sig"); m == nil || m["from"] != hy["id"] {
		t.Fatalf("sig: %v", m)
	}
	y.c.CloseNow()
	if m := x.next("leave"); m == nil || m["id"] != hy["id"] {
		t.Fatalf("leave: %v", m)
	}
	if nearbyRoom("2001:db8:1:2:aa::1") != nearbyRoom("2001:db8:1:2:bb::9") || nearbyRoom("10.0.0.1") == nearbyRoom("10.0.0.2") {
		t.Fatal("room grouping")
	}
}
