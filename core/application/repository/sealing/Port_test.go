// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package sealing_test

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/repository/sealing"
)

// The census the operator reads is the sum over workspaces read one at a time, and a key one
// workspace names and another does not must count once, not vanish and not double.
func TestTheInstallationsCensusIsTheSumOfTheWorkspaces(t *testing.T) {
	total := sealing.Sum(
		map[string]int64{"k1": 3, "gone": 1},
		map[string]int64{"k1": 2, "k2": 5},
		nil,
	)
	want := map[string]int64{"k1": 5, "k2": 5, "gone": 1}
	if len(total) != len(want) {
		t.Fatalf("summed %v, want %v", total, want)
	}
	for keyID, count := range want {
		if total[keyID] != count {
			t.Errorf("%s: %d, want %d", keyID, total[keyID], count)
		}
	}
	if empty := sealing.Sum(); empty == nil || len(empty) != 0 {
		t.Errorf("no workspaces summed to %v, want an empty census", empty)
	}
}
