package app

import (
	"testing"
)

// The dashboard must be able to name a basic note from the server when the
// owner's vault blob is empty or the vault is closed: the server can read a
// basic note anyway (/raw prints it). An encrypted note has no title here.
func TestMyNotesNameBasicNotesWithoutTheVault(t *testing.T) {
	c := newClient(t, newTestApp(t))
	if code, _, raw := c.do("POST", "/api/register", map[string]any{"username": "ivan", "auth": key()}, nil); code != 201 {
		t.Fatalf("register: %d %s", code, raw)
	}
	b := c.create(map[string]any{"encrypted": false, "content": `{"t":"Shopping list","b":"- milk"}`, "owner_blob": ""})
	e := c.create(map[string]any{"owner_blob": ""})
	_, m, _ := c.do("GET", "/api/me/notes", nil, nil)
	rows := map[string]map[string]any{}
	for _, x := range m["notes"].([]any) {
		r := x.(map[string]any)
		rows[r["token"].(string)] = r
	}
	if r := rows[b.token]; r == nil || r["title"] != "Shopping list" || r["encrypted"] != false || r["blob"] != "" {
		t.Fatalf("basic row: %v", rows[b.token])
	}
	if r := rows[e.token]; r == nil || r["title"] != "" || r["encrypted"] != true {
		t.Fatalf("encrypted row: %v", rows[e.token])
	}
	for _, r := range rows {
		if _, leaked := r["content"]; leaked {
			t.Fatalf("a dashboard row carries the content: %v", r)
		}
	}
}

// The row's link is a read-only one when the note has one, so a basic note
// shown without a key is never shown through a link that could write.
func TestMyNotesPreferTheReadLink(t *testing.T) {
	c := newClient(t, newTestApp(t))
	if code, _, _ := c.do("POST", "/api/register", map[string]any{"username": "ivan", "auth": key()}, nil); code != 201 {
		t.Fatal("register")
	}
	n := c.create(map[string]any{"encrypted": false, "content": `{"t":"Together","b":"x"}`, "mode": "edit", "owner_blob": ""})
	_, m, _ := c.do("GET", "/api/me/notes", nil, nil)
	if row := m["notes"].([]any)[0].(map[string]any); row["token"] != n.token || row["mode"] != "edit" {
		t.Fatalf("only an edit link: %v", row)
	}
	code, l, raw := c.do("POST", "/api/n/"+n.token+"/links", map[string]any{"mode": "read"}, map[string]string{"X-Admin": n.admin})
	if code != 201 && code != 200 {
		t.Fatalf("add read link: %d %s", code, raw)
	}
	_, m, _ = c.do("GET", "/api/me/notes", nil, nil)
	if row := m["notes"].([]any)[0].(map[string]any); row["token"] != l["token"] || row["mode"] != "read" {
		t.Fatalf("read link preferred: %v", row)
	}
}

// A note made while the vault was closed sits in the account with an empty
// blob. Its owner can seal the key later through /claim; another account can't.
func TestClaimSealsAnOwnedNoteWithoutABlob(t *testing.T) {
	srv := newTestApp(t)
	c := newClient(t, srv)
	if code, _, _ := c.do("POST", "/api/register", map[string]any{"username": "ivan", "auth": key()}, nil); code != 201 {
		t.Fatal("register")
	}
	n := c.create(map[string]any{"owner_blob": ""})
	other := newClient(t, srv)
	if code, _, _ := other.do("POST", "/api/register", map[string]any{"username": "mira", "auth": key()}, nil); code != 201 {
		t.Fatal("register other")
	}
	if code, _, _ := other.do("POST", "/api/n/"+n.token+"/claim", map[string]any{"owner_blob": "theirs"}, map[string]string{"X-Admin": n.admin}); code != 409 {
		t.Fatalf("another account must not take the note: %d", code)
	}
	if code, _, raw := c.do("POST", "/api/n/"+n.token+"/claim", map[string]any{"owner_blob": "sealed-later"}, map[string]string{"X-Admin": n.admin}); code != 200 {
		t.Fatalf("owner seals the key: %d %s", code, raw)
	}
	_, m, _ := c.do("GET", "/api/me/notes", nil, nil)
	if row := m["notes"].([]any)[0].(map[string]any); row["blob"] != "sealed-later" {
		t.Fatalf("blob after claim: %v", row)
	}
	// Once sealed, the blob is not replaced through /claim (the editor saves it with the note).
	if code, _, _ := c.do("POST", "/api/n/"+n.token+"/claim", map[string]any{"owner_blob": "again"}, map[string]string{"X-Admin": n.admin}); code != 409 {
		t.Fatalf("a sealed note is not re-claimed: %d", code)
	}
}
