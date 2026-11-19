// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"testing"

	searchrepo "github.com/Jersyfi/hubtask/core/application/repository/search"
	"github.com/Jersyfi/hubtask/core/application/service/search"
	st "github.com/Jersyfi/hubtask/core/application/service/search/searchtest"
	domain "github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

func searchSteward(reader st.HubReader) Steward {
	return Steward{UnitOfWork: st.UnitOfWork{}, Reader: reader}
}

// A search finds the goals of the workspace and of the hubs the reader reads, never those of a
// hub they do not (UC-WRK-18 check 9, Q-5).
func TestASearchFindsTheGoalsOfReadablePlacesAlone(t *testing.T) {
	workspace, seen, hidden := shared.ID("0193a0c0-0000-7000-8000-0000000000c0"), shared.ID("0193a0c0-0000-7000-8000-0000000000c1"), shared.ID("0193a0c0-0000-7000-8000-0000000000c2")
	finder := GoalFinder{
		Steward: searchSteward(st.HubReader{Hubs: map[shared.ID]bool{st.HubA: true}, Workspace: true}),
		Names:   st.Names{Rows: []searchrepo.Row{st.Row(workspace, "", "Grow"), st.Row(hidden, st.HubB, "Grow B"), st.Row(seen, st.HubA, "Grow A")}},
	}
	found, err := finder.Find(t.Context(), st.Actor(itemsRead), search.Question{Terms: []string{"grow"}, Size: 5})
	if err != nil {
		t.Fatal(err)
	}
	if found.Count != 2 || found.Hits[0].ID != workspace || found.Hits[1].ID != seen {
		t.Fatalf("found %+v, want the workspace's goal and hub A's", found)
	}
}

// A search finds one's own dashboards and those shared with the readers of a hub one reads; a
// private one of somebody else, or one shared in a hub one does not read, stays hidden.
func TestASearchFindsTheDashboardsTheReaderSees(t *testing.T) {
	other := shared.ID("0193a0c0-0000-7000-8000-0000000000ff")
	own, shared1, private, elsewhere := shared.ID("0193a0c0-0000-7000-8000-0000000000d1"), shared.ID("0193a0c0-0000-7000-8000-0000000000d2"),
		shared.ID("0193a0c0-0000-7000-8000-0000000000d3"), shared.ID("0193a0c0-0000-7000-8000-0000000000d4")
	rows := []searchrepo.Row{
		{ID: own, HubID: st.HubB, OwnerID: st.Account, Sharing: string(domain.SharingPrivate), Title: "Mine"},
		{ID: shared1, HubID: st.HubA, OwnerID: other, Sharing: string(domain.SharingScope), Title: "Team"},
		{ID: private, HubID: st.HubA, OwnerID: other, Sharing: string(domain.SharingPrivate), Title: "Theirs"},
		{ID: elsewhere, HubID: st.HubB, OwnerID: other, Sharing: string(domain.SharingScope), Title: "Elsewhere"},
	}
	finder := DashboardFinder{Steward: searchSteward(st.HubReader{Hubs: map[shared.ID]bool{st.HubA: true}}), Names: st.Names{Rows: rows}}
	found, err := finder.Find(t.Context(), st.Actor(containersRead), search.Question{Terms: []string{"x"}, Size: 5})
	if err != nil {
		t.Fatal(err)
	}
	if found.Count != 2 || found.Hits[0].ID != own || found.Hits[1].ID != shared1 {
		t.Fatalf("found %+v, want one's own and the shared one", found)
	}
}

// The query language spells exactly the states a goal may be in.
func TestTheQueryKnowsEveryGoalState(t *testing.T) {
	for _, state := range view.GoalStates {
		if !domain.Status(state).Valid() {
			t.Errorf("the query spells %s, which no goal is in", state)
		}
	}
	if len(view.GoalStates) != 6 {
		t.Errorf("the query spells %d states", len(view.GoalStates))
	}
}

// A search finds the workspace's portfolios for a reader of the workspace, and none for one who
// does not read it; a portfolio stands in no hub (UC-WRK-18 check 9, Q-5).
func TestASearchFindsThePortfoliosOfAWorkspaceTheReaderReads(t *testing.T) {
	growth := shared.ID("0193a0c0-0000-7000-8000-0000000000e1")
	names := st.Names{Rows: []searchrepo.Row{{ID: growth, Title: "Growth", State: "ON_TRACK"}}}
	reads := PortfolioFinder{Steward: searchSteward(st.HubReader{Workspace: true}), Names: names}
	found, err := reads.Find(t.Context(), st.Actor(itemsRead), search.Question{Terms: []string{"grow"}, Size: 5})
	if err != nil {
		t.Fatal(err)
	}
	if found.Count != 1 || found.Hits[0].ID != growth || found.Hits[0].State != "ON_TRACK" || !found.Hits[0].HubID.IsZero() {
		t.Fatalf("found %+v, want the portfolio", found)
	}
	outsider := PortfolioFinder{Steward: searchSteward(st.HubReader{Hubs: map[shared.ID]bool{st.HubA: true}}), Names: names}
	if found, err := outsider.Find(t.Context(), st.Actor(itemsRead), search.Question{Terms: []string{"grow"}, Size: 5}); err != nil || found.Count != 0 {
		t.Fatalf("a reader outside the workspace found %+v, %v", found, err)
	}
}
