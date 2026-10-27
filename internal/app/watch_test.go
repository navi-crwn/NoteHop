package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

func TestWatch(t *testing.T) {
	d, _ := db.Open(":memory:")
	a, _ := New(Config{PrivateHooks: true}, d)
	srv := httptest.NewServer(a.Handler())
	defer func() { srv.Close(); d.Close() }()
	c := newClient(t, srv)

	got := make(chan *http.Request, 1)
	bodies := make(chan []byte, 1)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got <- r
		bodies <- b
	}))
	defer hook.Close()

	n := c.create(map[string]any{"mode": "edit"})
	adm := map[string]string{"X-Admin": n.admin}
	code, m, _ := c.do("POST", "/api/n/"+n.token+"/config", map[string]any{"watch": hook.URL + "/x"}, adm)
	if code != 200 || m["watch"] != hook.URL+"/x" || len(m["watch_key"].(string)) != 32 {
		t.Fatalf("config: %d %v", code, m)
	}
	key := m["watch_key"].(string)
	w := map[string]string{"X-Write": n.write}
	c.do("PUT", "/api/n/"+n.token, map[string]any{"content": fakeCipher("v2"), "version": 1}, w)
	c.do("POST", "/api/n/"+n.token+"/entries", map[string]any{"content": fakeCipher("e")}, w)
	if len(a.watch.due(time.Now())) != 0 {
		t.Fatal("sent before the quiet period")
	}
	for id, j := range a.watch.due(time.Now().Add(watchQuiet)) {
		a.deliverWatch(id, j)
	}
	r, body := <-got, <-bodies
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	if r.Header.Get("X-NoteHop-Signature") != "sha256="+hex.EncodeToString(mac.Sum(nil)) {
		t.Fatal("bad signature")
	}
	var p struct {
		Note    string   `json:"note"`
		Events  []string `json:"events"`
		Version int      `json:"version"`
	}
	json.Unmarshal(body, &p)
	if p.Note != n.token || strings.Join(p.Events, ",") != "updated,entry" || p.Version != 2 || strings.Contains(string(body), "v2") {
		t.Fatalf("payload: %s", body)
	}

	// Without PrivateHooks: https only, and never to private addresses.
	strict, _ := New(Config{}, d)
	if strict.hookURLOK("http://example.com/x") || !strict.hookURLOK("https://example.com/x") {
		t.Fatal("scheme rule")
	}
	for _, ip := range []string{"127.0.0.1", "10.1.2.3", "192.168.1.1", "169.254.169.254", "100.64.0.1", "::1", "fd00::1", "0.0.0.0",
		"64:ff9b::7f00:1", "64:ff9b::a9fe:a9fe", "64:ff9b:1::a00:1", "2002:7f00:1::1", "2002:c0a8:101::1"} {
		if publicIP(net.ParseIP(ip)) {
			t.Fatalf("%s counted as public", ip)
		}
	}
	for _, ip := range []string{"93.184.216.34", "64:ff9b::5db8:d822", "2606:4700::1111"} {
		if !publicIP(net.ParseIP(ip)) {
			t.Fatalf("public address %s refused", ip)
		}
	}
	if _, err := strict.hookClient().Get(hook.URL); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("loopback reached: %v", err)
	}
}

func TestInbox(t *testing.T) {
	_, d, c := newAppDB(t)
	n := c.create(map[string]any{"mode": "read"})
	adm := map[string]string{"X-Admin": n.admin}
	priv, _ := ecdh.P256().GenerateKey(rand.Reader)
	code, m, _ := c.do("POST", "/api/n/"+n.token+"/inbox", map[string]any{"pub": b64u.EncodeToString(priv.PublicKey().Bytes()), "priv": strings.Repeat("s", 60)}, adm)
	inbox, _ := m["inbox"].(string)
	if code != 200 || !strings.Contains(inbox, "/in/") || m["inbox_key"] != strings.Repeat("s", 60) {
		t.Fatalf("inbox on: %d %v", code, m)
	}
	path := inbox[strings.Index(inbox, "/in/"):]
	req, _ := http.NewRequest("POST", c.srv.URL+path, strings.NewReader(`{"text":"server down","from":"uptime"}`))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 201 {
		t.Fatalf("post: %v %d", err, res.StatusCode)
	}
	var stored string
	d.QueryRow(`SELECT content FROM entries`).Scan(&stored)
	if strings.Contains(stored, "server down") || !strings.HasPrefix(stored, "x1.") {
		t.Fatalf("stored in the clear: %s", stored)
	}
	// Decrypt the way the browser does.
	parts := strings.Split(stored, ".")
	ephRaw, _ := b64u.DecodeString(parts[1])
	ct, _ := b64u.DecodeString(parts[2])
	eph, _ := ecdh.P256().NewPublicKey(ephRaw)
	shared, _ := priv.ECDH(eph)
	key, _ := hkdf.Key(sha256.New, shared, nil, "notehop/inbox", 32)
	blk, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(blk)
	plain, err := gcm.Open(nil, ct[:12], ct[12:], nil)
	if err != nil || string(plain) != `{"b":"server down","n":"uptime"}` {
		t.Fatalf("decrypt: %s %v", plain, err)
	}
	// Turned off: the address stops working.
	c.do("DELETE", "/api/n/"+n.token+"/inbox", nil, adm)
	res, _ = http.Post(c.srv.URL+path, "text/plain", strings.NewReader("hi"))
	if res.StatusCode != 404 {
		t.Fatalf("after off: %d", res.StatusCode)
	}
}
