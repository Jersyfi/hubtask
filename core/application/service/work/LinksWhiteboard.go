// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	whiteboardrepo "github.com/Jersyfi/hubtask/core/application/repository/whiteboard"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	whiteboarddomain "github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// BoardSummaries answers the whiteboards among identifiers that are not trashed, and the boards
// that point at an object - of which an entry's links keep those drawing it (M4).
type BoardSummaries interface {
	Summaries(ctx context.Context, ids []shared.ID) ([]whiteboardrepo.Summary, error)
	Referencing(ctx context.Context, targetKind string, targetID shared.ID, limit int) ([]whiteboardrepo.Pointing, error)
}

// BoardBacklink is a whiteboard that points at an entry - its card or a link in its words - and
// how strongly.
type BoardBacklink struct {
	ID       shared.ID
	HubID    shared.ID
	Title    string
	Strength reference.Strength
}

// visibleBoards answers the boards among ids the reader may read: READ at the board's hub, the
// whiteboard module's rule.
func (l Linking) visibleBoards(
	ctx context.Context, actor appshared.ActorContext, ids []shared.ID, action audit.Action,
) (map[shared.ID]whiteboardrepo.Summary, error) {
	visible := map[shared.ID]whiteboardrepo.Summary{}
	if len(ids) == 0 || l.Boards == nil {
		return visible, nil
	}
	var summaries []whiteboardrepo.Summary
	err := l.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		read, err := l.Boards.Summaries(ctx, ids)
		summaries = read
		return err
	})
	if err != nil || len(summaries) == 0 {
		return visible, err
	}
	allowed, err := l.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead, Action: action, TokenScope: itemsRead, TargetType: "whiteboard",
	}, boardPaths(summaries))
	if err != nil {
		return nil, err
	}
	for i, s := range summaries {
		if i < len(allowed) && allowed[i] {
			visible[s.Board.ID] = s
		}
	}
	return visible, nil
}

func boardPaths(summaries []whiteboardrepo.Summary) [][]identity.Scope {
	paths := make([][]identity.Scope, 0, len(summaries))
	for _, s := range summaries {
		paths = append(paths, []identity.Scope{identity.TenantScope(), identity.HubScope(s.Board.HubID)})
	}
	return paths
}

// boardBacklinks answers the boards that draw or mention the entry and that the reader may read;
// a board in a hub out of reach is left out, as a page out of reach is (P-05).
func (h ListEntryLinks) boardBacklinks(ctx context.Context, actor appshared.ActorContext, itemID shared.ID) ([]BoardBacklink, error) {
	if h.Linking.Boards == nil {
		return nil, nil
	}
	var pointing []whiteboardrepo.Pointing
	err := h.Linking.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		pointing, err = h.Linking.Boards.Referencing(ctx, string(reference.KindEntry), itemID, maxBacklinks)
		return err
	})
	if err != nil || len(pointing) == 0 {
		return nil, err
	}
	strengths := map[shared.ID]reference.Strength{}
	var summaries []whiteboardrepo.Summary
	for _, row := range pointing {
		if _, seen := strengths[row.ID]; !seen {
			summaries = append(summaries, whiteboardrepo.Summary{Board: boardOf(row)})
		}
		if strength := reference.Strength(row.Strength); rank(strength) >= rank(strengths[row.ID]) {
			strengths[row.ID] = strength
		}
	}
	allowed, err := h.Linking.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead, Action: ItemReadAction, TokenScope: itemsRead, TargetType: "whiteboard",
	}, boardPaths(summaries))
	if err != nil {
		return nil, err
	}
	out := make([]BoardBacklink, 0, len(summaries))
	for i, s := range summaries {
		if i < len(allowed) && allowed[i] {
			out = append(out, BoardBacklink{ID: s.Board.ID, HubID: s.Board.HubID, Title: s.Board.Title, Strength: strengths[s.Board.ID]})
		}
	}
	return out, nil
}

// boardTarget is a readable board as a reference shows it (schema ReferenceTarget).
func boardTarget(id, hubID shared.ID, title string, elements *int) usecase.Output {
	out := usecase.Output{
		"kind": string(reference.KindWhiteboard), "id": id.String(), "anchor": nil,
		"available": true, "title": title, "hub_id": hubID.String(),
	}
	if elements != nil {
		out["element_count"] = *elements
	}
	return out
}

func boardOf(row whiteboardrepo.Pointing) whiteboarddomain.Board {
	return whiteboarddomain.Board{ID: row.ID, HubID: row.HubID, Title: row.Title}
}
