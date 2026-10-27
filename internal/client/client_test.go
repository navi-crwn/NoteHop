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
