package app

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// serverClosed reports whether the server ended the connection within a few
// seconds. A read that only times out means the peer is still connected.
func (p *wsPeer) serverClosed() bool {
	p.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	for {
		if _, _, err := p.c.Read(ctx); err != nil {
			return !errors.Is(err, context.DeadlineExceeded)
		}
	}
}

// stillWorks checks that a peer is connected and the room answers it.
func (p *wsPeer) stillWorks() bool {
	p.send(map[string]any{"t": "need"})
	return p.next("nosync") != nil
}

func liveRoomsGone(t *testing.T, a *App) {
	t.Helper()
	liveRoomsLeft(t, a, 0)
}

// liveRoomsLeft waits for the count to settle: a closed peer's connection ends the
// moment it is cancelled, and its handler leaves the room just after that.
func liveRoomsLeft(t *testing.T, a *App, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for a.live.count() != want {
		if time.Now().After(deadline) {
			t.Fatalf("%d live room(s) open, want %d", a.live.count(), want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A note with a writer and an idle reader in its live room.
func liveFixture(t *testing.T, c *client) (n note, writer, reader *wsPeer, readToken string) {
	t.Helper()
	n = c.create(map[string]any{"mode": "edit"})
	_, rl, _ := c.do("POST", "/api/n/"+n.token+"/links", map[string]any{"mode": "read"}, map[string]string{"X-Admin": n.admin})
	readToken = rl["token"].(string)
	writer, hello := dialLive(t, c, n.token, n.write, n.admin)
	if hello == nil || hello["w"] != true {
		t.Fatalf("writer hello: %v", hello)
	}
	reader, hello = dialLive(t, c, readToken, n.write, "")
	if hello == nil || hello["w"] != false {
		t.Fatalf("reader hello: %v", hello)
	}
	return n, writer, reader, readToken
}

func TestLiveDeleteClosesWriterAndIdleReader(t *testing.T) {
	a, _, c := newAppDB(t)
	n, writer, reader, _ := liveFixture(t, c)
	if code, _, raw := c.do("DELETE", "/api/n/"+n.token, nil, map[string]string{"X-Admin": n.admin}); code != http.StatusOK {
		t.Fatalf("delete: %d %s", code, raw)
	}
	if !reader.serverClosed() {
		t.Fatal("idle reader still connected after the note was deleted")
	}
	if !writer.serverClosed() {
		t.Fatal("writer still connected after the note was deleted")
	}
	liveRoomsGone(t, a)
}

func TestLiveRevokedLinkClosesOnlyThatLink(t *testing.T) {
	a, _, c := newAppDB(t)
	n, writer, reader, readToken := liveFixture(t, c)
	if code, _, raw := c.do("DELETE", "/api/n/"+n.token+"/links/"+readToken, nil, map[string]string{"X-Admin": n.admin}); code != http.StatusOK {
		t.Fatalf("revoke: %d %s", code, raw)
	}
	if !reader.serverClosed() {
		t.Fatal("reader still connected after its link was revoked")
	}
	if !writer.stillWorks() {
		t.Fatal("writer on another link was dropped")
	}
	if got := a.live.count(); got != 1 {
		t.Fatalf("rooms: %d", got)
	}
	writer.c.Close(websocket.StatusNormalClosure, "done")
}

// Expiry has no request to hook into: the janitor's first pass must do it, and
// leave rooms of notes that are still valid alone.
func TestLiveJanitorClosesExpiredRoomOnly(t *testing.T) {
	a, d, c := newAppDB(t)
	n, writer, reader, _ := liveFixture(t, c)
	kept := c.create(map[string]any{"mode": "edit"})
	keptWriter, hello := dialLive(t, c, kept.token, kept.write, kept.admin)
	if hello == nil {
		t.Fatal("kept writer did not join")
	}
	x, _, err := d.NoteByLink(n.token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`UPDATE notes SET expires = ? WHERE id = ?`, time.Now().Unix()-1, x.ID); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { a.Janitor(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	if !reader.serverClosed() || !writer.serverClosed() {
		t.Fatal("peers of an expired note are still connected")
	}
	liveRoomsLeft(t, a, 1)
	if !keptWriter.stillWorks() {
		t.Fatal("room of a valid note was closed")
	}
}

// Takedown from the command line runs in another process and cannot call the
// hub; the janitor's pass is what ends those rooms.
func TestLiveJanitorClosesRoomOfNoteDeletedElsewhere(t *testing.T) {
	a, d, c := newAppDB(t)
	n, writer, reader, _ := liveFixture(t, c)
	x, _, err := d.NoteByLink(n.token)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Takedown(x.ID); err != nil {
		t.Fatal(err)
	}
	a.dropGoneRooms()
	if !reader.serverClosed() || !writer.serverClosed() {
		t.Fatal("peers of a removed note are still connected")
	}
	liveRoomsGone(t, a)
}

func TestLiveHopActionsCloseRooms(t *testing.T) {
	srv, a, d := hopApp(t, Config{})
	c := newClient(t, srv)
	act := func(n note, action string) {
		t.Helper()
		x, _, err := d.NoteByLink(n.token)
		if err != nil {
			t.Fatal(err)
		}
		if code, env := call(t, srv, "imperium", "POST", "/hop/content/note/"+strconv.FormatInt(x.ID, 10)+"/actions", map[string]any{"action": action, "reason": "test"}); code != 200 {
			t.Fatalf("%s: %d %v", action, code, env)
		}
	}
	for _, action := range []string{"delete", "takedown"} {
		n, writer, reader, _ := liveFixture(t, c)
		act(n, action)
		if !reader.serverClosed() || !writer.serverClosed() {
			t.Fatalf("peers still connected after hop %s", action)
		}
		liveRoomsGone(t, a)
	}

	// A report takedown.
	n, writer, reader, _ := liveFixture(t, c)
	if code, _, raw := c.do("POST", "/api/n/"+n.token+"/report", map[string]any{"reason": "spam"}, nil); code != 200 && code != 201 {
		t.Fatalf("report: %d %s", code, raw)
	}
	var id int64
	if err := d.QueryRow(`SELECT MAX(id) FROM reports`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if code, env := call(t, srv, "imperium", "POST", "/hop/reports/"+strconv.FormatInt(id, 10)+"/actions", map[string]any{"action": "takedown"}); code != 200 {
		t.Fatalf("report takedown: %d %v", code, env)
	}
	if !reader.serverClosed() || !writer.serverClosed() {
		t.Fatal("peers still connected after report takedown")
	}
	liveRoomsGone(t, a)
}

func TestLiveHopUserActionsCloseRooms(t *testing.T) {
	for _, action := range []string{"delete_notes", "purge"} {
		t.Run(action, func(t *testing.T) {
			srv, a, _ := hopApp(t, Config{})
			u := ssoUser(t, srv, "42", "budi@example.com", nil)
			n, writer, reader, _ := liveFixture(t, u)
			if code, env := call(t, srv, "imperium", "POST", "/hop/users/42/actions", map[string]any{"action": action, "reason": "test"}); code != 200 {
				t.Fatalf("%s: %d %v", action, code, env)
			}
			_ = n
			if !reader.serverClosed() || !writer.serverClosed() {
				t.Fatalf("peers still connected after %s", action)
			}
			liveRoomsGone(t, a)
		})
	}
}
