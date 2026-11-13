// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"testing"

	searchrepo "github.com/Jersyfi/hubtask/core/application/repository/search"
	"github.com/Jersyfi/hubtask/core/application/service/search"
	st "github.com/Jersyfi/hubtask/core/application/service/search/searchtest"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A search finds the files and comments on the entries the reader may read - asked of each
// entry's own path - and cuts a comment's passage around the word (UC-WRK-18 check 9, Q-5).
func TestASearchFindsFilesAndCommentsOnReadableEntriesAlone(t *testing.T) {
	readable, other := shared.ID("0193a0c0-0000-7000-8000-000000000101"), shared.ID("0193a0c0-0000-7000-8000-000000000102")
	collection := shared.ID("0193a0c0-0000-7000-8000-000000000103")
	rows := []searchrepo.Row{
		{ID: shared.ID("0193a0c0-0000-7000-8000-000000000111"), HubID: st.HubA, CollectionID: collection, OnID: other, Title: "x", Text: "the invoice"},
		{ID: shared.ID("0193a0c0-0000-7000-8000-000000000112"), HubID: st.HubA, CollectionID: collection, OnID: readable, Title: "Pay", Text: "please send the invoice by Friday"},
	}
	for _, comments := range []bool{false, true} {
		finder := OnEntries{
			Reader: st.HubReader{Entries: map[shared.ID]bool{readable: true}}, Names: st.Names{Rows: rows},
			UnitOfWork: st.UnitOfWork{}, Comments: comments,
		}
		found, err := finder.Find(t.Context(), st.Actor(itemsRead), search.Question{Terms: []string{"invoice"}, Size: 5})
		if err != nil {
			t.Fatal(err)
		}
		if found.Count != 1 || found.Hits[0].OnID != readable || found.Hits[0].OnKind != search.KindEntry {
			t.Fatalf("comments %v: found %+v, want the one on the readable entry", comments, found)
		}
		if comments && found.Hits[0].Snippet != "please send the «invoice» by Friday" {
			t.Errorf("the passage is %q", found.Hits[0].Snippet)
		}
	}
}

// UC-WRK-18 check 11. A ticket key jumps to its entry only where the reader may read that entry; a key nobody
// holds, or a token without items:read, finds nothing.
func TestATicketKeyFindsOnlyAReadableEntry(t *testing.T) {
	readable, hidden := shared.ID("0193a0c0-0000-7000-8000-000000000201"), shared.ID("0193a0c0-0000-7000-8000-000000000202")
	collection := shared.ID("0193a0c0-0000-7000-8000-000000000203")
	names := st.Names{Keys: map[string]searchrepo.Row{
		"sa-12": {ID: readable, HubID: st.HubA, CollectionID: collection, Title: "Printer jams", Tile: "TASK", State: "OPEN", Key: "SA-12"},
		"sa-13": {ID: hidden, HubID: st.HubA, CollectionID: collection, Title: "Salaries", Key: "SA-13"},
	}}
	finder := ByKey{Reader: st.HubReader{Entries: map[shared.ID]bool{readable: true}}, Names: names, UnitOfWork: st.UnitOfWork{}}
	cases := []struct {
		key   searchrepo.Key
		actor string
		want  shared.ID
	}{
		{searchrepo.Key{Word: "SA", Number: 12}, itemsRead, readable},
		{searchrepo.Key{Word: "sa", Number: 13}, itemsRead, ""},
		{searchrepo.Key{Word: "sa", Number: 99}, itemsRead, ""},
		{searchrepo.Key{Word: "sa", Number: 12}, "pages:read", ""},
	}
	for _, c := range cases {
		hit, found, err := finder.EntryByKey(t.Context(), st.Actor(c.actor), c.key, "")
		if err != nil {
			t.Fatal(err)
		}
		if found != (c.want != "") || hit.ID != c.want {
			t.Errorf("%+v as %s found %v %+v, want %q", c.key, c.actor, found, hit, c.want)
		}
		if found && (hit.Key != "SA-12" || hit.Title != "Printer jams" || hit.HubID != st.HubA) {
			t.Errorf("the hit is %+v", hit)
		}
	}
}
