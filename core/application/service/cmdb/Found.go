// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"

	searchrepo "github.com/Jersyfi/hubtask/core/application/repository/search"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/search"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// Finder finds the inventory items whose names hold the words, in the hubs the reader may read -
// the question ReadCi asks of one (UC-WRK-18 check 9).
type Finder struct {
	Inventory Inventory
	Reader    Reader
	Names     searchrepo.Names
}

// Find narrows the candidates to the readable hubs.
func (f Finder) Find(ctx context.Context, actor appshared.ActorContext, q search.Question) (search.Found, error) {
	if actor.RequireScope(cisRead) != nil {
		return search.Found{}, nil
	}
	rows, err := search.Within(ctx, f.Inventory.UnitOfWork, actor, f.Names.Cis, q.Ask())
	if err != nil || len(rows) == 0 {
		return search.Found{}, err
	}
	paths := make([][]identity.Scope, len(rows))
	for i, row := range rows {
		paths[i] = hubPath(row.HubID)
	}
	allowed, err := f.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead, Action: CIReadAction, TokenScope: cisRead, TargetType: ciTarget,
	}, paths)
	if err != nil {
		return search.Found{}, err
	}
	return search.Narrowed(rows, allowed, q.Size, func(row searchrepo.Row) search.Hit {
		return search.Hit{ID: row.ID, HubID: row.HubID, Title: row.Title, Tile: row.Tile, State: row.State, UpdatedAt: row.UpdatedAt}
	}), nil
}
