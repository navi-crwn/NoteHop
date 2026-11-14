package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// The family announcement: one card on the dashboard home from the HOP ID hub's public list
// (GET <hub>/announcements.json?app=notehop, contract L-1). The SERVER asks, not the browser (the page's policy
// allows only its own origin), so NoteHop keeps its promise of no trackers: the request carries no cookie, no
// account and no address of a visitor, it is one request every five minutes at most, and it is not made at all when
// no hub is configured. A hub that is down or slow never holds a page up for more than two seconds, and not even
// that more than once a minute: the last good copy (or nothing) is used meanwhile.

const (
	hubNewsTimeout = 2 * time.Second
	hubNewsFresh   = 5 * time.Minute
	hubNewsBackoff = time.Minute
	hubNewsBody    = 300 // characters shown
)

type hubItem struct {
	ID       string
	Level    string // info | warning | critical
	Title    string
	Body     string
	URL      string
	Closable bool // critical cannot be hidden
	pub      time.Time
	exp      time.Time // zero = no end
}

type hubNews struct {
	mu       sync.Mutex
	items    []hubItem
	okAt     time.Time // last good answer
	failedAt time.Time
	client   *http.Client
}

var hubIDRe = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// hubAnnouncement is the card to show now, nil when there is none (or no hub).
func (a *App) hubAnnouncement(now time.Time) *hubItem {
	if a.cfg.SSOURL == "" {
		return nil
	}
	n := &a.news
	n.mu.Lock()
	stale := n.okAt.IsZero() || now.Sub(n.okAt) >= hubNewsFresh
	backoff := !n.failedAt.IsZero() && now.Sub(n.failedAt) < hubNewsBackoff && !n.failedAt.Before(n.okAt)
	n.mu.Unlock()
	if stale && !backoff {
		items, err := n.fetch(a.cfg.SSOURL)
		n.mu.Lock()
		if err != nil {
			n.failedAt = now
		} else {
			n.items, n.okAt = items, now
		}
		n.mu.Unlock()
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return pickHubItem(n.items, now)
}

func (n *hubNews) fetch(hub string) ([]hubItem, error) {
	if n.client == nil {
		n.client = &http.Client{Timeout: hubNewsTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	ctx, cancel := context.WithTimeout(context.Background(), hubNewsTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(hub, "/")+"/announcements.json?app=notehop", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "notehop-announcements")
	res, err := n.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hub answered %d", res.StatusCode)
	}
	var doc struct {
		Items []struct {
			ID          any      `json:"id"`
			Level       string   `json:"level"`
			Title       string   `json:"title"`
			Body        string   `json:"body"`
			URL         string   `json:"url"`
			PublishedAt string   `json:"published_at"`
			ExpiresAt   *string  `json:"expires_at"`
			Apps        []string `json:"apps"`
		} `json:"items"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 128<<10)).Decode(&doc); err != nil {
		return nil, err
	}
	var out []hubItem
	for _, it := range doc.Items {
		id := hubIDRe.ReplaceAllString(fmt.Sprint(it.ID), "")
		if id == "" || len(id) > 40 || strings.TrimSpace(it.Title) == "" {
			continue
		}
		if len(it.Apps) > 0 {
			ok := false
			for _, ap := range it.Apps {
				ok = ok || ap == "*" || ap == "notehop"
			}
			if !ok {
				continue
			}
		}
		h := hubItem{ID: id, Level: "info", Title: clip(it.Title, 120), Body: cutText(strings.TrimSpace(it.Body), hubNewsBody)}
		if it.Level == "warning" || it.Level == "critical" {
			h.Level = it.Level
		}
		if u, err := url.Parse(it.URL); err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
			h.URL = u.String()
		}
		h.pub, _ = time.Parse(time.RFC3339, it.PublishedAt)
		if it.ExpiresAt != nil {
			h.exp, _ = time.Parse(time.RFC3339, *it.ExpiresAt) // checked when picking: the copy may outlive the notice
		}
		h.Closable = h.Level != "critical"
		out = append(out, h)
	}
	return out, nil
}

func cutText(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}

// pickHubItem: critical first, then the newest; one card only. Items past their end time are skipped even
// when the copy is older than the notice.
func pickHubItem(items []hubItem, now time.Time) *hubItem {
	var live []hubItem
	for _, it := range items {
		if !it.exp.IsZero() && !it.exp.After(now) {
			continue
		}
		live = append(live, it)
	}
	if len(live) == 0 {
		return nil
	}
	sort.SliceStable(live, func(i, j int) bool {
		ci, cj := live[i].Level == "critical", live[j].Level == "critical"
		if ci != cj {
			return ci
		}
		return live[i].pub.After(live[j].pub)
	})
	return &live[0]
}
