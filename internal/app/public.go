package app

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Features that only exist for public (unencrypted) notes, because the
// server has to read the text: profile pages and the mock JSON API.

type profileNote struct {
	Title   string
	URL     string
	Updated int64
}

// profile serves /@username: the user's public notes they chose to list.
// Only read-only links are shown, so listing a note never hands out edit rights.
func (a *App) profile(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.PathValue("at"), "@")
	if name == r.PathValue("at") || name == "" {
		a.notFound(w, r)
		return
	}
	u, err := a.db.UserByName(strings.ToLower(name))
	if err != nil {
		a.notFound(w, r)
		return
	}
	notes, _ := a.db.ProfileNotes(u.ID)
	var list []profileNote
	for _, n := range notes {
		if n.MaxViews > 0 || runBeforeRead(n, time.Now(), "") != nil {
			continue
		}
		links, _ := a.db.Links(n.ID)
		for _, l := range links {
			if l.Mode == modeRead {
				var c struct{ T string }
				json.Unmarshal([]byte(n.Content), &c)
				list = append(list, profileNote{Title: c.T, URL: notePath(l.Token), Updated: n.Updated})
				break
			}
		}
	}
	if len(list) == 0 {
		a.notFound(w, r) // nothing public: don't confirm the account exists
		return
	}
	l := a.locale(w, r)
	a.render(w, r, http.StatusOK, "profile", pageData{Title: "@" + u.Username, Desc: l.T("profile.lead", "name", u.Username), Profile: list})
}

var mockFence = regexp.MustCompile("(?s)```json\\s*\\n(.*?)\\n```")

// mock serves /mock/<link>/<path…>: the JSON in a public note (the whole
// text, or its first ```json block), optionally narrowed by path segments
// (object keys or array indexes). For prototypes: CORS is open.
func (a *App) mock(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Cache-Control", "no-store")
	fail := func(code int, msg string) {
		h.Set("Content-Type", "application/json")
		w.WriteHeader(code)
		w.Write(mustJSON(map[string]string{"error": msg}))
	}
	if r.Method == http.MethodOptions {
		h.Set("Access-Control-Allow-Methods", "GET")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	n, _, err := a.db.NoteByLink(r.PathValue("token"))
	if err != nil {
		fail(http.StatusNotFound, "not found")
		return
	}
	if n.Encrypted || n.MaxViews > 0 {
		fail(http.StatusForbidden, "only public notes without a view limit can be used as a mock API")
		return
	}
	if e := runBeforeRead(n, time.Now(), a.clientIP(r)); e != nil {
		fail(e.status, e.key)
		return
	}
	var c struct{ B string }
	json.Unmarshal([]byte(n.Content), &c)
	src := strings.TrimSpace(c.B)
	if m := mockFence.FindStringSubmatch(c.B); !json.Valid([]byte(src)) && m != nil {
		src = m[1]
	}
	var v any
	if json.Unmarshal([]byte(src), &v) != nil {
		fail(http.StatusUnprocessableEntity, "the note holds no valid JSON (whole text or a ```json block)")
		return
	}
	for _, seg := range strings.Split(strings.Trim(r.PathValue("path"), "/"), "/") {
		if seg == "" {
			continue
		}
		switch cur := v.(type) {
		case map[string]any:
			next, ok := cur[seg]
			if !ok {
				fail(http.StatusNotFound, "no "+seg)
				return
			}
			v = next
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(cur) {
				fail(http.StatusNotFound, "no "+seg)
				return
			}
			v = cur[i]
		default:
			fail(http.StatusNotFound, "no "+seg)
			return
		}
	}
	h.Set("Content-Type", "application/json")
	out, _ := json.MarshalIndent(v, "", "  ")
	w.Write(append(out, '\n'))
}
