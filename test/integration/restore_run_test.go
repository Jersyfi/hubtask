// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The restore run against a real database (E-06): the claim that is a statement, the resumption
// that is the same claim, the report that survives a round trip, and the tenant boundary per
// method (gate SG-3, BK-10).

func restoreRepo() postgres.RestoreRunRepository { return postgres.NewRestoreRunRepository() }

// clearRestores empties the tenant's restore log before a test that cares about the lock.
//
// The lock is tenant-wide by design - §8.3 has one restore at a time - so a run another test left
// RUNNING would make every later test in the package look busy. The alternative, a tenant per test,
// would hide exactly the interference these tests exist to check.
func clearRestores(ctx context.Context, t *testing.T, tenant shared.ID) {
	t.Helper()
	if _, err := adminPool(ctx, t).Exec(ctx,
		`DELETE FROM restore_run WHERE tenant_id = $1`, tenant.String()); err != nil {
		t.Fatalf("clearing the restore log: %v", err)
	}
}

func restoreIn(tenant, target, id shared.ID, mode domain.RestoreMode) domain.Restore {
	return domain.Restore{
		ID: id, TargetID: target, TenantID: tenant,
		SourceArchive: "hubtask-backup-" + tenant.String() + "-20260101T030000Z-full",
		Mode:          mode, ConflictRule: domain.ConflictSkip, DryRun: true,
		Status: domain.RestorePending, RequestedBy: authorA,
	}
}

func TestARestoreIsWrittenWhenItIsAcceptedAndReadBackWhole(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	clearRestores(ctx, t, tenantA)
	target := seedTarget(ctx, t, tenantA)
	id := freshID(t)

	restore := restoreIn(tenantA, target, id, domain.RestoreSelective)
	restore.Selection = domain.Selection{ContainerIDs: []shared.ID{authorA}, ItemIDs: []shared.ID{authorA}}

	var found domain.Restore
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := restoreRepo().Insert(ctx, restore); err != nil {
			return err
		}
		var err error
		found, err = restoreRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("writing the restore: %v", err)
	}

	switch {
	case found.Status != domain.RestorePending:
		t.Errorf("an accepted restore is %s, want PENDING - a caller polling result_url has to "+
			"see it rather than a 404", found.Status)
	case found.Mode != domain.RestoreSelective:
		t.Errorf("the mode came back as %s", found.Mode)
	case found.SourceArchive != restore.SourceArchive:
		t.Errorf("the archive came back as %q", found.SourceArchive)
	case len(found.Selection.ContainerIDs) != 1 || len(found.Selection.ItemIDs) != 1:
		t.Errorf("the selection came back as %+v", found.Selection)
	case !found.DryRun:
		t.Errorf("a dry run came back as an execution")
	}
}

// The lock: one restore at a time per tenant, and the second one finds out by getting no row back
// rather than by a check that ran a moment earlier.
func TestOnlyOneRestoreRunsInATenantAtATime(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	clearRestores(ctx, t, tenantA)
	target := seedTarget(ctx, t, tenantA)
	first, second := freshID(t), freshID(t)

	var claimedFirst, claimedSecond bool
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := restoreRepo().Insert(ctx, restoreIn(tenantA, target, first, domain.RestoreMerge)); err != nil {
			return err
		}
		if err := restoreRepo().Insert(ctx, restoreIn(tenantA, target, second, domain.RestoreMerge)); err != nil {
			return err
		}
		var err error
		if claimedFirst, err = restoreRepo().Claim(ctx, first, created); err != nil {
			return err
		}
		claimedSecond, err = restoreRepo().Claim(ctx, second, created)
		return err
	}); err != nil {
		t.Fatalf("claiming: %v", err)
	}

	if !claimedFirst {
		t.Error("the first restore did not get the tenant")
	}
	if claimedSecond {
		t.Error("a second restore got the tenant while the first was running")
	}
}

// BK-7's restore half. A worker that died leaves a RUNNING row; the attempt that takes over claims
// the same run again rather than being told the tenant is busy - and the run still says when it
// actually began.
func TestAResumedRestoreClaimsItsOwnRunAgain(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	clearRestores(ctx, t, tenantA)
	target := seedTarget(ctx, t, tenantA)
	id := freshID(t)
	later := created.Add(time.Hour)

	var again bool
	var found domain.Restore
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := restoreRepo().Insert(ctx, restoreIn(tenantA, target, id, domain.RestoreMerge)); err != nil {
			return err
		}
		if _, err := restoreRepo().Claim(ctx, id, created); err != nil {
			return err
		}
		var err error
		if again, err = restoreRepo().Claim(ctx, id, later); err != nil {
			return err
		}
		found, err = restoreRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("resuming: %v", err)
	}

	if !again {
		t.Fatal("a resumed restore could not claim its own run")
	}
	if !found.StartedAt.Equal(created) {
		t.Errorf("the run says it began at %s; the resumption moved it", found.StartedAt)
	}
}

func TestAFinishedRestoreCarriesItsReportAndCannotBeFinishedTwice(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	clearRestores(ctx, t, tenantA)
	target := seedTarget(ctx, t, tenantA)
	id := freshID(t)

	report := domain.Report{
		New: 4, Overwritten: 1, Skipped: 2, Conflicts: 3, Media: 7,
		Withheld: map[string]int{domain.WithheldDeleted: 5},
		Entities: map[string]int{"work_items": 4},
	}

	var second error
	var found domain.Restore
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := restoreRepo().Insert(ctx, restoreIn(tenantA, target, id, domain.RestoreMerge)); err != nil {
			return err
		}
		if _, err := restoreRepo().Claim(ctx, id, created); err != nil {
			return err
		}
		outcome := domain.RestoreOutcome{
			ID: id, Status: domain.RestoreSucceeded, Report: report, FinishedAt: created,
		}
		if err := restoreRepo().Finish(ctx, outcome); err != nil {
			return err
		}
		second = restoreRepo().Finish(ctx, outcome)
		var err error
		found, err = restoreRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("finishing: %v", err)
	}

	if found.Status != domain.RestoreSucceeded {
		t.Fatalf("the restore is %s", found.Status)
	}
	if found.Report.New != 4 || found.Report.Deleted() != 5 || found.Report.Entities["work_items"] != 4 {
		t.Errorf("the report came back as %+v", found.Report)
	}
	if !errors.Is(second, shared.ErrConflict) {
		t.Errorf("a finished restore was finished again: %v", second)
	}
}

// The way back from a destructive mode has to be findable from the run even if the run then fails,
// so the copy is recorded before the mode runs rather than with the outcome.
func TestTheSafetyCopyIsRecordedBeforeTheOutcome(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	clearRestores(ctx, t, tenantA)
	target := seedTarget(ctx, t, tenantA)
	id, backup := freshID(t), freshID(t)

	var found domain.Restore
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := restoreRepo().Insert(ctx, restoreIn(tenantA, target, id, domain.RestoreReplaceTenant)); err != nil {
			return err
		}
		if _, err := runRepo().Start(ctx, runIn(tenantA, target, backup, domain.ModeFull)); err != nil {
			return err
		}
		if err := restoreRepo().RecordSafetyCopy(ctx, id, backup); err != nil {
			return err
		}
		var err error
		found, err = restoreRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("recording the safety copy: %v", err)
	}

	if found.SafetyRunID != backup {
		t.Fatalf("the safety copy is %s, want %s", found.SafetyRunID, backup)
	}
}

// Gate SG-3, and BK-10 at every method: another tenant's restore is not findable, not claimable,
// not finishable, and does not make this tenant look busy.
func TestARestoreIsInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	clearRestores(ctx, t, tenantA)
	clearRestores(ctx, t, tenantB)
	target := seedTarget(ctx, t, tenantB)
	id := freshID(t)

	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if err := restoreRepo().Insert(ctx, restoreIn(tenantB, target, id, domain.RestoreMerge)); err != nil {
			return err
		}
		_, err := restoreRepo().Claim(ctx, id, created)
		return err
	}); err != nil {
		t.Fatalf("seeding B's restore: %v", err)
	}

	var find error
	var claimed, busy bool
	var finish error
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, find = restoreRepo().Find(ctx, id)
		var err error
		if claimed, err = restoreRepo().Claim(ctx, id, created); err != nil {
			return err
		}
		finish = restoreRepo().Finish(ctx, domain.RestoreOutcome{
			ID: id, Status: domain.RestoreFailed, FinishedAt: created,
		})
		busy, err = restoreRepo().InProgress(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading from A: %v", err)
	}

	if !errors.Is(find, shared.ErrNotFound) {
		t.Errorf("tenant A found tenant B's restore: %v", find)
	}
	if claimed {
		t.Error("tenant A claimed tenant B's restore")
	}
	if !errors.Is(finish, shared.ErrConflict) {
		t.Errorf("tenant A finished tenant B's restore: %v", finish)
	}
	if busy {
		t.Error("tenant B's restore made tenant A look busy")
	}
}

// The one method a destructive restore asks the tenant about, and gate SG-3 for it: what a
// workspace is called is read from the tenant of the running transaction and from no other, so this
// cannot be used to learn another workspace's name in order to type it.
func TestTheWorkspaceNameIsTheTransactionsOwn(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	var fromA, fromB string
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		fromA, err = postgres.NewWorkspaceRepository().Name(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading A's name: %v", err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		fromB, err = postgres.NewWorkspaceRepository().Name(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading B's name: %v", err)
	}

	if fromA != "A" || fromB != "B" {
		t.Fatalf("A reads %q and B reads %q", fromA, fromB)
	}
}
