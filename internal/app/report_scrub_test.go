package app

import (
	"context"
	"strings"
	"testing"

	"github.com/navi-crwn/NoteHop/internal/db"
)

const scrubFakeKey = "FAKEKEY0notehop0s2test00000000000000000000"

func TestScrubKeyFragmentsSpellings(t *testing.T) {
	for _, s := range []string{
		"https://note.hel.ink/AbC123xyz0#k=" + scrubFakeKey,
		"https://note.hel.ink/AbC123xyz0#K=" + scrubFakeKey,
		"https://note.hel.ink/AbC123xyz0#k%3D" + scrubFakeKey,
		"https://note.hel.ink/AbC123xyz0#x=1&a=" + scrubFakeKey,
		"https://note.hel.ink/AbC123xyz0%23k=" + scrubFakeKey,
		"https://note.hel.ink/AbC123xyz0%23K%3d" + scrubFakeKey,
		"https://note.hel.ink/AbC123xyz0%2523k%253D" + scrubFakeKey,
		"https://note.hel.ink/AbC123xyz0%23x=1%26a=" + scrubFakeKey,
		"https://note.hel.ink/AbC123xyz0 k=" + scrubFakeKey,
		"k=" + scrubFakeKey,
		"the key: k=" + scrubFakeKey + ", thanks",
		"a=" + scrubFakeKey,
		"(k=" + scrubFakeKey + ")",
	} {
		if out := scrubKeyFragments(s); strings.Contains(out, scrubFakeKey) {
			t.Errorf("%q kept the key: %q", s, out)
		}
	}
	// Ordinary words and short values are not keys.
	for _, s := range []string{
		"task=abcdefghijkl",
		"break=everything-here",
		"k=short",
		"https://example.com/page?sort=newest-first",
		"100%23k",
	} {
		if out := scrubKeyFragments(s); out != s {
			t.Errorf("%q changed to %q", s, out)
		}
	}
}

// Reports written before the scrub, or before it knew a spelling, lose the key when the server starts.
func TestJanitorScrubsOlderReports(t *testing.T) {
	a, d, _ := newAppDB(t)
	old := &db.Report{NoteID: 1, Token: "tok", Reason: "abuse", Note: "see https://note.hel.ink/x%23k=" + scrubFakeKey + " please", IPHash: "h"}
	clean := &db.Report{NoteID: 1, Token: "tok", Reason: "spam", Note: "nothing secret here", IPHash: "h"}
	for _, r := range []*db.Report{old, clean} {
		if err := d.AddReport(r); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.Janitor(ctx)

	got, err := d.Report(old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Note, scrubFakeKey) || !strings.HasPrefix(got.Note, "see https://note.hel.ink/x") || !strings.HasSuffix(got.Note, " please") {
		t.Errorf("older report: %q", got.Note)
	}
	if got.Reason != "abuse" || got.Status != "open" || got.Created != old.Created {
		t.Errorf("the rest of the report changed: %+v", got)
	}
	if got, _ := d.Report(clean.ID); got.Note != clean.Note {
		t.Errorf("a clean report changed: %q", got.Note)
	}
}
