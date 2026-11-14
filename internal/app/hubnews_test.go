package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// The family announcement on the dashboard home, taken from a stand-in hub (contract: GET /announcements.json).

type fakeNews struct {
	srv   *httptest.Server
	mu    sync.Mutex
	body  string
	delay time.Duration
	hits  atomic.Int32
	last  *http.Request
}

func newFakeNews(t *testing.T, body string) *fakeNews {
	f := &fakeNews{body: body}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		f.mu.Lock()
		f.last = r.Clone(r.Context())
		body, d := f.body, f.delay
		f.mu.Unlock()
		if d > 0 {
			time.Sleep(d)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, body)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

const oneItem = `{"version":1,"items":[{"id":"12","level":"info","title":"Scheduled maintenance","body":"Sunday 03:00 UTC, about ten minutes.","url":"https://hel.ink/announcements/12","published_at":"2026-11-14T09:00:00Z","expires_at":null,"apps":["*"]}]}`

func dash(t *testing.T, c *client) string {
	t.Helper()
	res, err := c.hc.Get(c.srv.URL + "/dashboard")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		t.Fatalf("dashboard: %d", res.StatusCode)
	}
	return string(b)
}

func TestFamilyAnnouncementOnTheDashboard(t *testing.T) {
	hub := newFakeNews(t, oneItem)
	srv, _ := quotaApp(t, Config{SSOURL: hub.srv.URL})
	c := signedIn(t, srv, "helena")

	// (a) it shows, as a card with a link, and the hub was asked the way the contract says, without any visitor data
	page := dash(t, c)
	if !strings.Contains(page, `id="hubnews"`) || !strings.Contains(page, "Scheduled maintenance") || !strings.Contains(page, "about ten minutes") ||
		!strings.Contains(page, `href="https://hel.ink/announcements/12"`) {
		t.Fatalf("no card:\n%s", page[strings.Index(page, "<main"):][:min(1500, len(page))])
	}
	if hub.last.URL.Path != "/announcements.json" || hub.last.URL.Query().Get("app") != "notehop" || hub.last.Header.Get("Cookie") != "" || hub.last.Header.Get("Authorization") != "" {
		t.Fatalf("the request: %s cookie=%q", hub.last.URL, hub.last.Header.Get("Cookie"))
	}
	// kept for five minutes: a second load asks nobody
	dash(t, c)
	if hub.hits.Load() != 1 {
		t.Fatalf("hub asked %d times", hub.hits.Load())
	}
	// it can be closed (the hide button comes from the shared script), and then it is gone, also after a reload
	if !strings.Contains(page, `data-hop-remind="ann-12"`) {
		t.Fatal("no way to close it")
	}
	if code, _, raw := c.do("POST", "/api/me/reminders/dismiss", map[string]string{"key": "ann-12"}, nil); code != 200 {
		t.Fatalf("dismiss: %d %s", code, raw)
	}
	if strings.Contains(dash(t, c), `id="hubnews"`) {
		t.Fatal("closed, yet shown again")
	}
	if code, _, _ := c.do("POST", "/api/me/reminders/dismiss", map[string]string{"key": "ann-../x"}, nil); code != http.StatusBadRequest {
		t.Fatalf("a bad key: %d", code)
	}
	// another member still sees it
	if !strings.Contains(dash(t, signedIn(t, srv, "bob_barker")), `id="hubnews"`) {
		t.Fatal("closing it for one member hid it for another")
	}
}

func TestFamilyAnnouncementIsPlainTextOnly(t *testing.T) {
	hub := newFakeNews(t, `{"version":1,"items":[{"id":"7","level":"warning","title":"<img src=x onerror=alert(1)>","body":"<script>alert(1)</script> & more","url":"javascript:alert(1)","published_at":"2026-11-14T09:00:00Z","apps":["notehop"]}]}`)
	srv, _ := quotaApp(t, Config{SSOURL: hub.srv.URL})
	c := signedIn(t, srv, "helena")
	page := dash(t, c)
	if strings.Contains(page, "<script>alert(1)") || strings.Contains(page, "<img src=x") || strings.Contains(page, "javascript:") {
		t.Fatal("hub text reached the page as markup")
	}
	if !strings.Contains(page, "&lt;script&gt;alert(1)&lt;/script&gt;") || !strings.Contains(page, "announce hubnews") || !strings.Contains(page, "alert warn") {
		t.Fatalf("expected escaped text on a warning card")
	}
	// long text is cut
	h2 := newFakeNews(t, `{"items":[{"id":"8","title":"T","body":"`+strings.Repeat("a", 500)+`"}]}`)
	s2, _ := quotaApp(t, Config{SSOURL: h2.srv.URL})
	if p := dash(t, signedIn(t, s2, "helena")); strings.Contains(p, strings.Repeat("a", 301)) || !strings.Contains(p, strings.Repeat("a", 290)) {
		t.Fatal("the text is not cut at about 300 characters")
	}
}

func TestFamilyAnnouncementCriticalCannotBeClosedAndComesFirst(t *testing.T) {
	hub := newFakeNews(t, `{"version":1,"items":[
	  {"id":"3","level":"info","title":"Newer info","body":"x","published_at":"2026-11-14T12:00:00Z","apps":["*"]},
	  {"id":"2","level":"critical","title":"Security update needed","body":"y","published_at":"2026-11-13T12:00:00Z","apps":["*"]},
	  {"id":"1","level":"info","title":"Expired","body":"z","published_at":"2026-11-14T13:00:00Z","expires_at":"2020-01-01T00:00:00Z","apps":["*"]}]}`)
	srv, _ := quotaApp(t, Config{SSOURL: hub.srv.URL})
	c := signedIn(t, srv, "helena")
	page := dash(t, c)
	if !strings.Contains(page, "Security update needed") || strings.Contains(page, "Newer info") || strings.Contains(page, "Expired") {
		t.Fatal("wrong card chosen (critical first, one card, expired never)")
	}
	if strings.Contains(page, `data-hop-remind="ann-2"`) {
		t.Fatal("a critical notice has a close button")
	}
	c.do("POST", "/api/me/reminders/dismiss", map[string]string{"key": "ann-2"}, nil) // even a made-up request does not hide it
	if !strings.Contains(dash(t, c), "Security update needed") {
		t.Fatal("a critical notice was hidden")
	}
}

// A hub that is down, slow or sends rubbish never breaks or holds up the dashboard; and with no hub nothing is asked.
func TestFamilyAnnouncementNeverHurtsTheDashboard(t *testing.T) {
	hub := newFakeNews(t, oneItem)
	hub.delay = 3 * time.Second
	srv, _ := quotaApp(t, Config{SSOURL: hub.srv.URL})
	c := signedIn(t, srv, "helena")
	start := time.Now()
	page := dash(t, c)
	if took := time.Since(start); took > hubNewsTimeout+time.Second || strings.Contains(page, `id="hubnews"`) {
		t.Fatalf("slow hub: took %v, card=%v", took, strings.Contains(page, `id="hubnews"`))
	}
	// and the next loads do not wait again for a minute
	start = time.Now()
	dash(t, c)
	if took := time.Since(start); took > 700*time.Millisecond {
		t.Fatalf("second load waited %v for a hub that just failed", took)
	}

	for name, body := range map[string]string{"rubbish": "<html>", "wrong shape": `{"items":"no"}`, "empty": `{"version":1,"items":[]}`} {
		h := newFakeNews(t, body)
		s, _ := quotaApp(t, Config{SSOURL: h.srv.URL})
		if p := dash(t, signedIn(t, s, "helena")); strings.Contains(p, `id="hubnews"`) {
			t.Errorf("%s: a card", name)
		}
	}
	down := httptest.NewServer(http.NotFoundHandler())
	url := down.URL
	down.Close() // nothing listens any more
	s, _ := quotaApp(t, Config{SSOURL: url})
	start = time.Now()
	if p := dash(t, signedIn(t, s, "helena")); strings.Contains(p, `id="hubnews"`) || time.Since(start) > hubNewsTimeout+time.Second {
		t.Fatal("dead hub")
	}

	// no hub configured: no request at all, and no card
	none := newFakeNews(t, oneItem)
	s2, _ := quotaApp(t, Config{})
	if p := dash(t, signedIn(t, s2, "helena")); strings.Contains(p, `id="hubnews"`) {
		t.Fatal("a card without a hub")
	}
	if none.hits.Load() != 0 {
		t.Fatal("a request without a hub")
	}
}

// The last good copy is kept when the hub stops answering.
func TestFamilyAnnouncementKeepsTheLastCopy(t *testing.T) {
	hub := newFakeNews(t, oneItem)
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(Config{SSOURL: hub.srv.URL, Registration: true}, d)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() { srv.Close(); d.Close() })
	c := signedIn(t, srv, "helena")
	if !strings.Contains(dash(t, c), "Scheduled maintenance") {
		t.Fatal("no card")
	}
	hub.srv.Close()
	a.news.mu.Lock()
	a.news.okAt = time.Now().Add(-10 * time.Minute) // the copy has gone stale; the hub is gone
	a.news.mu.Unlock()
	if !strings.Contains(dash(t, c), "Scheduled maintenance") {
		t.Fatal("the last copy was dropped when the hub failed")
	}
}
