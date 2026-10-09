// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	backupservice "github.com/Jersyfi/hubtask/core/application/service/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/backupstorage"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A legal hold stops a reset of the workspace to a backup (backup-restore.md §8.2,
// data-protection.md §5), against the real database and a real archive: the job reads the holds in
// the transaction that would empty the workspace and refuses while one stands; once none does, the
// replace runs and the workspace keeps its own hold records rather than the archive's.

func rowsOfTenant(ctx context.Context, t *testing.T, table string, tenant shared.ID) int {
	t.Helper()
	var n int
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT count(*) FROM `+table+` WHERE tenant_id = $1`, tenant.String()).Scan(&n); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	return n
}

func TestAReplaceIsRefusedUnderAHoldAndKeepsTheWorkspacesHoldsWithout(t *testing.T) {
	ctx := context.Background()
	store, err := backupstorage.NewLocalStore(t.TempDir(), "archives")
	if err != nil {
		t.Fatalf("opening the local store: %v", err)
	}
	workspace, prefix := foreignWorkspace(ctx, t, store, "Keep the invoices")
	admin := adminPool(ctx, t)
	t.Cleanup(func() {
		done := context.WithoutCancel(ctx)
		for _, statement := range []string{
			`DELETE FROM restore_run WHERE tenant_id = $1`,
			`DELETE FROM backup_target WHERE tenant_id = $1`,
		} {
			if _, err := adminPool(done, t).Exec(done, statement, workspace.String()); err != nil {
				t.Errorf("clearing up: %v", err)
			}
		}
		deleteForGood(done, t, workspace)
	})

	var author string
	if err := admin.QueryRow(ctx, `SELECT id FROM account WHERE tenant_id = $1`, workspace.String()).
		Scan(&author); err != nil {
		t.Fatalf("reading the workspace's person: %v", err)
	}
	target := targetIn(t, workspace, shared.ID(author), freshName(t))
	insertTarget(ctx, t, workspace, target, crypto.Sealed{})

	// Placed after the archive was written: the archive knows nothing of it.
	hold := freshID(t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO legal_hold (id, tenant_id, scope_kind, reason, placed_by, placed_at)
		VALUES ($1, $2, 'TENANT', 'Tax audit', $3, now())`,
		hold.String(), workspace.String(), author); err != nil {
		t.Fatalf("placing the hold: %v", err)
	}

	applier := foreignRestore{store: store, uow: postgres.NewUnitOfWork(appPool(ctx, t))}.applier(t)
	applier.Holds = postgres.NewLifecycleRepository()
	replace := func() (shared.ID, error) {
		id := freshID(t)
		if err := write(ctx, t, workspace, func(ctx context.Context) error {
			return restoreRepo().Insert(ctx, domain.Restore{
				ID: id, TargetID: target.ID, TenantID: workspace, SourceArchive: prefix,
				Mode: domain.RestoreReplaceTenant, ConflictRule: domain.ConflictSkip,
				Status: domain.RestorePending, RequestedBy: shared.ID(author),
			})
		}); err != nil {
			t.Fatalf("accepting the restore: %v", err)
		}
		_, err := applier.Apply(ctx, backupservice.ApplyInput{RestoreID: id, TenantID: workspace})
		return id, err
	}

	refused, err := replace()
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != lifecycle.CodeLegalHold {
		t.Fatalf("the replace under a hold answered %v", err)
	}
	if items := rowsOfTenant(ctx, t, "work_item", workspace); items != 1 {
		t.Errorf("%d entries after the refused replace, want the one that was there", items)
	}
	var status, code string
	if err := admin.QueryRow(ctx, `SELECT status, coalesce(error_code, '') FROM restore_run WHERE id = $1`,
		refused.String()).Scan(&status, &code); err != nil {
		t.Fatalf("reading the run: %v", err)
	}
	if status != "FAILED" || code != lifecycle.CodeLegalHold {
		t.Errorf("the run is %s with %q", status, code)
	}

	// Released: the replace runs, and the released hold is still the workspace's, as it was lifted.
	if _, err := admin.Exec(ctx, `
		UPDATE legal_hold SET released_at = now(), released_by = $2, released_reason = 'Audit closed'
		WHERE id = $1`, hold.String(), author); err != nil {
		t.Fatalf("releasing the hold: %v", err)
	}
	if _, err := replace(); err != nil {
		t.Fatalf("the replace with no hold in force: %v", err)
	}
	var reason string
	if err := admin.QueryRow(ctx,
		`SELECT coalesce(released_reason, '') FROM legal_hold WHERE tenant_id = $1 AND id = $2`,
		workspace.String(), hold.String()).Scan(&reason); err != nil {
		t.Fatalf("the workspace's hold is gone after the replace: %v", err)
	}
	if reason != "Audit closed" {
		t.Errorf("the released hold came back with %q as its release", reason)
	}
	if holds := rowsOfTenant(ctx, t, "legal_hold", workspace); holds != 1 {
		t.Errorf("%d holds after the replace, want the workspace's one", holds)
	}
	if items := rowsOfTenant(ctx, t, "work_item", workspace); items != 1 {
		t.Errorf("%d entries after the replace, want the archive's one", items)
	}
}
