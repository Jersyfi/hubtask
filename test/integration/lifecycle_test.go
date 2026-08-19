// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The guards a hard delete has to pass, against the real database (B-10): the holds that forbid it,
// and the two records it leaves behind. Plus a cross-tenant negative for each (gate SG-3) - a legal
// hold read across the boundary would not be a wrong answer but somebody else's obligation ignored.

func lifecycleRepo() postgres.LifecycleRepository { return postgres.NewLifecycleRepository() }

func placeHold(
	ctx context.Context, t *testing.T, tenant shared.ID, scope domain.HoldScope, scopeID shared.ID,
) shared.ID {
	t.Helper()
	id := freshID(t)

	var scopeArg any
	if !scopeID.IsZero() {
		scopeArg = scopeID.String()
	}
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO legal_hold (id, tenant_id, scope_kind, scope_id, reason, placed_by)
		VALUES ($1, $2, $3, $4, 'Pending litigation', $5)`,
		id.String(), tenant.String(), string(scope), scopeArg, authorA.String()); err != nil {
		t.Fatalf("placing the hold: %v", err)
	}
	return id
}

func activeHolds(ctx context.Context, t *testing.T, tenant shared.ID) domain.Holds {
	t.Helper()

	var holds domain.Holds
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		holds, err = lifecycleRepo().Active(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading the holds: %v", err)
	}
	return holds
}

func TestAReleasedHoldIsNoLongerInForce(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hubID, _ := hubWithCollection(ctx, t, tenantA, authorA)

	held := placeHold(ctx, t, tenantA, domain.HoldContainer, hubID)

	found := false
	for _, hold := range activeHolds(ctx, t, tenantA) {
		if hold.ID == held {
			found = true
			if hold.Scope != domain.HoldContainer || hold.ScopeID != hubID {
				t.Errorf("the hold reads as %s/%s, want CONTAINER/%s", hold.Scope, hold.ScopeID, hubID)
			}
			if hold.Reason == "" || hold.PlacedAt.IsZero() {
				t.Errorf("the hold carries no reason or no date: %+v", hold)
			}
		}
	}
	if !found {
		t.Fatal("a placed hold is not in force")
	}

	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE legal_hold SET released_at = now(), released_by = $2 WHERE id = $1`,
		held.String(), authorA.String()); err != nil {
		t.Fatalf("releasing the hold: %v", err)
	}
	for _, hold := range activeHolds(ctx, t, tenantA) {
		if hold.ID == held {
			t.Error("a released hold is still in force")
		}
	}
}

// A tenant-wide hold names nothing, which is a NULL rather than a missing row - and it has to read
// back as an empty identifier rather than as an error about a null.
func TestATenantWideHoldNamesNothing(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	held := placeHold(ctx, t, tenantB, domain.HoldTenant, "")

	for _, hold := range activeHolds(ctx, t, tenantB) {
		if hold.ID != held {
			continue
		}
		if !hold.ScopeID.IsZero() {
			t.Errorf("a tenant-wide hold names %q", hold.ScopeID)
		}
		if _, blocked := (domain.Holds{hold}).Blocking(domain.Target{ItemID: freshID(t)}); !blocked {
			t.Error("a tenant-wide hold did not block")
		}
	}
}

// The two records a removal leaves behind, written together. Either on its own is an orphan: a
// journal entry without a tombstone lets a device that was offline recreate the row, and a tombstone
// without a journal entry lets a restore from backup bring it back (ADR-0020 §6).
func TestARemovalWritesBothTheJournalEntryAndTheTombstone(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	itemID, containerID := freshID(t), freshID(t)
	at := created.Add(4 * time.Hour)
	purgeAfter := at.Add(90 * 24 * time.Hour)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return lifecycleRepo().Record(ctx, []domain.Removal{
			{Entity: "work_item", EntityID: itemID, Reason: domain.DeletedByRetention},
			{Entity: "container", EntityID: containerID, Reason: domain.DeletedByRetention},
		}, at, purgeAfter)
	}); err != nil {
		t.Fatalf("recording the removals: %v", err)
	}

	for entity, id := range map[string]shared.ID{"work_item": itemID, "container": containerID} {
		if rows := countIn(ctx, t,
			`SELECT count(*) FROM deletion_journal
			 WHERE tenant_id = $1 AND entity = $2 AND entity_id = $3 AND reason = 'RETENTION'`,
			tenantA.String(), entity, id.String()); rows != 1 {
			t.Errorf("%s has %d journal entries, want 1", entity, rows)
		}
		if rows := countIn(ctx, t,
			`SELECT count(*) FROM tombstone
			 WHERE tenant_id = $1 AND entity = $2 AND entity_id = $3 AND purge_after = $4`,
			tenantA.String(), entity, id.String(), purgeAfter); rows != 1 {
			t.Errorf("%s has %d tombstones with the right window, want 1", entity, rows)
		}
	}
}

// The whole job is at-least-once (ADR-0008): a run that died after writing these and is picked up
// again writes the same rows. A failure there would make the retry impossible rather than harmless.
func TestRecordingTheSameRemovalTwiceIsHarmless(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	itemID := freshID(t)
	at := created.Add(4 * time.Hour)

	record := func() error {
		return write(ctx, t, tenantA, func(ctx context.Context) error {
			return lifecycleRepo().Record(ctx, []domain.Removal{
				{Entity: "work_item", EntityID: itemID, Reason: domain.DeletedByUser},
			}, at, at.Add(90*24*time.Hour))
		})
	}
	if err := record(); err != nil {
		t.Fatalf("the first record: %v", err)
	}
	if err := record(); err != nil {
		t.Fatalf("the second record: %v", err)
	}

	if rows := countIn(ctx, t,
		`SELECT count(*) FROM deletion_journal WHERE tenant_id = $1 AND entity_id = $2`,
		tenantA.String(), itemID.String()); rows != 1 {
		t.Errorf("%d journal entries after two records, want 1", rows)
	}
}

// An empty batch is not a statement. The commonest retention run is the one with nothing to remove.
func TestRecordingNothingIsNotAStatement(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	// A read-only transaction: an INSERT reaching the database here would fail outright, which is
	// what makes this an assertion rather than a hope.
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		return lifecycleRepo().Record(ctx, nil, created, created)
	}); err != nil {
		t.Errorf("recording an empty batch reached the database: %v", err)
	}
}

// The cross-tenant negatives (gate SG-3).
func TestNoTenantsLifecycleRecordsReachAnother(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hubID, _ := hubWithCollection(ctx, t, tenantA, authorA)
	held := placeHold(ctx, t, tenantA, domain.HoldContainer, hubID)
	itemID := freshID(t)

	t.Run("a hold is invisible from another tenant", func(t *testing.T) {
		for _, hold := range activeHolds(ctx, t, tenantB) {
			if hold.ID == held {
				t.Error("another tenant's legal hold is in force here")
			}
		}
	})

	t.Run("a removal is journalled into its own tenant", func(t *testing.T) {
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return lifecycleRepo().Record(ctx, []domain.Removal{
				{Entity: "work_item", EntityID: itemID, Reason: domain.DeletedByUser},
			}, created, created.Add(time.Hour))
		}); err != nil {
			t.Fatalf("recording the removal: %v", err)
		}

		if rows := countIn(ctx, t,
			`SELECT count(*) FROM deletion_journal WHERE tenant_id = $1 AND entity_id = $2`,
			tenantA.String(), itemID.String()); rows != 0 {
			t.Error("a removal recorded in one tenant landed in another")
		}
		if rows := countIn(ctx, t,
			`SELECT count(*) FROM tombstone WHERE tenant_id = $1 AND entity_id = $2`,
			tenantB.String(), itemID.String()); rows != 1 {
			t.Errorf("%d tombstones in the acting tenant, want 1", rows)
		}
	})
}

// The port is one method rather than two, so that a caller cannot write one record and forget the
// other. A removal missing what it needs is a defect in this process rather than something a client
// sent, and is refused as one.
func TestAnIncompleteRemovalIsRefused(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	for _, removal := range []domain.Removal{
		{EntityID: freshID(t), Reason: domain.DeletedByUser},
		{Entity: "work_item", Reason: domain.DeletedByUser},
		{Entity: "work_item", EntityID: freshID(t)},
	} {
		err := write(ctx, t, tenantA, func(ctx context.Context) error {
			return lifecycleRepo().Record(ctx, []domain.Removal{removal}, created, created)
		})
		if err == nil {
			t.Errorf("an incomplete removal was recorded: %+v", removal)
		}
	}

	// Two reasons in one call is two acts written as one, and is refused for the same reason.
	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return lifecycleRepo().Record(ctx, []domain.Removal{
			{Entity: "work_item", EntityID: freshID(t), Reason: domain.DeletedByUser},
			{Entity: "work_item", EntityID: freshID(t), Reason: domain.DeletedByRetention},
		}, created, created)
	})
	if err == nil {
		t.Error("one call recorded two different reasons for the same table")
	}
}

var _ repository.Removals = postgres.LifecycleRepository{}

// The periods are data, seeded by the run rather than by a migration (ADR-0020): a migration covers
// the tenants that existed when it ran and no others, so the first tenant created afterwards would
// be one with no policy at all.
func TestTheDefaultPeriodsAreSeededAndNotOverwritten(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	ensure := func(tenant shared.ID) {
		t.Helper()
		if err := write(ctx, t, tenant, func(ctx context.Context) error {
			return lifecycleRepo().Ensure(ctx, domain.DefaultPolicies())
		}); err != nil {
			t.Fatalf("seeding the policies: %v", err)
		}
	}
	ensure(tenantA)

	policy := findPolicy(ctx, t, tenantA, domain.KindTrash)
	if policy.RetainDays != 30 || policy.MinDays != 7 {
		t.Errorf("the seeded period is %d days with a floor of %d, want 30 and 7",
			policy.RetainDays, policy.MinDays)
	}

	// A tenant that has decided something has decided it. A sweep that reset the period every time
	// it ran would be one that quietly overrode the tenant it is running for.
	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE retention_policy SET retain_days = 14 WHERE tenant_id = $1 AND data_kind = 'TRASH'`,
		tenantA.String()); err != nil {
		t.Fatalf("changing the period: %v", err)
	}
	ensure(tenantA)

	if policy := findPolicy(ctx, t, tenantA, domain.KindTrash); policy.RetainDays != 14 {
		t.Errorf("the period was reset to %d days, want the tenant's 14", policy.RetainDays)
	}
}

// A tenant with no policy is told so rather than answered with a zero, which would be a trash
// emptied the moment something landed in it.
func TestATenantWithNoPolicyIsToldSo(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	err := read(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := lifecycleRepo().Find(ctx, domain.DataKind("COMMENT"))
		return err
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a missing policy reported %v, want not found", err)
	}
}

// The cross-tenant negative: one tenant's period is not another's, and seeding one does not seed the
// other (gate SG-3).
func TestAPeriodBelongsToOneTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return lifecycleRepo().Ensure(ctx, []domain.Policy{
			{DataKind: domain.DataKind("SESSION"), RetainDays: 30, MinDays: 1},
		})
	}); err != nil {
		t.Fatalf("seeding the policy: %v", err)
	}

	err := read(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := lifecycleRepo().Find(ctx, domain.DataKind("SESSION"))
		return err
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("another tenant's period is visible here: %v", err)
	}
}

func findPolicy(
	ctx context.Context, t *testing.T, tenant shared.ID, kind domain.DataKind,
) domain.Policy {
	t.Helper()

	var policy domain.Policy
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		policy, err = lifecycleRepo().Find(ctx, kind)
		return err
	}); err != nil {
		t.Fatalf("reading the policy: %v", err)
	}
	return policy
}
