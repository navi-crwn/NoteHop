package app

import (
	"html/template"
	"net/url"
	"regexp"
	"strings"
)

/*
The Feedback link opens the HOP ID form in a new tab and does nothing else: this app has no form, no upload and
no route that receives anything. The address is {hub}/feedback?app=notehop&from=<screen>&v=<version>. The hub
address is the one the family announcement already uses (Config.SSOURL); without it there is no link and no
Privacy sentence.

The screen name is the name of the page template ("dashboard", "note", "vault", "faq" ...), never the address
that was asked for: a note's address is its way in, so no token, id, key, query or fragment may reach the hub.
*/

const feedbackApp = "notehop"

var (
	feedbackFromBad = regexp.MustCompile(`[^A-Za-z0-9._:/{}-]`)
	feedbackVerBad  = regexp.MustCompile(`[^A-Za-z0-9.+-]`)
)

// feedbackHub is the hub address the link and the Privacy sentence use: an http(s) address without a trailing
// slash, or "" (no link, no sentence).
func feedbackHub(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host + strings.TrimRight(u.Path, "/")
}

// feedbackURL is the link for one screen.
func feedbackURL(hub, screen, version string) string {
	base := feedbackHub(hub)
	if base == "" {
		return ""
	}
	q := url.Values{"app": {feedbackApp}}
	if from := feedbackFromBad.ReplaceAllString(screen, ""); from != "" {
		q.Set("from", feedbackTrunc(from, 80))
	}
	if v := feedbackVerBad.ReplaceAllString(version, ""); v != "" {
		q.Set("v", feedbackTrunc(v, 24))
	}
	return base + "/feedback?" + q.Encode()
}

func feedbackTrunc(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// FeedbackPrivacy is the Privacy sentence, with its link to the hub's privacy policy; empty without a hub.
func (d pageData) FeedbackPrivacy() template.HTML {
	if d.Feedback == "" {
		return ""
	}
	link := `<a href="` + template.HTMLEscapeString(feedbackHub(d.FeedbackHub)+"/privacy") + `" rel="noopener">` +
		template.HTMLEscapeString(d.L.T("privacy.feedback_link")) + `</a>`
	return template.HTML(d.L.T("privacy.feedback", "link", link))
}
