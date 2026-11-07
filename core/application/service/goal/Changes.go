// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"context"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The entities the module's rows are in the change log and the replica as (offline-sync.md §5).
// Each is filed under its hub, none for the workspace's; a private dashboard reaches its owner
// alone.
const (
	PeriodEntity    = "goal_period"
	GoalEntity      = "goal"
	CheckInEntity   = "goal_check_in"
	DashboardEntity = "dashboard"
	// PortfolioEntity is a portfolio under no hub, for every reader of the workspace; its projects
	// narrow per reader where the walk and the stream hand it out (UC-GOL-03).
	PortfolioEntity = "portfolio"
)

// PortfolioRecord is a portfolio as a device keeps it: its fields and every project by its
// container, never a name, a count or a budget computed for its writer. The sync service takes
// from `items` what the reader may not read.
func PortfolioRecord(p domain.Portfolio) usecase.Output {
	items := make([]any, 0, len(p.Items))
	for _, item := range p.Items {
		items = append(items, map[string]any{
			"container_id": item.ContainerID.String(), "start_on": dayText(item.StartOn), "end_on": dayText(item.EndOn),
			"status": string(item.Status),
		})
	}
	return usecase.Output{
		"id": p.ID.String(), "parent_id": idText(p.ParentID), "name": p.Name, "description": p.Description,
		"owner_id": idText(p.OwnerID), "status": string(p.Status), "items": items, "version": p.Version,
	}
}

// portfoliosChanged re-reads portfolios inside the caller's transaction and records each as it now
// stands.
func (s Steward) portfoliosChanged(ctx context.Context, actor appshared.ActorContext, ids ...shared.ID) error {
	if s.Changes == nil {
		return nil
	}
	for _, id := range ids {
		p, err := s.Store.Portfolio(ctx, id)
		if err != nil {
			return err
		}
		if err := s.changed(ctx, actor, PortfolioEntity, p.ID, "", PortfolioRecord(p)); err != nil {
			return err
		}
	}
	return nil
}

// GoalRecord is a goal as a device keeps it: its definition and its key results' manual values,
// never a number computed for a reader - progress, a view's live value, the share of linked
// entries done differ per reader (P-05) and are asked for online.
func GoalRecord(g domain.Goal) usecase.Output {
	results := make([]usecase.Output, 0, len(g.KeyResults))
	for _, r := range g.KeyResults {
		var current any
		if r.Source == domain.SourceManual && r.Kind != domain.ResultMilestone {
			current = r.Current
		}
		results = append(results, usecase.Output{
			"id": r.ID.String(), "title": r.Title, "kind": string(r.Kind), "start": r.Start, "target": r.Target,
			"current": current, "unit": r.Unit, "source": string(r.Source), "view_id": idText(r.ViewID),
			"entry_id": idText(r.EntryID),
		})
	}
	out := usecase.Output{
		"id": g.ID.String(), "period_id": g.PeriodID.String(), "hub_id": idText(g.HubID),
		"parent_id": idText(g.ParentID), "title": g.Title, "description": g.Description,
		"owner_id": idText(g.OwnerID), "status": string(g.Status), "confidence": nil, "grade": nil,
		"key_results": results, "version": g.Version,
	}
	if g.Confidence >= 0 {
		out["confidence"] = g.Confidence
	}
	if g.Grade != nil {
		out["grade"] = *g.Grade
	}
	return out
}

// CheckInRecord is a check-in as a device keeps it.
func CheckInRecord(c domain.CheckIn) usecase.Output { return checkInsOutput([]domain.CheckIn{c})[0] }

// changed records one row's change inside the caller's transaction; a nil payload is its
// removal. Without a change log nothing is recorded, a test's convenience.
func (s Steward) changed(
	ctx context.Context, actor appshared.ActorContext, entity string, id, hub shared.ID, payload usecase.Output,
) error {
	if s.Changes == nil {
		return nil
	}
	op, body := changelog.Upsert, map[string]any(payload)
	if payload == nil {
		op, body = changelog.Delete, nil
	}
	return s.Changes.Record(ctx, changelog.Change{
		TenantID: actor.TenantID, Entity: entity, EntityID: id, Op: op, ContainerID: hub,
		ActorID: actor.AccountID, HLC: s.HLC.Next(), Payload: body,
	})
}

// goalsChanged re-reads goals inside the caller's transaction and records each as it now stands.
func (s Steward) goalsChanged(ctx context.Context, actor appshared.ActorContext, ids ...shared.ID) error {
	if s.Changes == nil {
		return nil
	}
	for _, id := range ids {
		g, err := s.Store.Goal(ctx, id)
		if err != nil {
			return err
		}
		if err := s.changed(ctx, actor, GoalEntity, g.ID, g.HubID, GoalRecord(g)); err != nil {
			return err
		}
	}
	return nil
}
