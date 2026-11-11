package client_test

import (
	"bytes"
	"encoding/base64"
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

// Fixed vectors made by Seal (the same scheme as seal() in crypto.js), so the offline
// decrypt keeps reading what the web client writes.
const (
	vecKey   = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"
	vecSalt  = "ZGVmZ2hpamtsbW5vcHFycw"
	vecPlain = "MzFY731hJ5bG2Efb1AZvxBKB1FrWAzq3cxylDCFVyUggGcZ4BOA_fBVjQE3_tTMIDuEE99yMQot1IUYx2Cwrq7Pt_CV9AdXddLNydkWzj59Xog"
	vecPass  = "p39-tn6eYs35z1-mhgiBOT9TXKzX4H0XBVxrV03XGD3XyK7o89Jbk8UXWX8q_a1vjoC_O3Bt0YxtSf5JtDwNiOwuto-Nm75QvHST0FX_LnzLGw"
)

func TestDecryptVector(t *testing.T) {
	want := Note{Title: "Vector", Body: "Sealed by Seal; opened offline.\n"}
	for name, c := range map[string]struct{ payload, key, pw, salt string }{
		"bare key":        {vecPlain, vecKey, "", ""},
		"link":            {vecPlain, "https://note.example/AbCdEfGh12#k=" + vecKey, "", ""},
		"admin link":      {vecPlain, "https://note.example/AbCdEfGh12#k=" + vecKey + "&a=xyz", "", ""},
		"fragment":        {vecPlain, "k=" + vecKey, "", ""},
		"padded + spaces": {"  " + vecPlain[:40] + "\n" + vecPlain[40:] + "\n", vecKey, "", ""},
		"open JSON":       {`{"content":"` + vecPlain + `","version":3}`, vecKey, "", ""},
		"password":        {vecPass, vecKey, "rahasia", vecSalt},
	} {
		got, err := Decrypt(c.payload, c.key, c.pw, c.salt)
		if err != nil || got != want {
			t.Errorf("%s: %+v %v", name, got, err)
		}
	}
	for name, c := range map[string]struct{ payload, key, pw, salt string }{
		"wrong key":      {vecPlain, key32('x'), "", ""},
		"no key":         {vecPlain, "", "", ""},
		"password lost":  {vecPass, vecKey, "", ""},
		"wrong password": {vecPass, vecKey, "salah", vecSalt},
		"no salt":        {vecPass, vecKey, "rahasia", ""},
		"not a payload":  {"hello", vecKey, "", ""},
		"tampered":       {vecPlain[:20] + "A" + vecPlain[21:], vecKey, "", ""},
		"json no field":  {`{"x":1}`, vecKey, "", ""},
	} {
		if _, err := Decrypt(c.payload, c.key, c.pw, c.salt); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func key32(c byte) string { return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{c}, 32)) }

// What Create writes, Decrypt reads: the stored content of a new note.
func TestDecryptReadsCreate(t *testing.T) {
	d, _ := db.Open(":memory:")
	a, _ := app.New(app.Config{}, d)
	srv := httptest.NewServer(a.Handler())
	defer func() { srv.Close(); d.Close() }()
	c, err := Create(srv.URL, Note{Title: "T", Body: "b\n"}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	d.QueryRow(`SELECT content FROM notes`).Scan(&stored)
	if n, err := Decrypt(stored, c.Share, "", ""); err != nil || n.Title != "T" || n.Body != "b\n" {
		t.Fatalf("%+v %v", n, err)
	}
}

func TestKeyFrom(t *testing.T) {
	for in, want := range map[string]string{
		"abc": "abc", " abc\n": "abc", "k=abc&a=def": "abc", "#k=abc": "abc",
		"https://n.example/T#k=abc&a=def": "abc", "https://n.example/T": "https://n.example/T",
	} {
		if got := KeyFrom(in); got != want {
			t.Errorf("KeyFrom(%q) = %q, want %q", in, got, want)
		}
	}
}
