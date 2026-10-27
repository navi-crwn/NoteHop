package app

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"sync"
	"time"
)

// The home page's "built in the open" numbers. The browser may only talk to
// this site (CSP), so the server asks GitHub's public API and keeps the answer
// for an hour (well inside the 60 requests an hour GitHub allows without a token).
// NOTEHOP_GITHUB names the repository ("owner/name"); "-" turns this off.

var githubAPI = "https://api.github.com" // tests point this at a fake

var repoRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}/[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)

type langShare struct {
	Name  string  `json:"name"`
	Share float64 `json:"share"` // 0..1
}

type repoStats struct {
	Repo    string      `json:"repo"`
	URL     string      `json:"url"`
	Stars   int         `json:"stars"`
	Forks   int         `json:"forks"`
	Issues  int         `json:"issues"`
	License string      `json:"license"`
	Pushed  int64       `json:"pushed"`
	Langs   []langShare `json:"langs"`
	Weeks   []int       `json:"weeks"` // commits per week, the last 52
	Fetched int64       `json:"fetched"`
}

type repoCache struct {
	sync.Mutex
	stats *repoStats
	at    time.Time
	busy  bool
}

func (c Config) repo() string {
	if repoRe.MatchString(c.GitHub) {
		return c.GitHub
	}
	return ""
}

func (a *App) apiRepo(w http.ResponseWriter, r *http.Request) {
	repo := a.cfg.repo()
	if repo == "" {
		a.notFound(w, r)
		return
	}
	c := &a.repoCache
	c.Lock()
	// c.at also marks a failed try, so a GitHub outage costs one call per 10 minutes, not one per visit.
	fresh := !c.at.IsZero() && time.Since(c.at) < time.Hour
	// One fetch at a time; the others get what is cached (or 503 before the first).
	if !fresh && !c.busy {
		c.busy = true
		c.Unlock()
		s, err := fetchRepo(repo)
		c.Lock()
		c.busy = false
		if err != nil {
			log.Printf("github stats: %v", err)
			c.at = time.Now().Add(-50 * time.Minute) // try again in 10 minutes
		} else {
			c.stats, c.at = s, time.Now()
		}
	}
	s := c.stats
	c.Unlock()
	if s == nil {
		apiErr(w, http.StatusServiceUnavailable, "err.repo_unavailable")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=600")
	writeJSON(w, http.StatusOK, s)
}

var githubClient = &http.Client{Timeout: 6 * time.Second}

func githubGet(path string, v any) (int, error) {
	req, _ := http.NewRequest("GET", githubAPI+path, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "NoteHop")
	res, err := githubClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return res.StatusCode, nil
	}
	return res.StatusCode, json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(v)
}

func fetchRepo(repo string) (*repoStats, error) {
	var info struct {
		HTMLURL string `json:"html_url"`
		Stars   int    `json:"stargazers_count"`
		Forks   int    `json:"forks_count"`
		Issues  int    `json:"open_issues_count"`
		Pushed  string `json:"pushed_at"`
		License *struct {
			SPDX string `json:"spdx_id"`
		} `json:"license"`
	}
	code, err := githubGet("/repos/"+repo, &info)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("repo %s: HTTP %d", repo, code)
	}
	s := &repoStats{Repo: repo, URL: info.HTMLURL, Stars: info.Stars, Forks: info.Forks, Issues: info.Issues, Fetched: time.Now().Unix()}
	if info.License != nil && info.License.SPDX != "NOASSERTION" {
		s.License = info.License.SPDX
	}
	if t, err := time.Parse(time.RFC3339, info.Pushed); err == nil {
		s.Pushed = t.Unix()
	}
	// Languages and weekly commits are extras: a failure leaves them empty.
	var langs map[string]float64
	if code, err := githubGet("/repos/"+repo+"/languages", &langs); err == nil && code == http.StatusOK {
		total := 0.0
		for _, b := range langs {
			total += b
		}
		for n, b := range langs {
			if total > 0 {
				s.Langs = append(s.Langs, langShare{Name: n, Share: b / total})
			}
		}
		sort.Slice(s.Langs, func(i, j int) bool { return s.Langs[i].Share > s.Langs[j].Share })
		if len(s.Langs) > 6 {
			s.Langs = s.Langs[:6]
		}
	}
	var part struct {
		All []int `json:"all"`
	}
	if code, err := githubGet("/repos/"+repo+"/stats/participation", &part); err == nil && code == http.StatusOK {
		s.Weeks = part.All // 202 means GitHub is still counting: no weeks this time
	}
	return s, nil
}
