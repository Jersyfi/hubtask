// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"context"
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
	TakeBaselineName    = "TakeBaseline"
	ListBaselinesName   = "ListBaselines"
	CompareBaselineName = "CompareBaseline"
	DeleteBaselineName  = "DeleteBaseline"
)

// BaselineOutput is a baseline as every door returns it (schema PlanBaseline).
func BaselineOutput(b domain.Baseline) usecase.Output {
	var takenBy any
	if !b.TakenBy.IsZero() {
		takenBy = b.TakenBy.String()
	}
	return usecase.Output{
		"id": b.ID.String(), "plan_id": b.PlanID.String(), "name": b.Name, "taken_at": b.TakenAt,
		"taken_by": takenBy, "entry_count": b.EntryCount,
	}
}

// TakeBaselineCommand is the input, typed.
type TakeBaselineCommand struct {
	PlanID shared.ID
	Name   string
}

// TakeBaseline keeps a plan's dates as they are agreed now, under a name (PLN-04). A plan keeps at
// most twenty; a new agreement is a new baseline, never a changed one.
type TakeBaseline struct{ Planner Planner }

// Execute takes the baseline.
func (h TakeBaseline) Execute(ctx context.Context, actor appshared.ActorContext, cmd TakeBaselineCommand) (domain.Baseline, error) {
	plan, err := h.Planner.plan(ctx, actor, cmd.PlanID, service.PermissionWriteItems, BaselineTakenAction, itemsWrite)
	if err != nil {
		return domain.Baseline{}, err
	}
	now := h.Planner.Clock.Now()
	baseline, err := domain.NewBaseline(domain.NewBaselineInput{
		ID: h.Planner.IDs.NewID(), TenantID: actor.TenantID, PlanID: plan.ID, TakenBy: actor.AccountID,
		Name: cmd.Name, Now: now, Text: h.Planner.Text,
	})
	if err != nil {
		return domain.Baseline{}, err
	}
	err = h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		count, err := h.Planner.Baselines.Count(ctx, plan.ID)
		if err != nil {
			return err
		}
		if count >= domain.MaxBaselinesPerPlan {
			return shared.ErrConflict.WithDetail("baselines.limit").
				WithParams(map[string]string{"maximum": strconv.Itoa(domain.MaxBaselinesPerPlan)})
		}
		copied, err := h.Planner.Baselines.Take(ctx, baseline, domain.MaxBaselineEntries)
		if err != nil {
			return err
		}
		baseline.EntryCount = copied
		return h.Planner.record(ctx, actor, BaselineTakenAction, baselineTarget, baseline.ID, now,
			audit.Change{Field: "plan_id", Classification: audit.Open, To: plan.ID.String()},
			audit.Change{Field: "name", Classification: audit.Sensitive, To: baseline.Name},
			audit.Change{Field: "entry_count", Classification: audit.Open, To: strconv.Itoa(copied)})
	})
	if err != nil {
		return domain.Baseline{}, err
	}
	return baseline, nil
}

// Descriptor is the catalogue entry.
func (h TakeBaseline) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: TakeBaselineName, Permission: service.PermissionWriteItems,
		Summary: "Keeps the start and due of every entry of a hub or a collection as they stand now, under a " +
			"name: the plan as it was agreed, to compare today's with. A plan keeps at most 20 " +
			"(baselines.limit); a baseline is never changed.",
		SideEffects: "Writes the baseline with a copy of the plan's dates and an audit entry.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "plan_id", Kind: usecase.KindID, Required: true, Description: "The hub or collection the plan is."},
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "Up to 120 characters, on one line."},
		},
		Audit:    usecase.AuditDeclaration{Action: BaselineTakenAction},
		Activity: usecase.ActivityDeclaration{Exempt: "a baseline copies dates and changes no entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			plan, err := in.ID("plan_id")
			if err != nil {
				return nil, err
			}
			baseline, err := h.Execute(ctx, actor, TakeBaselineCommand{PlanID: plan, Name: in.String("name")})
			if err != nil {
				return nil, err
			}
			return BaselineOutput(baseline), nil
		}),
	}, baselineTarget)
}

// ListBaselines reads a plan's baselines, the newest first (PLN-04).
type ListBaselines struct{ Planner Planner }

// Execute lists them for a reader of the plan.
func (h ListBaselines) Execute(ctx context.Context, actor appshared.ActorContext, planID shared.ID) ([]domain.Baseline, error) {
	if _, err := h.Planner.plan(ctx, actor, planID, service.PermissionRead, BaselinesReadAction, itemsRead); err != nil {
		return nil, err
	}
	var out []domain.Baseline
	err := h.Planner.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		out, err = h.Planner.Baselines.List(ctx, planID)
		return err
	})
	return out, err
}

// Descriptor is the catalogue entry.
func (h ListBaselines) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListBaselinesName, Permission: service.PermissionRead,
		Summary:     "Lists a hub's or a collection's baselines, the newest first, each with its name, when and by whom it was taken, and how many entries it holds.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "plan_id", Kind: usecase.KindID, Required: true, Description: "The hub or collection the plan is."},
		},
		Audit: usecase.AuditDeclaration{Action: BaselinesReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			plan, err := in.ID("plan_id")
			if err != nil {
				return nil, err
			}
			baselines, err := h.Execute(ctx, actor, plan)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(baselines))
			for _, b := range baselines {
				data = append(data, BaselineOutput(b))
			}
			return usecase.Output{"data": data}, nil
		}),
	}, baselineTarget)
}

// Comparison is a baseline beside today's plan.
type Comparison struct {
	Baseline  domain.Baseline
	Variances []domain.Variance
}

// CompareBaseline compares a baseline with today's dates of the plan, entry by entry (PLN-04). An
// entry the reader may not see is left out on both sides; one gone since shows as gone.
type CompareBaseline struct{ Planner Planner }

// Execute compares.
func (h CompareBaseline) Execute(ctx context.Context, actor appshared.ActorContext, baselineID shared.ID) (Comparison, error) {
	baseline, err := h.Planner.findBaseline(ctx, actor, baselineID)
	if err != nil {
		return Comparison{}, err
	}
	if _, err := h.Planner.plan(ctx, actor, baseline.PlanID, service.PermissionRead, BaselineComparedAction, itemsRead); err != nil {
		return Comparison{}, err
	}
	loaded, err := h.Planner.load(ctx, actor, baseline.PlanID, "", "")
	if err != nil {
		return Comparison{}, err
	}
	current := make([]domain.BaselineEntry, 0, len(loaded.Entries))
	for _, entry := range loaded.Plan.Entries {
		if loaded.External[entry.ID] {
			continue
		}
		e := loaded.Entries[entry.ID]
		current = append(current, domain.BaselineEntry{EntryID: e.ID, StartAt: e.StartAt, DueAt: e.DueAt})
	}
	// An entry of the baseline outside today's visible plan is either gone or out of reach; only
	// one that no longer exists may be shown, as gone.
	var elsewhere []shared.ID
	for _, b := range baseline.Entries {
		if _, ok := loaded.Entries[b.EntryID]; !ok {
			elsewhere = append(elsewhere, b.EntryID)
		}
	}
	exists := map[shared.ID]bool{}
	if len(elsewhere) > 0 {
		if err := h.Planner.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			summaries, err := h.Planner.References.Entries(ctx, elsewhere)
			for _, s := range summaries {
				exists[s.ID] = true
			}
			return err
		}); err != nil {
			return Comparison{}, err
		}
	}
	kept := make([]domain.BaselineEntry, 0, len(baseline.Entries))
	for _, b := range baseline.Entries {
		_, inPlan := loaded.Entries[b.EntryID]
		if (inPlan && !loaded.External[b.EntryID]) || !exists[b.EntryID] {
			kept = append(kept, b)
		}
	}
	return Comparison{Baseline: baseline, Variances: domain.Compare(kept, current, time.UTC)}, nil
}

func (p Planner) findBaseline(ctx context.Context, actor appshared.ActorContext, id shared.ID) (domain.Baseline, error) {
	if id.IsZero() {
		return domain.Baseline{}, invalid("/baseline_id", "baselines.id_required")
	}
	var baseline domain.Baseline
	err := p.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := p.Baselines.Find(ctx, id)
		baseline = found
		return err
	})
	return baseline, err
}

func timeOrNil(at *time.Time) any {
	if at == nil {
		return nil
	}
	return at.UTC()
}

func intOrNil(n *int) any {
	if n == nil {
		return nil
	}
	return *n
}

// Output is the shape every door returns (schema BaselineComparison).
func (c Comparison) Output() usecase.Output {
	entries := make([]usecase.Output, 0, len(c.Variances))
	for _, v := range c.Variances {
		entries = append(entries, usecase.Output{
			"item_id": v.EntryID.String(), "in_baseline": v.InBaseline, "in_plan": v.InPlan,
			"baseline_start_at": timeOrNil(v.BaselineStart), "baseline_due_at": timeOrNil(v.BaselineDue),
			"start_at": timeOrNil(v.CurrentStart), "due_at": timeOrNil(v.CurrentDue),
			"start_variance_days": intOrNil(v.StartVarianceDays), "finish_variance_days": intOrNil(v.FinishVarianceDays),
		})
	}
	return usecase.Output{"baseline": BaselineOutput(c.Baseline), "entries": entries}
}

// Descriptor is the catalogue entry.
func (h CompareBaseline) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: CompareBaselineName, Permission: service.PermissionRead,
		Summary: "Compares a baseline with today's plan entry by entry: the start and due it holds, today's, " +
			"and how many calendar days each slipped (positive is later); entries added since and gone " +
			"since are marked. An entry the caller may not see is left out.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "baseline_id", Kind: usecase.KindID, Required: true, Description: "The baseline."},
		},
		Audit: usecase.AuditDeclaration{Action: BaselineComparedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("baseline_id")
			if err != nil {
				return nil, err
			}
			comparison, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			return comparison.Output(), nil
		}),
	}, baselineTarget)
}

// DeleteBaseline removes a baseline nobody needs any more (PLN-04).
type DeleteBaseline struct{ Planner Planner }

// Execute deletes it.
func (h DeleteBaseline) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) error {
	baseline, err := h.Planner.findBaseline(ctx, actor, id)
	if err != nil {
		return err
	}
	if _, err := h.Planner.plan(ctx, actor, baseline.PlanID, service.PermissionWriteItems, BaselineDeletedAction, itemsWrite); err != nil {
		return err
	}
	now := h.Planner.Clock.Now()
	return h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Planner.Baselines.Delete(ctx, id); err != nil {
			return err
		}
		return h.Planner.record(ctx, actor, BaselineDeletedAction, baselineTarget, id, now,
			audit.Change{Field: "plan_id", Classification: audit.Open, From: baseline.PlanID.String()})
	})
}

// Descriptor is the catalogue entry.
func (h DeleteBaseline) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DeleteBaselineName, Permission: service.PermissionWriteItems,
		Summary:     "Deletes a baseline and the dates it holds; the plan's entries are not touched.",
		SideEffects: "Deletes the baseline and writes an audit entry.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "baseline_id", Kind: usecase.KindID, Required: true, Description: "The baseline."},
		},
		Audit:    usecase.AuditDeclaration{Action: BaselineDeletedAction},
		Activity: usecase.ActivityDeclaration{Exempt: "a baseline is a copy of dates; deleting it changes no entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("baseline_id")
			if err != nil {
				return nil, err
			}
			if err := h.Execute(ctx, actor, id); err != nil {
				return nil, err
			}
			return usecase.Output{}, nil
		}),
	}, baselineTarget)
}
