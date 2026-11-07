// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// boardSide is the whiteboard module as the pages see it: one board under a parent, and what a
// trash lifted.
type boardSide struct {
	board  BoardPlace
	parent shared.ID
	lifted struct {
		pages          []shared.ID
		parent         shared.ID
		previous, next string
	}
}

func (b *boardSide) IsBoard(_ context.Context, id shared.ID) (bool, error) { return id == b.board.ID, nil }

func (b *boardSide) Siblings(_ context.Context, _, parent shared.ID) ([]BoardPlace, error) {
	if parent == b.parent {
		return []BoardPlace{b.board}, nil
	}
	return nil, nil
}

func (b *boardSide) LiftFrom(_ context.Context, _ appshared.ActorContext, pages []shared.ID, parent shared.ID, previous, next string) error {
	b.lifted.pages, b.lifted.parent, b.lifted.previous, b.lifted.next = pages, parent, previous, next
	return nil
}

// UC-CNV-10 checks 2-4 on the pages' side: a page is placed after a sibling board, a board is no
// parent, a new page goes after the boards too, and trashing a page lifts the boards under it and
// under its subpages to its parent, at its place.
func TestPagesAndBoardsShareOneOrder(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	team := f.create(t, anna, hub, "", "Team", "")
	product := f.create(t, anna, hub, "", "Product", "")
	side := &boardSide{board: BoardPlace{ID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e001"), OrderKey: team.OrderKey + "V", IsBoard: true}}
	f.space.Boards = side

	moved, err := (MovePage{Space: f.space}).Execute(ctx, as(anna), MovePageCommand{PageID: product.ID, AfterID: side.board.ID})
	if err != nil || moved.OrderKey <= side.board.OrderKey {
		t.Fatalf("moved %+v, %v; want it after the board", moved, err)
	}
	if _, err := (MovePage{Space: f.space}).Execute(ctx, as(anna), MovePageCommand{PageID: product.ID, ParentID: side.board.ID}); code(err) != "whiteboards.no_children" {
		t.Errorf("under a board: %v", err)
	}
	if _, err := (CreatePage{Space: f.space}).Execute(ctx, as(anna), CreatePageCommand{HubID: hub, ParentID: side.board.ID, Title: "Child"}); code(err) != "whiteboards.no_children" {
		t.Errorf("created under a board: %v", err)
	}

	side.board.OrderKey, side.parent = moved.OrderKey+"V", ""
	last := f.create(t, anna, hub, "", "Last", "")
	if last.OrderKey <= side.board.OrderKey {
		t.Errorf("a new page at %s, want it after the board at %s", last.OrderKey, side.board.OrderKey)
	}

	sub := f.create(t, anna, hub, team.ID, "Retros", "")
	if _, err := (TrashPage{Space: f.space}).Execute(ctx, as(anna), team.ID); err != nil {
		t.Fatalf("trashing: %v", err)
	}
	lifted := side.lifted
	if !lifted.parent.IsZero() || lifted.previous != team.OrderKey || len(lifted.pages) != 2 {
		t.Fatalf("lifted %+v; want the boards under Team and Retros to the top after Team's key", lifted)
	}
	holds := map[shared.ID]bool{}
	for _, id := range lifted.pages {
		holds[id] = true
	}
	if !holds[team.ID] || !holds[sub.ID] || lifted.next == "" || lifted.next <= team.OrderKey {
		t.Errorf("lifted %+v; want Team and its subpage, up to the next sibling", lifted)
	}
}
