package app

import (
	"io"
	"strings"
	"testing"
	"time"
)

// The recovery-code card: shown while the account has no codes, hidden for
// seven days after "Hide", back after that, and gone for good once the codes
// exist. The endpoint takes known keys from a signed-in member only.
func TestReminderCard(t *testing.T) {
	_, d, c := newAppDB(t)
	card := func() bool {
		resp, err := c.hc.Get(c.srv.URL + "/dashboard")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return strings.Contains(string(b), `data-hop-remind="recovery-codes" data-hop-remind-url="/api/me/reminders/dismiss"`)
	}
	dismiss := func(key string) int {
		code, _, _ := c.do("POST", "/api/me/reminders/dismiss", map[string]any{"key": key}, nil)
		return code
	}
	if got := dismiss("recovery-codes"); got != 401 {
		t.Fatalf("signed out: %d", got)
	}
	pw := key()
	if code, _, raw := c.do("POST", "/api/register", map[string]any{"username": "rani", "auth": pw}, nil); code != 201 {
		t.Fatalf("register: %d %s", code, raw)
	}
	if !card() {
		t.Fatal("no recovery codes yet: the card must be on the dashboard")
	}
	if got := dismiss("something-else"); got != 400 {
		t.Fatalf("unknown key: %d", got)
	}
	if !card() {
		t.Fatal("an unknown key must not hide anything")
	}
	if got := dismiss("recovery-codes"); got != 200 {
		t.Fatalf("dismiss: %d", got)
	}
	if card() {
		t.Fatal("hidden just now: the card must be gone")
	}
	// Six days later it is still hidden; after seven it is back, because the task is still open.
	age := func(days int) {
		if _, err := d.Exec(`UPDATE reminders SET dismissed_at = ?`, time.Now().Add(-time.Duration(days)*24*time.Hour).Unix()); err != nil {
			t.Fatal(err)
		}
	}
	age(6)
	if card() {
		t.Fatal("six days after hiding: still hidden")
	}
	age(7)
	if !card() {
		t.Fatal("seven days after hiding, with no codes made: the card must be back")
	}
	// Hiding it again starts another week.
	if got := dismiss("recovery-codes"); got != 200 || card() {
		t.Fatalf("second dismiss: %d", got)
	}
	// Once the codes exist the card is gone for good, however old the dismissal is.
	proofs := make([]string, recoveryCount)
	for i := range proofs {
		proofs[i] = key()
	}
	if code, _, raw := c.do("POST", "/api/me/recovery", map[string]any{"current": pw, "codes": codeSet(proofs)}, nil); code != 200 && code != 201 {
		t.Fatalf("make codes: %d %s", code, raw)
	}
	age(30)
	if card() {
		t.Fatal("the codes exist: the card must not come back")
	}
}

// The migration can meet a database that already has the table.
func TestReminderMigrationIsIdempotent(t *testing.T) {
	_, d, _ := newAppDB(t)
	stmt := `CREATE TABLE IF NOT EXISTS reminders (user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, key TEXT NOT NULL,
		dismissed_at INTEGER NOT NULL, PRIMARY KEY (user_id, key));`
	if _, err := d.Exec(stmt); err != nil {
		t.Fatalf("second run: %v", err)
	}
}
