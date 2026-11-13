// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"testing"

	searchrepo "github.com/Jersyfi/hubtask/core/application/repository/search"
	"github.com/Jersyfi/hubtask/core/application/service/search"
	st "github.com/Jersyfi/hubtask/core/application/service/search/searchtest"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A search finds the releases of the hubs the reader reads, never those of a hub they do not
// (UC-WRK-18 check 9, Q-5).
func TestASearchFindsTheReleasesOfReadableHubsAlone(t *testing.T) {
	seen, hidden := shared.ID("0193a0c0-0000-7000-8000-0000000000f1"), shared.ID("0193a0c0-0000-7000-8000-0000000000f2")
	finder := ReleaseFinder{
		Planner: Planner{UnitOfWork: st.UnitOfWork{}, Reader: st.HubReader{Hubs: map[shared.ID]bool{st.HubA: true}}},
		Names: st.Names{Rows: []searchrepo.Row{
			{ID: hidden, HubID: st.HubB, Title: "Autumn B", State: "PLANNED"},
			{ID: seen, HubID: st.HubA, Title: "Autumn A", State: "RELEASED"},
		}},
	}
	found, err := finder.Find(t.Context(), st.Actor(itemsRead), search.Question{Terms: []string{"autumn"}, Size: 5})
	if err != nil {
		t.Fatal(err)
	}
	if found.Count != 1 || found.Hits[0].ID != seen || found.Hits[0].HubID != st.HubA || found.Hits[0].State != "RELEASED" {
		t.Fatalf("found %+v, want hub A's release", found)
	}
}

// A search finds the sprints of the collections the reader reads, each with its collection and
// hub; a sprint of a collection they do not read stays hidden, though its hub is readable.
func TestASearchFindsTheSprintsOfReadableCollectionsAlone(t *testing.T) {
	seen, hidden := shared.ID("0193a0c0-0000-7000-8000-0000000000f3"), shared.ID("0193a0c0-0000-7000-8000-0000000000f4")
	open, closed := shared.ID("0193a0c0-0000-7000-8000-0000000000c3"), shared.ID("0193a0c0-0000-7000-8000-0000000000c4")
	var asked [][]identity.Scope
	finder := SprintFinder{
		Planner: Planner{UnitOfWork: st.UnitOfWork{}, Reader: st.HubReader{Hubs: map[shared.ID]bool{open: true, st.HubA: true}, Asked: &asked}},
		Names: st.Names{Rows: []searchrepo.Row{
			{ID: hidden, HubID: st.HubA, CollectionID: closed, Title: "Sprint 4", State: "ACTIVE"},
			{ID: seen, HubID: st.HubA, CollectionID: open, Title: "Sprint 4", State: "ACTIVE"},
		}},
	}
	found, err := finder.Find(t.Context(), st.Actor(itemsRead), search.Question{Terms: []string{"sprint"}, Size: 5})
	if err != nil {
		t.Fatal(err)
	}
	if found.Count != 1 || found.Hits[0].ID != seen || found.Hits[0].CollectionID != open || found.Hits[0].HubID != st.HubA {
		t.Fatalf("found %+v, want the open collection's sprint", found)
	}
	if len(asked) != 2 || len(asked[0]) != 3 || asked[0][2] != identity.CollectionScope(closed) {
		t.Errorf("asked %v, want each sprint's collection under its hub", asked)
	}
}
