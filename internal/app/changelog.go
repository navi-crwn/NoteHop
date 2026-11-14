package app

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"github.com/navi-crwn/NoteHop/web"
)

// changeItem is one line of a release: what changed (Type), where (Where) and the plain-language Text.
// encoding/json fills these from "type", "where" and "text" (names match without regard to case).
type changeItem struct{ Type, Where, Text string }

// changeEntry is one release. Entries from before the plain-language changelog have no Items and show as a title only.
type changeEntry struct {
	Kind, Date, Title, URL, ID, App, Repo string
	Version, Summary                      string
	Items                                 []changeItem
}

// changeGroup is the items of one type, in the order the page shows them.
type changeGroup struct {
	Type  string
	Items []changeItem
}

var changeTypes = []string{"added", "changed", "fixed", "security", "removed", "deprecated"}

// Groups sorts a release's items by type (added, changed, fixed, security, removed, deprecated), keeping the written
// order inside a type, so the template needs no logic.
func (e changeEntry) Groups() []changeGroup {
	var out []changeGroup
	for _, t := range changeTypes {
		var items []changeItem
		for _, it := range e.Items {
			if it.Type == t {
				items = append(items, it)
			}
		}
		if len(items) > 0 {
			out = append(out, changeGroup{Type: t, Items: items})
		}
	}
	return out
}

type changeHistory struct {
	Sources map[string]string
	Entries []changeEntry
}

type changePage struct {
	Entries            []changeEntry
	Sources            map[string]string
	Page, Pages, Total int
	Previous, Next     string
}

var changes = sync.OnceValue(func() changeHistory {
	var history changeHistory
	if err := json.Unmarshal(web.Changelog, &history); err != nil {
		panic(err)
	}
	return history
})

func historyPage(r *http.Request) changePage {
	history := changes()
	total := len(history.Entries)
	pages := max(1, (total+39)/40)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	page = max(1, min(page, pages))
	link := func(n int) string {
		q := url.Values{}
		q.Set("page", strconv.Itoa(n))
		if lang := r.URL.Query().Get("lang"); lang != "" {
			q.Set("lang", lang)
		}
		return "/changelog?" + q.Encode()
	}
	p := changePage{Entries: history.Entries[(page-1)*40 : min(page*40, total)], Sources: history.Sources, Page: page, Pages: pages, Total: total}
	if page > 1 {
		p.Previous = link(page - 1)
	}
	if page < pages {
		p.Next = link(page + 1)
	}
	return p
}

func (a *App) changelog(w http.ResponseWriter, r *http.Request) {
	l := a.locale(w, r)
	history := historyPage(r)
	a.render(w, r, http.StatusOK, "changelog", pageData{Title: l.T("changelog.title"), Changelog: &history})
}
