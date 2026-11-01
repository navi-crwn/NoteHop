package app

import (
	"io"
	"strings"
	"testing"
)

// The signed-in dashboard has the same chrome as the other HOP dashboards: this app's menu and the HOP family in
// the sidebar, settings-type controls in the top bar (no second list of apps), a compact footer.
func TestDashboardShell(t *testing.T) {
	_, _, c := newAppDB(t)
	if code, _, raw := c.do("POST", "/api/register", map[string]string{"username": "mira", "auth": key()}, nil); code != 200 && code != 201 {
		t.Fatalf("register: %d %s", code, raw)
	}
	get := func(path string) string {
		resp, err := c.hc.Get(c.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	body := get("/dashboard")
	for _, want := range []string{`class="shell"`, `class="shell-side"`, `class="shell-nav shell-family"`, `class="shell-top"`, `class="shell-foot"`, `id="logout"`} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard lacks %s", want)
		}
	}
	top, _, _ := strings.Cut(strings.SplitN(body, `class="shell-top"`, 2)[1], "</header>")
	if strings.Contains(top, "family-switcher") {
		t.Error("the app grid is back in the top bar")
	}
	for _, want := range []string{`id="lang-menu"`, `id="theme"`, `id="menu"`} {
		if !strings.Contains(top, want) {
			t.Errorf("top bar lacks %s", want)
		}
	}
	if pub := get("/about"); strings.Contains(pub, `class="shell-side"`) || !strings.Contains(pub, `class="footer-columns"`) {
		t.Error("public pages keep the public chrome")
	}
}
