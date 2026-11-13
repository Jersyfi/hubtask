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
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// OnEntries finds what is on an entry - its files by their names, a person's comments by their
// words - on the entries the reader may read, asked of each entry's own path as the entry search
// asks it (SearchItems.visible). A file's contents are not searched (UC-WRK-18 check 9).
type OnEntries struct {
	Reader     Reader
	Names      searchrepo.Names
	UnitOfWork persistence.UnitOfWork
	// Comments finds comments; otherwise files.
	Comments bool
}

// Find narrows the candidates to the readable entries.
func (f OnEntries) Find(ctx context.Context, actor appshared.ActorContext, q search.Question) (search.Found, error) {
	if actor.RequireScope(itemsRead) != nil {
		return search.Found{}, nil
	}
	read := f.Names.EntryFiles
	if f.Comments {
		read = f.Names.EntryComments
	}
	rows, err := search.Within(ctx, f.UnitOfWork, actor, read, q.Ask())
	if err != nil || len(rows) == 0 {
		return search.Found{}, err
	}
	paths := make([][]identity.Scope, len(rows))
	for i, row := range rows {
		path := []identity.Scope{identity.TenantScope()}
		if !row.HubID.IsZero() {
			path = append(path, identity.HubScope(row.HubID))
		}
		paths[i] = append(path, identity.CollectionScope(row.CollectionID), identity.ItemScope(row.OnID))
	}
	action := ItemReadAction
	if f.Comments {
		action = CommentsReadAction
	}
	allowed, err := f.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead, Action: action, TokenScope: itemsRead, TargetType: searchTarget,
	}, paths)
	if err != nil {
		return search.Found{}, err
	}
	return search.Narrowed(rows, allowed, q.Size, func(row searchrepo.Row) search.Hit {
		hit := search.Hit{
			ID: row.ID, HubID: row.HubID, CollectionID: row.CollectionID, Title: row.Title, Tile: row.Tile,
			UpdatedAt: row.UpdatedAt, OnKind: search.KindEntry, OnID: row.OnID,
		}
		if f.Comments {
			hit.Snippet = search.Snippet(row.Text, q.Terms)
		}
		return hit
	}), nil
}
