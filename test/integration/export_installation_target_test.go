// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	backupservice "github.com/Jersyfi/hubtask/core/application/service/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	backupport "github.com/Jersyfi/hubtask/core/port/backupstorage"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/backupstorage"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	cryptoadapter "github.com/Jersyfi/hubtask/infrastructure/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-BAK-07 check 1 in multi-tenant operation with tenant targets off (multi-tenancy.md §5): the
// workspace has no target of its own, the operator configured one for the installation, and the
// export is written there - through the real repository, the real scopes and the application role.
// The workspace's own reads still name no installation target, and the tenant-only opener every
// other writer to a target holds still finds none.

// localOpener answers every target with one local store: which adapter a kind selects is the
// adapters' business, and this test is about which scopes may open the target at all.
type localOpener struct{ store backupport.Store }

func (o localOpener) Open(context.Context, backupport.Spec) (backupport.Store, error) {
	return o.store, nil
}

func (o localOpener) Kinds() []domain.TargetKind { return []domain.TargetKind{domain.KindLocal} }

func TestAWorkspaceIsExportedToTheInstallationsTarget(t *testing.T) {
	ctx := context.Background()
	seedExportTenants(ctx, t)

	root := t.TempDir()
	store, err := backupstorage.NewLocalStore(root, "exports")
	if err != nil {
		t.Fatalf("opening the local store: %v", err)
	}
	work := postgres.NewUnitOfWork(appPool(ctx, t))
	targets := postgres.NewBackupTargetRepository()

	// The operator's target: no workspace, written in the installation's scope.
	target, err := domain.NewTarget(domain.NewTargetInput{
		ID: freshID(t), Name: freshName(t), Kind: domain.KindLocal,
		Config: domain.TargetConfig{"path": "exports"}, CreatedBy: exportAccountX, Now: created,
	})
	if err != nil {
		t.Fatalf("building the target: %v", err)
	}
	if err := work.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		return targets.Insert(ctx, target, cryptoport.Sealed{})
	}); err != nil {
		t.Fatalf("configuring the installation's target: %v", err)
	}
	t.Cleanup(func() {
		_ = work.Within(context.Background(), persistence.SystemScope(), func(ctx context.Context) error {
			_, err := targets.Delete(ctx, target.ID)
			return err
		})
	})

	opener := backupservice.StoreOpener{Targets: targets, Opener: localOpener{store: store}, UnitOfWork: work}
	exports := backupservice.ExportStoreOpener{StoreOpener: opener}

	// Accepted: the target is usable for this workspace's export.
	if err := exports.Usable(ctx, exportTenantX, target.ID); err != nil {
		t.Fatalf("the installation's target is not usable for the export: %v", err)
	}
	// An identifier nobody configured is refused when the export is accepted.
	if err := exports.Usable(ctx, exportTenantX, freshID(t)); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("an unknown target: %v", err)
	}

	archivist := adminservice.TenantExportArchivist{
		Targets:       exports,
		Rows:          postgres.NewBackupExportRepository(postgres.DefaultExportBatch),
		Objects:       exportObjects{key: "media/x/one"},
		Cipher:        cryptoadapter.NewStream(clockadapter.CryptoRandom{}),
		Snapshot:      work,
		Clock:         clockadapter.System{},
		SchemaVersion: "test", ProductVersion: "test",
	}
	if _, err := archivist.Write(ctx, adminservice.ExportArchiveRequest{
		ExportID: freshID(t), TenantID: exportTenantX, TargetID: target.ID,
	}); err != nil {
		t.Fatalf("exporting to the installation's target: %v", err)
	}
	manifests := 0
	_ = filepath.WalkDir(filepath.Join(root, "exports"), func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.HasSuffix(path, "manifest.json") &&
			strings.Contains(path, "hubtask-export-"+exportTenantX.String()) {
			manifests++
		}
		return nil
	})
	if manifests != 1 {
		t.Errorf("found %d manifests of the export at the target, want one", manifests)
	}

	// The workspace's own reads name no installation target.
	if err := work.WithinReadOnly(ctx, persistence.Scope{TenantID: exportTenantX}, func(ctx context.Context) error {
		listed, err := targets.List(ctx)
		for _, each := range listed {
			if each.ID == target.ID {
				t.Error("the workspace lists the installation's target")
			}
		}
		return err
	}); err != nil {
		t.Fatalf("listing as the workspace: %v", err)
	}
	// Nor does the opener the data subject export, the audit archive and anchoring hold.
	if _, err := opener.OpenTarget(ctx, exportTenantX, target.ID); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the tenant-only opener reached the installation's target: %v", err)
	}
}
