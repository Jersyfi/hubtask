// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"slices"

	searchrepo "github.com/Jersyfi/hubtask/core/application/repository/search"
	"github.com/Jersyfi/hubtask/core/application/service/search"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// OnPages finds what is on a page - its files by their names, its comments by their words - on
// the pages the reader may read: a readable space and a restriction that admits them, the
// question ListPageComments asks of one (UC-WRK-18 check 9). A file's contents are not searched.
type OnPages struct {
	Space Space
	Names searchrepo.Names
	// Comments finds comments; otherwise files.
	Comments bool
}

// Find narrows the candidates to the readable pages.
func (f OnPages) Find(ctx context.Context, actor appshared.ActorContext, q search.Question) (search.Found, error) {
	if actor.RequireScope(pagesRead) != nil {
		return search.Found{}, nil
	}
	read := f.Names.PageFiles
	if f.Comments {
		read = f.Names.PageComments
	}
	var rows []searchrepo.Row
	var readers map[shared.ID]domain.Readers
	err := f.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if rows, err = read(ctx, q.Ask()); err != nil || len(rows) == 0 {
			return err
		}
		pages := make([]shared.ID, 0, len(rows))
		for _, row := range rows {
			if !slices.Contains(pages, row.OnID) {
				pages = append(pages, row.OnID)
			}
		}
		readers, err = f.Space.Pages.Readers(ctx, pages)
		return err
	})
	if err != nil || len(rows) == 0 {
		return search.Found{}, err
	}
	var hubs []shared.ID
	for _, row := range rows {
		if !slices.Contains(hubs, row.HubID) {
			hubs = append(hubs, row.HubID)
		}
	}
	readable, err := f.Space.readableHubs(ctx, actor, hubs)
	if err != nil {
		return search.Found{}, err
	}
	allowed := make([]bool, len(rows))
	for i, row := range rows {
		allowed[i] = slices.Contains(readable, row.HubID) && readers[row.OnID].Admits(actor.AccountID)
	}
	return search.Narrowed(rows, allowed, q.Size, func(row searchrepo.Row) search.Hit {
		hit := search.Hit{
			ID: row.ID, HubID: row.HubID, Title: row.Title, Tile: row.Tile, UpdatedAt: row.UpdatedAt,
			OnKind: search.KindPage, OnID: row.OnID,
		}
		if f.Comments {
			hit.Snippet = search.Snippet(row.Text, q.Terms)
		}
		return hit
	}), nil
}
