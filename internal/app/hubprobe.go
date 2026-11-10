package app

import (
	"net/http"
	"sync"
	"time"
)

// hubProbe says whether the hub answers, so a re-check never sends a browser to an account server that is down
// (it would land on the browser's own error page). One light request with a short timeout, the
// answer kept for a minute; any HTTP answer below 500 counts as reachable.
type hubProbe struct {
	url    string
	mu     sync.Mutex
	at     time.Time
	up     bool
	client *http.Client
}

const hubProbeTTL = time.Minute

func newHubProbe(url string) *hubProbe {
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
	if res, err := p.client.Head(p.url); err == nil {
		res.Body.Close()
		up = res.StatusCode < 500
	}
	p.at, p.up = time.Now(), up
	return up
}
