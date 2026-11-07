// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"

	goalrepo "github.com/Jersyfi/hubtask/core/application/repository/goal"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	goal "github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// GoalSummaries answers goals and dashboards by identifier (M8).
type GoalSummaries interface {
	Goals(ctx context.Context, filter goalrepo.GoalFilter) ([]goal.Goal, error)
	Dashboard(ctx context.Context, id shared.ID) (goal.Dashboard, error)
}

// placeOf is the workspace's path, or a hub's: where a goal or a dashboard is read.
func placeOf(hub shared.ID) []identity.Scope {
	if hub.IsZero() {
		return []identity.Scope{identity.TenantScope()}
	}
	return []identity.Scope{identity.TenantScope(), identity.HubScope(hub)}
}

// visibleGoals answers the goals among ids the reader may read: READ at the goal's place, the goals
// module's rule.
func (l Linking) visibleGoals(ctx context.Context, actor appshared.ActorContext, ids []shared.ID, action audit.Action) (map[shared.ID]goal.Goal, error) {
	visible := map[shared.ID]goal.Goal{}
	if len(ids) == 0 || l.Goals == nil {
		return visible, nil
	}
	var goals []goal.Goal
	err := l.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		read, err := l.Goals.Goals(ctx, goalrepo.GoalFilter{IDs: ids, Max: len(ids)})
		goals = read
		return err
	})
	if err != nil || len(goals) == 0 {
		return visible, err
	}
	paths := make([][]identity.Scope, 0, len(goals))
	for _, g := range goals {
		paths = append(paths, placeOf(g.HubID))
	}
	allowed, err := l.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead, Action: action, TokenScope: itemsRead, TargetType: "goal",
	}, paths)
	if err != nil {
		return nil, err
	}
	for i, g := range goals {
		if i < len(allowed) && allowed[i] {
			visible[g.ID] = g
		}
	}
	return visible, nil
}

// visibleDashboard answers a dashboard the reader sees: its owner always, anybody else when it is
// shared with its place, or with a group they are a member of, and they read there - the goals
// module's rule.
func (l Linking) visibleDashboard(ctx context.Context, actor appshared.ActorContext, id shared.ID, action audit.Action) (goal.Dashboard, bool, error) {
	if l.Goals == nil {
		return goal.Dashboard{}, false, nil
	}
	var d goal.Dashboard
	err := l.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := l.Goals.Dashboard(ctx, id)
		d = found
		return err
	})
	if errors.Is(err, shared.ErrNotFound) {
		return goal.Dashboard{}, false, nil
	}
	if err != nil {
		return goal.Dashboard{}, false, err
	}
	if d.OwnerID == actor.AccountID {
		return d, true, nil
	}
	switch d.Sharing {
	case goal.SharingScope:
	case goal.SharingGroups:
		member, err := l.inAnyGroup(ctx, actor, d.GroupIDs)
		if err != nil || !member {
			return goal.Dashboard{}, false, err
		}
	default:
		return goal.Dashboard{}, false, nil
	}
	allowed, err := l.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead, Action: action, TokenScope: containersRead, TargetType: "dashboard",
	}, [][]identity.Scope{placeOf(d.HubID)})
	if err != nil {
		return goal.Dashboard{}, false, err
	}
	if len(allowed) == 0 || !allowed[0] {
		return goal.Dashboard{}, false, nil
	}
	return d, true, nil
}

// GroupMembers answers whether an account is a member of a group.
type GroupMembers interface {
	InGroup(ctx context.Context, accountID, groupID shared.ID) (bool, error)
}

// inAnyGroup answers whether the reader is a member of one of a dashboard's groups; nil Groups
// answers no.
func (l Linking) inAnyGroup(ctx context.Context, actor appshared.ActorContext, groups []shared.ID) (bool, error) {
	if l.Groups == nil {
		return false, nil
	}
	for _, g := range groups {
		var member bool
		if err := l.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			var err error
			member, err = l.Groups.InGroup(ctx, actor.AccountID, g)
			return err
		}); err != nil || member {
			return member, err
		}
	}
	return false, nil
}

// goalTarget is a readable goal as a reference shows it (schema ReferenceTarget): its title, status
// and place - never its description or its key results, which a reference does not carry.
func goalTarget(g goal.Goal, anchor string) usecase.Output {
	out := usecase.Output{
		"kind": string(reference.KindGoal), "id": g.ID.String(), "anchor": anchorOrNil(anchor),
		"available": true, "title": g.Title, "status": string(g.Status),
	}
	if !g.HubID.IsZero() {
		out["hub_id"] = g.HubID.String()
	}
	return out
}

// dashboardTarget is a visible dashboard as a reference shows it: its name and place, what a live
// view draws in place with the reader's own figures.
func dashboardTarget(d goal.Dashboard, anchor string) usecase.Output {
	out := usecase.Output{
		"kind": string(reference.KindDashboard), "id": d.ID.String(), "anchor": anchorOrNil(anchor),
		"available": true, "title": d.Name,
	}
	if !d.HubID.IsZero() {
		out["hub_id"] = d.HubID.String()
	}
	return out
}
