// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"context"

	searchrepo "github.com/Jersyfi/hubtask/core/application/repository/search"
	"github.com/Jersyfi/hubtask/core/application/service/search"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// GoalFinder finds the goals whose titles hold the words, of the workspace and the hubs the
// reader may read - the question ReadGoal asks of one (UC-WRK-18 check 9).
type GoalFinder struct {
	Steward Steward
	Names   searchrepo.Names
}

// Find narrows the candidates to the readable places.
func (f GoalFinder) Find(ctx context.Context, actor appshared.ActorContext, q search.Question) (search.Found, error) {
	if person(actor) != nil || actor.RequireScope(itemsRead) != nil {
		return search.Found{}, nil
	}
	rows, err := search.Within(ctx, f.Steward.UnitOfWork, actor, f.Names.Goals, q.Ask())
	if err != nil || len(rows) == 0 {
		return search.Found{}, err
	}
	hubs := make([]shared.ID, len(rows))
	for i, row := range rows {
		hubs[i] = row.HubID
	}
	reads, err := f.Steward.readsPlaces(ctx, actor, hubs, itemsRead, goalTarget)
	if err != nil {
		return search.Found{}, err
	}
	allowed := make([]bool, len(rows))
	for i, row := range rows {
		allowed[i] = reads[row.HubID]
	}
	return search.Narrowed(rows, allowed, q.Size, func(row searchrepo.Row) search.Hit {
		return search.Hit{ID: row.ID, HubID: row.HubID, Title: row.Title, State: row.State, UpdatedAt: row.UpdatedAt}
	}), nil
}

// DashboardFinder finds the dashboards whose names hold the words that the reader sees: their
// own, and those shared with them - the question ReadDashboard asks of one (UC-WRK-18 check 9).
type DashboardFinder struct {
	Steward Steward
	Names   searchrepo.Names
}

// Find narrows the candidates to the visible ones.
func (f DashboardFinder) Find(ctx context.Context, actor appshared.ActorContext, q search.Question) (search.Found, error) {
	if person(actor) != nil || actor.RequireScope(containersRead) != nil {
		return search.Found{}, nil
	}
	rows, err := search.Within(ctx, f.Steward.UnitOfWork, actor, f.Names.Dashboards, q.Ask())
	if err != nil || len(rows) == 0 {
		return search.Found{}, err
	}
	ds := make([]domain.Dashboard, len(rows))
	for i, row := range rows {
		ds[i] = domain.Dashboard{ID: row.ID, HubID: row.HubID, OwnerID: row.OwnerID, Sharing: domain.Sharing(row.Sharing), GroupIDs: row.GroupIDs}
	}
	allowed, err := f.Steward.visible(ctx, actor, ds)
	if err != nil {
		return search.Found{}, err
	}
	return search.Narrowed(rows, allowed, q.Size, func(row searchrepo.Row) search.Hit {
		return search.Hit{ID: row.ID, HubID: row.HubID, Title: row.Title, State: row.State, UpdatedAt: row.UpdatedAt}
	}), nil
}

// PortfolioFinder finds the workspace's portfolios whose names hold the words, for a reader of
// the workspace - the question ListPortfolios asks (UC-WRK-18 check 9).
type PortfolioFinder struct {
	Steward Steward
	Names   searchrepo.Names
}

// Find keeps the candidates where the reader reads the workspace.
func (f PortfolioFinder) Find(ctx context.Context, actor appshared.ActorContext, q search.Question) (search.Found, error) {
	if person(actor) != nil || actor.RequireScope(itemsRead) != nil {
		return search.Found{}, nil
	}
	rows, err := search.Within(ctx, f.Steward.UnitOfWork, actor, f.Names.Portfolios, q.Ask())
	if err != nil || len(rows) == 0 {
		return search.Found{}, err
	}
	reads, err := f.Steward.permitted(ctx, actor, service.PermissionRead, [][]identity.Scope{workspace}, PortfolioReadAction, itemsRead, portfolioTarget)
	if err != nil {
		return search.Found{}, err
	}
	allowed := make([]bool, len(rows))
	for i := range rows {
		allowed[i] = len(reads) > 0 && reads[0]
	}
	return search.Narrowed(rows, allowed, q.Size, func(row searchrepo.Row) search.Hit {
		return search.Hit{ID: row.ID, Title: row.Title, State: row.State, UpdatedAt: row.UpdatedAt}
	}), nil
}
