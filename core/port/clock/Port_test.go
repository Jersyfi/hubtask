// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package clock

import "testing"

func TestAScriptIsPlayedInOrderAndCycles(t *testing.T) {
	source := NewScripted(3, 0, 2)

	got := []int{source.IntN(5), source.IntN(5), source.IntN(5), source.IntN(5)}
	want := []int{3, 0, 2, 3}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("draw %d = %d, want %d (all draws: %v)", i, got[i], want[i], got)
		}
	}
}

func TestAScriptedValueIsFoldedIntoTheBound(t *testing.T) {
	source := NewScripted(7)

	if value := source.IntN(3); value != 1 {
		t.Fatalf("IntN(3) with a script of 7 = %d, want 7 %% 3 = 1", value)
	}
}
