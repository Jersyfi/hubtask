// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	backupservice "github.com/Jersyfi/hubtask/core/application/service/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/backupstorage"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Milestones in the archives (UC-WRK-29 check 4): a milestone is the workspace's and goes through a
// backup and its restore with its collection's identity and its moment.

func TestAMilestoneComesBackWithARestoreAsItWas(t *testing.T) {
	ctx := context.Background()
	store, err := backupstorage.NewLocalStore(t.TempDir(), "archives")
	if err != nil {
		t.Fatalf("opening the local store: %v", err)
	}
	workspace, prefix := foreignWorkspace(ctx, t, store, "Paint the hall")
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

	var collection, author string
	if err := admin.QueryRow(ctx, `SELECT container_id FROM container_milestone WHERE tenant_id = $1`,
		workspace.String()).Scan(&collection); err != nil {
		t.Fatalf("reading the milestone: %v", err)
	}
	if err := admin.QueryRow(ctx, `SELECT id FROM account WHERE tenant_id = $1`, workspace.String()).
		Scan(&author); err != nil {
		t.Fatalf("reading the workspace's person: %v", err)
	}
	// Lost after the archive was written: what comes back is the archive's.
	if _, err := admin.Exec(ctx, `DELETE FROM container_milestone WHERE tenant_id = $1`, workspace.String()); err != nil {
		t.Fatalf("losing the milestone: %v", err)
	}

	target := targetIn(t, workspace, shared.ID(author), freshName(t))
	insertTarget(ctx, t, workspace, target, crypto.Sealed{})
	restoreID := freshID(t)
	if err := write(ctx, t, workspace, func(ctx context.Context) error {
		return restoreRepo().Insert(ctx, domain.Restore{
			ID: restoreID, TargetID: target.ID, TenantID: workspace, SourceArchive: prefix,
			Mode: domain.RestoreReplaceTenant, ConflictRule: domain.ConflictSkip,
			Status: domain.RestorePending, RequestedBy: shared.ID(author),
		})
	}); err != nil {
		t.Fatalf("accepting the restore: %v", err)
	}
	applier := foreignRestore{store: store, uow: postgres.NewUnitOfWork(appPool(ctx, t))}.applier(t)
	applier.Holds = postgres.NewLifecycleRepository()
	if _, err := applier.Apply(ctx, backupservice.ApplyInput{RestoreID: restoreID, TenantID: workspace}); err != nil {
		t.Fatalf("restoring: %v", err)
	}

	var restored string
	var reached time.Time
	if err := admin.QueryRow(ctx, `SELECT container_id, reached_at FROM container_milestone WHERE tenant_id = $1`,
		workspace.String()).Scan(&restored, &reached); err != nil {
		t.Fatalf("the milestone did not come back: %v", err)
	}
	if restored != collection || !reached.Equal(foreignMilestone) {
		t.Errorf("the milestone came back on %s at %v, want %s at %v", restored, reached, collection, foreignMilestone)
	}
}
