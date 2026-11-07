// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package whiteboard

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/whiteboard"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	MoveWhiteboardName = "MoveWhiteboard"

	// BoardMovedAction is the audit code of a board's new place in its hub's content tree.
	BoardMovedAction audit.Action = "whiteboard.moved"
)

// checkParent judges a parent named for a board of hubID: a page of the same hub outside the
// trash, never a board (UC-CNV-10 check 3). Zero is the top of the space.
func (s Studio) checkParent(ctx context.Context, hubID, parent shared.ID) error {
	if parent.IsZero() || s.Contents == nil {
		return nil
	}
	pageHub, err := s.Contents.ParentHub(ctx, parent)
	if err == nil {
		return domain.CheckParent(hubID, domain.ParentCandidate{HubID: pageHub})
	}
	if !errors.Is(err, shared.ErrNotFound) {
		return err
	}
	if board, err := s.Boards.Find(ctx, parent); err == nil && !board.IsTrashed() {
		return domain.CheckParent(hubID, domain.ParentCandidate{IsBoard: true, HubID: board.HubID})
	}
	return shared.ErrNotFound.WithDetail("whiteboards.parent_not_found").
		WithFields(shared.FieldError{Path: "/parent_page_id", Code: "whiteboards.parent_not_found"})
}

// keyAt answers the key a board takes under parent: after the sibling after names - a page or a
// board - or, with none named, first or (last true) after every sibling. moving is left out of
// the siblings, so a board moved among its own keeps no stale neighbour.
func (s Studio) keyAt(ctx context.Context, hubID, parent, moving, after shared.ID, last bool) (string, error) {
	if s.Contents == nil {
		return service.OrderKeyBetween("", "")
	}
	all, err := s.Contents.Siblings(ctx, hubID, parent)
	if err != nil {
		return "", err
	}
	siblings := make([]repository.Sibling, 0, len(all))
	for _, sibling := range all {
		if sibling.ID != moving {
			siblings = append(siblings, sibling)
		}
	}
	return KeyAfter(siblings, after, last)
}

// KeyAfter answers the order key between the sibling after names and the one following it; with
// none named, before the first or (last true) after the last. A name that is no sibling is
// refused whiteboards.after_not_sibling.
func KeyAfter(siblings []repository.Sibling, after shared.ID, last bool) (string, error) {
	if after.IsZero() {
		if len(siblings) == 0 {
			return service.OrderKeyBetween("", "")
		}
		if last {
			return service.OrderKeyBetween(siblings[len(siblings)-1].OrderKey, "")
		}
		return service.OrderKeyBetween("", siblings[0].OrderKey)
	}
	for i, sibling := range siblings {
		if sibling.ID != after {
			continue
		}
		next := ""
		if i+1 < len(siblings) {
			next = siblings[i+1].OrderKey
		}
		return service.OrderKeyBetween(sibling.OrderKey, next)
	}
	return "", shared.ErrValidation.WithDetail("whiteboards.after_not_sibling").
		WithFields(shared.FieldError{Path: "/after_id", Code: "whiteboards.after_not_sibling"})
}

// MoveWhiteboardCommand is the input, typed.
type MoveWhiteboardCommand struct {
	WhiteboardID shared.ID
	// ParentPageID is the page it moves under; zero for the top of the space.
	ParentPageID shared.ID
	// AfterID is the sibling page or board it follows; zero places it first.
	AfterID         shared.ID
	ExpectedVersion int
}

// MoveWhiteboard places a board under a page of its hub or at the top, among the pages and boards
// there (UC-CNV-10 check 2).
type MoveWhiteboard struct{ Studio Studio }

// Execute moves the board.
func (h MoveWhiteboard) Execute(ctx context.Context, actor appshared.ActorContext, cmd MoveWhiteboardCommand) (domain.Board, error) {
	place, err := h.Studio.placeOf(ctx, actor, cmd.WhiteboardID, false)
	if err != nil {
		return domain.Board{}, err
	}
	if err := h.Studio.authorizeWrite(ctx, actor, place.HubID, place.ID, BoardMovedAction); err != nil {
		return domain.Board{}, err
	}
	var board domain.Board
	err = h.Studio.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := h.Studio.find(ctx, cmd.WhiteboardID, true, false)
		if err != nil {
			return err
		}
		if cmd.ExpectedVersion != 0 && cmd.ExpectedVersion != current.Version {
			return shared.ErrVersionConflict.WithDetail("whiteboards.version_conflict")
		}
		if err := h.Studio.checkParent(ctx, current.HubID, cmd.ParentPageID); err != nil {
			return err
		}
		key, err := h.Studio.keyAt(ctx, current.HubID, cmd.ParentPageID, current.ID, cmd.AfterID, false)
		if err != nil {
			return err
		}
		next := current
		next.ParentPageID, next.OrderKey = cmd.ParentPageID, key
		next.UpdatedBy, next.UpdatedAt = actor.AccountID, h.Studio.Clock.Now()
		if err := h.Studio.Boards.Update(ctx, next, current.Version); err != nil {
			return err
		}
		next.Version = current.Version + 1
		board = next
		if err := h.Studio.announce(ctx, actor, next); err != nil {
			return err
		}
		return h.Studio.record(ctx, actor, BoardMovedAction, next, []string{domain.FieldPlace})
	})
	return board, err
}

// Descriptor is the catalogue entry.
func (h MoveWhiteboard) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: MoveWhiteboardName, Permission: service.PermissionWriteItems,
		Summary: "Places a whiteboard under a page of its hub or at the top of the space, after a sibling " +
			"page or board or first. A page of another hub is refused (whiteboards.parent_other_space), " +
			"a board as the parent too (whiteboards.no_children). The board's access stays its hub's.",
		SideEffects: "Writes the board's place and an audit entry.",
		TokenScope:  boardsWrite,
		Input: []usecase.Field{
			{Name: "whiteboard_id", Kind: usecase.KindID, Required: true, Description: "The board to move."},
			{Name: "parent_page_id", Kind: usecase.KindID, Description: "The page it moves under; empty for the top of the space."},
			{Name: "after_id", Kind: usecase.KindID, Description: "The sibling page or board it follows; empty to place it first."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read."},
		},
		Audit: usecase.AuditDeclaration{Action: BoardMovedAction, TargetType: boardTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			ids := map[string]shared.ID{}
			for _, field := range []string{"whiteboard_id", "parent_page_id", "after_id"} {
				id, err := in.ID(field)
				if err != nil {
					return nil, err
				}
				ids[field] = id
			}
			board, err := h.Execute(ctx, actor, MoveWhiteboardCommand{
				WhiteboardID: ids["whiteboard_id"], ParentPageID: ids["parent_page_id"], AfterID: ids["after_id"],
				ExpectedVersion: in.Int("expected_version"),
			})
			if err != nil {
				return nil, err
			}
			return BoardOutput(board, len(board.Scene.Elements), false), nil
		}),
	})
}

// LiftFrom lifts the boards standing directly under any of the pages to parent, between the keys
// previous and next, in their order - a page's trash does it to the boards under the pages it
// trashes (UC-CNV-10 check 4). Inside the caller's transaction; the caller asked the permission.
func (s Studio) LiftFrom(
	ctx context.Context, actor appshared.ActorContext, pages []shared.ID, parent shared.ID, previous, next string,
) error {
	if s.Contents == nil || len(pages) == 0 {
		return nil
	}
	boards, err := s.Contents.Under(ctx, pages)
	if err != nil {
		return err
	}
	for _, id := range boards {
		key, err := service.OrderKeyBetween(previous, next)
		if err != nil {
			return err
		}
		current, err := s.find(ctx, id, true, false)
		if err != nil {
			return err
		}
		lifted := current
		lifted.ParentPageID, lifted.OrderKey = parent, key
		lifted.UpdatedBy, lifted.UpdatedAt = actor.AccountID, s.Clock.Now()
		if err := s.Boards.Update(ctx, lifted, current.Version); err != nil {
			return err
		}
		lifted.Version = current.Version + 1
		if err := s.announce(ctx, actor, lifted); err != nil {
			return err
		}
		if err := s.record(ctx, actor, BoardMovedAction, lifted, []string{domain.FieldPlace}); err != nil {
			return err
		}
		previous = key
	}
	return nil
}

// PageSide is the board module as the pages see it (knowledge.BoardPlaces), wired by the
// composition root so that neither module imports the other.
type PageSide struct{ Studio Studio }

// IsBoard answers whether the identifier names a board outside the trash.
func (p PageSide) IsBoard(ctx context.Context, id shared.ID) (bool, error) {
	board, err := p.Studio.Boards.Find(ctx, id)
	if errors.Is(err, shared.ErrNotFound) {
		return false, nil
	}
	return err == nil && !board.IsTrashed(), err
}

// Siblings answers the keyed boards directly under parent, in order.
func (p PageSide) Siblings(ctx context.Context, hubID, parent shared.ID) ([]repository.Sibling, error) {
	if p.Studio.Contents == nil {
		return nil, nil
	}
	all, err := p.Studio.Contents.Siblings(ctx, hubID, parent)
	if err != nil {
		return nil, err
	}
	boards := make([]repository.Sibling, 0, len(all))
	for _, sibling := range all {
		if sibling.IsBoard {
			boards = append(boards, sibling)
		}
	}
	return boards, nil
}

// LiftFrom is Studio.LiftFrom.
func (p PageSide) LiftFrom(ctx context.Context, actor appshared.ActorContext, pages []shared.ID, parent shared.ID, previous, next string) error {
	return p.Studio.LiftFrom(ctx, actor, pages, parent, previous, next)
}
