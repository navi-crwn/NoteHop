package app

import (
	"html"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestFeedbackURL(t *testing.T) {
	for _, c := range []struct{ hub, screen, ver, want string }{
		{"https://hel.ink", "dashboard", "1.6.0", "https://hel.ink/feedback?app=notehop&from=dashboard&v=1.6.0"},
		{"https://hub.hel.ink/", "faq", "dev", "https://hub.hel.ink/feedback?app=notehop&from=faq&v=dev"},
		{"https://hel.ink", "note?x=1#key", "1.6.0 beta", "https://hel.ink/feedback?app=notehop&from=notex1key&v=1.6.0beta"},
		{"https://hel.ink", "", "", "https://hel.ink/feedback?app=notehop"},
		{"", "home", "1.6.0", ""},
		{"javascript:alert(1)", "home", "1.6.0", ""},
		{"hel.ink", "home", "1.6.0", ""},
	} {
		if got := feedbackURL(c.hub, c.screen, c.ver); got != c.want {
			t.Errorf("feedbackURL(%q,%q,%q) = %q, want %q", c.hub, c.screen, c.ver, got, c.want)
		}
	}
	long := feedbackURL("https://hel.ink", strings.Repeat("a", 200), strings.Repeat("1", 60))
	u, _ := url.Parse(long)
	if len(u.Query().Get("from")) != 80 || len(u.Query().Get("v")) != 24 {
		t.Errorf("lengths not capped: %d %d", len(u.Query().Get("from")), len(u.Query().Get("v")))
	}
}

var feedbackHref = regexp.MustCompile(`href="([^"]*/feedback\?[^"]*)"`)

func feedbackLinks(body string) []string {
	var out []string
	for _, m := range feedbackHref.FindAllStringSubmatch(body, -1) {
		out = append(out, html.UnescapeString(m[1]))
	}
	return out
}

func feedbackApp2(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	hub := newFakeNews(t, `{"version":1,"items":[]}`)
	srv, _ := quotaApp(t, Config{SSOURL: hub.srv.URL, Version: "1.6.0"})
	return srv, hub.srv.URL
}

// With a hub address the dashboard foot has seven items, Feedback second, opening a new tab with the app, the
// screen name and the version.
func TestSidebarFootHasFeedbackSecondWithAHub(t *testing.T) {
	srv, hub := feedbackApp2(t)
	c := signedIn(t, srv, "helena")
	body := pageOf(t, c, "/dashboard")
	side, _, _ := strings.Cut(body, "</aside>")
	foot := side[strings.Index(side, `class="shell-side-foot"`):]
	if strings.Count(foot, "<li>") != 7 {
		t.Fatalf("foot has %d items, want 7", strings.Count(foot, "<li>"))
	}
	last := -1
	for _, href := range []string{`href="/faq"`, `/feedback?app=notehop`, `href="/terms"`, `href="/privacy"`, `href="/changelog"`, `href="https://github.com/navi-crwn/NoteHop"`, `href="https://hel.ink/announcements"`, `class="part-of"`} {
		i := strings.Index(foot, href)
		if i < 0 || i < last {
			t.Fatalf("foot item %s missing or out of order", href)
		}
		last = i
	}
	links := feedbackLinks(foot)
	if len(links) != 1 || links[0] != hub+"/feedback?app=notehop&from=dashboard&v=1.6.0" {
		t.Fatalf("sidebar link = %v", links)
	}
	if !strings.Contains(foot, `href="`+strings.ReplaceAll(links[0], "&", "&amp;")+`" target="_blank" rel="noopener">Feedback</a>`) {
		t.Error("the link does not open a new tab with rel=noopener")
	}
}

// Guests find the link in the public footer (columns and one-line) and on /faq and /contact; the Privacy page has
// the sentence with a link to the hub's privacy policy.
func TestFeedbackOnPublicPages(t *testing.T) {
	srv, hub := feedbackApp2(t)
	c := newClient(t, srv)
	for path, screen := range map[string]string{"/": "home", "/about": "about", "/faq": "faq", "/contact": "contact", "/privacy": "privacy", "/terms": "terms", "/new": "new", "/login": "login"} {
		body := pageOf(t, c, path)
		links := feedbackLinks(body)
		if len(links) == 0 {
			t.Errorf("%s: no Feedback link", path)
			continue
		}
		for _, l := range links {
			if l != hub+"/feedback?app=notehop&from="+screen+"&v=1.6.0" {
				t.Errorf("%s: link %q", path, l)
			}
		}
	}
	if n := len(feedbackLinks(pageOf(t, c, "/faq"))); n != 2 {
		t.Errorf("/faq has %d links, want footer + Send feedback", n)
	}
	if !strings.Contains(pageOf(t, c, "/contact"), ">Send feedback</a>") || !strings.Contains(pageOf(t, c, "/faq"), ">Send feedback</a>") {
		t.Error("/faq and /contact lack the Send feedback link")
	}
	p := pageOf(t, c, "/privacy")
	if !strings.Contains(p, "The Feedback link opens a form on the HOP ID site") || !strings.Contains(p, `<a href="`+hub+`/privacy" rel="noopener">HOP ID privacy policy</a>`) {
		t.Errorf("privacy sentence missing")
	}
}

// Another language reaches the same links.
func TestFeedbackLabelFollowsTheLanguage(t *testing.T) {
	srv, _ := feedbackApp2(t)
	c := newClient(t, srv)
	if !strings.Contains(pageOf(t, c, "/faq?lang=id"), ">Masukan</a>") {
		t.Error("no Indonesian label")
	}
}

// Without a hub address nothing shows: no link, no sentence, six foot items.
func TestNoFeedbackWithoutAHub(t *testing.T) {
	srv := newTestApp(t)
	c := newClient(t, srv)
	for _, p := range []string{"/", "/faq", "/contact", "/privacy", "/new", "/about"} {
		body := pageOf(t, c, p)
		if strings.Contains(body, "/feedback") || strings.Contains(body, "Send feedback") || strings.Contains(body, "The Feedback link") {
			t.Errorf("%s shows Feedback without a hub", p)
		}
	}
	m := signedIn(t, srv, "helena")
	body := pageOf(t, m, "/dashboard")
	if strings.Contains(body, "/feedback") {
		t.Error("dashboard shows Feedback without a hub")
	}
}

// A note's address is a way in. The screen name stays "note" whatever was asked: no token, key, query or fragment
// reaches the link, on the read page, with a query, and on the profile and vault screens.
func TestFeedbackFromNeverCarriesANoteAddress(t *testing.T) {
	srv, hub := feedbackApp2(t)
	c := signedIn(t, srv, "helena")
	n := c.create(nil)
	for _, p := range []string{"/" + n.token, "/" + n.token + "?edit=1&k=secretkey", "/" + n.token + "?admin=" + n.admin} {
		body := pageOf(t, c, p)
		links := feedbackLinks(body)
		if len(links) == 0 {
			t.Fatalf("%s: no link", p)
		}
		for _, l := range links {
			if l != hub+"/feedback?app=notehop&from=note&v=1.6.0" {
				t.Errorf("%s: link %q", p, l)
			}
			if strings.Contains(l, n.token) || strings.Contains(l, n.admin) || strings.Contains(l, "secretkey") {
				t.Errorf("%s: link carries the note address: %q", p, l)
			}
		}
	}
	for path, screen := range map[string]string{"/dashboard": "dashboard", "/security": "security", "/nope-" + n.token: "error"} {
		for _, l := range feedbackLinks(pageOf(t, c, path)) {
			if l != hub+"/feedback?app=notehop&from="+screen+"&v=1.6.0" {
				t.Errorf("%s: link %q", path, l)
			}
		}
	}
}
