// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"testing"

	searchrepo "github.com/Jersyfi/hubtask/core/application/repository/search"
	"github.com/Jersyfi/hubtask/core/application/service/search"
	st "github.com/Jersyfi/hubtask/core/application/service/search/searchtest"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A search finds the inventory items of the hubs the reader reads, with their class and stage
// (UC-WRK-18 check 9, Q-5).
func TestASearchFindsTheItemsOfReadableHubsAlone(t *testing.T) {
	seen, hidden := shared.ID("0193a0c0-0000-7000-8000-0000000000e1"), shared.ID("0193a0c0-0000-7000-8000-0000000000e2")
	b := st.Row(seen, st.HubA, "Exchange Online Connector")
	b.Tile, b.State = "service", "IN_USE"
	finder := Finder{
		Inventory: Inventory{UnitOfWork: st.UnitOfWork{}},
		Reader:    st.HubReader{Hubs: map[shared.ID]bool{st.HubA: true}},
		Names:     st.Names{Rows: []searchrepo.Row{st.Row(hidden, st.HubB, "Exchange"), b}},
	}
	found, err := finder.Find(t.Context(), st.Actor(cisRead), search.Question{Terms: []string{"exchange"}, Size: 5})
	if err != nil {
		t.Fatal(err)
	}
	if found.Count != 1 || found.Hits[0].ID != seen || found.Hits[0].Tile != "service" || found.Hits[0].State != "IN_USE" {
		t.Fatalf("found %+v, want hub A's item with its class and stage", found)
	}
}
