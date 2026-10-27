package app

import (
	"net/http"
	"strings"
	"testing"
)

func TestProfileAndMock(t *testing.T) {
	_, _, c := newAppDB(t)
	c.do("POST", "/api/register", map[string]any{"username": "budi", "auth": key()}, nil)
	pub := c.create(map[string]any{"encrypted": false, "content": `{"t":"Menu API","b":"contoh:\n` + "```json\\n" + `{\"items\":[{\"name\":\"kopi\"},{\"name\":\"teh\"}]}` + "\\n```" + `"}`})
	enc := c.create(map[string]any{})
	adm := func(n note) map[string]string { return map[string]string{"X-Admin": n.admin} }

	// Profile is only for public notes.
	if code, _, _ := c.do("POST", "/api/n/"+enc.token+"/config", map[string]any{"profile": true}, adm(enc)); code != 400 {
		t.Fatalf("encrypted note on profile: %d", code)
	}
	if code, _, _ := c.do("GET", "/@budi", nil, nil); code != 404 {
		t.Fatalf("empty profile should 404: %d", code)
	}
	if code, m, _ := c.do("POST", "/api/n/"+pub.token+"/config", map[string]any{"profile": true}, adm(pub)); code != 200 || m["profile"] != true {
		t.Fatalf("profile on: %d %v", code, m)
	}
	code, _, page := c.do("GET", "/@budi", nil, nil)
	if code != 200 || !strings.Contains(page, "Menu API") || !strings.Contains(page, "/n/"+pub.token) {
		t.Fatalf("profile page: %d", code)
	}
	if code, _, _ := c.do("GET", "/@nobody", nil, nil); code != 404 {
		t.Fatalf("unknown user: %d", code)
	}
	if code, _, _ := c.do("GET", "/budi", nil, nil); code != 404 {
		t.Fatalf("bare name: %d", code)
	}

	// Mock API
	res, err := http.Get(c.srv.URL + "/mock/" + pub.token + "/items/1/name")
	if err != nil || res.StatusCode != 200 || res.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("mock: %v %d", err, res.StatusCode)
	}
	buf := make([]byte, 64)
	nb, _ := res.Body.Read(buf)
	if strings.TrimSpace(string(buf[:nb])) != `"teh"` {
		t.Fatalf("mock value: %q", buf[:nb])
	}
	for path, want := range map[string]int{"/items/9": 404, "/nope": 404, "": 200} {
		if res, _ := http.Get(c.srv.URL + "/mock/" + pub.token + path); res.StatusCode != want {
			t.Fatalf("mock %s: %d", path, res.StatusCode)
		}
	}
	if res, _ := http.Get(c.srv.URL + "/mock/" + enc.token); res.StatusCode != 403 {
		t.Fatalf("mock of encrypted note: %d", res.StatusCode)
	}
}
