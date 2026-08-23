// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package clock

import "testing"

func TestEveryDrawStaysInsideTheBound(t *testing.T) {
	source := CryptoRandom{}

	seen := make(map[int]bool)
	for range 1000 {
		value := source.IntN(5)
		if value < 0 || value >= 5 {
			t.Fatalf("IntN(5) = %d, outside [0, 5)", value)
		}
		seen[value] = true
	}

	// Uniformity is the standard library's promise, not something a unit test can prove - but a
	// source that never leaves one value is broken in a way 1000 draws will show.
	if len(seen) < 2 {
		t.Fatalf("1000 draws produced only %v - the source is not drawing", seen)
	}
}

func TestABoundOfOneHasOneAnswer(t *testing.T) {
	source := CryptoRandom{}

	for range 10 {
		if value := source.IntN(1); value != 0 {
			t.Fatalf("IntN(1) = %d, want 0", value)
		}
	}
}
