// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func (s *snapshotStore) Sprints(_ context.Context, after shared.ID, batch int) ([]planning.Sprint, error) {
	return pageAfter(s.sprints, func(r planning.Sprint) string { return r.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) SprintEntries(_ context.Context, after shared.ID, batch int) ([]repository.SprintEntryOf, error) {
	return pageAfter(s.members, func(r repository.SprintEntryOf) string { return r.EntryID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) EntryEstimates(_ context.Context, after shared.ID, batch int) ([]repository.EstimateOf, error) {
	return pageAfter(s.estimates, func(r repository.EstimateOf) string { return r.Estimate.EntryID.String() }, after.String(), batch), nil
}

// UC-PLN-06 and UC-PLN-07 offline: a sprint, an entry's open sprint and its estimate travel to a
// device that may read the collection, and never to one that may not.
func TestTheWalkCarriesSprintsAndEstimatesWhereTheCollectionIsRead(t *testing.T) {
	pull, f, store := walking(t)
	sprint, hiddenSprint := idAt('9', 1), idAt('9', 2)
	seen, hidden := idAt('9', 3), idAt('9', 4)
	store.sprints = []planning.Sprint{
		{ID: sprint, CollectionID: collectionA, Name: "Sprint 1", State: planning.SprintActive, Unit: planning.Points, Version: 2},
		{ID: hiddenSprint, CollectionID: collectionB, Name: "Elsewhere", State: planning.SprintPlanned, Unit: planning.Points},
	}
	store.members = []repository.SprintEntryOf{
		{EntryID: seen, SprintID: sprint, CollectionID: collectionA, Committed: true},
		{EntryID: hidden, SprintID: hiddenSprint, CollectionID: collectionB},
	}
	store.estimates = []repository.EstimateOf{
		{Estimate: planning.Estimate{EntryID: seen, Unit: planning.Size, Value: 5, Size: "L"}, CollectionID: collectionA},
		{Estimate: planning.Estimate{EntryID: hidden, Unit: planning.Points, Value: 2}, CollectionID: collectionB},
	}
	f.auth.allowed[collectionB] = false

	records, _ := wholeWalk(t, pull, 100)

	got := map[string][]Record{}
	for _, record := range records {
		switch record.Entity {
		case "sprint", "sprint_entry", "entry_estimate":
			got[record.Entity] = append(got[record.Entity], record)
		}
	}
	if s := got["sprint"]; len(s) != 1 || s[0].EntityID != sprint || s[0].Payload["state"] != "ACTIVE" {
		t.Errorf("sprints delivered %v, want only the readable one", s)
	}
	if m := got["sprint_entry"]; len(m) != 1 || m[0].EntityID != seen || m[0].Payload["sprint_id"] != sprint.String() || m[0].Payload["committed"] != true {
		t.Errorf("memberships delivered %v, want the readable entry's", m)
	}
	if e := got["entry_estimate"]; len(e) != 1 || e[0].EntityID != seen || e[0].Payload["size"] != "L" || e[0].Payload["value"] != 5.0 {
		t.Errorf("estimates delivered %v, want the readable entry's", e)
	}
}
