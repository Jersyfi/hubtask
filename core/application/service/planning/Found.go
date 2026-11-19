// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"context"

	searchrepo "github.com/Jersyfi/hubtask/core/application/repository/search"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/search"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// ReleaseFinder finds the releases whose names hold the words, of the hubs the reader may read -
// the question ListReleases asks of a hub (UC-WRK-18 check 9).
type ReleaseFinder struct {
	Planner Planner
	Names   searchrepo.Names
}

// Find narrows the candidates to the readable hubs.
func (f ReleaseFinder) Find(ctx context.Context, actor appshared.ActorContext, q search.Question) (search.Found, error) {
	return f.Planner.found(ctx, actor, q, f.Names.Releases, ReleaseReadAction, releaseTarget, func(row searchrepo.Row) []identity.Scope {
		return []identity.Scope{identity.TenantScope(), identity.HubScope(row.HubID)}
	})
}

// SprintFinder finds the sprints whose names hold the words, of the collections the reader may
// read - the question ListSprints asks of a collection (UC-WRK-18 check 9).
type SprintFinder struct {
	Planner Planner
	Names   searchrepo.Names
}

// Find narrows the candidates to the readable collections.
func (f SprintFinder) Find(ctx context.Context, actor appshared.ActorContext, q search.Question) (search.Found, error) {
	return f.Planner.found(ctx, actor, q, f.Names.Sprints, SprintReadAction, sprintTarget, func(row searchrepo.Row) []identity.Scope {
		path := []identity.Scope{identity.TenantScope()}
		if !row.HubID.IsZero() {
			path = append(path, identity.HubScope(row.HubID))
		}
		return append(path, identity.CollectionScope(row.CollectionID))
	})
}

// found reads the candidates and keeps those whose place the reader may read; a reader without
// the scope finds none rather than a refusal, as every finder does.
func (p Planner) found(
	ctx context.Context, actor appshared.ActorContext, q search.Question,
	read func(context.Context, searchrepo.Ask) ([]searchrepo.Row, error), action audit.Action, target string,
	place func(searchrepo.Row) []identity.Scope,
) (search.Found, error) {
	if !actor.IsAuthenticated() || actor.RequireScope(itemsRead) != nil {
		return search.Found{}, nil
	}
	rows, err := search.Within(ctx, p.UnitOfWork, actor, read, q.Ask())
	if err != nil || len(rows) == 0 {
		return search.Found{}, err
	}
	paths := make([][]identity.Scope, len(rows))
	for i, row := range rows {
		paths[i] = place(row)
	}
	allowed, err := p.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead, Action: action, TokenScope: itemsRead, TargetType: target,
	}, paths)
	if err != nil {
		return search.Found{}, err
	}
	return search.Narrowed(rows, allowed, q.Size, func(row searchrepo.Row) search.Hit {
		return search.Hit{ID: row.ID, HubID: row.HubID, CollectionID: row.CollectionID, Title: row.Title, State: row.State, UpdatedAt: row.UpdatedAt}
	}), nil
}
