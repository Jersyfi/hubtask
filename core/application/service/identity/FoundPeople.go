// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"

	searchrepo "github.com/Jersyfi/hubtask/core/application/repository/search"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/search"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// PeopleFinder finds people by their display names, for a member of the workspace - the
// question GetAccount asks before it names another account; a requester finds nobody
// (UC-SVC-05). Name, kind and status only (data-protection.md §9).
type PeopleFinder struct {
	Members    Permitter
	Names      searchrepo.Names
	UnitOfWork persistence.UnitOfWork
}

// Find answers the people, or nobody for one who is not a member.
func (f PeopleFinder) Find(ctx context.Context, actor appshared.ActorContext, q search.Question) (search.Found, error) {
	if actor.RequireScope(accountsRead) != nil || f.Members == nil {
		return search.Found{}, nil
	}
	member, err := f.Members.Permits(ctx, actor, access.Request{
		AnyMember: true, Action: AccountReadAction, TokenScope: accountsRead, TargetType: accountTarget,
	})
	if err != nil || !member {
		return search.Found{}, err
	}
	rows, err := search.Within(ctx, f.UnitOfWork, actor, f.Names.People, q.Ask())
	if err != nil {
		return search.Found{}, err
	}
	allowed := make([]bool, len(rows))
	for i := range allowed {
		allowed[i] = true
	}
	return search.Narrowed(rows, allowed, q.Size, func(row searchrepo.Row) search.Hit {
		return search.Hit{ID: row.ID, Title: row.Title, Tile: row.Tile, State: row.State}
	}), nil
}
