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

// UC-WRK-18 check 9: a file or comment on a board is found only in the hubs the reader reads, on
// the board it is on; a token without the scope finds none.
func TestWhatIsOnABoardIsFoundInReadableHubsAlone(t *testing.T) {
	board := shared.ID("0193a0c0-0000-7000-8000-0000000000b9")
	seen, hidden := st.Row("0193a0c0-0000-7000-8000-0000000000b3", st.HubA, "Plan A"), st.Row("0193a0c0-0000-7000-8000-0000000000b4", st.HubB, "Plan B")
	seen.OnID, seen.Text = board, "move the launch"
	for _, comments := range []bool{false, true} {
		finder := OnBoards{
			Studio:   Studio{UnitOfWork: st.UnitOfWork{}, Reader: st.HubReader{Hubs: map[shared.ID]bool{st.HubA: true}}},
			Names:    st.Names{Rows: []searchrepo.Row{hidden, seen}},
			Comments: comments,
		}
		found, err := finder.Find(t.Context(), st.Actor(boardsRead), search.Question{Terms: []string{"launch"}, Size: 5})
		if err != nil {
			t.Fatal(err)
		}
		if found.Count != 1 || found.Hits[0].ID != seen.ID || found.Hits[0].OnKind != search.KindWhiteboard || found.Hits[0].OnID != board {
			t.Fatalf("found %+v, want the one on the readable hub's board", found)
		}
		if comments != (found.Hits[0].Snippet == "move the «launch»") {
			t.Errorf("snippet %q", found.Hits[0].Snippet)
		}
		if none, err := finder.Find(t.Context(), st.Actor(), search.Question{Terms: []string{"launch"}, Size: 5}); err != nil || none.Count != 0 {
			t.Fatalf("a token without the scope found %+v, %v", none, err)
		}
	}
}
