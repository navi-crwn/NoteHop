package client_test

import (
	"os/exec"
	"strings"
	"testing"

	"net/http/httptest"

	"github.com/navi-crwn/NoteHop/internal/app"
	. "github.com/navi-crwn/NoteHop/internal/client"
	"github.com/navi-crwn/NoteHop/internal/db"
)

// Vectors made by crypto.js (NHC.passwordKeys) in Node WebCrypto: the browser and this client must agree, or a
// note locked in one cannot be opened in the other.
var passwordVectors = []struct{ password, salt, want string }{
	{"correct horse battery staple", "AAAAAAAAAAAAAAAAAAAAAA", "NZzTYgPWLfqfnjYgO5HN_9gxN1pjdndr_HCj6qSJqMo"},
	{"pässwörd 密码", "q83vEjRWeJq83vEjRWeJqg", "dRMzlfJ9wB7X3rs2rEkb1mciq8j2hZpXd7zlvJzxqIc"},
}

func TestPasswordKeysMatchTheBrowserVectors(t *testing.T) {
	for _, v := range passwordVectors {
		got, err := PasswordKeys(v.password, v.salt)
		if err != nil || got != v.want {
			t.Errorf("PasswordKeys(%q, %q) = %q, %v; want %q", v.password, v.salt, got, err, v.want)
		}
	}
	// the note key plays no part: the proof is not the one a note key and the same password would give
	if k, _ := Derive("AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8", "correct horse battery staple", "AAAAAAAAAAAAAAAAAAAAAA"); k.Write == passwordVectors[0].want {
		t.Error("the basic-note proof equals an encrypted note's proof")
	}
	if _, err := PasswordKeys("x", "!!"); err == nil {
		t.Error("a bad salt was accepted")
	}
}

// Runs the real web/static/js/crypto.js under Node (when Node is installed) and compares: the vectors above could
// drift from the file otherwise.
func TestPasswordKeysMatchTheRealCryptoJS(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; the vectors above still pin the Go side")
	}
	for _, v := range passwordVectors {
		out, err := exec.Command(node, "../../scripts/check-crypto-vector.js", v.password, v.salt).CombinedOutput()
		if err != nil {
			t.Fatalf("node: %v\n%s", err, out)
		}
		if got := strings.TrimSpace(string(out)); got != v.want {
			t.Errorf("crypto.js gives %q for %q; want %q", got, v.password, v.want)
		}
	}
}

// A basic note with a password, made and read through the command-line client: locked for others, open with the
// password, and its short link (no note key anywhere) works.
func TestBasicNoteWithPasswordThroughTheClient(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := app.New(app.Config{}, d)
	srv := httptest.NewServer(a.Handler())
	defer func() { srv.Close(); d.Close() }()

	c, err := Create(srv.URL, Note{Title: "Rapat", Body: "isi biasa\n"}, Options{Public: true, Password: "pw", Slug: "rapat-kunci"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(c.Share, "#") || !strings.Contains(c.Admin, "#a=") {
		t.Fatalf("links: %+v", c)
	}
	var pw int
	d.QueryRow(`SELECT password FROM notes`).Scan(&pw)
	if pw != 1 {
		t.Fatal("the note has no password")
	}
	if _, err := Get(c.Share, ""); err == nil {
		t.Fatal("opened without the password")
	}
	if _, err := Get(c.Share, "wrong"); err == nil {
		t.Fatal("wrong password accepted")
	}
	if n, err := Get(srv.URL+"/rapat-kunci", "pw"); err != nil || n.Body != "isi biasa\n" {
		t.Fatalf("short link with the password: %+v %v", n, err)
	}
}
