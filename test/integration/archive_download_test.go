// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/infrastructure/backupstorage"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	cryptoadapter "github.com/Jersyfi/hubtask/infrastructure/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

type ownerAlong struct{}

func (ownerAlong) RoleAlong(context.Context, appshared.ActorContext, []identity.Scope) (identity.Role, bool, error) {
	return identity.RoleOwner, true, nil
}

type anyProof struct{}

func (anyProof) Available() bool { return true }
func (anyProof) Methods(context.Context, shared.ID, shared.ID) ([]stepup.Method, error) {
	return nil, nil
}
func (anyProof) Satisfied(context.Context, shared.ID, string) (bool, error) { return true, nil }

type noExportQuota struct{}

func (noExportQuota) ExportJobs(context.Context, string) error { return nil }

func archiveDownload(ctx context.Context, t *testing.T, at time.Time) adminservice.DownloadWorkspaceArchive {
	t.Helper()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	return adminservice.DownloadWorkspaceArchive{
		Roles: ownerAlong{}, StepUp: anyProof{}, Downloads: postgres.NewAdminTenantRepository(),
		Quota: noExportQuota{}, Audit: discardAudit{}, UnitOfWork: uow,
		Clock: clock.Fixed(at), IDs: clockadapter.NewUUIDv7(clockadapter.System{}),
		Archive: adminservice.TenantExportArchivist{
			Rows:    postgres.NewBackupExportRepository(postgres.DefaultExportBatch),
			Objects: exportObjects{}, Cipher: cryptoadapter.NewStream(clockadapter.CryptoRandom{}),
			Snapshot: uow, Clock: clockadapter.System{}, SchemaVersion: "test", ProductVersion: "test",
		},
	}
}

func downloadedAt(ctx context.Context, t *testing.T, tenant shared.ID) *time.Time {
	t.Helper()
	var at *time.Time
	if err := adminPool(ctx, t).QueryRow(ctx, `SELECT archive_downloaded_at FROM tenant WHERE id = $1`,
		tenant.String()).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at
}

// UC-BAK-07 check 7: the owner's download is the whole archive as one ZIP file that restores as a
// new workspace; a second one inside 24 hours is refused; the window is the workspace's own, so
// another workspace's owner neither takes nor frees it.
func TestTheOwnersDownloadRestoresAndComesOnceADay(t *testing.T) {
	ctx := context.Background()
	f := newForeignRestore(ctx, t)
	scratch, err := backupstorage.NewLocalStore(t.TempDir(), "scratch")
	if err != nil {
		t.Fatal(err)
	}
	source, _ := foreignWorkspace(ctx, t, scratch, "Carry the boxes")
	neighbour, _ := foreignWorkspace(ctx, t, scratch, "Stay where you are")
	owner := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: source, AccountID: freshID(t)}
	at := time.Now().UTC().Truncate(time.Microsecond)

	download, err := archiveDownload(ctx, t, at).Start(ctx, owner, adminservice.DownloadWorkspaceArchiveCommand{StepUpToken: "p"})
	if err != nil {
		t.Fatalf("the owner's download was refused: %v", err)
	}
	var stream bytes.Buffer
	zipped := rest.NewZipStream(&stream)
	if err := download.WriteTo(ctx, zipped); err != nil {
		t.Fatalf("streaming the archive: %v", err)
	}
	if err := zipped.Close(); err != nil {
		t.Fatal(err)
	}
	if downloadedAt(ctx, t, neighbour) != nil {
		t.Error("the neighbour's window was taken by another workspace's download")
	}

	// Unpacked onto a target of "another installation", it restores as a new workspace.
	unpacked, err := zip.NewReader(bytes.NewReader(stream.Bytes()), int64(stream.Len()))
	if err != nil {
		t.Fatalf("the download is no ZIP file: %v", err)
	}
	for _, member := range unpacked.File {
		opened, err := member.Open()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.Put(ctx, member.Name, opened); err != nil {
			t.Fatal(err)
		}
	}
	described, err := archive.NewReader(f.store, cryptoadapter.NewStream(clockadapter.CryptoRandom{})).List(ctx, "")
	if err != nil || len(described) != 1 || described[0].Manifest.Scope.ID != source.String() {
		t.Fatalf("the unpacked archive lists %+v, %v", described, err)
	}
	deleteForGood(ctx, t, source)
	if _, minted, err := f.restoreAsNewWorkspace(ctx, t, described[0].Prefix); err != nil {
		t.Fatalf("restoring the download: %v", err)
	} else {
		var title string
		if err := adminPool(ctx, t).QueryRow(ctx, `SELECT title FROM work_item WHERE tenant_id = $1`,
			minted.String()).Scan(&title); err != nil || title != "Carry the boxes" {
			t.Errorf("the restored workspace holds %q (%v)", title, err)
		}
		// A restore names its new workspace by the fixed clock, as the other foreign restores do.
		deleteForGood(ctx, t, minted)
	}

	// Inside the window: the neighbour downloads, its own window; a second request of the
	// neighbour's is refused with the time left.
	later := at.Add(time.Hour)
	neighbourOwner := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: neighbour, AccountID: freshID(t)}
	if _, err := archiveDownload(ctx, t, later).Start(ctx, neighbourOwner, adminservice.DownloadWorkspaceArchiveCommand{StepUpToken: "p"}); err != nil {
		t.Fatalf("the neighbour was refused by another workspace's window: %v", err)
	}
	_, err = archiveDownload(ctx, t, later.Add(time.Hour)).Start(ctx, neighbourOwner, adminservice.DownloadWorkspaceArchiveCommand{StepUpToken: "p"})
	if !errors.Is(err, shared.ErrRateLimited) || shared.AsError(err).Params["retry_after"] != "82800" {
		t.Errorf("a second download inside 24 hours answered %v", err)
	}
}
