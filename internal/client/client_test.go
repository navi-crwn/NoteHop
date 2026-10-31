package client_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/internal/app"
	. "github.com/navi-crwn/NoteHop/internal/client"
	"github.com/navi-crwn/NoteHop/internal/db"
)

// Vectors from WebCrypto (crypto.js), so CLI and browser agree.
func TestDeriveMatchesBrowser(t *testing.T) {
	k, salt := "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8", "ZGVmZ2hpamtsbW5vcHFycw"
	if got, _ := Derive(k, "", ""); got.Write != "uidR6i4_4kgoydGLLdFxgggjNrrz4RJ9BLLlMrfH1lM" {
		t.Fatalf("no password: %s", got.Write)
	}
	if got, _ := Derive(k, "rahasia", salt); got.Write != "T3iTTUT2TFLAbthI-XAtBJZGkgoZcX2T2mZX7NqdA1U" {
		t.Fatalf("password: %s", got.Write)
	}
}

func TestDerive128Bit(t *testing.T) {
	k128 := NewKey128()
	if len(k128) != 22 {
		t.Fatalf("expected 22 chars for 128-bit key, got %d (%s)", len(k128), k128)
	}
	keys, err := Derive(k128, "", "")
	if err != nil {
		t.Fatalf("derive 128-bit key: %v", err)
	}
	if len(keys.AES) != 32 {
		t.Fatalf("expected 32 bytes AES key from HMAC, got %d", len(keys.AES))
	}
	plain := []byte("Catatan rahasia dengan kunci 128 bit")
	sealed, err := Seal(keys.AES, plain)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	opened, err := Open(keys.AES, sealed)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if string(opened) != string(plain) {
		t.Fatalf("got %q, want %q", string(opened), string(plain))
	}
}

func TestCreateGet(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := app.New(app.Config{}, d)
	srv := httptest.NewServer(a.Handler())
	defer func() { srv.Close(); d.Close() }()

	c, err := Create(srv.URL, Note{Title: "Rapat", Body: "agenda rahasia\n"}, Options{Password: "pw"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.Admin, "&a=") || strings.Contains(c.Share, "&a=") {
		t.Fatalf("links: %+v", c)
	}
	var stored string
	d.QueryRow(`SELECT content FROM notes`).Scan(&stored)
	if strings.Contains(stored, "rahasia") {
		t.Fatal("plaintext reached the server")
	}
	if _, err := Get(c.Share, ""); err == nil {
		t.Fatal("password note opened without password")
	}
	if _, err := Get(c.Share, "wrong"); err == nil {
		t.Fatal("wrong password accepted")
	}
	n, err := Get(c.Share, "pw")
	if err != nil || n.Title != "Rapat" || n.Body != "agenda rahasia\n" {
		t.Fatalf("get: %+v %v", n, err)
	}

	p, err := Create(srv.URL, Note{Body: "halo"}, Options{Public: true, Slug: "halo-dunia"})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := Get(srv.URL+"/n/halo-dunia", ""); err != nil || n.Body != "halo" || p.Token != "halo-dunia" {
		t.Fatalf("public: %+v %v %s", n, err, p.Token)
	}
}

func TestToken(t *testing.T) {
	for in, want := range map[string]string{
		"https://note.example/n/rapat-senin#k=abc&a=def": "rapat-senin",
		"https://n.example/Ab3dE6gH9k#k=abc":             "Ab3dE6gH9k",
		"https://n.example/Ab3dE6gH9k?x=1":               "Ab3dE6gH9k",
		"/n/Ab3dE6gH9k":                                  "Ab3dE6gH9k",
		"Ab3dE6gH9k":                                     "Ab3dE6gH9k",
	} {
		if got := Token(in); got != want {
			t.Errorf("Token(%q) = %q, want %q", in, got, want)
		}
	}
}
