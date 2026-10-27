package app

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/navi-crwn/NoteHop/internal/db"
)

func TestTelegram(t *testing.T) {
	sent := make(chan map[string]any, 10)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		json.NewDecoder(r.Body).Decode(&m)
		m["_path"] = r.URL.Path
		sent <- m
	}))
	defer api.Close()
	old := telegramAPI
	telegramAPI = api.URL
	defer func() { telegramAPI = old }()

	d, _ := db.Open(":memory:")
	a, _ := New(Config{TelegramToken: "123:abc", BaseURL: "https://note.test"}, d)
	srv := httptest.NewServer(a.Handler())
	defer func() { srv.Close(); d.Close() }()
	c := newClient(t, srv)

	if err := a.SetupTelegram(t.Context()); err != nil {
		t.Fatal(err)
	}
	hook := <-sent
	if hook["_path"] != "/bot123:abc/setWebhook" || hook["url"] != "https://note.test/api/telegram/"+a.telegramSecret() {
		t.Fatalf("setWebhook: %v", hook)
	}

	n := c.create(map[string]any{})
	priv, _ := ecdh.P256().GenerateKey(rand.Reader)
	_, m, _ := c.do("POST", "/api/n/"+n.token+"/inbox", map[string]any{"pub": b64u.EncodeToString(priv.PublicKey().Bytes()), "priv": strings.Repeat("s", 60)}, map[string]string{"X-Admin": n.admin})
	inbox := m["inbox"].(string)

	say := func(text string, secret string) (int, string) {
		body := `{"message":{"text":` + string(mustJSON(text)) + `,"chat":{"id":42},"from":{"first_name":"Ani","language_code":"id"}}}`
		req, _ := http.NewRequest("POST", srv.URL+"/api/telegram/"+a.telegramSecret(), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, res.Body)
		if res.StatusCode != 200 {
			return res.StatusCode, ""
		}
		select {
		case r := <-sent:
			return 200, r["text"].(string)
		case <-time.After(3 * time.Second):
			t.Fatal("no reply")
		}
		return 0, ""
	}
	if code, _ := say("hai", "wrong"); code != 404 {
		t.Fatalf("bad secret: %d", code)
	}
	if _, r := say("hai", a.telegramSecret()); !strings.Contains(r, "/help") {
		t.Fatalf("not connected reply: %q", r)
	}
	if _, r := say("/connect "+inbox, a.telegramSecret()); !strings.Contains(r, "Tersambung") {
		t.Fatalf("connect (Indonesian reply expected): %q", r)
	}
	say("beli susu", a.telegramSecret())
	var content string
	d.QueryRow(`SELECT content FROM entries`).Scan(&content)
	if !strings.HasPrefix(content, "x1.") || strings.Contains(content, "susu") {
		t.Fatalf("entry: %q", content)
	}
	say("/disconnect", a.telegramSecret())
	if _, r := say("lagi", a.telegramSecret()); !strings.Contains(r, "/help") {
		t.Fatalf("after disconnect: %q", r)
	}
}
