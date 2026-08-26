// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The REPEATABLE READ snapshot an export reads under (E-04, backup-restore.md §5). Without it a
// run that reads containers, then items three minutes later, then comments after that produces an
// archive in which an item belongs to a container that does not exist yet - which restores as a
// foreign key violation on the worst possible day.

func snapshotUnitOfWork(ctx context.Context, t *testing.T) *postgres.UnitOfWork {
	t.Helper()
	return postgres.NewUnitOfWork(appPool(ctx, t))
}

// The acceptance criterion: a write concurrent with an export appears wholly or not at all.
func TestAWriteDuringASnapshotIsInvisibleToIt(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	before := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return containerRepo().Insert(ctx, containerIn(tenantA, authorA, before, freshName(t), "ae"))
	}); err != nil {
		t.Fatalf("seeding the container that was already there: %v", err)
	}

	during := freshID(t)
	var seenBefore, seenDuring error

	err := snapshotUnitOfWork(ctx, t).WithinSnapshot(ctx, persistence.Scope{TenantID: tenantA},
		func(snapshotCtx context.Context, _ time.Time) error {
			// The snapshot is taken on the first read, so read once before anything else happens.
			if _, err := containerRepo().Find(snapshotCtx, before); err != nil {
				return err
			}

			// Now somebody commits, on another connection, while the snapshot is open. This is
			// the ordinary case rather than a contrived one: a backup runs for minutes and the
			// installation keeps working.
			if err := write(ctx, t, tenantA, func(writeCtx context.Context) error {
				return containerRepo().Insert(writeCtx, containerIn(tenantA, authorA, during, freshName(t), "af"))
			}); err != nil {
				return err
			}

			_, seenBefore = containerRepo().Find(snapshotCtx, before)
			_, seenDuring = containerRepo().Find(snapshotCtx, during)
			return nil
		})
	if err != nil {
		t.Fatalf("the snapshot: %v", err)
	}

	if seenBefore != nil {
		t.Fatalf("a row that was there before the snapshot went missing inside it: %v", seenBefore)
	}
	if !errors.Is(seenDuring, shared.ErrNotFound) {
		t.Fatalf("a row committed during the snapshot was visible to it: %v", seenDuring)
	}

	// And it really was committed - the snapshot did not simply lose it.
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := containerRepo().Find(ctx, during)
		return err
	}); err != nil {
		t.Fatalf("the concurrent write did not land at all: %v", err)
	}
}

// The instant comes from the database rather than from the process, because it is the clock the
// rows' own timestamps were written by. A process clock a second ahead would leave a hole in the
// chain of incrementals that nothing would ever report.
func TestTheSnapshotInstantComesFromTheDatabase(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	var first, second time.Time
	unitOfWork := snapshotUnitOfWork(ctx, t)

	for _, at := range []*time.Time{&first, &second} {
		if err := unitOfWork.WithinSnapshot(ctx, persistence.Scope{TenantID: tenantA},
			func(_ context.Context, taken time.Time) error {
				*at = taken
				return nil
			}); err != nil {
			t.Fatalf("the snapshot: %v", err)
		}
	}

	switch {
	case first.IsZero() || second.IsZero():
		t.Fatal("a snapshot without an instant")
	case first.Location() != time.UTC:
		t.Fatalf("the instant is not UTC: %v", first)
	case !second.After(first):
		t.Fatalf("two snapshots, one instant: %v and %v", first, second)
	case time.Since(second) > time.Minute:
		t.Fatalf("the instant is nowhere near now: %v", second)
	}
}

// A snapshot cannot join a running transaction: the isolation level is fixed when a transaction
// begins, so joining would quietly hand back READ COMMITTED under a method that promises otherwise.
func TestASnapshotRefusesToJoinARunningTransaction(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	unitOfWork := snapshotUnitOfWork(ctx, t)

	err := unitOfWork.Within(ctx, persistence.Scope{TenantID: tenantA}, func(inner context.Context) error {
		return unitOfWork.WithinSnapshot(inner, persistence.Scope{TenantID: tenantA},
			func(context.Context, time.Time) error { return nil })
	})
	if !errors.Is(err, shared.ErrInternal) {
		t.Fatalf("a snapshot joined a running transaction: %v", err)
	}
}

// Read-only is enforced by the database, so an export that tried to write would fail loudly rather
// than quietly succeeding.
func TestASnapshotCannotWrite(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	err := snapshotUnitOfWork(ctx, t).WithinSnapshot(ctx, persistence.Scope{TenantID: tenantA},
		func(snapshotCtx context.Context, _ time.Time) error {
			return containerRepo().Insert(snapshotCtx, containerIn(tenantA, authorA, freshID(t), freshName(t), "ag"))
		})
	if err == nil {
		t.Fatal("a write inside a snapshot succeeded")
	}
}

// The tenant boundary holds under a snapshot exactly as it does under a transaction: the wrapper
// sets the same context, and row level security does not care which isolation level asked.
func TestASnapshotSeesOnlyItsOwnTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	inA := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return containerRepo().Insert(ctx, containerIn(tenantA, authorA, inA, freshName(t), "ah"))
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	var seen error
	if err := snapshotUnitOfWork(ctx, t).WithinSnapshot(ctx, persistence.Scope{TenantID: tenantB},
		func(snapshotCtx context.Context, _ time.Time) error {
			_, seen = containerRepo().Find(snapshotCtx, inA)
			return nil
		}); err != nil {
		t.Fatalf("the snapshot: %v", err)
	}
	if !errors.Is(seen, shared.ErrNotFound) {
		t.Fatalf("tenant B read tenant A's container under a snapshot: %v", seen)
	}
}

// A scope that cannot bound a transaction cannot bound a snapshot either. Failing closed is the
// rule: without a tenant, row level security returns nothing and the caller reads that as "the
// tenant is empty" - which, in a backup, is an empty archive nobody notices.
func TestASnapshotWithoutAScopeIsRefused(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	err := snapshotUnitOfWork(ctx, t).WithinSnapshot(ctx, persistence.Scope{},
		func(context.Context, time.Time) error { return nil })
	if !errors.Is(err, shared.ErrInternal) {
		t.Fatalf("a snapshot without a scope: %v", err)
	}
}
