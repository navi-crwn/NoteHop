package app

import (
	"net/http"
	"sync"
	"testing"
	"time"
)

// Review fixes for the guess limiter (pwguess.go).

// 1. Encrypted notes with a password (vault notes, decoys) are never limited or refused with "too many tries": the
// answer would give a decoy away and lock out the holder of the real password.
func TestEncryptedNotesWithAPasswordAreNeverRateLimited(t *testing.T) {
	e := newLockedEnv(t, Config{})
	c := newClient(t, e.srv)
	n := c.create(map[string]any{"password": true, "salt": key()[:22]})
	for i := 0; i < 3*pwTriesPerMinute; i++ {
		if code, m, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, map[string]string{"X-Write": key()}); code != http.StatusForbidden || m["code"] == "err.password_tries" {
			t.Fatalf("wrong guess %d on an encrypted note: %d %v", i, code, m)
		}
	}
	if code, _, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, map[string]string{"X-Write": n.write}); code != 200 {
		t.Fatalf("the real password after many wrong guesses: %d", code)
	}
}

// 2. Whoever holds only the admin link must know the password: guesses with the admin key count like any other.
func TestAdminKeyDoesNotLiftTheGuessLimit(t *testing.T) {
	e := newLockedEnv(t, Config{})
	c := newClient(t, e.srv)
	n := lockedNote(c, nil)
	for i := 0; i < pwTriesPerMinute; i++ {
		if code, _, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, map[string]string{"X-Write": key(), "X-Admin": n.admin}); code != http.StatusForbidden {
			t.Fatalf("guess %d with the admin key: %d", i, code)
		}
	}
	if code, _, _ := c.do("POST", "/api/n/"+n.token+"/open", nil, map[string]string{"X-Write": key(), "X-Admin": n.admin}); code != http.StatusTooManyRequests {
		t.Fatalf("guesses with the admin key were not limited: %d", code)
	}
}

// 3. Concurrent guesses cannot pass the limit (check and count are one step). Run with -race.
func TestConcurrentGuessesCannotPassTheLimit(t *testing.T) {
	e := newLockedEnv(t, Config{})
	c := newClient(t, e.srv)
	n := lockedNote(c, nil)
	note, _, err := e.db.NoteByLink(n.token)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	answered := 0
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if match, wait := e.a.writeProof(note, key(), "198.51.100.50", func() bool { return false }); !match && wait == 0 {
				mu.Lock()
				answered++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if answered != pwTriesPerMinute {
		t.Fatalf("%d wrong guesses were answered, want exactly %d", answered, pwTriesPerMinute)
	}
}

// 4. Strangers cannot lock a note for those who have the password: an address that opened it keeps getting in when the
// note is over its hourly allowance; a right proof clears that address's own wrong count; taking the note over starts
// the counts again.
func TestStrangersCannotLockOutThoseWhoHaveThePassword(t *testing.T) {
	e := newLockedEnv(t, Config{})
	c := newClient(t, e.srv)
	n := lockedNote(c, nil)
	note, _, _ := e.db.NoteByLink(n.token)
	no := func() bool { return false }
	if m, w := e.a.writeProof(note, n.write, "192.0.2.1", no); !m || w != 0 {
		t.Fatalf("the right password: %v %v", m, w)
	}
	for i := 0; i < pwTriesPerMinute-1; i++ { // mistyped a few times from the same address, then right
		e.a.writeProof(note, key(), "192.0.2.1", no)
	}
	if m, _ := e.a.writeProof(note, n.write, "192.0.2.1", no); !m {
		t.Fatal("a right proof after a few mistakes was refused")
	}
	for i := 0; i < pwTriesPerHour; i++ { // a crowd
		e.a.writeProof(note, key(), "203.0.113."+time.Duration(i).String(), no)
	}
	if m, w := e.a.writeProof(note, n.write, "192.0.2.99", no); m || w == 0 {
		t.Fatalf("a stranger's address past the note's allowance: %v %v", m, w)
	}
	if m, w := e.a.writeProof(note, n.write, "192.0.2.1", no); !m || w != 0 {
		t.Fatalf("the address that already opened the note was shut out: %v %v", m, w)
	}
	e.a.pwTries.forget(note.ID)
	if m, w := e.a.writeProof(note, n.write, "192.0.2.99", no); !m || w != 0 {
		t.Fatalf("after the counts started again: %v %v", m, w)
	}
}
