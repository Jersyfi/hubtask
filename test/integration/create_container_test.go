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
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/infrastructure/automation"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The whole use case, against a real database: the row, the event, the change log entry and the
// audit entry in one transaction, with row level security underneath. Everything below the
// adapters is the production wiring - only the clock is fixed.

func catalogueFor(t *testing.T) *usecase.Registry {
	t.Helper()
	ctx := context.Background()

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	ids := clockadapter.NewUUIDv7(fixed)
	hybrid, err := clockadapter.NewHybridClock(fixed, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	sink := postgres.NewAuditSink(ids)

	registry, err := usecase.NewRegistry(nil, work.CreateContainer{
		Containers: containerRepo(),
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

func administrator(tenant, account shared.ID) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenant, AccountID: account,
		AccountName: "Anna Beispiel", Scopes: []string{"containers:write"},
	}
}

func countIn(ctx context.Context, t *testing.T, query string, args ...any) int {
	t.Helper()

	var count int
	if err := adminPool(ctx, t).QueryRow(ctx, query, args...).Scan(&count); err != nil {
		t.Fatalf("counting: %v", err)
	}
	return count
}

// One write owes four things, and here they are proven against the database rather than against
// fakes: the container, the event, the change for offline clients, and the audit entry.
func TestCreatingAHubWritesEverythingInOneTransaction(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	name := freshName(t)

	out, err := catalogueFor(t).Invoke(ctx, "CreateContainer", administrator(tenantA, authorA),
		usecase.Input{"type": "HUB", "name": name, "description": "Everything personal"})
	if err != nil {
		t.Fatalf("creating the hub: %v", err)
	}

	id := out.String("id")
	if id == "" || out.String("order_key") == "" || out.Int("version") != 1 {
		t.Fatalf("unexpected result: %v", out)
	}

	if rows := countIn(ctx, t, `SELECT count(*) FROM container WHERE id = $1 AND tenant_id = $2`,
		id, tenantA.String()); rows != 1 {
		t.Errorf("%d container rows", rows)
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM outbox_event WHERE subject = $1`,
		"container/"+id); rows != 1 {
		t.Errorf("%d events for the new hub", rows)
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM change_log WHERE entity_id = $1`, id); rows != 1 {
		t.Errorf("%d change log entries for the new hub", rows)
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM audit_log WHERE target_id = $1 AND action = 'container.created'`,
		id); rows != 1 {
		t.Errorf("%d audit entries for the new hub", rows)
	}

	// Rule 10 where it is easiest to break: the trail carries the actor's name and not the
	// container's.
	var actorLabel string
	var changes string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT actor_label, changes::text FROM audit_log WHERE target_id = $1`, id).
		Scan(&actorLabel, &changes); err != nil {
		t.Fatalf("reading the audit entry: %v", err)
	}
	if actorLabel != "Anna Beispiel" {
		t.Errorf("actor label %q", actorLabel)
	}
	if strings.Contains(changes, name) {
		t.Errorf("the container name reached the trail: %s", changes)
	}
}

// Test AT-5 against the database: a write that fails takes its audit entry and its event with it.
// The second container with the same name is refused by the unique index, after the event and the
// entry would already have been written had they not been in the same transaction.
func TestAFailedWriteLeavesNothingBehind(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := catalogueFor(t)
	name := freshName(t)
	actor := administrator(tenantA, authorA)

	if _, err := registry.Invoke(ctx, "CreateContainer", actor,
		usecase.Input{"type": "HUB", "name": name}); err != nil {
		t.Fatalf("creating the first hub: %v", err)
	}

	before := countIn(ctx, t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1`, tenantA.String())
	events := countIn(ctx, t, `SELECT count(*) FROM outbox_event WHERE tenant_id = $1`, tenantA.String())

	_, err := registry.Invoke(ctx, "CreateContainer", actor, usecase.Input{"type": "HUB", "name": name})
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("error %v, want a conflict on the duplicate name", err)
	}

	if after := countIn(ctx, t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1`, tenantA.String()); after != before {
		t.Errorf("%d audit entries after the failed write, want %d", after, before)
	}
	if after := countIn(ctx, t, `SELECT count(*) FROM outbox_event WHERE tenant_id = $1`, tenantA.String()); after != events {
		t.Errorf("%d events after the failed write, want %d", after, events)
	}
}

// The permission check is not decoration: an account with no role in this tenant is refused, and
// the refusal is recorded even though nothing else was written (test AT-3).
func TestAnAccountWithoutARoleIsRefusedAndRecorded(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	stranger := freshID(t)

	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Stranger')`,
		stranger.String(), tenantA.String()); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}

	before := countIn(ctx, t,
		`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND outcome = 'DENIED'`, tenantA.String())

	_, err := catalogueFor(t).Invoke(ctx, "CreateContainer", administrator(tenantA, stranger),
		usecase.Input{"type": "HUB", "name": freshName(t)})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want forbidden", err)
	}

	after := countIn(ctx, t,
		`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND outcome = 'DENIED'`, tenantA.String())
	if after != before+1 {
		t.Errorf("%d denied entries, want %d - a refusal that leaves no record is invisible to an auditor",
			after, before)
	}
}

// Test AT-6 for real: the same use case reached as an automation action produces the same writes
// as the catalogue call a REST request makes.
func TestTheAutomationChannelWritesWhatTheDirectCallWrites(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := catalogueFor(t)

	dispatcher := automation.NewActionDispatcher(registry)
	rule := administrator(tenantA, authorA)
	rule.Kind = appshared.ActorAutomation

	out, err := dispatcher.Dispatch(ctx, rule, automation.Action{
		Kind:   "CREATE_CONTAINER",
		Params: map[string]any{"type": "HUB", "name": freshName(t)},
	})
	if err != nil {
		t.Fatalf("the action failed: %v", err)
	}

	id := out.String("id")
	for table, query := range map[string]string{
		"container":    `SELECT count(*) FROM container WHERE id = $1`,
		"change_log":   `SELECT count(*) FROM change_log WHERE entity_id = $1`,
		"audit_log":    `SELECT count(*) FROM audit_log WHERE target_id = $1`,
		"outbox_event": `SELECT count(*) FROM outbox_event WHERE payload->>'id' = $1`,
	} {
		if rows := countIn(ctx, t, query, id); rows != 1 {
			t.Errorf("%d rows in %s for a container created by automation", rows, table)
		}
	}

	// The actor type is what tells an auditor that a rule did this rather than a person.
	var actorType string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT actor_type FROM audit_log WHERE target_id = $1`, id).Scan(&actorType); err != nil {
		t.Fatalf("reading the audit entry: %v", err)
	}
	if actorType != string(appshared.ActorAutomation) {
		t.Errorf("actor type %q, want AUTOMATION", actorType)
	}
}

// A collection under a hub of another tenant is not a permission problem but an absence: the hub
// is invisible, and invisible reads as absent (multi-tenancy.md §2).
func TestAHubOfAnotherTenantIsNotFound(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := catalogueFor(t)

	foreign, err := registry.Invoke(ctx, "CreateContainer", administrator(tenantA, authorA),
		usecase.Input{"type": "HUB", "name": freshName(t)})
	if err != nil {
		t.Fatalf("creating the hub: %v", err)
	}

	// Tenant B holds the role it would need, in its own tenant.
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO membership (id, tenant_id, account_id, scope_type, role)
		 VALUES ($1, $2, $3, 'TENANT', 'ADMIN') ON CONFLICT (id) DO NOTHING`,
		freshID(t).String(), tenantB.String(), authorB.String()); err != nil {
		t.Fatalf("seeding the membership: %v", err)
	}

	_, err = registry.Invoke(ctx, "CreateContainer", administrator(tenantB, authorB),
		usecase.Input{"type": "COLLECTION", "parent_id": foreign.String("id"), "name": freshName(t)})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("error %v, want not found", err)
	}
	if code := shared.AsError(err).DetailCode; code != "containers.parent_not_found" {
		t.Errorf("detail code %s", code)
	}
}

// The ranks come out of the domain and the database sorts them the same way - which is what the
// order key exists for.
func TestCollectionsAreRankedAfterTheirSiblings(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := catalogueFor(t)
	actor := administrator(tenantA, authorA)

	hub, err := registry.Invoke(ctx, "CreateContainer", actor,
		usecase.Input{"type": "HUB", "name": freshName(t)})
	if err != nil {
		t.Fatalf("creating the hub: %v", err)
	}

	var previous string
	for range 3 {
		collection, err := registry.Invoke(ctx, "CreateContainer", actor, usecase.Input{
			"type": "COLLECTION", "parent_id": hub.String("id"), "name": freshName(t),
		})
		if err != nil {
			t.Fatalf("creating the collection: %v", err)
		}
		if key := collection.String("order_key"); key <= previous {
			t.Fatalf("order key %q does not sort after %q", key, previous)
		} else {
			previous = key
		}
	}

	// And the change log entry of a collection filters on its hub, so a device subscribed to the
	// hub sees it appear.
	var containerID string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT container_id::text FROM change_log WHERE entity_id = $1`,
		mustLastCollection(ctx, t, hub.String("id"))).Scan(&containerID); err != nil {
		t.Fatalf("reading the change log: %v", err)
	}
	if containerID != hub.String("id") {
		t.Errorf("the change filters on %s rather than on the hub", containerID)
	}
}

func mustLastCollection(ctx context.Context, t *testing.T, hubID string) string {
	t.Helper()

	var id string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT id::text FROM container WHERE parent_id = $1 ORDER BY order_key DESC LIMIT 1`,
		hubID).Scan(&id); err != nil {
		t.Fatalf("reading the last collection: %v", err)
	}
	return id
}

// A type the capability matrix does not know is refused before anything is written, and the
// refusal names the field - which is what lets a client, or an agent, correct itself.
func TestAnInvalidInputIsRefusedByTheCatalogue(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)

	_, err := catalogueFor(t).Invoke(ctx, "CreateContainer", administrator(tenantA, authorA),
		usecase.Input{"type": "PROJECT", "name": freshName(t)})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want a validation error", err)
	}
	if fields := shared.AsError(err).Fields; len(fields) != 1 || fields[0].Path != "/type" {
		t.Errorf("the finding does not point at the field: %+v", fields)
	}

	// And the domain's own vocabulary is what the enum was built from.
	if len(domain.ContainerTypes()) != 2 {
		t.Errorf("the container types changed without this test: %v", domain.ContainerTypes())
	}
}
