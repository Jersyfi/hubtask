// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"context"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The entities sprints are in the change log and the replica as (offline-sync.md §3.1), each
// filed under the sprint's collection: a sprint itself, an entry's open membership of one and an
// entry's estimate - the last two by the entry's identifier, since an entry is in at most one
// open sprint and has at most one estimate. A past membership is the sprint's history, read
// online.
const (
	SprintEntity      = "sprint"
	SprintEntryEntity = "sprint_entry"
	EstimateEntity    = "entry_estimate"
)

// MembershipRecord is an entry's open membership as a device keeps it.
func MembershipRecord(entryID, sprintID, collectionID shared.ID, committed bool) usecase.Output {
	return usecase.Output{
		"item_id": entryID.String(), "sprint_id": sprintID.String(),
		"collection_id": collectionID.String(), "committed": committed,
	}
}

// EstimateRecord is an entry's estimate as a device keeps it.
func EstimateRecord(e domain.Estimate, collectionID shared.ID) usecase.Output {
	var size any
	if e.Size != "" {
		size = e.Size
	}
	return usecase.Output{
		"item_id": e.EntryID.String(), "collection_id": collectionID.String(),
		"unit": string(e.Unit), "value": e.Value, "size": size,
	}
}

// changed records one row's change inside the caller's transaction; a nil payload is its
// removal. Without a change log nothing is recorded, a test's convenience.
func (p Planner) changed(
	ctx context.Context, actor appshared.ActorContext, entity string, id, collection shared.ID, payload usecase.Output,
) error {
	if p.Changes == nil {
		return nil
	}
	op, body := changelog.Upsert, map[string]any(payload)
	if payload == nil {
		op, body = changelog.Delete, nil
	}
	return p.Changes.Record(ctx, changelog.Change{
		TenantID: actor.TenantID, Entity: entity, EntityID: id, Op: op, ContainerID: collection,
		ActorID: actor.AccountID, HLC: p.HLC.Next(), Payload: body,
	})
}

// sprintChanged records a sprint as it now stands.
func (p Planner) sprintChanged(ctx context.Context, actor appshared.ActorContext, s domain.Sprint) error {
	return p.changed(ctx, actor, SprintEntity, s.ID, s.CollectionID, SprintOutput(s))
}

// membershipsChanged records the entries' open membership of a sprint, or - sprint zero - that
// they are in none.
func (p Planner) membershipsChanged(
	ctx context.Context, actor appshared.ActorContext, collection, sprint shared.ID, committed bool, entries []shared.ID,
) error {
	for _, entry := range entries {
		var payload usecase.Output
		if !sprint.IsZero() {
			payload = MembershipRecord(entry, sprint, collection, committed)
		}
		if err := p.changed(ctx, actor, SprintEntryEntity, entry, collection, payload); err != nil {
			return err
		}
	}
	return nil
}
