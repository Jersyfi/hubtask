// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

func announcement(t *testing.T, tenant shared.ID, container work.Container) event.Envelope {
	t.Helper()
	envelope, err := event.NewContainerCreated(freshID(t), container,
		event.Actor{Kind: shared.ActorUser, ID: authorA}, created, event.Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}
	return envelope
}

func outboxRows(ctx context.Context, t *testing.T, tenant shared.ID) map[string]string {
	t.Helper()
	admin := adminPool(ctx, t)

	rows, err := admin.Query(ctx,
		`SELECT id::text, event_type, subject, payload::text, dispatched_at IS NULL
		 FROM outbox_event WHERE tenant_id = $1`, tenant.String())
	if err != nil {
		t.Fatalf("reading the outbox: %v", err)
	}
	defer rows.Close()

	found := map[string]string{}
	for rows.Next() {
		var id, eventType, subject, payload string
		var pending bool
		if err := rows.Scan(&id, &eventType, &subject, &payload, &pending); err != nil {
			t.Fatalf("scanning the outbox: %v", err)
		}
		if !pending {
			t.Errorf("event %s was written as already dispatched", id)
		}
		found[id] = eventType + " " + subject + " " + payload
	}
	return found
}

// An event is written with its payload, its subject and its causal chain, and it is pending -
// nothing here delivers it, which is the whole point of an outbox (ADR-0007).
func TestAnEventIsWrittenIntoTheOutbox(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	name := freshName(t)

	container := containerIn(tenantA, authorA, freshID(t), name, "a0")
	envelope := announcement(t, tenantA, container)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.NewOutbox().Append(ctx, envelope)
	}); err != nil {
		t.Fatalf("writing the event: %v", err)
	}

	row, found := outboxRows(ctx, t, tenantA)[envelope.ID.String()]
	if !found {
		t.Fatal("the event is not in the outbox")
	}
	if !strings.Contains(row, string(event.ContainerCreated)) {
		t.Errorf("the event type did not survive: %s", row)
	}
	if !strings.Contains(row, "container/"+container.ID.String()) {
		t.Errorf("the subject did not survive: %s", row)
	}
	if !strings.Contains(row, name) {
		t.Errorf("the snapshot did not survive: %s", row)
	}
}

// The cross-tenant negative test for Append: the tenant comes from the transaction, so an event
// cannot be written into another tenant's stream - a subscriber of tenant A must not be able to
// be fed by tenant B.
func TestAnEventCannotBeWrittenIntoAnotherTenantsStream(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	// The container claims tenant A; the transaction belongs to tenant B.
	envelope := announcement(t, tenantA, containerIn(tenantA, authorA, freshID(t), freshName(t), "a0"))

	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return postgres.NewOutbox().Append(ctx, envelope)
	}); err != nil {
		t.Fatalf("writing the event: %v", err)
	}

	if _, found := outboxRows(ctx, t, tenantA)[envelope.ID.String()]; found {
		t.Error("the event landed in tenant A although the transaction was tenant B's")
	}
	if _, found := outboxRows(ctx, t, tenantB)[envelope.ID.String()]; !found {
		t.Error("the event is not in the tenant that wrote it")
	}
}

// The change log is the other half of a write: the same snapshot, a different recipient
// (offline-sync.md §10).
func TestAChangeIsRecordedWithItsClock(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	container := containerIn(tenantA, authorA, freshID(t), freshName(t), "a0")
	reading, err := shared.HLC{}.Tick(created, "server-1")
	if err != nil {
		t.Fatalf("stamping: %v", err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.NewChangeLog().Record(ctx, changelog.Change{
			TenantID: tenantA, Entity: "container", EntityID: container.ID,
			Op: changelog.Upsert, ContainerID: container.ID, ActorID: authorA,
			HLC: reading, Payload: map[string]any{"name": container.Name},
		})
	}); err != nil {
		t.Fatalf("recording the change: %v", err)
	}

	admin := adminPool(ctx, t)
	var seq int64
	var hlc, op string
	err = admin.QueryRow(ctx, `
		SELECT seq, hlc, op FROM change_log
		WHERE tenant_id = $1 AND entity_id = $2`, tenantA.String(), container.ID.String()).
		Scan(&seq, &hlc, &op)
	if err != nil {
		t.Fatalf("reading the change log: %v", err)
	}

	if seq <= 0 {
		t.Errorf("the cursor value is %d - a client cannot page on it", seq)
	}
	if hlc != reading.String() {
		t.Errorf("the clock reading is %q, want %q", hlc, reading.String())
	}
	if op != string(changelog.Upsert) {
		t.Errorf("operation %q", op)
	}
}

// The cross-tenant negative test for Record: a change is written into the tenant of the
// transaction, so `:pull` in another tenant can never see it (test SY-11).
func TestAChangeCannotBeRecordedForAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	entityID := freshID(t)
	reading, err := shared.HLC{}.Tick(created, "server-1")
	if err != nil {
		t.Fatalf("stamping: %v", err)
	}

	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return postgres.NewChangeLog().Record(ctx, changelog.Change{
			TenantID: tenantA, Entity: "container", EntityID: entityID,
			Op: changelog.Upsert, HLC: reading,
		})
	}); err != nil {
		t.Fatalf("recording the change: %v", err)
	}

	admin := adminPool(ctx, t)
	var inA, inB int
	if err := admin.QueryRow(ctx,
		`SELECT count(*) FROM change_log WHERE tenant_id = $1 AND entity_id = $2`,
		tenantA.String(), entityID.String()).Scan(&inA); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if err := admin.QueryRow(ctx,
		`SELECT count(*) FROM change_log WHERE tenant_id = $1 AND entity_id = $2`,
		tenantB.String(), entityID.String()).Scan(&inB); err != nil {
		t.Fatalf("counting: %v", err)
	}

	if inA != 0 {
		t.Error("the change landed in tenant A although the transaction was tenant B's")
	}
	if inB != 1 {
		t.Errorf("%d changes in the tenant that wrote it, want 1", inB)
	}
}

// A change without a clock reading cannot be merged against a concurrent edit, so it is refused
// rather than written as unmergeable.
func TestAChangeWithoutAClockIsRefused(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.NewChangeLog().Record(ctx, changelog.Change{
			TenantID: tenantA, Entity: "container", EntityID: freshID(t), Op: changelog.Upsert,
		})
	})
	if err == nil || shared.AsError(err).DetailCode != "sync.change_without_clock" {
		t.Fatalf("error %v, want the change to be refused", err)
	}
}
