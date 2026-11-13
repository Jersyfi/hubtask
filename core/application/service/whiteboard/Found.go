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

// Finder finds the boards whose titles hold the words, in the hubs the reader may read - the
// question ReadWhiteboard asks of one (UC-WRK-18 check 9).
type Finder struct {
	Studio Studio
	Names  searchrepo.Names
}

// Find narrows the candidates to the readable hubs.
func (f Finder) Find(ctx context.Context, actor appshared.ActorContext, q search.Question) (search.Found, error) {
	if actor.RequireScope(boardsRead) != nil {
		return search.Found{}, nil
	}
	rows, err := search.Within(ctx, f.Studio.UnitOfWork, actor, f.Names.Whiteboards, q.Ask())
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
		return search.Hit{ID: row.ID, HubID: row.HubID, Title: row.Title, UpdatedAt: row.UpdatedAt}
	}), nil
}
