package app

import (
	"strings"
	"net/http"
	"sync"
	"time"
)

// hubProbe says whether the hub answers, so a re-check never sends a browser to an account server that is down
// (it would land on the browser's own error page). One light GET of <hub>/up with a short timeout, the
// answer kept for a minute; only a 2xx answer counts as reachable (a 502 from a proxy in front of a dead hub does not).
type hubProbe struct {
	url    string
	mu     sync.Mutex
	at     time.Time
	up     bool
	client *http.Client
}

const hubProbeTTL = time.Minute

func newHubProbe(hub string) *hubProbe {
	url := strings.TrimRight(hub, "/") + "/up"
	return &hubProbe{url: url, client: &http.Client{Timeout: 1500 * time.Millisecond,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (p *hubProbe) Up() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.at.IsZero() && time.Since(p.at) < hubProbeTTL {
		return p.up
	}
	up := false
	if res, err := p.client.Get(p.url); err == nil {
		res.Body.Close()
		up = res.StatusCode >= 200 && res.StatusCode < 300
	}
	p.at, p.up = time.Now(), up
	return up
}
