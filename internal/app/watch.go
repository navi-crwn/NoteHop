package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"
	"syscall"
	"time"
)

// Watch Note: the admin gives a URL, and NoteHop POSTs to it when the note
// changes. The payload says what happened and when, never what: the server
// can't read encrypted notes, and a webhook is the wrong place for content
// anyway. Events within a few seconds are sent together, so typing doesn't
// fire one request per autosave.
//
//	POST <url>
//	X-NoteHop-Signature: sha256=<hex HMAC of the body with the note's watch key>
//	{"note": "<link token>", "events": ["updated", "entry"], "version": 7, "time": 1790000000}

var watchNote = feature{
	name: "watch",
	afterUpdate: func(c hookCtx) {
		if c.cfg.Watch != "" && c.app != nil {
			c.app.watch.add(c.note.ID, c.cfg.Watch, c.cfg.WatchKey, c.event, c.note.Version)
		}
	},
	meta: func(c hookCtx, m *metaResp) {
		if m.Admin {
			m.Watch, m.WatchKey = c.cfg.Watch, c.cfg.WatchKey
		}
	},
}

const watchQuiet = 5 * time.Second

type watchJob struct {
	url, key string
	events   []string
	version  int64
	due      time.Time
}

type watcher struct {
	mu   sync.Mutex
	jobs map[int64]*watchJob
}

func (w *watcher) add(note int64, u, key, event string, version int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.jobs == nil {
		w.jobs = map[int64]*watchJob{}
	}
	j := w.jobs[note]
	if j == nil {
		j = &watchJob{due: time.Now().Add(watchQuiet)}
		w.jobs[note] = j
	}
	j.url, j.key, j.version = u, key, version
	for _, e := range j.events {
		if e == event {
			return
		}
	}
	j.events = append(j.events, event)
}

// pending is how many webhooks wait to be sent, for stats.
func (w *watcher) pending() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.jobs)
}

func (w *watcher) due(now time.Time) map[int64]*watchJob {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := map[int64]*watchJob{}
	for id, j := range w.jobs {
		if !now.Before(j.due) {
			out[id] = j
			delete(w.jobs, id)
		}
	}
	return out
}

// runWatch delivers due webhooks once a second until ctx ends.
func (a *App) runWatch(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			for id, j := range a.watch.due(now) {
				go a.deliverWatch(id, j)
			}
		}
	}
}

func (a *App) deliverWatch(noteID int64, j *watchJob) {
	token := ""
	if links, err := a.db.Links(noteID); err == nil && len(links) > 0 {
		token = links[0].Token
	}
	body := mustJSON(map[string]any{"note": token, "events": j.events, "version": j.version, "time": time.Now().Unix()})
	mac := hmac.New(sha256.New, []byte(j.key))
	mac.Write(body)
	req, err := http.NewRequest("POST", j.url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NoteHop-Watch/1")
	req.Header.Set("X-NoteHop-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	res, err := a.hookClient().Do(req)
	if err != nil {
		log.Printf("watch: note %d: %v", noteID, err)
		return
	}
	res.Body.Close()
}

// hookURLOK accepts http(s) URLs; https only unless private hooks are allowed
// (a self-hosted setup talking to its own services).
func (a *App) hookURLOK(s string) bool {
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || len(s) > 500 || u.User != nil {
		return false
	}
	return u.Scheme == "https" || (u.Scheme == "http" && a.cfg.PrivateHooks)
}

var errPrivateAddr = errors.New("address not allowed")

// publicIP refuses loopback, private, link-local and similar addresses, so
// a webhook can't be pointed at the server's own network (SSRF).
func publicIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	if v4 := ip.To4(); v4 != nil && (v4[0] == 100 && v4[1]&0xc0 == 64 || v4[0] == 0) { // CGNAT, "this network"
		return false
	}
	// IPv6 addresses that carry an IPv4 one inside: a NAT64 gateway (64:ff9b::/96,
	// 64:ff9b:1::/48) or 6to4 relay (2002::/16) would forward 64:ff9b::7f00:1 to
	// 127.0.0.1. Judge them by the IPv4 address inside.
	if ip16 := ip.To16(); ip16 != nil && ip.To4() == nil {
		switch {
		case nat64.Contains(ip16), nat64Local.Contains(ip16):
			return publicIP(net.IP(ip16[12:16]))
		case sixToFour.Contains(ip16):
			return publicIP(net.IP(ip16[2:6]))
		}
	}
	return true
}

var (
	nat64      = mustCIDR("64:ff9b::/96")
	nat64Local = mustCIDR("64:ff9b:1::/48")
	sixToFour  = mustCIDR("2002::/16")
)

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

func (a *App) hookClient() *http.Client {
	d := &net.Dialer{Timeout: 5 * time.Second}
	if !a.cfg.PrivateHooks {
		// Checked on the address actually dialled, after DNS: a name that
		// resolves to 127.0.0.1 is refused too.
		d.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil || !publicIP(net.ParseIP(host)) {
				return errPrivateAddr
			}
			return nil
		}
	}
	return &http.Client{
		Timeout:       10 * time.Second,
		Transport:     &http.Transport{DialContext: d.DialContext, Proxy: nil, TLSHandshakeTimeout: 5 * time.Second},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
