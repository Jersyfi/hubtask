// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	searchrepo "github.com/Jersyfi/hubtask/core/application/repository/search"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/search"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// ByKey reads the entry a ticket key names, asked of the entry's own path as the entry search
// asks it, so a key jumps only to what the reader may read (UC-WRK-18 check 10, rule 2).
type ByKey struct {
	Reader     Reader
	Names      searchrepo.Names
	UnitOfWork persistence.UnitOfWork
}

var _ search.KeyReader = ByKey{}

// EntryByKey answers the entry as a hit, or nothing where none is named or the reader may not
// read it.
func (b ByKey) EntryByKey(ctx context.Context, actor appshared.ActorContext, key searchrepo.Key, hubID shared.ID) (search.Hit, bool, error) {
	if actor.RequireScope(itemsRead) != nil {
		return search.Hit{}, false, nil
	}
	var rows []searchrepo.Row
	err := b.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		rows, err = b.Names.EntryByKey(ctx, key, hubID)
		return err
	})
	if err != nil || len(rows) == 0 {
		return search.Hit{}, false, err
	}
	row := rows[0]
	path := []identity.Scope{identity.TenantScope()}
	if !row.HubID.IsZero() {
		path = append(path, identity.HubScope(row.HubID))
	}
	path = append(path, identity.CollectionScope(row.CollectionID), identity.ItemScope(row.ID))
	allowed, err := b.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead, Action: ItemReadAction, TokenScope: itemsRead, TargetType: searchTarget,
	}, [][]identity.Scope{path})
	if err != nil || len(allowed) == 0 || !allowed[0] {
		return search.Hit{}, false, err
	}
	return search.Hit{
		ID: row.ID, HubID: row.HubID, CollectionID: row.CollectionID, Title: row.Title, Tile: row.Tile,
		State: row.State, Key: row.Key, UpdatedAt: row.UpdatedAt,
	}, true, nil
}
