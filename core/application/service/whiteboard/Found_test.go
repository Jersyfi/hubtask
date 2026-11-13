// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package whiteboard

import (
	"testing"

	searchrepo "github.com/Jersyfi/hubtask/core/application/repository/search"
	"github.com/Jersyfi/hubtask/core/application/service/search"
	st "github.com/Jersyfi/hubtask/core/application/service/search/searchtest"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A search finds the boards of the hubs the reader reads and counts only those; a token without
// the scope finds none rather than being refused (UC-WRK-18 check 9, Q-5).
func TestASearchFindsTheBoardsOfReadableHubsAlone(t *testing.T) {
	seen, hidden := shared.ID("0193a0c0-0000-7000-8000-0000000000b1"), shared.ID("0193a0c0-0000-7000-8000-0000000000b2")
	finder := Finder{
		Studio: Studio{UnitOfWork: st.UnitOfWork{}, Reader: st.HubReader{Hubs: map[shared.ID]bool{st.HubA: true}}},
		Names:  st.Names{Rows: []searchrepo.Row{st.Row(hidden, st.HubB, "Plan B"), st.Row(seen, st.HubA, "Plan A")}},
	}
	found, err := finder.Find(t.Context(), st.Actor(boardsRead), search.Question{Terms: []string{"plan"}, Size: 5})
	if err != nil {
		t.Fatal(err)
	}
	if found.Count != 1 || len(found.Hits) != 1 || found.Hits[0].ID != seen {
		t.Fatalf("found %+v, want only the board of the readable hub", found)
	}
	none, err := finder.Find(t.Context(), st.Actor(), search.Question{Terms: []string{"plan"}, Size: 5})
	if err != nil || none.Count != 0 {
		t.Fatalf("a token without the scope found %+v, %v", none, err)
	}
}
