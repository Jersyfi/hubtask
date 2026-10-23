// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	backupservice "github.com/Jersyfi/hubtask/core/application/service/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/backupstorage"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Milestones and kudos in the archives (UC-WRK-29 checks 4 to 6): a milestone is the workspace's
// and goes through a backup and its restore with its collection's identity and its moment; kudos
// are the two people's, so the copy a person gets of their own data carries the ones they gave and
// received, and no archive an administrator reads carries any.

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

func TestThePersonsOwnCopyCarriesTheirKudosAndNoWorkspaceArchiveDoes(t *testing.T) {
	ctx := context.Background()
	anna := seedSubject(ctx, t, "kudos-"+freshID(t).String()+"@example.test")
	admin := adminPool(ctx, t)
	ben, carl, dana := freshID(t), freshID(t), freshID(t)
	for name, account := range map[string]shared.ID{"Ben": ben, "Carl": carl, "Dana": dana} {
		if _, err := admin.Exec(ctx, `INSERT INTO account (id, tenant_id, email, display_name) VALUES ($1, $2, $3, $4)`,
			account.String(), anna.tenant.String(), strings.ToLower(name)+"-"+account.String()+"@example.test", name); err != nil {
			t.Fatalf("seeding %s: %v", name, err)
		}
	}
	given := map[string][2]shared.ID{
		"Ben to Anna":  {ben, anna.account},
		"Anna to Carl": {anna.account, carl},
		"Ben to Carl":  {ben, carl},
	}
	for name, pair := range given {
		if _, err := admin.Exec(ctx, `
			INSERT INTO kudos (tenant_id, item_id, given_by, given_to, given_at) VALUES ($1, $2, $3, $4, now())`,
			anna.tenant.String(), anna.item.String(), pair[0].String(), pair[1].String()); err != nil {
			t.Fatalf("seeding the kudos %s: %v", name, err)
		}
	}
	record := func(name string) string {
		pair := given[name]
		return `"id":"` + anna.item.String() + "/" + pair[0].String() + "/" + pair[1].String() + `"`
	}

	// Anna's copy holds both directions - who, on which entry, when - and nothing between Ben and
	// Carl.
	own := ownCopyOf(ctx, t, anna.tenant, anna.account)
	for _, name := range []string{"Ben to Anna", "Anna to Carl"} {
		if !strings.Contains(own, record(name)) {
			t.Errorf("Anna's copy lacks the kudos %s", name)
		}
	}
	if !strings.Contains(own, `"given_at"`) {
		t.Error("Anna's copy does not say when the kudos were given")
	}
	if strings.Contains(own, record("Ben to Carl")) {
		t.Error("Anna's copy carries kudos between two other people")
	}

	// Dana gave and received none, and her copy carries nobody else's.
	other := ownCopyOf(ctx, t, anna.tenant, dana)
	for name := range given {
		if strings.Contains(other, record(name)) {
			t.Errorf("Dana's copy carries the kudos %s", name)
		}
	}

	// What an administrator reads - the workspace export and a backup - carries no kudos at all.
	for name, written := range map[string]string{
		"the workspace export": workspaceExportOf(ctx, t, anna.tenant),
		"the backup":           backupOf(ctx, t, anna.tenant, anna.account),
	} {
		if strings.Contains(written, "given_by") || strings.Contains(written, archive.DataName("kudos")) {
			t.Errorf("%s carries kudos", name)
		}
	}
}
