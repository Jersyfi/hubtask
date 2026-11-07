// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func (s *snapshotStore) EntrySchedules(_ context.Context, after shared.ID, batch int) ([]repository.ScheduleOf, error) {
	return pageAfter(s.schedules, func(r repository.ScheduleOf) string { return r.EntryID.String() }, after.String(), batch), nil
}

// UC-PLN-02 check 2 offline: an entry's scheduling mode and milestone mark travel to a device that
// may read the entry's collection, and never to one that may not.
func TestTheWalkCarriesAnEntrysSchedulingWhereTheEntryIsRead(t *testing.T) {
	pull, f, store := walking(t)
	seen, hidden := idAt('8', 1), idAt('8', 2)
	store.schedules = []repository.ScheduleOf{
		{EntryID: seen, CollectionID: collectionA, Mode: "AUTOMATIC", Milestone: true, Version: 2},
		{EntryID: hidden, CollectionID: collectionB, Mode: "AUTOMATIC", Version: 1},
	}
	f.auth.allowed[collectionB] = false

	records, _ := wholeWalk(t, pull, 100)

	var got []Record
	for _, record := range records {
		if record.Entity == "entry_schedule" {
			got = append(got, record)
		}
	}
	if len(got) != 1 || got[0].EntityID != seen {
		t.Fatalf("schedules delivered %v, want only %s", got, seen)
	}
	payload := got[0].Payload
	if payload["mode"] != "AUTOMATIC" || payload["milestone"] != true || payload["collection_id"] != collectionA.String() {
		t.Errorf("payload = %v, want the mode, the mark and the collection", payload)
	}
}
