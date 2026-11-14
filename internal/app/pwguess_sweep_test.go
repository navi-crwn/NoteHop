package app

import (
	"strconv"
	"testing"
	"time"
)

// 5. Tidying is spread over calls: with very many old entries one call touches only a few notes, and the map still empties.
func TestGuessCountsAreTidiedAFewAtATime(t *testing.T) {
	var g pwGuards
	old := time.Now().Add(-48 * time.Hour)
	for i := 0; i < 5000; i++ {
		g.attempt(int64(i), "198.51.100."+strconv.Itoa(i%250), false, old)
	}
	now := time.Now()
	before := len(g.m)
	g.attempt(999999, "192.0.2.1", false, now)
	if removed := before - len(g.m) + 1; removed > pwSweepPerCall+1 {
		t.Fatalf("one call tidied %d notes, the limit is %d", removed, pwSweepPerCall)
	}
	for i := 0; i < 2000 && len(g.m) > 1; i++ {
		g.attempt(999999, "192.0.2.1", false, now)
	}
	if len(g.m) > 1 {
		t.Fatalf("%d old entries are still kept", len(g.m))
	}
	if g.m[999999] == nil || g.m[999999].hourN == 0 {
		t.Fatal("the live note's count was lost")
	}
}
