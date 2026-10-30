// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"context"
	"slices"
	"strconv"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ReadPlanScheduleName  = "ReadPlanSchedule"
	ApplyPlanScheduleName = "ApplyPlanSchedule"
)

// ScheduleQuery names a plan and the calendar it is counted on.
type ScheduleQuery struct {
	PlanID shared.ID
	// Calendar is a business calendar's key; empty counts Monday to Friday in Zone.
	Calendar string
	// Zone is the IANA zone the plan's days are local to when no calendar names one; empty is UTC.
	Zone string
}

// Scheduled is a plan read and scheduled.
type Scheduled struct {
	Loaded Loaded
	Result domain.Result
}

// ReadPlanSchedule reads a plan as the scheduler places it: every visible entry's dates, the moves
// it proposes, the dependencies it cannot keep, the float and the critical path (PLN-02, PLN-03).
// It changes nothing.
type ReadPlanSchedule struct{ Planner Planner }

// Execute schedules the plan for the reader.
func (h ReadPlanSchedule) Execute(ctx context.Context, actor appshared.ActorContext, q ScheduleQuery) (Scheduled, error) {
	if _, err := h.Planner.plan(ctx, actor, q.PlanID, service.PermissionRead, PlanReadAction, itemsRead); err != nil {
		return Scheduled{}, err
	}
	return h.Planner.schedule(ctx, actor, q)
}

func (p Planner) schedule(ctx context.Context, actor appshared.ActorContext, q ScheduleQuery) (Scheduled, error) {
	loaded, err := p.load(ctx, actor, q.PlanID, q.Calendar, q.Zone)
	if err != nil {
		return Scheduled{}, err
	}
	result, err := domain.Schedule(loaded.Plan)
	if err != nil {
		return Scheduled{}, err
	}
	return Scheduled{Loaded: loaded, Result: result}, nil
}

// Output is the shape every door returns (api/openapi.yaml, schema PlanSchedule).
func (s Scheduled) Output() usecase.Output {
	entries := make([]usecase.Output, 0, len(s.Result.Placements))
	for _, p := range s.Result.Placements {
		entry := s.Loaded.Entries[p.ID]
		mode := entry.Mode
		if !mode.Valid() {
			mode = domain.Manual
		}
		var parent any
		if !entry.ParentID.IsZero() && !s.Loaded.External[p.ID] {
			parent = entry.ParentID.String()
		}
		entries = append(entries, usecase.Output{
			"item_id": p.ID.String(), "title": entry.Title, "parent_id": parent,
			"collection_id": entry.CollectionID.String(), "start": p.Start.String(), "finish": p.Finish.String(),
			"duration_days": p.Duration, "moved": p.Moved, "total_float_days": p.TotalFloat,
			"free_float_days": p.FreeFloat, "critical": p.Critical, "summary": p.Summary,
			"external": s.Loaded.External[p.ID], "mode": string(mode), "milestone": entry.Milestone,
			"done": entry.Done,
		})
	}
	conflicts := make([]usecase.Output, 0, len(s.Result.Conflicts))
	for _, c := range s.Result.Conflicts {
		conflicts = append(conflicts, usecase.Output{
			"predecessor_id": c.Predecessor.String(), "successor_id": c.Successor.String(),
			"dependency_kind": string(c.Kind), "earliest": c.Earliest.String(),
		})
	}
	dependencies := make([]usecase.Output, 0, len(s.Loaded.Plan.Dependencies))
	for i, d := range s.Loaded.Plan.Dependencies {
		dependencies = append(dependencies, usecase.Output{
			"link_id": s.Loaded.Links[i].ID.String(), "type": s.Loaded.Links[i].Type,
			"predecessor_id": d.Predecessor.String(), "successor_id": d.Successor.String(),
			"dependency_kind": string(d.Timing.Kind), "lag_days": d.Timing.LagDays,
		})
	}
	unscheduled := make([]string, 0, len(s.Result.Unscheduled))
	for _, id := range s.Result.Unscheduled {
		if !s.Loaded.External[id] {
			unscheduled = append(unscheduled, id.String())
		}
	}
	critical := make([]string, 0, len(s.Result.CriticalPath))
	for _, id := range s.Result.CriticalPath {
		critical = append(critical, id.String())
	}
	var finish any
	if len(s.Result.Placements) > 0 {
		finish = s.Result.Finish.String()
	}
	return usecase.Output{
		"zone": s.Loaded.Zone.String(), "finish": finish, "entries": entries, "dependencies": dependencies,
		"conflicts": conflicts, "unscheduled": unscheduled, "critical_path": critical,
	}
}

func scheduleInput() []usecase.Field {
	return []usecase.Field{
		{Name: "plan_id", Kind: usecase.KindID, Required: true, Description: "The hub or collection the plan is."},
		{Name: "calendar", Kind: usecase.KindString, Description: "A business calendar's key whose working days the plan counts; without one, Monday to Friday."},
		{Name: "zone", Kind: usecase.KindString, Description: "The IANA time zone the plan's days are local to when no calendar names one; UTC without."},
	}
}

func scheduleQueryOf(in usecase.Input) (ScheduleQuery, error) {
	plan, err := in.ID("plan_id")
	if err != nil {
		return ScheduleQuery{}, err
	}
	return ScheduleQuery{PlanID: plan, Calendar: in.String("calendar"), Zone: in.String("zone")}, nil
}

// Descriptor is the catalogue entry.
func (h ReadPlanSchedule) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadPlanScheduleName, Permission: service.PermissionRead,
		Summary: "Reads a hub's or a collection's plan as the scheduler places it: each visible entry's start " +
			"and finish, the moves it proposes for automatic entries, the dependencies it cannot keep for " +
			"manual ones, every entry's total and free float in working days, and the critical path that " +
			"decides the end date. Changes nothing.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead, ReadOnly: true,
		Input: scheduleInput(),
		Audit: usecase.AuditDeclaration{Action: PlanReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			q, err := scheduleQueryOf(in)
			if err != nil {
				return nil, err
			}
			scheduled, err := h.Execute(ctx, actor, q)
			if err != nil {
				return nil, err
			}
			return scheduled.Output(), nil
		}),
	}, planTarget)
}

// ApplyPlanScheduleCommand is the input, typed.
type ApplyPlanScheduleCommand struct {
	ScheduleQuery
	// ItemIDs are the proposed moves the person accepts; a move not named stays a proposal.
	ItemIDs []shared.ID
}

// Applied is what applying a schedule moved.
type Applied struct {
	Moved []usecase.Output
}

// ApplyPlanSchedule applies the moves a person accepted (P-04): each through UpdateWorkItem, all or
// none. The plan is scheduled again first, so what moves is what the plan proposes now, never what
// it proposed when the person looked.
type ApplyPlanSchedule struct{ Planner Planner }

// Execute applies the accepted moves.
func (h ApplyPlanSchedule) Execute(ctx context.Context, actor appshared.ActorContext, cmd ApplyPlanScheduleCommand) (Applied, error) {
	if len(cmd.ItemIDs) == 0 {
		return Applied{}, invalid("/item_ids", "planning.nothing_accepted")
	}
	if len(cmd.ItemIDs) > maxMovesApplied {
		return Applied{}, shared.ErrValidation.WithDetail("planning.too_many_moves").
			WithParams(map[string]string{"maximum": strconv.Itoa(maxMovesApplied)})
	}
	if _, err := h.Planner.plan(ctx, actor, cmd.PlanID, service.PermissionWriteItems, PlanScheduledAction, itemsWrite); err != nil {
		return Applied{}, err
	}
	scheduled, err := h.Planner.schedule(ctx, actor, cmd.ScheduleQuery)
	if err != nil {
		return Applied{}, err
	}
	zone := scheduled.Loaded.Zone
	out := Applied{Moved: []usecase.Output{}}
	now := h.Planner.Clock.Now()
	err = h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		for _, move := range scheduled.Result.Moves() {
			if !slices.Contains(cmd.ItemIDs, move.ID) || scheduled.Loaded.External[move.ID] {
				continue
			}
			entry := scheduled.Loaded.Entries[move.ID]
			if _, err := h.Planner.Catalogue.Invoke(ctx, updateWorkItemName, actor, usecase.Input{
				"item_id": move.ID.String(), "start_at": move.Start.In(zone).Format(time.RFC3339),
				"due_at": move.Finish.In(zone).Format(time.RFC3339), "due_date_only": true,
				"due_time_zone": zone.String(), "expected_version": entry.Version,
			}); err != nil {
				return err
			}
			out.Moved = append(out.Moved, usecase.Output{
				"item_id": move.ID.String(), "start": move.Start.String(), "finish": move.Finish.String(),
			})
		}
		return h.Planner.record(ctx, actor, PlanScheduledAction, planTarget, cmd.PlanID, now,
			audit.Change{Field: "moved", Classification: audit.Open, To: strconv.Itoa(len(out.Moved))})
	})
	if err != nil {
		return Applied{}, err
	}
	return out, nil
}

// Descriptor is the catalogue entry.
func (h ApplyPlanSchedule) Descriptor() usecase.Descriptor {
	input := append(scheduleInput(), usecase.Field{
		Name: "item_ids", Kind: usecase.KindIDList, Required: true,
		Description: "The proposed moves accepted, by entry; at most 500. A move not named stays a proposal.",
	})
	return descriptor(usecase.Descriptor{
		Name: ApplyPlanScheduleName, Permission: service.PermissionWriteItems,
		Summary: "Applies the moves the scheduler proposes for the accepted automatic entries of a plan, all " +
			"or none, each as UpdateWorkItem changing its start and its all-day due date. The plan is " +
			"scheduled again first; nothing moves that the person did not accept.",
		SideEffects: "Changes the accepted entries' dates through UpdateWorkItem (each with its own event, " +
			"history and audit entry) and writes an audit entry for the plan.",
		TokenScope: itemsWrite,
		Input:      input,
		Audit:      usecase.AuditDeclaration{Action: PlanScheduledAction},
		Activity:   usecase.ActivityDeclaration{Exempt: "each move is recorded in its entry's history by UpdateWorkItem."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			q, err := scheduleQueryOf(in)
			if err != nil {
				return nil, err
			}
			ids, err := in.IDList("item_ids")
			if err != nil {
				return nil, err
			}
			applied, err := h.Execute(ctx, actor, ApplyPlanScheduleCommand{ScheduleQuery: q, ItemIDs: ids})
			if err != nil {
				return nil, err
			}
			return usecase.Output{"moved": applied.Moved}, nil
		}),
	}, planTarget)
}
