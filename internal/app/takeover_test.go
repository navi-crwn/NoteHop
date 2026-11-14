package app

import (
	"net/http"
	"testing"
)

// A basic note's owner gets new editing keys from the session alone; nobody else, no other kind of note, no other
// origin can. Every refusal leaves the keys as they were.
func TestTakeoverIsForTheOwnerOfABasicNoteOnly(t *testing.T) {
	e := newRecheckEnv(t)
	a := e.member("7001", 1)
	b := e.member("7002", 1)
	keys := func(token string) (string, string) {
		var ah, wh string
		if err := e.db.QueryRow(`SELECT n.admin_hash, n.write_hash FROM notes n JOIN links l ON l.note_id = n.id WHERE l.token = ?`, token).Scan(&ah, &wh); err != nil {
			t.Fatal(err)
		}
		return ah, wh
	}
	body := func() map[string]string { return map[string]string{"admin": key(), "write": key()} }
	path := func(tok string) string { return "/api/me/notes/" + tok + "/takeover" }
	oldA, oldW := keys(a.basic.token)

	anon := noRedirect(t, e.srv)
	if code, _, _ := anon.do("POST", path(a.basic.token), body(), nil); code != http.StatusUnauthorized {
		t.Errorf("without a session: %d", code)
	}
	if code, _, _ := b.c.do("POST", path(a.basic.token), body(), nil); code != http.StatusForbidden {
		t.Errorf("another account: %d", code)
	}
	if code, _, _ := a.c.do("POST", path(a.notes[0].token), body(), nil); code != http.StatusBadRequest {
		t.Errorf("an encrypted note: %d", code)
	}
	if code, _, _ := a.c.do("POST", path(a.basic.token), body(), map[string]string{"Sec-Fetch-Site": "cross-site"}); code != http.StatusForbidden {
		t.Errorf("from another site: %d", code)
	}
	if code, _, _ := a.c.do("POST", path("nosuchtoken"), body(), nil); code != http.StatusNotFound {
		t.Errorf("an unknown note: %d", code)
	}
	same := key()
	for name, bad := range map[string]map[string]string{"no keys": {}, "short keys": {"admin": "short", "write": "short"}, "the same key twice": {"admin": same, "write": same}} {
		if code, _, _ := a.c.do("POST", path(a.basic.token), bad, nil); code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, code)
		}
	}
	if _, err := e.db.Exec(`UPDATE notes SET frozen = 1 WHERE owner_id = ? AND encrypted = 0`, a.user.ID); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := a.c.do("POST", path(a.basic.token), body(), nil); code != http.StatusForbidden {
		t.Errorf("a snapshot: %d", code)
	}
	if _, err := e.db.Exec(`UPDATE notes SET frozen = 0 WHERE owner_id = ?`, a.user.ID); err != nil {
		t.Fatal(err)
	}
	// a stale session (the account server has not vouched for it lately) counts as nobody
	e.age()
	if code, _, _ := a.c.do("POST", path(a.basic.token), body(), nil); code != http.StatusUnauthorized {
		t.Errorf("a stale session: %d", code)
	}
	if ah, wh := keys(a.basic.token); ah != oldA || wh != oldW {
		t.Fatal("a refused takeover changed the keys")
	}

	// the owner, with a session the account server vouched for
	if _, err := e.db.Exec(`UPDATE sessions SET verified_at = strftime('%s','now')`); err != nil {
		t.Fatal(err)
	}
	nb := body()
	if code, _, raw := a.c.do("POST", path(a.basic.token), nb, nil); code != http.StatusOK {
		t.Fatalf("the owner: %d %s", code, raw)
	}
	ah, wh := keys(a.basic.token)
	if ah != hashHex(nb["admin"]) || wh != hashHex(nb["write"]) {
		t.Fatal("the new keys were not stored as hashes")
	}
	// the new keys work as the owner link; the old admin key no longer does
	if code, m, _ := noRedirect(t, e.srv).do("GET", "/api/n/"+a.basic.token, nil, map[string]string{"X-Admin": nb["admin"]}); code != 200 || m["admin"] != true {
		t.Errorf("the new admin key: %d %v", code, m)
	}
	if code, m, _ := noRedirect(t, e.srv).do("GET", "/api/n/"+a.basic.token, nil, map[string]string{"X-Admin": a.basic.admin}); code != 200 || m["admin"] == true {
		t.Errorf("the old admin key still works: %d %v", code, m)
	}
	if ah2, _ := keys(b.basic.token); ah2 == hashHex(nb["admin"]) {
		t.Error("another account's keys changed")
	}
}
