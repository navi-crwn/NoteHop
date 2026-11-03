package app

import (
	"encoding/json"
	"strings"
	"testing"
)

const fakeNoteKey = "FAKEKEY0notehop0s2audit0000000000000000000"

// Nothing the control center (/hop/*) can read may contain an encrypted note's
// ciphertext, its owner blob or a note key that a reporter pasted.
func TestHopNeverReturnsEncryptedContentOrKeys(t *testing.T) {
	srv, _, _ := hopApp(t, Config{FileMB: 10})
	u := ssoUser(t, srv, "9", "dewi@example.com", nil)
	enc := u.create(map[string]any{"content": fakeCipher("secret body"), "owner_blob": "OWNERBLOB-FAKE-0001"})
	c := newClient(t, srv)
	link := "https://note.test/" + enc.token + "#k=" + fakeNoteKey + "&a=ADMINTOKEN0FAKE000000000000000000000000000"
	if code, m, _ := c.do("POST", "/api/n/"+enc.token+"/report", map[string]any{"reason": "abuse", "note": "please look at " + link}, nil); code != 201 {
		t.Fatalf("report: %d %v", code, m)
	}
	_, env := call(t, srv, "imperium", "GET", "/hop/content?type=note", nil)
	id := list(env)[0].(map[string]any)["id"].(string)
	paths := []string{"/hop/content?type=note", "/hop/content/note/" + id, "/hop/search?q=secret", "/hop/audit", "/hop/metrics", "/hop/stats", "/hop/users?q=dewi", "/hop/reports?status=open"}
	for _, p := range paths {
		_, env := call(t, srv, "imperium", "GET", p, nil)
		raw, _ := json.Marshal(env)
		for _, bad := range []string{"secret body", fakeCipher("secret body"), "OWNERBLOB-FAKE-0001", "k=FAKE", fakeNoteKey, "ADMINTOKEN0FAKE"} {
			if strings.Contains(string(raw), bad) {
				t.Errorf("%s leaks %q: %.300s", p, bad, raw)
			}
		}
	}
}
