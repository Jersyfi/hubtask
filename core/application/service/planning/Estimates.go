// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"context"
	"strconv"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	SetEntryEstimateName   = "SetEntryEstimate"
	ClearEntryEstimateName = "ClearEntryEstimate"
)

// EntryEstimatedAction is the stable audit code of an estimate set or cleared (audit.md §2).
const EntryEstimatedAction audit.Action = "item.estimated"

// SetEntryEstimateCommand is the input, typed.
type SetEntryEstimateCommand struct {
	ItemID shared.ID
	Unit   domain.EstimateUnit
	Value  float64
	Size   string
}

// SetEntryEstimate says how big an entry is - in points, hours or a T-shirt size - so that a
// sprint's commitment and a team's velocity add up (PLN-07). Whoever may change the entry sets it.
type SetEntryEstimate struct{ Planner Planner }

// Execute writes the estimate.
func (h SetEntryEstimate) Execute(ctx context.Context, actor appshared.ActorContext, cmd SetEntryEstimateCommand) (domain.Estimate, error) {
	if cmd.ItemID.IsZero() {
		return domain.Estimate{}, invalid("/item_id", "items.item_id_required")
	}
	estimate, err := domain.NewEstimate(cmd.ItemID, cmd.Unit, cmd.Value, cmd.Size)
	if err != nil {
		return domain.Estimate{}, err
	}
	item, err := h.Planner.entryWritable(ctx, actor, cmd.ItemID, EntryEstimatedAction)
	if err != nil {
		return domain.Estimate{}, err
	}
	if err := item.EnsureEditable(); err != nil {
		return domain.Estimate{}, err
	}
	now := h.Planner.Clock.Now()
	estimate.UpdatedAt, estimate.UpdatedBy = now, actor.AccountID
	err = h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		before, err := h.Planner.Sprints.Estimates(ctx, []shared.ID{item.ID})
		if err != nil {
			return err
		}
		if err := h.Planner.Sprints.SetEstimate(ctx, estimate); err != nil {
			return err
		}
		if err := h.Planner.changed(ctx, actor, EstimateEntity, item.ID, item.CollectionID, EstimateRecord(estimate, item.CollectionID)); err != nil {
			return err
		}
		old := before[item.ID]
		return h.Planner.record(ctx, actor, EntryEstimatedAction, itemTarget, item.ID, now,
			audit.Change{Field: "unit", Classification: audit.Open, From: string(old.Unit), To: string(estimate.Unit)},
			audit.Change{Field: "value", Classification: audit.Open, From: valueText(old, before[item.ID].Unit != ""), To: valueText(estimate, true)})
	})
	if err != nil {
		return domain.Estimate{}, err
	}
	return estimate, nil
}

func valueText(e domain.Estimate, ok bool) string {
	if !ok {
		return ""
	}
	if e.Size != "" {
		return e.Size
	}
	return strconv.FormatFloat(e.Value, 'f', -1, 64)
}

// Descriptor is the catalogue entry.
func (h SetEntryEstimate) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SetEntryEstimateName, Permission: service.PermissionWriteItems,
		Summary: "Sets how big an entry is: POINTS or HOURS with a value of at most 10000 and two decimals, or a " +
			"T-shirt SIZE (XS, S, M, L, XL, XXL) that counts as 1, 2, 3, 5, 8 or 13 points. Needs write access " +
			"to the entry.",
		SideEffects: "Writes the estimate and an audit entry.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The entry."},
			{Name: "unit", Kind: usecase.KindString, Required: true, Enum: []string{"POINTS", "HOURS", "SIZE"}, Description: "What the estimate counts."},
			{Name: "value", Kind: usecase.KindAny, Description: "Points or hours, a number; left out for a size."},
			{Name: "size", Kind: usecase.KindString, Enum: domain.Sizes, Description: "A T-shirt size, for the unit SIZE."},
		},
		Audit:    usecase.AuditDeclaration{Action: EntryEstimatedAction},
		Activity: usecase.ActivityDeclaration{Exempt: "an estimate is planning beside the entry, recorded in the audit trail."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			var value float64
			switch v := in["value"].(type) {
			case nil:
			case float64:
				value = v
			case int:
				value = float64(v)
			case int64:
				value = float64(v)
			default:
				return nil, invalid("/value", "estimates.value_out_of_range")
			}
			estimate, err := h.Execute(ctx, actor, SetEntryEstimateCommand{
				ItemID: id, Unit: domain.EstimateUnit(in.String("unit")), Value: value, Size: in.String("size"),
			})
			if err != nil {
				return nil, err
			}
			out := usecase.Output{"item_id": id.String()}
			for k, v := range EstimateOutput(estimate, true).(usecase.Output) {
				out[k] = v
			}
			return out, nil
		}),
	}, itemTarget)
}

// ClearEntryEstimate removes an entry's estimate (PLN-07).
type ClearEntryEstimate struct{ Planner Planner }

// Execute clears it.
func (h ClearEntryEstimate) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) error {
	if id.IsZero() {
		return invalid("/item_id", "items.item_id_required")
	}
	item, err := h.Planner.entryWritable(ctx, actor, id, EntryEstimatedAction)
	if err != nil {
		return err
	}
	now := h.Planner.Clock.Now()
	return h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		cleared, err := h.Planner.Sprints.ClearEstimate(ctx, item.ID)
		if err != nil {
			return err
		}
		if !cleared {
			return shared.ErrNotFound.WithDetail("estimates.not_found")
		}
		if err := h.Planner.changed(ctx, actor, EstimateEntity, item.ID, item.CollectionID, nil); err != nil {
			return err
		}
		return h.Planner.record(ctx, actor, EntryEstimatedAction, itemTarget, item.ID, now,
			audit.Change{Field: "unit", Classification: audit.Open, To: ""})
	})
}

// Descriptor is the catalogue entry.
func (h ClearEntryEstimate) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ClearEntryEstimateName, Permission: service.PermissionWriteItems,
		Summary:     "Removes an entry's estimate; it then adds nothing to a commitment or a velocity. Needs write access to the entry.",
		SideEffects: "Deletes the estimate and writes an audit entry.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The entry."},
		},
		Audit:    usecase.AuditDeclaration{Action: EntryEstimatedAction},
		Activity: usecase.ActivityDeclaration{Exempt: "an estimate is planning beside the entry, recorded in the audit trail."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			return usecase.Output{}, h.Execute(ctx, actor, id)
		}),
	}, itemTarget)
}
