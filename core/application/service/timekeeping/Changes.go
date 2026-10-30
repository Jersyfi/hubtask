// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"context"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
)

// The entities the module's rows are in the change log and the replica as (offline-sync.md §3.1).
// A record and the running timer are their person's alone: filed under no container and
// addressed by `account_id`, a removal as well, so the stream delivers them to that account only
// (as a custom role's grant). The kinds of work and the policy are one record each under the
// workspace's identifier, for every member: what a device fills a record in with.
const (
	RecordEntity    = recordTarget
	TimerEntity     = timerTarget
	WorkTypesEntity = "time_work_types"
	PolicyEntity    = policyTarget
)

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
	return k.Changes.Record(ctx, changelog.Change{
		TenantID: actor.TenantID, Entity: entity, EntityID: id, Op: op,
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
