// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/archive"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	backupservice "github.com/Jersyfi/hubtask/core/application/service/backup"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	backupport "github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/backupstorage"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	cryptoadapter "github.com/Jersyfi/hubtask/infrastructure/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/storage"
)

// A workspace taken to another installation (backup-restore.md §8.2, tenant-export.md §10,
// UC-BAK-07 check 5) against the real database: the operator's NEW_TENANT restore takes an export
// whose workspace this installation does not hold, and still refuses one of a workspace it does
// (BK-10). "Another installation" is a workspace exported here and then deleted for good: from the
// database's side the two cannot be told apart, which is the property the rule rests on.

// foreignTarget hands the restore the one local store the export was written to; the target row
// exists for the run's foreign key, and its kind is the adapters' business, not this test's.
type foreignTarget struct{ store backupport.Store }

func (o foreignTarget) Open(context.Context, backupport.Spec) (backupport.Store, error) {
	return o.store, nil
}

func (foreignTarget) Kinds() []domain.TargetKind { return []domain.TargetKind{domain.KindLocal} }

type allowAll struct{}

func (allowAll) Authorize(context.Context, appshared.ActorContext, access.Request) error { return nil }

type discardAudit struct{}

func (discardAudit) Append(context.Context, audit.Entry) error { return nil }

// foreignWorkspace seeds a workspace with a hub, a collection and a task, exports it to the store
// as tenant-export.md writes an export, and answers its identifier and the archive's path there.
func foreignWorkspace(
	ctx context.Context, t *testing.T, store backupport.Store, title string,
) (shared.ID, string) {
	t.Helper()
	tenant, account, hub, collection, item := freshID(t), freshID(t), freshID(t), freshID(t), freshID(t)
	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'Moving out')`,
			[]any{tenant.String(), "moving-" + tenant.String()[24:]}},
		{`INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		  VALUES ($1, $2, 'USER', $3, 'Eva', 'ACTIVE')`,
			[]any{account.String(), tenant.String(), "eva-" + account.String() + "@example.org"}},
		{`INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		  VALUES ($1, $2, 'HUB', 'Home', 'a0', $3)`,
			[]any{hub.String(), tenant.String(), account.String()}},
		{`INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		  VALUES ($1, $2, 'COLLECTION', $3, 'Errands', 'a0', $4)`,
			[]any{collection.String(), tenant.String(), hub.String(), account.String()}},
		{`INSERT INTO work_item (id, tenant_id, collection_id, type, path, depth, title, order_key, created_by)
		  VALUES ($1, $2, $3, 'TASK', $4, 1, $5, 'a0', $6)`,
			[]any{item.String(), tenant.String(), collection.String(), "/" + item.String() + "/", title, account.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding the workspace: %v\n%s", err, statement.sql)
		}
	}

	archivist := adminservice.TenantExportArchivist{
		Targets:  exportTargetStore{store: store},
		Rows:     postgres.NewBackupExportRepository(postgres.DefaultExportBatch),
		Objects:  exportObjects{},
		Cipher:   cryptoadapter.NewStream(clockadapter.CryptoRandom{}),
		Snapshot: postgres.NewUnitOfWork(appPool(ctx, t)),
		Clock:    clockadapter.System{}, SchemaVersion: "test", ProductVersion: "test",
	}
	if _, err := archivist.Write(ctx, adminservice.ExportArchiveRequest{
		ExportID: freshID(t), TenantID: tenant, TargetID: freshID(t),
	}); err != nil {
		t.Fatalf("exporting the workspace: %v", err)
	}
	described, err := archive.NewReader(store, cryptoadapter.NewStream(clockadapter.CryptoRandom{})).List(ctx, "")
	if err != nil {
		t.Fatalf("listing the export: %v", err)
	}
	for _, description := range described {
		if description.Manifest.Scope.ID == tenant.String() {
			return tenant, description.Prefix
		}
	}
	t.Fatalf("the export of %s is not at the target", tenant)
	return "", ""
}

// deleteForGood removes the workspace the way the purge leaves it: no row of it remains.
func deleteForGood(ctx context.Context, t *testing.T, tenant shared.ID) {
	t.Helper()
	admin := adminPool(ctx, t)
	for _, statement := range []string{
		`DELETE FROM work_item WHERE tenant_id = $1`,
		`DELETE FROM container WHERE tenant_id = $1 AND parent_id IS NOT NULL`,
		`DELETE FROM container WHERE tenant_id = $1`,
		`DELETE FROM tenant WHERE id = $1`,
	} {
		if _, err := admin.Exec(ctx, statement, tenant.String()); err != nil {
			t.Fatalf("deleting the workspace for good: %v\n%s", err, statement)
		}
	}
}

type foreignRestore struct {
	store  backupport.Store
	target shared.ID
	uow    *postgres.UnitOfWork
}

func newForeignRestore(ctx context.Context, t *testing.T) foreignRestore {
	t.Helper()
	seedContainerTenants(ctx, t)
	clearRestores(ctx, t, tenantA)
	store, err := backupstorage.NewLocalStore(t.TempDir(), "archives")
	if err != nil {
		t.Fatalf("opening the local store: %v", err)
	}
	target := targetIn(t, tenantA, authorA, freshName(t))
	insertTarget(ctx, t, tenantA, target, crypto.Sealed{})
	return foreignRestore{store: store, target: target.ID, uow: postgres.NewUnitOfWork(appPool(ctx, t))}
}

func (f foreignRestore) applier(t *testing.T) backupservice.Applier {
	return backupservice.Applier{
		Restores: restoreRepo(), Targets: postgres.NewBackupTargetRepository(),
		Import:  postgres.NewBackupImportRepository(),
		Journal: postgres.NewDeletionJournalRepository(postgres.DefaultJournalBatch),
		Opener:  foreignTarget{store: f.store}, Cipher: cryptoadapter.NewStream(clockadapter.CryptoRandom{}),
		Objects: storage.NewLocalStorage(t.TempDir()), Epochs: postgres.NewEpochRepository(),
		UnitOfWork: f.uow, Clock: clock.Fixed(created), SchemaVersion: "test",
		Batch: backupservice.DefaultRestoreBatch,
	}
}

// restoreAsNewWorkspace accepts the operator's NEW_TENANT restore in tenant A and runs it.
func (f foreignRestore) restoreAsNewWorkspace(
	ctx context.Context, t *testing.T, prefix string,
) (shared.ID, shared.ID, error) {
	t.Helper()
	restoreID, minted := freshID(t), freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return restoreRepo().Insert(ctx, domain.Restore{
			ID: restoreID, TargetID: f.target, TenantID: minted, SourceArchive: prefix,
			Mode: domain.RestoreNewTenant, ConflictRule: domain.ConflictSkip,
			Status: domain.RestorePending, RequestedBy: authorA,
		})
	}); err != nil {
		t.Fatalf("accepting the restore: %v", err)
	}
	_, err := f.applier(t).Apply(ctx, backupservice.ApplyInput{RestoreID: restoreID, TenantID: tenantA})
	return restoreID, minted, err
}

func TestAnOperatorRestoresAnExportFromAnotherInstallationAsANewWorkspace(t *testing.T) {
	ctx := context.Background()
	f := newForeignRestore(ctx, t)
	source, prefix := foreignWorkspace(ctx, t, f.store, "Pick up the parcel")
	deleteForGood(ctx, t, source)

	// §8.1: the operator's listing at the target shows it; a member's does not.
	restorer := backupservice.Restorer{
		Targets: postgres.NewBackupTargetRepository(), Workspaces: postgres.NewBackupImportRepository(),
		Opener: foreignTarget{store: f.store}, Cipher: cryptoadapter.NewStream(clockadapter.CryptoRandom{}),
		Authorizer: allowAll{}, Audit: discardAudit{}, UnitOfWork: f.uow, Clock: clock.Fixed(created),
	}
	operator := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantA, AccountID: authorA,
		Scopes: []string{"backup:read", "admin:tenants"},
	}
	for name, actor := range map[string]appshared.ActorContext{
		"the operator": operator, "a member": {Kind: appshared.ActorUser, TenantID: tenantA, AccountID: authorA},
	} {
		listed, err := (backupservice.ListBackupsAtTarget{Restorer: restorer}).
			Execute(ctx, actor, f.target, shared.ID(""))
		if err != nil {
			t.Fatalf("%s listing: %v", name, err)
		}
		shown := false
		for _, found := range listed {
			shown = shown || found.Path == prefix
		}
		if shown != (name == "the operator") {
			t.Errorf("%s's listing shows the export from elsewhere: %v", name, shown)
		}
	}

	restoreID, minted, err := f.restoreAsNewWorkspace(ctx, t, prefix)
	if err != nil {
		t.Fatalf("restoring the export as a new workspace: %v", err)
	}

	var restore domain.Restore
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		restore, err = restoreRepo().Find(ctx, restoreID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if restore.Status != domain.RestoreSucceeded {
		t.Fatalf("the restore ended %s (%s)", restore.Status, restore.ErrorCode)
	}

	// The new workspace holds what was exported: the tree and the task, under identities of its own.
	admin := adminPool(ctx, t)
	var hubs, collections int
	var title string
	if err := admin.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE type = 'HUB'), count(*) FILTER (WHERE type = 'COLLECTION')
		FROM container WHERE tenant_id = $1`, minted.String()).Scan(&hubs, &collections); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(ctx, `SELECT title FROM work_item WHERE tenant_id = $1`,
		minted.String()).Scan(&title); err != nil {
		t.Fatalf("the task did not arrive: %v", err)
	}
	if hubs != 1 || collections != 1 || title != "Pick up the parcel" {
		t.Errorf("the new workspace holds %d hubs, %d collections and %q", hubs, collections, title)
	}
	var people int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM account WHERE tenant_id = $1`,
		minted.String()).Scan(&people); err != nil || people != 1 {
		t.Errorf("the new workspace holds %d accounts (%v), want the one exported", people, err)
	}
}

// BK-10 holds between this installation's workspaces: an export of a workspace that still exists
// here is refused, NEW_TENANT included, and nothing is created.
func TestAnExportOfAWorkspaceThisInstallationHoldsIsStillRefused(t *testing.T) {
	ctx := context.Background()
	f := newForeignRestore(ctx, t)
	_, prefix := foreignWorkspace(ctx, t, f.store, "Somebody else's task")

	_, minted, err := f.restoreAsNewWorkspace(ctx, t, prefix)

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != domain.CodeRestoreArchiveScopeMismatch {
		t.Fatalf("refused with %v, want %s", err, domain.CodeRestoreArchiveScopeMismatch)
	}
	var created int
	if err := adminPool(ctx, t).QueryRow(ctx, `SELECT count(*) FROM tenant WHERE id = $1`,
		minted.String()).Scan(&created); err != nil || created != 0 {
		t.Errorf("the refused restore created the workspace (%d, %v)", created, err)
	}
}
