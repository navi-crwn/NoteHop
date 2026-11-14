package app

import (
	"io"
	"strings"
	"testing"
)

func pageOf(t *testing.T, c *client, path string) string {
	t.Helper()
	res, err := c.hc.Get(c.srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return string(b)
}

// With a HOP ID hub the sign-in page leads with one HOP ID button; the local form and the sign-up link sit folded
// inside a details block (accounts made here before still sign in there). Stand-alone nothing changes.
func TestLoginPageLeadsWithHopIDAndFoldsTheLocalForm(t *testing.T) {
	e := newRecheckEnv(t)
	for _, path := range []string{"/login", "/register"} {
		p := pageOf(t, newClient(t, e.srv), path)
		open := strings.Index(p, `<details class="old-login"`)
		btn := strings.Index(p, `href="/auth/sso?next=`)
		user := strings.Index(p, `id="username"`)
		end := open + strings.Index(p[max(open, 0):], `</details>`)
		if open < 0 || btn < 0 || btn > open || user < open || user > end {
			t.Errorf("%s: HOP ID button at %d, folded block at %d..%d, username field at %d", path, btn, open, end, user)
		}
		if strings.Contains(p, `class="or"`) {
			t.Errorf("%s: a leftover \"or\" divider", path)
		}
	}
	if p := pageOf(t, newClient(t, e.srv), "/login"); !strings.Contains(p, `href="/register"`) || strings.Index(p, `href="/register"`) < strings.Index(p, `<details class="old-login"`) {
		t.Error("the local sign-up link must be inside the folded block")
	}

	// stand-alone: the form is the sign-in, no fold, no HOP ID button
	s := newClient(t, newTestApp(t))
	p := pageOf(t, s, "/login")
	if strings.Contains(p, "old-login") || strings.Contains(p, "/auth/sso") || !strings.Contains(p, `id="username"`) {
		t.Error("stand-alone login changed")
	}
}
