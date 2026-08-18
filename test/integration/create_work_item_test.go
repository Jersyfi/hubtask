// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The whole use case against a real database, with the seeded capability profiles deciding the
// hierarchy - which is the point of B-03: the rules are the rows in item_capability_profile that
// db/migrations/0002 wrote, not constants in the code.
//
// These tests work in tenant B. The package shares one database, and tenant A carries a narrowed
// TASK profile that capability_profile_test.go seeds - deliberately, because that suite is about
// narrowing. Working in A would mean testing the system matrix against a tenant that has replaced
// it, which is a different question and a confusing way to ask it.

func itemCatalogueFor(ctx context.Context, t *testing.T) *usecase.Registry {
	t.Helper()

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	ids := clockadapter.NewUUIDv7(fixed)
	hybrid, err := clockadapter.NewHybridClock(fixed, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	sink := postgres.NewAuditSink(ids)

	registry, err := usecase.NewRegistry(nil, work.CreateWorkItem{
		Items:      postgres.NewItemRepository(),
		Containers: postgres.NewContainerRepository(),
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
		UnitOfWork: unitOfWork,
		Clock:      fixed,
		IDs:        ids,
		HLC:        hybrid,
	}.Descriptor())
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	return registry
}

func itemWriter(tenant, account shared.ID) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenant, AccountID: account,
		AccountName: "Anna Beispiel", Scopes: []string{"items:write"},
	}
}

// The acceptance criterion of B-03, against the seeded profiles: all three levels can be created,
// each under the parent the matrix permits.
func TestTheThreeLevelsAreCreatedAgainstTheSeededProfiles(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	registry := itemCatalogueFor(ctx, t)
	actor := itemWriter(tenantB, authorB)

	task, err := registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "TASK", "collection_id": collection.String(), "title": "Weekly shop",
		"notes": "Before Friday",
	})
	if err != nil {
		t.Fatalf("the task: %v", err)
	}

	pkg, err := registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "WORK_PACKAGE", "parent_id": task.String("id"), "title": "Dairy aisle",
	})
	if err != nil {
		t.Fatalf("the work package: %v", err)
	}

	activity, err := registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "ACTIVITY", "parent_id": pkg.String("id"), "title": "Milk",
	})
	if err != nil {
		t.Fatalf("the activity: %v", err)
	}

	if task.Int("depth") != 1 || pkg.Int("depth") != 2 || activity.Int("depth") != 3 {
		t.Errorf("depths = %d, %d, %d", task.Int("depth"), pkg.Int("depth"), activity.Int("depth"))
	}
	// The collection reaches every level without the client repeating it.
	for _, out := range []usecase.Output{pkg, activity} {
		if out.String("collection_id") != collection.String() {
			t.Errorf("collection = %s, want %s", out.String("collection_id"), collection)
		}
	}
	// The prefix query the materialised path exists for: the whole subtree in one index scan.
	if rows := countIn(ctx, t,
		`SELECT count(*) FROM work_item WHERE tenant_id = $1 AND path LIKE $2 || '%'`,
		tenantB.String(), task.String("path")); rows != 3 {
		t.Errorf("%d items under the task, want 3", rows)
	}
}

// One write owes four things, proven against the database rather than against fakes.
func TestCreatingAnItemWritesEverythingInOneTransaction(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	title := freshName(t)

	out, err := itemCatalogueFor(ctx, t).Invoke(ctx, "CreateWorkItem", itemWriter(tenantB, authorB),
		usecase.Input{"type": "TASK", "collection_id": collection.String(), "title": title})
	if err != nil {
		t.Fatalf("creating the task: %v", err)
	}

	id := out.String("id")
	if id == "" || out.String("order_key") == "" || out.Int("version") != 1 {
		t.Fatalf("unexpected result: %v", out)
	}

	if rows := countIn(ctx, t, `SELECT count(*) FROM work_item WHERE id = $1 AND tenant_id = $2`,
		id, tenantB.String()); rows != 1 {
		t.Errorf("%d item rows", rows)
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM outbox_event WHERE subject = $1`,
		"item/"+id); rows != 1 {
		t.Errorf("%d events for the new item", rows)
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM change_log WHERE entity_id = $1`, id); rows != 1 {
		t.Errorf("%d change log entries for the new item", rows)
	}
	if rows := countIn(ctx, t,
		`SELECT count(*) FROM audit_log WHERE target_id = $1 AND action = 'item.created'`,
		id); rows != 1 {
		t.Errorf("%d audit entries for the new item", rows)
	}

	// The change is filed under the collection, which is the visibility filter a pull applies.
	var container string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT container_id::text FROM change_log WHERE entity_id = $1`, id).Scan(&container); err != nil {
		t.Fatalf("reading the change: %v", err)
	}
	if container != collection.String() {
		t.Errorf("the change is filed under %s rather than the collection", container)
	}

	// Rule 10 where it is easiest to break: the trail carries the actor's name and not the item's
	// title.
	var actorLabel, changes string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT actor_label, changes::text FROM audit_log WHERE target_id = $1`, id).
		Scan(&actorLabel, &changes); err != nil {
		t.Fatalf("reading the audit entry: %v", err)
	}
	if actorLabel != "Anna Beispiel" {
		t.Errorf("actor label %q", actorLabel)
	}
	if strings.Contains(changes, title) {
		t.Errorf("the item title reached the trail: %s", changes)
	}
}

// The acceptance criterion read backwards, against the seeded profiles: every forbidden
// combination is refused with the code that says which reason it was.
func TestEveryForbiddenPlacementIsRefusedAgainstTheDatabase(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	registry := itemCatalogueFor(ctx, t)
	actor := itemWriter(tenantB, authorB)

	task, err := registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "TASK", "collection_id": collection.String(), "title": freshName(t),
	})
	if err != nil {
		t.Fatalf("seeding the task: %v", err)
	}

	cases := []struct {
		name       string
		input      usecase.Input
		wantDetail string
	}{
		{
			name: "an activity directly in a task",
			input: usecase.Input{
				"type": "ACTIVITY", "parent_id": task.String("id"), "title": "Milk",
			},
			wantDetail: "items.parent_type_invalid",
		},
		{
			name: "a work package with no parent",
			input: usecase.Input{
				"type": "WORK_PACKAGE", "collection_id": collection.String(), "title": "Dairy",
			},
			wantDetail: "items.parent_item_required",
		},
		{
			name: "a note on a task's activity, whose profile has none",
			input: usecase.Input{
				"type": "ACTIVITY", "parent_id": task.String("id"), "title": "Milk",
				"notes": "Semi-skimmed",
			},
			// The parent is refused first, so the note never gets its say - which is the right
			// order: there is no activity to ask about until it has somewhere to sit.
			wantDetail: "items.parent_type_invalid",
		},
	}

	before := countIn(ctx, t, `SELECT count(*) FROM work_item WHERE tenant_id = $1`, tenantB.String())

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := registry.Invoke(ctx, "CreateWorkItem", actor, c.input)
			if got := shared.AsError(err).DetailCode; got != c.wantDetail {
				t.Errorf("error = %v, want %s", err, c.wantDetail)
			}
		})
	}

	if after := countIn(ctx, t, `SELECT count(*) FROM work_item WHERE tenant_id = $1`,
		tenantB.String()); after != before {
		t.Errorf("%d items after the refusals, want %d - a refusal wrote a row", after, before)
	}
}

// The capability gate, at the level where the seeded profiles actually differ: an activity has no
// notes, and saying so is refused rather than dropped (ADR-0006).
func TestANoteOnAnActivityIsRefusedByTheSeededProfile(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	registry := itemCatalogueFor(ctx, t)
	actor := itemWriter(tenantB, authorB)

	task, err := registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "TASK", "collection_id": collection.String(), "title": freshName(t),
	})
	if err != nil {
		t.Fatalf("seeding the task: %v", err)
	}
	pkg, err := registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "WORK_PACKAGE", "parent_id": task.String("id"), "title": freshName(t),
		"notes": "A work package does have notes",
	})
	if err != nil {
		t.Fatalf("seeding the work package: %v", err)
	}

	_, err = registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "ACTIVITY", "parent_id": pkg.String("id"), "title": "Milk",
		"notes": "Semi-skimmed",
	})
	if !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Fatalf("error = %v, want capability_not_supported", err)
	}

	// Without the note it is accepted, so the profile refused the field and not the level.
	if _, err := registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "ACTIVITY", "parent_id": pkg.String("id"), "title": "Milk",
	}); err != nil {
		t.Errorf("an activity without a note was refused: %v", err)
	}
}

// A field no use case writes yet is refused by name rather than accepted and dropped.
func TestAFieldNoUseCaseWritesYetIsRefusedByName(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)

	_, err := itemCatalogueFor(ctx, t).Invoke(ctx, "CreateWorkItem", itemWriter(tenantB, authorB),
		usecase.Input{
			"type": "TASK", "collection_id": collection.String(), "title": "Buy milk",
			"due_at": "2026-09-01T09:00:00Z",
		})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error = %v, want a validation error", err)
	}

	fields := shared.AsError(err).Fields
	if len(fields) != 1 || fields[0].Path != "/due_at" {
		t.Errorf("field errors = %v, want one naming /due_at", fields)
	}
}

// The tenant boundary through the whole use case: tenant B cannot create an item in tenant A's
// collection, and the collection is not found rather than forbidden - anything else confirms that
// another tenant's data exists (multi-tenancy.md §2).
func TestAnItemCannotBeCreatedInAnotherTenantsCollection(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	// The collection belongs to the other tenant this time, and the actor is the one working here.
	collection := collectionFor(ctx, t, tenantA, authorA)

	_, err := itemCatalogueFor(ctx, t).Invoke(ctx, "CreateWorkItem", itemWriter(tenantB, authorB),
		usecase.Input{
			"type": "TASK", "collection_id": collection.String(), "title": "Buy milk",
		})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("error = %v, want not found", err)
	}
	if got := shared.AsError(err).DetailCode; got != "items.collection_not_found" {
		t.Errorf("detail code = %s", got)
	}

	if rows := countIn(ctx, t, `SELECT count(*) FROM work_item WHERE collection_id = $1`,
		collection.String()); rows != 0 {
		t.Errorf("%d items in tenant A's collection", rows)
	}
}

// The permission check is not decoration: an account with no role in this tenant is refused, and
// the refusal is recorded even though nothing else was written (test AT-3).
func TestAnAccountWithoutARoleCannotCreateAnItem(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	stranger := freshID(t)

	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Stranger')`,
		stranger.String(), tenantB.String()); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}

	before := countIn(ctx, t,
		`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND outcome = 'DENIED'`, tenantB.String())

	_, err := itemCatalogueFor(ctx, t).Invoke(ctx, "CreateWorkItem", itemWriter(tenantB, stranger),
		usecase.Input{"type": "TASK", "collection_id": collection.String(), "title": "Buy milk"})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error = %v, want forbidden", err)
	}

	after := countIn(ctx, t,
		`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND outcome = 'DENIED'`, tenantB.String())
	if after != before+1 {
		t.Errorf("%d denied entries, want %d - a refusal that leaves no record is invisible to an auditor",
			after, before)
	}
}
