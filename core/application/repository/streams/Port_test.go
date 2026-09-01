// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package streams_test

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/repository/streams"
)

// The closed set and its bounds, pinned: the two SECURITY DEFINER functions validate against
// exactly these names (migration 0068), and the drop cutoff's floor comes from the retention
// catalogue - a stream without a period there never loses a month.
func TestTheStreamsAndTheirFloorsComeFromTheCatalogue(t *testing.T) {
	want := map[string]int{
		"activity_entry": 0,  // no period in the catalogue: no month of it ever falls
		"outbox_event":   7,  // the catalogue's shortest default
		"rule_run":       30, // the run log's month
	}

	tables := streams.Tables()
	if len(tables) != len(want) {
		t.Fatalf("the closed set is %v", tables)
	}
	for _, table := range tables {
		days, held := want[table]
		if !held {
			t.Errorf("%s is not a stream the functions accept - change both together", table)
			continue
		}
		if got := streams.DefaultDays(table); got != days {
			t.Errorf("%s's floor is %d, the catalogue says %d - the drop cutoff would drift",
				table, got, days)
		}
	}

	if streams.DefaultDays("tenant") != 0 {
		t.Error("an unknown table answered a floor")
	}
}
