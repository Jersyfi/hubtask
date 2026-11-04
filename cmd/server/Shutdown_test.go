// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"testing"
	"time"
)

// A pool whose Close waits for a connection that never comes back - a loop abandoned at the grace
// period, a database gone from the network - must not hold the process after SIGTERM (rule 7).
func TestCloseWithinGivesUpOnACloseThatNeverReturns(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	start := time.Now()
	finished := closeWithin("database", 50*time.Millisecond, func() { <-release })
	if finished {
		t.Fatal("a close that never returned was reported as finished")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("closeWithin waited %s for a close bounded to 50ms", elapsed)
	}
}

func TestCloseWithinReportsACloseThatReturns(t *testing.T) {
	closed := false
	if !closeWithin("database", time.Second, func() { closed = true }) || !closed {
		t.Fatal("a close that returned was not reported as finished")
	}
}
