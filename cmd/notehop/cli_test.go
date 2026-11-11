package main

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/navi-crwn/NoteHop/internal/app"
	"github.com/navi-crwn/NoteHop/internal/client"
	"github.com/navi-crwn/NoteHop/internal/db"
)

func testServer(t *testing.T) (*httptest.Server, *db.DB) {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	a, err := app.New(app.Config{}, d)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() { srv.Close(); d.Close() })
	return srv, d
}

func get(t *testing.T, stdin string, terminal bool, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := runGet(args, strings.NewReader(stdin), &out, &errb, terminal)
	return code, out.String(), errb.String()
}

// notehop new | notehop get: the link comes from stdin, and so do "TOKEN KEY" and a flag key.
func TestGetFromStdin(t *testing.T) {
	srv, _ := testServer(t)
	c, err := client.Create(srv.URL, client.Note{Title: "Rapat", Body: "agenda\n"}, client.Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := "# Rapat\n\nagenda\n"

	if code, out, e := get(t, c.Share+"\n", false); code != 0 || out != want {
		t.Fatalf("link on stdin: %d %q %q", code, out, e)
	}
	if code, out, e := get(t, c.Share, false, c.Share); code != 0 || out != want { // an argument wins over stdin
		t.Fatalf("argument: %d %q %q", code, out, e)
	}
	key := client.KeyFrom(c.Share)
	if code, out, e := get(t, c.Token+" "+key+"\n", false, "-server", srv.URL); code != 0 || out != want {
		t.Fatalf("TOKEN KEY: %d %q %q", code, out, e)
	}
	if code, out, e := get(t, c.Token+"\n", false, "-server", srv.URL, "-key", c.Share); code != 0 || out != want {
		t.Fatalf("TOKEN with -key: %d %q %q", code, out, e)
	}
	if code, _, e := get(t, c.Token+"\n", false, "-server", srv.URL); code == 0 || !strings.Contains(e, "no key") {
		t.Fatalf("TOKEN without a key: %d %q", code, e)
	}
	if code, _, e := get(t, "", false); code != 2 || !strings.Contains(e, "nothing on stdin") {
		t.Fatalf("empty stdin: %d %q", code, e)
	}
	if code, _, e := get(t, "", true); code != 2 || !strings.Contains(e, "usage") { // a terminal gets help, not a wait
		t.Fatalf("terminal: %d %q", code, e)
	}
	if code, _, _ := get(t, "", false, "a", "b"); code != 2 {
		t.Fatalf("two arguments: %d", code)
	}
}

func TestGetPasswordFromEnv(t *testing.T) {
	srv, _ := testServer(t)
	c, _ := client.Create(srv.URL, client.Note{Body: "x\n"}, client.Options{Password: "pw"})
	if code, _, _ := get(t, c.Share, false); code == 0 {
		t.Fatal("opened without the password")
	}
	t.Setenv("NOTEHOP_PASSWORD", "pw")
	if code, out, e := get(t, c.Share, false); code != 0 || out != "x\n" {
		t.Fatalf("%d %q %q", code, out, e)
	}
}

// noNetwork makes any HTTP request in this test a failure: the default transport (which the
// client package's http.Client falls back to) refuses and records the attempt.
type deadTransport struct{ t *testing.T }

func (d deadTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	d.t.Errorf("network used: %s %s", r.Method, r.URL)
	return nil, errors.New("network is off in this test")
}

func noNetwork(t *testing.T) {
	t.Helper()
	old := http.DefaultTransport
	http.DefaultTransport = deadTransport{t}
	t.Cleanup(func() { http.DefaultTransport = old })
}

func decrypt(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := runDecrypt(args, strings.NewReader(stdin), &out, &errb, false)
	return code, out.String(), errb.String()
}

// The fixed vector, from a file and from stdin, with a server that does not exist:
// decrypt never connects.
func TestDecryptOffline(t *testing.T) {
	const payload = "MzFY731hJ5bG2Efb1AZvxBKB1FrWAzq3cxylDCFVyUggGcZ4BOA_fBVjQE3_tTMIDuEE99yMQot1IUYx2Cwrq7Pt_CV9AdXddLNydkWzj59Xog"
	const link = "http://127.0.0.1:1/AbCdEfGh12#k=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"
	want := "# Vector\n\nSealed by Seal; opened offline.\n"
	noNetwork(t)

	if code, out, e := decrypt(t, payload+"\n", "-key", link); code != 0 || out != want {
		t.Fatalf("stdin: %d %q %q", code, out, e)
	}
	f := filepath.Join(t.TempDir(), "p.txt")
	os.WriteFile(f, []byte(payload), 0o600)
	if code, out, e := decrypt(t, "", "-key", link, f); code != 0 || out != want {
		t.Fatalf("file: %d %q %q", code, out, e)
	}
	if code, out, _ := decrypt(t, payload, "-key", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8", "-"); code != 0 || out != want {
		t.Fatalf("bare key, dash: %d %q", code, out)
	}
	if code, _, e := decrypt(t, payload, "-key", "k="+client.NewKey()); code != 1 || !strings.Contains(e, "wrong key") {
		t.Fatalf("wrong key: %d %q", code, e)
	}
	if code, _, _ := decrypt(t, payload); code != 2 { // the key is required
		t.Fatalf("no key: %d", code)
	}
	if code, _, _ := decrypt(t, "", "-key", link, filepath.Join(t.TempDir(), "missing")); code != 1 {
		t.Fatalf("missing file: %d", code)
	}
	var out, errb bytes.Buffer
	if code := runDecrypt([]string{"-key", link}, strings.NewReader(""), &out, &errb, true); code != 2 {
		t.Fatalf("terminal, no file: %d", code)
	}
}

// A real round trip against the server: what the server stores is what decrypt reads,
// with a password and its salt from the metadata.
func TestDecryptStoredNote(t *testing.T) {
	srv, d := testServer(t)
	c, _ := client.Create(srv.URL, client.Note{Title: "S", Body: "geheim\n"}, client.Options{Password: "pw"})
	var content, salt string
	d.QueryRow(`SELECT content, salt FROM notes`).Scan(&content, &salt)
	if strings.Contains(content, "geheim") {
		t.Fatal("plaintext on the server")
	}
	if code, _, _ := decrypt(t, content, "-key", c.Share); code == 0 {
		t.Fatal("decrypted a password note without the password")
	}
	if code, out, e := decrypt(t, content, "-key", c.Share, "-password", "pw", "-salt", salt); code != 0 || out != "# S\n\ngeheim\n" {
		t.Fatalf("%d %q %q", code, out, e)
	}
}

func TestParseFileTTL(t *testing.T) {
	for in, want := range map[string]time.Duration{"": 24 * time.Hour, "24": 24 * time.Hour, " 6 ": 6 * time.Hour, "1": time.Hour, "720": 720 * time.Hour} {
		if got, err := parseFileTTL(in); err != nil || got != want {
			t.Errorf("%q: %v %v", in, got, err)
		}
	}
	for _, in := range []string{"0", "-5", "721", "24h", "1.5", "abc", "99999999999999999999"} {
		got, err := parseFileTTL(in)
		if err == nil || got != 24*time.Hour {
			t.Errorf("%q: want an error and the default, got %v %v", in, got, err)
		}
	}
}

// The key can come from a file or NOTEHOP_KEY, so it need not be on the command line.
func TestKeyFromFileAndEnv(t *testing.T) {
	const payload = "MzFY731hJ5bG2Efb1AZvxBKB1FrWAzq3cxylDCFVyUggGcZ4BOA_fBVjQE3_tTMIDuEE99yMQot1IUYx2Cwrq7Pt_CV9AdXddLNydkWzj59Xog"
	const key = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"
	want := "# Vector\n\nSealed by Seal; opened offline.\n"
	noNetwork(t)
	dir := t.TempDir()
	bare, link, empty := filepath.Join(dir, "k"), filepath.Join(dir, "l"), filepath.Join(dir, "e")
	os.WriteFile(bare, []byte(key+"\n"), 0o600)
	os.WriteFile(link, []byte("https://n.example/AbCdEfGh12#k="+key+"\n"), 0o600)
	os.WriteFile(empty, nil, 0o600)

	if code, out, e := decrypt(t, payload, "-key-file", bare); code != 0 || out != want {
		t.Fatalf("bare key file: %d %q %q", code, out, e)
	}
	if code, out, e := decrypt(t, payload, "-key-file", link); code != 0 || out != want {
		t.Fatalf("link file: %d %q %q", code, out, e)
	}
	if code, _, _ := decrypt(t, payload, "-key-file", empty); code != 1 {
		t.Fatalf("empty key file: %d", code)
	}
	if code, _, _ := decrypt(t, payload, "-key-file", filepath.Join(dir, "missing")); code != 1 {
		t.Fatalf("missing key file: %d", code)
	}
	t.Setenv("NOTEHOP_KEY", "https://n.example/AbCdEfGh12#k="+key)
	if code, out, e := decrypt(t, payload); code != 0 || out != want {
		t.Fatalf("env: %d %q %q", code, out, e)
	}
	// An explicit -key beats the environment.
	if code, _, _ := decrypt(t, payload, "-key", "k="+client.NewKey()); code != 1 {
		t.Fatalf("flag should win over env: %d", code)
	}
	// The help warns about -key.
	var out, errb bytes.Buffer
	runDecrypt([]string{"-h"}, strings.NewReader(""), &out, &errb, false)
	if !strings.Contains(errb.String(), "process list") || !strings.Contains(errb.String(), "-key-file") {
		t.Fatalf("help lacks the warning: %q", errb.String())
	}
}

func TestGetKeyFromFileAndEnv(t *testing.T) {
	srv, _ := testServer(t)
	c, _ := client.Create(srv.URL, client.Note{Title: "Rapat", Body: "agenda\n"}, client.Options{})
	want := "# Rapat\n\nagenda\n"
	kf := filepath.Join(t.TempDir(), "k")
	os.WriteFile(kf, []byte(client.KeyFrom(c.Share)+"\n"), 0o600)
	if code, out, e := get(t, c.Token+"\n", false, "-server", srv.URL, "-key-file", kf); code != 0 || out != want {
		t.Fatalf("key file: %d %q %q", code, out, e)
	}
	t.Setenv("NOTEHOP_KEY", client.KeyFrom(c.Share))
	if code, out, e := get(t, c.Token+"\n", false, "-server", srv.URL); code != 0 || out != want {
		t.Fatalf("env: %d %q %q", code, out, e)
	}
	// A link that carries its own key keeps it; the variable only fills a gap.
	t.Setenv("NOTEHOP_KEY", client.NewKey())
	if code, out, e := get(t, c.Share+"\n", false); code != 0 || out != want {
		t.Fatalf("own key kept: %d %q %q", code, out, e)
	}
}

func TestParseAccountMB(t *testing.T) {
	for in, want := range map[string]int{"": 100, "100": 100, "0": 0, " 250 ": 250, "1": 1} {
		if got, err := parseAccountMB(in); err != nil || got != want {
			t.Errorf("%q: %d %v", in, got, err)
		}
	}
	for _, in := range []string{"-1", "-100", "abc", "1.5", "10MB", "99999999999999999999"} {
		if got, err := parseAccountMB(in); err == nil || got != 100 {
			t.Errorf("%q: want an error and the default 100, got %d %v", in, got, err)
		}
	}
}
