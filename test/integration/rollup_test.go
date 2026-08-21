// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The roll-up end to end (B-07), which is the only thing that proves two links the unit tests cannot:
// that the completion policy is really read out of the `policies` column, and that the walk upwards writes
// through the real repository.
//
// Tenant B, for the reason create_work_item_test.go gives: tenant A carries a narrowed TASK profile that
// another suite seeds, and testing the system matrix against a tenant that replaced it asks a different
// question.

// completionCatalogueFor wires both directions of completion the way the composition root does - one
// dependency set, two entries.
func completionCatalogueFor(ctx context.Context, t *testing.T) *usecase.Registry {
	t.Helper()

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	ids := clockadapter.NewUUIDv7(fixed)
	hybrid, err := clockadapter.NewHybridClock(fixed, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	sink := postgres.NewAuditSink(ids)

	writer := work.CompletionWriter{
		Items:      itemRepo(),
		Containers: containerRepo(),
		Profiles:   postgres.NewCapabilityProfileRepository(),
		Authorizer: access.Service{
			Memberships: postgres.NewMembershipRepository(),
			UnitOfWork:  unitOfWork,
			Audit:       sink,
			Clock:       fixed,
		},
		Events:     postgres.NewOutbox(jobQueue(t)),
		Changes:    postgres.NewChangeLog(),
		Audit:      sink,
		Activity:   work.ActivityJournal{Entries: historyRepo(), IDs: ids},
		UnitOfWork: unitOfWork,
		Clock:      fixed,
		IDs:        ids,
		HLC:        hybrid,
	}

	registry, err := usecase.NewRegistry(nil,
		work.CompleteWorkItem{Completion: writer}.Descriptor(),
		work.ReopenWorkItem{Completion: writer}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	return registry
}

// setCompletionPolicy writes the collection's policy as the superuser. Nothing writes it through a use
// case yet - UpdateContainerPolicies is B-06 - so a fixture is the only way to have a collection that
// rolls up, and reading it back through the repository is the point of the test.
func setCompletionPolicy(ctx context.Context, t *testing.T, collection shared.ID, policy domain.CompletionPolicy) {
	t.Helper()

	tag, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE container SET policies = jsonb_set(policies, '{completion_policy}', to_jsonb($2::text))
		 WHERE id = $1`, collection.String(), string(policy))
	if err != nil {
		t.Fatalf("setting the completion policy: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("setting the policy matched %d rows, want 1", tag.RowsAffected())
	}
}

// subtree creates a task, a work package under it, and two activities under that, through the real create
// use case so that the placement is the one the seeded profiles permit.
func subtree(ctx context.Context, t *testing.T, collection shared.ID) (task, pack, first, second string) {
	t.Helper()

	registry := itemCatalogueFor(ctx, t)
	actor := itemWriter(tenantB, authorB)

	create := func(in usecase.Input) string {
		out, err := registry.Invoke(ctx, "CreateWorkItem", actor, in)
		if err != nil {
			t.Fatalf("creating %v: %v", in["type"], err)
		}
		return out.String("id")
	}

	task = create(usecase.Input{
		"type": "TASK", "collection_id": collection.String(), "title": freshName(t),
	})
	pack = create(usecase.Input{"type": "WORK_PACKAGE", "parent_id": task, "title": freshName(t)})
	first = create(usecase.Input{"type": "ACTIVITY", "parent_id": pack, "title": freshName(t)})
	second = create(usecase.Input{"type": "ACTIVITY", "parent_id": pack, "title": freshName(t)})
	return task, pack, first, second
}

func isCompleted(ctx context.Context, t *testing.T, id string) bool {
	t.Helper()
	return findItem(ctx, t, tenantB, shared.MustParseID(id)).Completion.IsCompleted
}

// The acceptance criterion, against the database and against a policy read out of the column: completing
// the last activity completes the work package, and the task above it.
func TestTheRollUpReachesEveryLevelAgainstTheDatabase(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	setCompletionPolicy(ctx, t, collection, domain.CompletionRollup)

	task, pack, first, second := subtree(ctx, t, collection)
	registry := completionCatalogueFor(ctx, t)
	actor := itemWriter(tenantB, authorB)

	// One of two activities: nothing above may move.
	if _, err := registry.Invoke(ctx, "CompleteWorkItem", actor, usecase.Input{"item_id": first}); err != nil {
		t.Fatalf("completing the first activity: %v", err)
	}
	if isCompleted(ctx, t, pack) {
		t.Fatal("the work package completed while an activity was still open")
	}

	// The last one: the package completes, and the task with it, since the package was its only child.
	if _, err := registry.Invoke(ctx, "CompleteWorkItem", actor, usecase.Input{"item_id": second}); err != nil {
		t.Fatalf("completing the second activity: %v", err)
	}
	if !isCompleted(ctx, t, pack) {
		t.Error("the work package did not complete when its last activity did")
	}
	if !isCompleted(ctx, t, task) {
		t.Error("the task did not complete when its only work package did")
	}
}

// The same tree under the default policy. This is the test that would fail if the policy were not really
// read from the column - it and the one above differ in nothing else.
func TestWithoutTheRollupPolicyNothingAboveMoves(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	// Deliberately not set: a collection that has never been configured behaves as MANUAL, which is what
	// the empty `policies` column has to read as.

	task, pack, first, second := subtree(ctx, t, collection)
	registry := completionCatalogueFor(ctx, t)
	actor := itemWriter(tenantB, authorB)

	for _, activity := range []string{first, second} {
		if _, err := registry.Invoke(ctx, "CompleteWorkItem", actor, usecase.Input{"item_id": activity}); err != nil {
			t.Fatalf("completing an activity: %v", err)
		}
	}

	if isCompleted(ctx, t, pack) || isCompleted(ctx, t, task) {
		t.Error("an unconfigured collection rolled up")
	}
}

// And the other direction: reopening one activity reopens the completed items above it.
func TestReopeningRollsUpThroughTheDatabase(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	setCompletionPolicy(ctx, t, collection, domain.CompletionRollup)

	task, pack, first, second := subtree(ctx, t, collection)
	registry := completionCatalogueFor(ctx, t)
	actor := itemWriter(tenantB, authorB)

	for _, activity := range []string{first, second} {
		if _, err := registry.Invoke(ctx, "CompleteWorkItem", actor, usecase.Input{"item_id": activity}); err != nil {
			t.Fatalf("completing: %v", err)
		}
	}
	if !isCompleted(ctx, t, task) {
		t.Fatal("the tree did not complete, so reopening proves nothing")
	}

	if _, err := registry.Invoke(ctx, "ReopenWorkItem", actor, usecase.Input{"item_id": first}); err != nil {
		t.Fatalf("reopening: %v", err)
	}

	if isCompleted(ctx, t, pack) {
		t.Error("the work package stayed completed with an open activity under it")
	}
	if isCompleted(ctx, t, task) {
		t.Error("the task stayed completed with an open work package under it")
	}
	if !isCompleted(ctx, t, second) {
		t.Error("reopening one activity reopened its sibling")
	}
}

// A repeat writes nothing, which against a real database means the version does not move. That is what
// makes an at-least-once delivery of the same automation action harmless.
func TestCompletingTwiceLeavesTheVersionAlone(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	setCompletionPolicy(ctx, t, collection, domain.CompletionRollup)

	_, _, first, _ := subtree(ctx, t, collection)
	registry := completionCatalogueFor(ctx, t)
	actor := itemWriter(tenantB, authorB)

	if _, err := registry.Invoke(ctx, "CompleteWorkItem", actor, usecase.Input{"item_id": first}); err != nil {
		t.Fatalf("the first completion: %v", err)
	}
	once := findItem(ctx, t, tenantB, shared.MustParseID(first))

	if _, err := registry.Invoke(ctx, "CompleteWorkItem", actor, usecase.Input{"item_id": first}); err != nil {
		t.Fatalf("the second completion: %v", err)
	}
	twice := findItem(ctx, t, tenantB, shared.MustParseID(first))

	if twice.Version != once.Version {
		t.Errorf("a repeat moved the version from %d to %d", once.Version, twice.Version)
	}
	if !twice.Completion.CompletedAt.Equal(*once.Completion.CompletedAt) {
		t.Error("a repeat moved completed_at")
	}
}
