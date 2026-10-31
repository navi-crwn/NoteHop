package app

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"github.com/navi-crwn/NoteHop/web"
)

type changeEntry struct {
	Kind, Date, Title, URL, ID, App, Repo string
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
