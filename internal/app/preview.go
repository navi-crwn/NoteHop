package app

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/navi-crwn/NoteHop/internal/db"
	"github.com/navi-crwn/NoteHop/internal/i18n"
)

// Link previews in chat apps. The note page never contains content (it is
// fetched by script after a click), so a preview can't spend a burn view
// anyway; this only makes the card say something useful. Encrypted notes
// show their status only. A public note's title is shown, since anyone with
// the link can already read it with curl.

// Only crawler names, never in-app browsers: a person whose browser matched
// would get the bare preview page instead of the note.
var previewBots = []string{
	"whatsapp/", "telegrambot", "discordbot", "slackbot", "slack-imgproxy", "facebookexternalhit",
	"facebot", "twitterbot", "linkedinbot", "skypeuripreview", "pinterestbot", "redditbot",
	"embedly", "vkshare", "mastodon",
}

func isPreviewBot(ua string) bool {
	ua = strings.ToLower(ua)
	for _, b := range previewBots {
		if strings.Contains(ua, b) {
			return true
		}
	}
	return false
}

// previewText is the card title and description for a note link.
func previewText(n *db.Note, l i18n.Locale) (title, desc string) {
	title = l.T("preview.title_encrypted")
	if !n.Encrypted {
		title = l.T("preview.title_public")
		var body struct {
			T string `json:"t"`
		}
		if json.Unmarshal([]byte(n.Content), &body) == nil {
			if t := strings.TrimSpace(body.T); t != "" {
				title = clipRunes(t, 90)
			}
		}
	}
	if n.Frozen {
		title = l.T("preview.snapshot", "title", title)
	}
	var parts []string
	if n.Encrypted {
		parts = append(parts, l.T("preview.encrypted"))
	}
	if n.Password {
		parts = append(parts, l.T("preview.password"))
	}
	if n.MaxViews > 0 {
		parts = append(parts, l.T("preview.burn"))
	}
	if n.Expires > 0 {
		parts = append(parts, l.T("preview.expires"))
	}
	desc = l.T("preview.open")
	if len(parts) > 0 {
		desc = upperFirst(strings.Join(parts, l.T("preview.sep"))+l.T("preview.end")) + " " + desc
	}
	return title, desc
}

func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}

func clipRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

var previewTmpl = template.Must(template.New("preview").Parse(`<!doctype html>
<html lang="id"><head><meta charset="utf-8">
<title>{{.Title}} · {{.Site}}</title>
<meta name="robots" content="noindex, nofollow">
<meta name="description" content="{{.Desc}}">
<meta property="og:type" content="website">
<meta property="og:site_name" content="{{.Site}}">
<meta property="og:title" content="{{.Title}}">
<meta property="og:description" content="{{.Desc}}">
<meta property="og:image" content="{{.Origin}}/static/img/og.png">
<meta property="og:image:width" content="1200">
<meta property="og:image:height" content="630">
<meta name="twitter:card" content="summary_large_image">
</head><body><p>{{.Desc}}</p></body></html>
`))

func (a *App) preview(w http.ResponseWriter, r *http.Request, title, desc string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	_ = previewTmpl.Execute(w, map[string]string{"Title": title, "Desc": desc, "Site": a.cfg.Site, "Origin": a.origin(r)})
}
