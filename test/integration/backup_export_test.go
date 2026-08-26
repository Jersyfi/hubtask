// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The statements an archive reads a tenant through (E-05): they page on a key rather than an
// offset, they answer only the tenant the transaction is bound to, and a table that cannot date a
// change answers everything.

func exportRepo(batch int) postgres.BackupExportRepository {
	return postgres.NewBackupExportRepository(batch)
}

// exported collects one table's rows inside a snapshot, which is how a backup run reads them.
func exported(ctx context.Context, t *testing.T, tenant shared.ID, batch int, table string, since time.Time) []repository.Row {
	t.Helper()

	var rows []repository.Row
	err := postgres.NewUnitOfWork(appPool(ctx, t)).WithinSnapshot(ctx, persistence.Scope{TenantID: tenant},
		func(snapshotCtx context.Context, _ time.Time) error {
			return exportRepo(batch).Rows(snapshotCtx, table, since, func(row repository.Row) error {
				rows = append(rows, row)
				return nil
			})
		})
	if err != nil {
		t.Fatalf("exporting %s: %v", table, err)
	}
	return rows
}

// A page smaller than the data is the whole point: every row comes back exactly once, in order,
// however small the page is.
func TestTheExportPagesWithoutRepeatingOrSkippingARow(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	written := map[shared.ID]bool{}
	for i := range 7 {
		id := freshID(t)
		written[id] = true
		hub := containerIn(tenantA, authorA, id, freshName(t), "a"+string(rune('0'+i)))
		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			return containerRepo().Insert(ctx, hub)
		}); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}

	seen := map[string]int{}
	for _, row := range exported(ctx, t, tenantA, 2, "container", time.Time{}) {
		seen[row.ID]++
	}

	for id := range written {
		switch seen[id.String()] {
		case 1:
		case 0:
			t.Errorf("%s was never exported", id)
		default:
			t.Errorf("%s was exported %d times", id, seen[id.String()])
		}
	}
}

// tenant_id is taken out on the way, because a restore into another tenant must not carry the old
// one's identifier back in with it.
func TestAnExportedRowCarriesNoTenantIdentifier(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	id := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return containerRepo().Insert(ctx, containerIn(tenantA, authorA, id, freshName(t), "aa"))
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	for _, row := range exported(ctx, t, tenantA, 100, "container", time.Time{}) {
		if row.ID != id.String() {
			continue
		}
		if _, carries := row.Data["tenant_id"]; carries {
			t.Fatalf("the row carries a tenant identifier: %v", row.Data)
		}
		if row.ChangedAt.IsZero() {
			t.Fatal("a container came back without a change stamp")
		}
		if row.Data["name"] == nil {
			t.Fatalf("the row is not the row: %v", row.Data)
		}
		return
	}
	t.Fatal("the container was not exported")
}

// The period is what makes an incremental incremental.
func TestADeltaExportAnswersOnlyWhatChangedAfterThePeriod(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	early := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return containerRepo().Insert(ctx, containerIn(tenantA, authorA, early, freshName(t), "ab"))
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	var cut time.Time
	if err := postgres.NewUnitOfWork(appPool(ctx, t)).WithinSnapshot(ctx,
		persistence.Scope{TenantID: tenantA}, func(_ context.Context, at time.Time) error {
			cut = at
			return nil
		}); err != nil {
		t.Fatalf("the snapshot: %v", err)
	}

	late := freshID(t)
	// Stamped after the cut by hand: the rows carry the domain's clock rather than the database's,
	// which is exactly the property an incremental relies on - what a row says about itself is
	// what decides whether it is in the period.
	after := containerIn(tenantA, authorA, late, freshName(t), "ac")
	after.CreatedAt, after.UpdatedAt = cut.Add(time.Minute), cut.Add(time.Minute)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return containerRepo().Insert(ctx, after)
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	var sawEarly, sawLate bool
	for _, row := range exported(ctx, t, tenantA, 100, "container", cut) {
		switch row.ID {
		case early.String():
			sawEarly = true
		case late.String():
			sawLate = true
		}
	}
	if sawEarly {
		t.Error("a row that changed before the period was exported into an incremental")
	}
	if !sawLate {
		t.Error("a row that changed after the period was left out")
	}
}

// A table that cannot date a change answers everything, whatever period it is given - which is
// exactly why the archive marks those entities as written whole.
func TestATableWithNoChangeStampAnswersEverything(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	future := time.Now().Add(24 * time.Hour)
	whole := exported(ctx, t, tenantA, 100, "label", future)
	all := exported(ctx, t, tenantA, 100, "label", time.Time{})

	if len(whole) != len(all) {
		t.Fatalf("a period changed a whole table's answer: %d against %d", len(whole), len(all))
	}
	for _, row := range whole {
		if !row.ChangedAt.IsZero() {
			t.Fatalf("a table with no change stamp answered one: %v", row.ChangedAt)
		}
	}
}

// The tenant boundary holds per method, which is what gate SG-3 asks of every repository method.
func TestAnotherTenantExportsNoneOfIt(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	id := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return containerRepo().Insert(ctx, containerIn(tenantA, authorA, id, freshName(t), "ad"))
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	for _, row := range exported(ctx, t, tenantB, 100, "container", time.Time{}) {
		if row.ID == id.String() {
			t.Fatal("tenant B exported tenant A's container")
		}
	}

	var markers []repository.Tombstone
	err := postgres.NewUnitOfWork(appPool(ctx, t)).WithinSnapshot(ctx, persistence.Scope{TenantID: tenantB},
		func(snapshotCtx context.Context, at time.Time) error {
			return exportRepo(100).Tombstones(snapshotCtx, "container", at.Add(-time.Hour),
				func(marker repository.Tombstone) error {
					markers = append(markers, marker)
					return nil
				})
		})
	if err != nil {
		t.Fatalf("exporting the deletions: %v", err)
	}
	for _, marker := range markers {
		if marker.ID == id.String() {
			t.Fatal("tenant B saw a deletion marker of tenant A")
		}
	}
}

// A table nothing exports is a defect rather than an empty answer: an archive that silently held
// nothing of an entity would restore a tenant with a feature missing.
func TestATableWithNoStatementIsADefect(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	err := postgres.NewUnitOfWork(appPool(ctx, t)).WithinSnapshot(ctx, persistence.Scope{TenantID: tenantA},
		func(snapshotCtx context.Context, _ time.Time) error {
			return exportRepo(100).Rows(snapshotCtx, "access_token", time.Time{},
				func(repository.Row) error { return nil })
		})
	if err == nil {
		t.Fatal("a table with no export statement answered")
	}
}

// A full archive carries no deletion markers: it is the whole truth and has nothing to deny.
func TestAFullExportAsksForNoDeletions(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	var count int
	err := postgres.NewUnitOfWork(appPool(ctx, t)).WithinSnapshot(ctx, persistence.Scope{TenantID: tenantA},
		func(snapshotCtx context.Context, _ time.Time) error {
			return exportRepo(100).Tombstones(snapshotCtx, "work_item", time.Time{},
				func(repository.Tombstone) error {
					count++
					return nil
				})
		})
	if err != nil {
		t.Fatalf("exporting the deletions: %v", err)
	}
	if count != 0 {
		t.Fatalf("a full export asked for %d deletion markers", count)
	}
}
