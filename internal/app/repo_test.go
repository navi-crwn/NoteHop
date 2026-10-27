package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// A fake GitHub answers once; the home page numbers come from the cache after that.
func TestRepoStats(t *testing.T) {
	var calls int32
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.Header.Get("User-Agent") == "" {
			w.WriteHeader(403)
			return
		}
		switch r.URL.Path {
		case "/repos/navi-crwn/NoteHop":
			w.Write([]byte(`{"html_url":"https://github.com/navi-crwn/NoteHop","stargazers_count":12,"forks_count":3,"open_issues_count":1,"pushed_at":"2026-10-27T18:00:00Z","license":{"spdx_id":"MIT"}}`))
		case "/repos/navi-crwn/NoteHop/languages":
			w.Write([]byte(`{"Go":600,"JavaScript":300,"CSS":100}`))
		case "/repos/navi-crwn/NoteHop/stats/participation":
			w.WriteHeader(202) // still counting
		default:
			w.WriteHeader(404)
		}
	}))
	defer gh.Close()
	old := githubAPI
	githubAPI = gh.URL
	defer func() { githubAPI = old }()

	d, _ := db.Open(":memory:")
	defer d.Close()
	a, _ := New(Config{GitHub: "navi-crwn/NoteHop"}, d)
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	for i := 0; i < 3; i++ {
		res, err := http.Get(srv.URL + "/api/repo")
		if err != nil {
			t.Fatal(err)
		}
		var s repoStats
		json.NewDecoder(res.Body).Decode(&s)
		res.Body.Close()
		if res.StatusCode != 200 || s.Stars != 12 || s.Forks != 3 || s.License != "MIT" || len(s.Langs) != 3 || s.Langs[0].Name != "Go" || s.Langs[0].Share != .6 || s.Weeks != nil {
			t.Fatalf("stats: %d %+v", res.StatusCode, s)
		}
	}
	if n := atomic.LoadInt32(&calls); n != 3 {
		t.Fatalf("GitHub asked %d times, want 3 (one fetch, cached)", n)
	}

	// GitHub down before the first success: 503, and no new call on every visit.
	atomic.StoreInt32(&calls, 0)
	c, _ := New(Config{GitHub: "navi-crwn/Gone"}, d)
	cs := httptest.NewServer(c.Handler())
	for i := 0; i < 3; i++ {
		res, _ := http.Get(cs.URL + "/api/repo")
		res.Body.Close()
		if res.StatusCode != 503 {
			t.Errorf("down: %d", res.StatusCode)
		}
	}
	cs.Close()
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("GitHub down: asked %d times, want 1 (wait before trying again)", n)
	}

	// Hidden: no repository, or a bad name.
	for _, repo := range []string{"-", "", "not a repo", "../x"} {
		b, _ := New(Config{GitHub: repo}, d)
		s := httptest.NewServer(b.Handler())
		res, _ := http.Get(s.URL + "/api/repo")
		res.Body.Close()
		s.Close()
		if res.StatusCode != 404 {
			t.Errorf("GitHub=%q: %d", repo, res.StatusCode)
		}
	}
}
