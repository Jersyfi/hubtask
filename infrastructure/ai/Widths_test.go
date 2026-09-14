// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package ai

import (
	"testing"
	"time"
)

// The pool remembers a width per endpoint and model, refuses to learn nothing, keeps the first
// answer it was given, and can say how many of what it knows exceed the index (#569).
func TestTheWidthPoolRemembersPerEndpointAndModel(t *testing.T) {
	pool := &WidthPool{}
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)

	if pool.Known("http://a", "m") != 0 {
		t.Fatal("an empty pool knows something")
	}
	pool.Record("http://a", "m", 0, now)
	if pool.Known("http://a", "m") != 0 {
		t.Fatal("zero was recorded as a width")
	}
	pool.Record("http://a", "m", 768, now)
	pool.Record("http://a", "m", 1024, now.Add(time.Hour))
	if pool.Known("http://a", "m") != 768 {
		t.Errorf("the pool answers %d, want the first answer kept", pool.Known("http://a", "m"))
	}
	if pool.Known("http://b", "m") != 0 || pool.Known("http://a", "n") != 0 {
		t.Error("a width leaked across endpoints or models")
	}

	pool.Record("http://b", "wide", 3072, now.Add(-time.Hour))
	count, since := pool.Wider(1536, now)
	if count != 1 || !since.Equal(now.Add(-time.Hour)) {
		t.Errorf("%d wider than the index since %v", count, since)
	}

	var none *WidthPool
	none.Record("x", "y", 1, now)
	if none.Known("x", "y") != 0 {
		t.Error("a nil pool learned something")
	}
}

// Bounded like the breaker pool: past the cap it starts again rather than growing with the
// configuration of every tenant that ever embedded.
func TestTheWidthPoolStartsAgainPastItsCap(t *testing.T) {
	pool := &WidthPool{Cap: 2}
	now := time.Now()
	pool.Record("e", "one", 1, now)
	pool.Record("e", "two", 2, now)
	pool.Record("e", "three", 3, now)
	if pool.Known("e", "one") != 0 || pool.Known("e", "three") != 3 {
		t.Errorf("the pool holds one=%d three=%d past its cap", pool.Known("e", "one"), pool.Known("e", "three"))
	}
	// A key the pool already holds is confirmed, not re-recorded: at the cap, every batch used to
	// wipe the pool to one entry, because the wipe came before the look-up.
	pool.Record("e", "two", 2, now)
	pool.Record("e", "three", 3, now)
	if pool.Known("e", "two") != 2 || pool.Known("e", "three") != 3 {
		t.Errorf("a full pool forgot what it held when a known key was recorded again: two=%d three=%d",
			pool.Known("e", "two"), pool.Known("e", "three"))
	}
}

// An entry nobody asks for goes stale and stops counting for the health probe: nothing can tell
// the pool a workspace switched models, so the question stopping is the signal. Confirming - what
// the embedding pass does every pass - keeps it counting; reading it, as the manifest does, does
// not.
func TestAWidthNobodyAsksForStopsCountingAsDegraded(t *testing.T) {
	pool := &WidthPool{StaleAfter: time.Hour}
	learned := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	pool.Record("e", "wide", 3072, learned)

	if count, _ := pool.Wider(1536, learned.Add(30*time.Minute)); count != 1 {
		t.Fatalf("a width confirmed half an hour ago counts %d", count)
	}
	if pool.Known("e", "wide") != 3072 {
		t.Fatal("the width was forgotten")
	}
	if count, _ := pool.Wider(1536, learned.Add(2*time.Hour)); count != 0 {
		t.Errorf("a width nobody has asked for in two hours still counts %d", count)
	}

	// The pass asks again: confirmed, and counting, with `since` still the first learning.
	if got := pool.Confirm("e", "wide", learned.Add(2*time.Hour)); got != 3072 {
		t.Fatalf("confirming answered %d", got)
	}
	count, since := pool.Wider(1536, learned.Add(150*time.Minute))
	if count != 1 || !since.Equal(learned) {
		t.Errorf("after confirming: %d since %v, want one since first learned", count, since)
	}
	// Reading does not confirm.
	pool.Known("e", "wide")
	if count, _ := pool.Wider(1536, learned.Add(4*time.Hour)); count != 0 {
		t.Errorf("a read kept an entry alive: %d", count)
	}
	if pool.Confirm("e", "unknown", learned) != 0 {
		t.Error("confirming a key nobody recorded invented one")
	}
}
