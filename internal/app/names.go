package app

import (
	_ "embed"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// Rules for names people choose: custom link names (/n/…) and usernames
// (/@…). Short names, site and brand words and offensive words are kept
// back, and a name that belonged to a deleted note is held for a while, so
// an old link never opens someone else's note. Site admins (NOTEHOP_ADMINS)
// may use short and reserved names, e.g. for marketing links.

// minSlug is the shortest custom link name for everyone but site admins.
const minSlug = 7

var (
	//go:embed names/reserved.txt
	reservedTxt string
	//go:embed names/blocked.txt
	blockedTxt string

	reservedNames, reservedPrefixes = parseReserved(reservedTxt)
	blockedWords                    = wordSet(blockedTxt)
	wordSplit                       = regexp.MustCompile(`[^a-z]+`)
)

func listWords(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		out = append(out, strings.Fields(strings.ToLower(line))...)
	}
	return out
}

func wordSet(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range listWords(s) {
		m[strings.ReplaceAll(w, "-", "")] = true
	}
	return m
}

func parseReserved(s string) (map[string]bool, []string) {
	names, prefixes := map[string]bool{}, []string(nil)
	for _, w := range listWords(s) {
		if p, ok := strings.CutSuffix(w, "*"); ok {
			prefixes = append(prefixes, p)
		} else {
			names[w] = true
		}
	}
	return names, prefixes
}

// normName treats "_" like "-", so @help_desk and /n/help-desk are checked alike.
func normName(name string) string { return strings.ReplaceAll(strings.ToLower(name), "_", "-") }

func (a *App) reservedName(name string) bool {
	n := normName(name)
	if reservedNames[n] || slices.Contains(a.cfg.Reserved, n) {
		return true
	}
	for _, p := range reservedPrefixes {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return false
}

// blockedName looks at whole words only, so "class-notes" isn't caught for
// containing "ass"; the name written together counts as one word too.
func blockedName(name string) bool {
	n := strings.ToLower(name)
	if blockedWords[wordSplit.ReplaceAllString(n, "")] {
		return true
	}
	for _, w := range wordSplit.Split(n, -1) {
		if blockedWords[w] {
			return true
		}
	}
	return false
}

func (a *App) siteAdmin(u *db.User) bool {
	return u != nil && slices.Contains(a.cfg.Admins, u.Username)
}

// slugProblem returns the catalog key (and its values) explaining why a
// custom link name can't be used, or "" if it can.
func (a *App) slugProblem(r *http.Request, slug string) (string, []any) {
	u := a.currentUser(r)
	admin := a.siteAdmin(u)
	switch {
	case strings.HasSuffix(slug, "-") || strings.Contains(slug, "--"):
		return "err.slug_format", nil
	case blockedName(slug):
		return "err.name_blocked", nil
	case !admin && len(slug) < minSlug:
		return "err.slug_short", []any{"n", minSlug}
	case !admin && a.reservedName(slug):
		return "err.slug_reserved", nil
	}
	if owner, until, held := a.db.SlugHold(slug); held && !admin && !(owner.Valid && u != nil && owner.Int64 == u.ID) {
		return "err.slug_held", []any{"time", until} // unix: the browser formats it
	}
	return "", nil
}

// usernameProblem is the same idea for usernames; the admins listed in
// NOTEHOP_ADMINS may register a reserved name (their own).
func (a *App) usernameProblem(name string) string {
	switch {
	case blockedName(name):
		return "err.name_blocked"
	case a.reservedName(name) && !slices.Contains(a.cfg.Admins, name):
		return "err.username_reserved"
	}
	return ""
}
