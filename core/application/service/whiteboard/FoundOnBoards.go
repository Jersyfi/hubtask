// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package whiteboard

import (
	"context"

	searchrepo "github.com/Jersyfi/hubtask/core/application/repository/search"
	"github.com/Jersyfi/hubtask/core/application/service/search"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// OnBoards finds what is on a board - its files by their names, its comments by their words - on
// the boards of the hubs the reader may read, the question ListWhiteboardComments asks of one
// (UC-WRK-18 check 9). A file's contents are not searched.
type OnBoards struct {
	Studio Studio
	Names  searchrepo.Names
	// Comments finds comments; otherwise files.
	Comments bool
}

// Find narrows the candidates to the readable hubs.
func (f OnBoards) Find(ctx context.Context, actor appshared.ActorContext, q search.Question) (search.Found, error) {
	if actor.RequireScope(boardsRead) != nil {
		return search.Found{}, nil
	}
	read := f.Names.WhiteboardFiles
	if f.Comments {
		read = f.Names.WhiteboardComments
	}
	rows, err := search.Within(ctx, f.Studio.UnitOfWork, actor, read, q.Ask())
	if err != nil || len(rows) == 0 {
		return search.Found{}, err
	}
	hubs := make([]shared.ID, len(rows))
	for i, row := range rows {
		hubs[i] = row.HubID
	}
	readable, err := f.Studio.readableHubs(ctx, actor, hubs)
	if err != nil {
		return search.Found{}, err
	}
	allowed := make([]bool, len(rows))
	for i, row := range rows {
		allowed[i] = readable[row.HubID]
	}
	return search.Narrowed(rows, allowed, q.Size, func(row searchrepo.Row) search.Hit {
		hit := search.Hit{
			ID: row.ID, HubID: row.HubID, Title: row.Title, Tile: row.Tile, UpdatedAt: row.UpdatedAt,
			OnKind: search.KindWhiteboard, OnID: row.OnID,
		}
		if f.Comments {
			hit.Snippet = search.Snippet(row.Text, q.Terms)
		}
		return hit
	}), nil
}
