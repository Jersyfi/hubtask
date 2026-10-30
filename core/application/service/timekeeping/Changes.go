// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"context"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
)

// The entities the module's rows are in the change log and the replica as (offline-sync.md §3.1).
// A record, the running timer, a week and a person's capacities are their person's alone: filed
// under no container and addressed by `account_id`, a removal as well, so the stream delivers them
// to that account only (as a custom role's grant). The kinds of work, the policy and the absences
// - the shared calendar - reach every member. The cost rates are one record under the workspace's
// identifier and a budget is filed under its hub or collection, both for MANAGE_COSTS alone.
const (
	RecordEntity     = recordTarget
	TimerEntity      = timerTarget
	SheetEntity      = sheetTarget
	CapacitiesEntity = "time_capacities"
	AbsenceEntity    = absenceTarget
	WorkTypesEntity  = "time_work_types"
	PolicyEntity     = policyTarget
	RatesEntity      = "time_cost_rates"
	BudgetEntity     = budgetTarget
)

// CapacitiesKept and RatesKept bound what one record of capacities or rates holds.
const (
	CapacitiesKept = maxCapacitiesKept
	RatesKept      = maxRatesKept
)

// CostsRequest is what a copy of a rate or a budget asks at a place: what ListCostRates and
// ReadBudget ask.
func CostsRequest(path []identity.Scope) access.Request {
	return costsRequest("", "", "", path, "")
}

// CapacitiesRecord is a person's capacities as one record.
func CapacitiesRecord(account shared.ID, list []domain.Capacity) usecase.Output {
	data := make([]usecase.Output, 0, len(list))
	for _, c := range list {
		data = append(data, CapacityOutput(c))
	}
	return usecase.Output{"account_id": account.String(), "data": data}
}

// RatesRecord is the workspace's cost rates as one record.
func RatesRecord(list []domain.CostRate) usecase.Output {
	data := make([]usecase.Output, 0, len(list))
	for _, r := range list {
		data = append(data, RateOutput(r))
	}
	return usecase.Output{"data": data}
}

// BudgetRecord is a plan's budget as a device keeps it: never its burn, which the server sums.
func BudgetRecord(b domain.Budget) usecase.Output {
	return usecase.Output{
		"plan_id": b.PlanID.String(), "budget_minutes": b.Minutes, "budget_amount_minor": b.AmountMinor,
		"currency": b.Currency, "version": b.Version,
	}
}

// TimerRecord is a running timer as a device keeps it, by its person: no elapsed minutes, which a
// device counts from `started_at` itself.
func TimerRecord(t domain.Timer) usecase.Output {
	return usecase.Output{
		"account_id": t.AccountID.String(), "running": true, "entry_id": idText(t.EntryID),
		"work_type": t.WorkType, "note": t.Note, "billable": t.Billable, "started_at": t.StartedAt,
	}
}

// WorkTypesRecord is the workspace's kinds of work as one record.
func WorkTypesRecord(kinds []domain.WorkType) usecase.Output {
	data := make([]usecase.Output, 0, len(kinds))
	for _, k := range kinds {
		data = append(data, WorkTypeOutput(k))
	}
	return usecase.Output{"data": data}
}

// changed records a change in the caller's transaction; without a change log nothing is recorded,
// a test's convenience.
func (k Keeper) changed(
	ctx context.Context, actor appshared.ActorContext, entity string, id shared.ID, op changelog.Operation, payload usecase.Output,
) error {
	if k.Changes == nil || k.HLC == nil {
		return nil
	}
	return k.changedUnder(ctx, actor, entity, id, "", op, payload)
}

// changedUnder records a change filed under a container.
func (k Keeper) changedUnder(
	ctx context.Context, actor appshared.ActorContext, entity string, id, container shared.ID, op changelog.Operation, payload usecase.Output,
) error {
	if k.Changes == nil || k.HLC == nil {
		return nil
	}
	return k.Changes.Record(ctx, changelog.Change{
		TenantID: actor.TenantID, Entity: entity, EntityID: id, ContainerID: container, Op: op,
		ActorID: actor.AccountID, HLC: k.HLC.Next(), Payload: map[string]any(payload),
	})
}

// recordChanged records a person's record as it stands.
func (k Keeper) recordChanged(ctx context.Context, actor appshared.ActorContext, r domain.Record) error {
	return k.changed(ctx, actor, RecordEntity, r.ID, changelog.Upsert, RecordOutput(r))
}

// recordRemoved records a record's removal, addressed to its person.
func (k Keeper) recordRemoved(ctx context.Context, actor appshared.ActorContext, id, account shared.ID) error {
	return k.changed(ctx, actor, RecordEntity, id, changelog.Delete, usecase.Output{"account_id": account.String()})
}

// timerChanged records the person's timer started, or - nil - stopped or discarded.
func (k Keeper) timerChanged(ctx context.Context, actor appshared.ActorContext, account shared.ID, t *domain.Timer) error {
	if t == nil {
		return k.changed(ctx, actor, TimerEntity, account, changelog.Delete, usecase.Output{"account_id": account.String()})
	}
	return k.changed(ctx, actor, TimerEntity, account, changelog.Upsert, TimerRecord(*t))
}

// workTypesChanged re-reads the kinds of work inside the caller's transaction and records them.
func (k Keeper) workTypesChanged(ctx context.Context, actor appshared.ActorContext) error {
	if k.Changes == nil || k.HLC == nil {
		return nil
	}
	kinds, err := k.Store.WorkTypes(ctx)
	if err != nil {
		return err
	}
	return k.changed(ctx, actor, WorkTypesEntity, actor.TenantID, changelog.Upsert, WorkTypesRecord(kinds))
}

// policyChanged records the workspace's time policy.
func (k Keeper) policyChanged(ctx context.Context, actor appshared.ActorContext, p domain.Policy) error {
	return k.changed(ctx, actor, PolicyEntity, actor.TenantID, changelog.Upsert, PolicyOutput(p))
}

// sheetChanged records a person's week as it stands.
func (k Keeper) sheetChanged(ctx context.Context, actor appshared.ActorContext, s domain.Timesheet) error {
	return k.changed(ctx, actor, SheetEntity, s.ID, changelog.Upsert, SheetOutput(s))
}

// capacitiesChanged re-reads a person's capacities inside the caller's transaction and records them.
func (k Keeper) capacitiesChanged(ctx context.Context, actor appshared.ActorContext, account shared.ID) error {
	if k.Changes == nil || k.HLC == nil {
		return nil
	}
	list, err := k.Resources.Capacities(ctx, []shared.ID{account}, maxCapacitiesKept)
	if err != nil {
		return err
	}
	return k.changed(ctx, actor, CapacitiesEntity, account, changelog.Upsert, CapacitiesRecord(account, list))
}

// absenceChanged records an absence as it stands, or - nil - its removal.
func (k Keeper) absenceChanged(ctx context.Context, actor appshared.ActorContext, id shared.ID, a *domain.Absence) error {
	if a == nil {
		return k.changed(ctx, actor, AbsenceEntity, id, changelog.Delete, nil)
	}
	return k.changed(ctx, actor, AbsenceEntity, id, changelog.Upsert, AbsenceOutput(*a))
}

// ratesChanged re-reads the cost rates inside the caller's transaction and records them.
func (k Keeper) ratesChanged(ctx context.Context, actor appshared.ActorContext) error {
	if k.Changes == nil || k.HLC == nil {
		return nil
	}
	list, err := k.Resources.Rates(ctx, maxRatesKept)
	if err != nil {
		return err
	}
	return k.changed(ctx, actor, RatesEntity, actor.TenantID, changelog.Upsert, RatesRecord(list))
}

// budgetChanged records a plan's budget, or - nil - its removal, filed under the plan.
func (k Keeper) budgetChanged(ctx context.Context, actor appshared.ActorContext, plan shared.ID, b *domain.Budget) error {
	if b == nil {
		return k.changedUnder(ctx, actor, BudgetEntity, plan, plan, changelog.Delete, nil)
	}
	return k.changedUnder(ctx, actor, BudgetEntity, plan, plan, changelog.Upsert, BudgetRecord(*b))
}
