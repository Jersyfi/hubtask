// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A bulk restores what a bulk trashed, all or nothing (UC-WRK-28 check 3): the undo of a selection
// moved to the trash is one atomic bulk of RESTORE_ITEM, ancestors first, against the database.

func restoringCatalogueFor(ctx context.Context, t *testing.T) *usecase.Registry {
	t.Helper()

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	hybrid, err := clockadapter.NewHybridClock(fixed, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	sink := postgres.NewAuditSink(ids)
	journal := work.ActivityJournal{Entries: historyRepo(), IDs: ids}
	outbox := postgres.NewOutbox(jobQueue(t))
	changes := postgres.NewChangeLog()
	authorizer := access.Service{
		Memberships: postgres.NewMembershipRepository(), UnitOfWork: unitOfWork, Audit: sink, Clock: fixed,
	}
	lifecycle := work.LifecycleWriter{
		Items: itemRepo(), Containers: containerRepo(), Authorizer: authorizer,
		Events: outbox, Changes: changes, Audit: sink, Activity: journal,
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid, Queue: jobQueue(t),
	}
	catalogue := &deferredRegistry{}

	registry, err := usecase.NewRegistry(nil,
		work.CreateWorkItem{
			Items: itemRepo(), Containers: containerRepo(), Profiles: postgres.NewCapabilityProfileRepository(),
			Authorizer: authorizer, Ownership: authorizer, Events: outbox, Changes: changes,
			Audit: sink, Activity: journal, UnitOfWork: unitOfWork,
			Clock: fixed, IDs: ids, HLC: hybrid,
		}.Descriptor(),
		work.TrashWorkItem{Lifecycle: lifecycle}.Descriptor(),
		work.RestoreWorkItem{Lifecycle: lifecycle}.Descriptor(),
		work.BulkUpdateWorkItems{
			Catalogue: catalogue, Audit: sink, UnitOfWork: unitOfWork, Clock: fixed,
		}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	catalogue.catalogue = registry
	return registry
}

func ops(op string, ids ...string) []any {
	operations := make([]any, 0, len(ids))
	for _, id := range ids {
		operations = append(operations, map[string]any{"op": op, "item_id": id})
	}
	return operations
}

func trashed(ctx context.Context, t *testing.T, id string) bool {
	t.Helper()
	return countIn(ctx, t, `SELECT count(*) FROM work_item WHERE id = $1 AND deleted_at IS NOT NULL`, id) == 1
}

func TestABulkRestoresAParentAndItsChildTogether(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	registry := restoringCatalogueFor(ctx, t)
	actor := itemWriter(tenantB, authorB)

	create := func(in usecase.Input) string {
		t.Helper()
		out, err := registry.Invoke(ctx, work.CreateWorkItemName, actor, in)
		if err != nil {
			t.Fatalf("creating %v: %v", in["type"], err)
		}
		return out.String("id")
	}
	parent := create(usecase.Input{"type": "TASK", "collection_id": collection.String(), "title": freshName(t)})
	child := create(usecase.Input{"type": "WORK_PACKAGE", "parent_id": parent, "title": freshName(t)})
	other := create(usecase.Input{"type": "TASK", "collection_id": collection.String(), "title": freshName(t)})

	// The selection went to the trash child first, so each is its own deletion.
	if _, err := registry.Invoke(ctx, work.BulkUpdateWorkItemsName, actor, usecase.Input{
		"atomic": true, "operations": ops("TRASH_ITEM", child, parent, other),
	}); err != nil {
		t.Fatalf("the trash bulk was refused: %v", err)
	}
	for _, id := range []string{parent, child, other} {
		if !trashed(ctx, t, id) {
			t.Fatalf("%s is not in the trash", id)
		}
	}

	t.Run("a child before its parent rolls the whole undo back", func(t *testing.T) {
		out, err := registry.Invoke(ctx, work.BulkUpdateWorkItemsName, actor, usecase.Input{
			"atomic": true, "operations": ops("RESTORE_ITEM", other, child),
		})
		if err != nil {
			t.Fatalf("the bulk was refused: %v", err)
		}
		if out.Int("applied") != 0 {
			t.Fatalf("%d applied, want none", out.Int("applied"))
		}
		results, _ := out["results"].([]usecase.Output)
		refused, _ := results[1]["problem"].(usecase.Output)
		if refused["detail_code"] != "items.parent_trashed" {
			t.Errorf("the child is refused as %v", results[1])
		}
		if !trashed(ctx, t, other) {
			t.Error("an entry restored before the refusal survived the rollback")
		}
	})

	t.Run("ancestors first, everything comes back", func(t *testing.T) {
		out, err := registry.Invoke(ctx, work.BulkUpdateWorkItemsName, actor, usecase.Input{
			"atomic": true, "operations": ops("RESTORE_ITEM", parent, child, other),
		})
		if err != nil {
			t.Fatalf("the bulk was refused: %v", err)
		}
		if out.Int("applied") != 3 {
			t.Fatalf("%d applied, want 3: %v", out.Int("applied"), out["results"])
		}
		for _, id := range []string{parent, child, other} {
			if trashed(ctx, t, id) {
				t.Errorf("%s is still in the trash", id)
			}
		}
		if placedUnder := countIn(ctx, t, `SELECT count(*) FROM work_item WHERE id = $1 AND parent_id = $2`,
			child, parent); placedUnder != 1 {
			t.Error("the child did not come back under its parent")
		}
	})
}
