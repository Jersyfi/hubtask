// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/text"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A legal hold freezes destruction, not work (UC-LIF-06 check 6). Every verb a person keeps using
// under a hold runs here through the real catalogue against PostgreSQL, so a hold check that crept
// into a write path, a repository statement or a trigger would show as a refusal.

// workUnderHoldCatalogueFor wires the editing, trash and archive verbs of entries and containers to
// the real adapters, as the composition root does.
func workUnderHoldCatalogueFor(ctx context.Context, t *testing.T) *usecase.Registry {
	t.Helper()

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	ids := clockadapter.NewUUIDv7(fixed)
	hybrid, err := clockadapter.NewHybridClock(fixed, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	sink := postgres.NewAuditSink(ids)
	profiles := postgres.NewCapabilityProfileRepository()
	journal := work.ActivityJournal{Entries: historyRepo(), IDs: ids}
	outbox := postgres.NewOutbox(jobQueue(t))
	changes := postgres.NewChangeLog()
	authorizer := access.Service{
		Memberships: postgres.NewMembershipRepository(), UnitOfWork: unitOfWork, Audit: sink, Clock: fixed,
	}
	dueDates := work.DueDateWriter{
		Items: itemRepo(), Containers: containerRepo(), Profiles: profiles, Authorizer: authorizer,
		Events: outbox, Changes: changes, Audit: sink, Activity: journal,
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
	}
	itemLifecycle := work.LifecycleWriter{
		Items: itemRepo(), Containers: containerRepo(), Authorizer: authorizer,
		Events: outbox, Changes: changes, Audit: sink, Activity: journal,
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid, Queue: jobQueue(t),
	}
	containerWriter := work.ContainerWriter{
		Containers: containerRepo(), Policies: postgres.AutoAssignPolicyRepository{},
		Authorizer: authorizer, Events: outbox, Changes: changes, Audit: sink,
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid, Queue: jobQueue(t),
		Text: text.Composing{},
	}

	registry, err := usecase.NewRegistry(nil,
		work.UpdateWorkItem{
			Items: itemRepo(), Buckets: bucketRepo(), Containers: containerRepo(), Profiles: profiles,
			Authorizer: authorizer, Events: outbox, Changes: changes, Audit: sink, Activity: journal,
			UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid, DueDates: dueDates,
			Text: text.Composing{},
		}.Descriptor(),
		work.TrashWorkItem{Lifecycle: itemLifecycle}.Descriptor(),
		work.RestoreWorkItem{Lifecycle: itemLifecycle}.Descriptor(),
		work.ArchiveWorkItem{Lifecycle: itemLifecycle}.Descriptor(),
		work.UnarchiveWorkItem{Lifecycle: itemLifecycle}.Descriptor(),
		work.RenameContainer{Writer: containerWriter}.Descriptor(),
		work.ArchiveContainer{Writer: containerWriter}.Descriptor(),
		work.UnarchiveContainer{Writer: containerWriter}.Descriptor(),
		work.TrashContainer{Writer: containerWriter}.Descriptor(),
		work.RestoreContainer{Writer: containerWriter}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	return registry
}

// placeHoldUntilCleanup places a hold through the repository and lifts it when the test ends: a
// workspace hold left in force would freeze every later test's purge in the shared database.
func placeHoldUntilCleanup(
	ctx context.Context, t *testing.T, tenant shared.ID, scope domain.HoldScope, scopeID shared.ID,
) domain.LegalHold {
	t.Helper()

	hold := holdIn(t, tenant, scope, scopeID)
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return holdRepo().Place(ctx, hold)
	}); err != nil {
		t.Fatalf("placing the %s hold: %v", scope, err)
	}
	t.Cleanup(func() {
		released, err := hold.Release(authorFor(tenant), "The proceedings ended", nil, created.Add(time.Hour))
		if err != nil {
			t.Errorf("lifting the %s hold: %v", scope, err)
			return
		}
		if err := write(ctx, t, tenant, func(ctx context.Context) error {
			_, err := holdRepo().Release(ctx, released)
			return err
		}); err != nil {
			t.Errorf("lifting the %s hold: %v", scope, err)
		}
	})
	return hold
}

func TestEditingTrashingRestoringAndArchivingGoOnUnderAHold(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	hub, collection := hubWithCollection(ctx, t, tenantB, authorB)
	task := seedTask(ctx, t, tenantB, authorB, collection)

	workspaceHold := placeHoldUntilCleanup(ctx, t, tenantB, domain.HoldTenant, "")
	hubHold := placeHoldUntilCleanup(ctx, t, tenantB, domain.HoldContainer, hub)

	registry := workUnderHoldCatalogueFor(ctx, t)
	// The workspace's owner: the one role that may trash a container, so no refusal below can be
	// a permission rather than the hold.
	owner := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantB, AccountID: authorB,
		AccountName: "Bert Beispiel", Scopes: []string{"items:write", "containers:write"},
	}

	steps := []struct {
		name  string
		input usecase.Input
	}{
		{work.UpdateWorkItemName, usecase.Input{"item_id": task.String(), "title": "Edited under a hold"}},
		{work.ArchiveWorkItemName, usecase.Input{"item_id": task.String()}},
		{work.UnarchiveWorkItemName, usecase.Input{"item_id": task.String()}},
		{work.TrashWorkItemName, usecase.Input{"item_id": task.String()}},
		{work.RestoreWorkItemName, usecase.Input{"item_id": task.String()}},
		{work.RenameContainerName, usecase.Input{"container_id": hub.String(), "name": freshName(t)}},
		{work.ArchiveContainerName, usecase.Input{"container_id": collection.String()}},
		{work.UnarchiveContainerName, usecase.Input{"container_id": collection.String()}},
		{work.ArchiveContainerName, usecase.Input{"container_id": hub.String()}},
		{work.UnarchiveContainerName, usecase.Input{"container_id": hub.String()}},
		{work.TrashContainerName, usecase.Input{"container_id": collection.String()}},
		{work.RestoreContainerName, usecase.Input{"container_id": collection.String()}},
	}
	for _, step := range steps {
		if _, err := registry.Invoke(ctx, step.name, owner, step.input); err != nil {
			t.Fatalf("%s %v was refused under a hold: %v", step.name, step.input, err)
		}
	}

	// The database, not the results: each verb's write landed, and its way back as well.
	if rows := countIn(ctx, t, `SELECT count(*) FROM work_item WHERE id = $1 AND title = 'Edited under a hold'
		AND archived_at IS NULL AND deleted_at IS NULL`, task.String()); rows != 1 {
		t.Error("the entry is not edited, back from the archive and back from the trash")
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM container WHERE id = ANY($1::uuid[])
		AND archived_at IS NULL AND deleted_at IS NULL`,
		[]string{hub.String(), collection.String()}); rows != 2 {
		t.Error("the hub and its collection are not both back from the archive and the trash")
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND outcome = 'SUCCESS'
		AND action IN ('item.updated', 'item.archived', 'item.unarchived', 'item.trashed', 'item.restored')
		AND target_id = $2`, tenantB.String(), task.String()); rows != 5 {
		t.Errorf("%d audit entries for the entry's five changes", rows)
	}

	// And the work did not lift the holds: they are what still stands between it and a purge.
	active := activeHolds(ctx, t, tenantB)
	if !containsHold(active, workspaceHold.ID) || !containsHold(active, hubHold.ID) {
		t.Error("working under the holds lifted one of them")
	}
}
