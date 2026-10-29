// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"slices"
	"testing"

	whiteboardrepo "github.com/Jersyfi/hubtask/core/application/repository/whiteboard"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
)

// boardsPointing are whiteboards, some of which draw every entry asked about.
type boardsPointing struct {
	boards   []whiteboardrepo.Summary
	pointing []whiteboardrepo.Pointing
}

func (b boardsPointing) Summaries(_ context.Context, ids []shared.ID) ([]whiteboardrepo.Summary, error) {
	var out []whiteboardrepo.Summary
	for _, s := range b.boards {
		if slices.Contains(ids, s.Board.ID) {
			out = append(out, s)
		}
	}
	return out, nil
}

func (b boardsPointing) Referencing(_ context.Context, kind string, _ shared.ID, _ int) ([]whiteboardrepo.Pointing, error) {
	if kind != string(reference.KindEntry) {
		return nil, nil
	}
	return b.pointing, nil
}

// P-19 and UC-CNV-02 check 3: a whiteboard resolves to its title for a reader of its hub, and an
// entry names the boards it is drawn on as a card - only those in a hub the reader may read
// (P-05); a board out of reach answers its address alone.
func TestAWhiteboardResolvesAndNamesTheEntriesItDraws(t *testing.T) {
	ctx := context.Background()
	h := newLinkHarness()
	item := h.one.ID
	open := shared.MustParseID("0192f000-0000-7000-8000-0000000006a1")
	closed := shared.MustParseID("0192f000-0000-7000-8000-0000000006a2")
	retro := shared.MustParseID("0192f000-0000-7000-8000-0000000006b1")
	secret := shared.MustParseID("0192f000-0000-7000-8000-0000000006b2")
	h.reader.hidden[closed] = true
	linking := h.linking()
	linking.Boards = boardsPointing{
		boards: []whiteboardrepo.Summary{
			{Board: whiteboard.Board{ID: retro, HubID: open, Title: "Retro"}, Elements: 12},
			{Board: whiteboard.Board{ID: secret, HubID: closed, Title: "Secret"}, Elements: 3},
		},
		pointing: []whiteboardrepo.Pointing{
			{ID: retro, HubID: open, Title: "Retro", Strength: "MENTION"},
			{ID: retro, HubID: open, Title: "Retro", Strength: "CARD"},
			{ID: secret, HubID: closed, Title: "Secret", Strength: "CARD"},
		},
	}
	resolved, err := ResolveReferences{Linking: linking}.Execute(ctx, linkActor(), []reference.Reference{
		{Kind: reference.KindWhiteboard, ID: retro, Anchor: "frame-1"}, {Kind: reference.KindWhiteboard, ID: secret},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved[0]["available"] != true || resolved[0]["title"] != "Retro" || resolved[0]["element_count"] != 12 ||
		resolved[0]["anchor"] != "frame-1" || resolved[0]["hub_id"] != open.String() {
		t.Errorf("a readable board resolves to %+v", resolved[0])
	}
	if resolved[1]["available"] != false || len(resolved[1]) != 4 {
		t.Errorf("a board out of reach resolves to %+v", resolved[1])
	}
	links, err := ListEntryLinks{Linking: linking}.Execute(ctx, linkActor(), item)
	if err != nil {
		t.Fatal(err)
	}
	if len(links.BoardBacklinks) != 1 || links.BoardBacklinks[0].ID != retro || links.BoardBacklinks[0].Strength != reference.Card {
		t.Fatalf("board backlinks %+v", links.BoardBacklinks)
	}
	out := links.Output(item)["whiteboard_backlinks"].([]usecase.Output)
	if board := out[0]["whiteboard"].(usecase.Output); board["title"] != "Retro" || board["available"] != true {
		t.Errorf("a board backlink answers %+v", out[0])
	}
}
