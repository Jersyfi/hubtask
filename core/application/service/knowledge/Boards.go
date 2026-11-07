// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"

	boardrepo "github.com/Jersyfi/hubtask/core/application/repository/whiteboard"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
)

// BoardPlace is a whiteboard standing among a parent's pages in the content tree (UC-CNV-10).
type BoardPlace = boardrepo.Sibling

// BoardPlaces is the whiteboard module as the pages see it: the boards share their siblings' key
// space, are no parents, and are lifted when the page they stand under is trashed. A nil one is a
// space without boards.
type BoardPlaces interface {
	// IsBoard answers whether the identifier names a board outside the trash.
	IsBoard(ctx context.Context, id shared.ID) (bool, error)
	// Siblings answers the keyed boards directly under parent (zero: the top) of a hub, in order.
	Siblings(ctx context.Context, hubID, parent shared.ID) ([]BoardPlace, error)
	// LiftFrom lifts the boards directly under any of the pages to parent, between previous and
	// next, inside the caller's transaction.
	LiftFrom(ctx context.Context, actor appshared.ActorContext, pages []shared.ID, parent shared.ID, previous, next string) error
}

// refuseBoardParent answers whiteboards.no_children when parent names a board.
func (s Space) refuseBoardParent(ctx context.Context, parent shared.ID) error {
	if s.Boards == nil || parent.IsZero() {
		return nil
	}
	board, err := s.Boards.IsBoard(ctx, parent)
	if err != nil {
		return err
	}
	if board {
		return shared.ErrValidation.WithDetail("whiteboards.no_children").
			WithFields(shared.FieldError{Path: "/parent_id", Code: "whiteboards.no_children"})
	}
	return nil
}

// boardSiblings answers the boards under parent as pages of the key space, for placeAfter.
func (s Space) boardSiblings(ctx context.Context, hubID, parent shared.ID) ([]domain.Page, error) {
	if s.Boards == nil {
		return nil, nil
	}
	boards, err := s.Boards.Siblings(ctx, hubID, parent)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Page, 0, len(boards))
	for _, board := range boards {
		out = append(out, domain.Page{ID: board.ID, HubID: hubID, ParentID: parent, OrderKey: board.OrderKey})
	}
	return out, nil
}

// lastKey answers the last key under parent across pages and boards.
func (s Space) lastKey(ctx context.Context, hubID, parent shared.ID, pages string) (string, error) {
	boards, err := s.boardSiblings(ctx, hubID, parent)
	if err != nil {
		return "", err
	}
	last := pages
	for _, board := range boards {
		if board.OrderKey > last {
			last = board.OrderKey
		}
	}
	return last, nil
}

// liftBoards lifts the boards under a page about to be trashed, and under every page beneath it,
// to the page's parent at the page's place: between its key and the next sibling's (UC-CNV-10
// check 4).
func (s Space) liftBoards(ctx context.Context, actor appshared.ActorContext, page domain.Page) error {
	if s.Boards == nil {
		return nil
	}
	all, err := s.Pages.List(ctx, page.HubID, true, false, domain.MaxPagesPerHub)
	if err != nil {
		return err
	}
	subtree := map[shared.ID]bool{page.ID: true}
	for grew := true; grew; {
		grew = false
		for _, p := range all {
			if !subtree[p.ID] && subtree[p.ParentID] {
				subtree[p.ID], grew = true, true
			}
		}
	}
	ids := make([]shared.ID, 0, len(subtree))
	for id := range subtree {
		ids = append(ids, id)
	}
	boards, err := s.boardSiblings(ctx, page.HubID, page.ParentID)
	if err != nil {
		return err
	}
	next := ""
	for _, sibling := range append(all, boards...) {
		if sibling.ParentID != page.ParentID || sibling.ID == page.ID || sibling.OrderKey <= page.OrderKey {
			continue
		}
		if next == "" || sibling.OrderKey < next {
			next = sibling.OrderKey
		}
	}
	return s.Boards.LiftFrom(ctx, actor, ids, page.ParentID, page.OrderKey, next)
}
