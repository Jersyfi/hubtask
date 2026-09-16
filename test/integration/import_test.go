// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"testing"

	importrepo "github.com/Jersyfi/hubtask/core/application/repository/importer"
	backupservice "github.com/Jersyfi/hubtask/core/application/service/backup"
	importservice "github.com/Jersyfi/hubtask/core/application/service/importer"
	backupdomain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	storageport "github.com/Jersyfi/hubtask/core/port/storage"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/crypto"
	importadapter "github.com/Jersyfi/hubtask/infrastructure/importer"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/storage"
)

// The import from another system against the real boundary (P-08): the run's row round-trips,
// no method of it crosses a tenant (gate SG-3), and the whole runner - the file read back, the
// CSV converted, the archive applied through the restore's applier - lands a collection under the
// hub, and lands nothing the second time.

func importRunRepo() postgres.ImportRunRepository { return postgres.NewImportRunRepository() }

func importRunIn(tenant, id, hub, mediaID shared.ID) domain.Run {
	return domain.Run{
		ID: id, TenantID: tenant, RequestedBy: authorA, Kind: domain.KindCSV,
		MediaID: mediaID, HubID: hub, Mapping: map[string]string{"title": "Aufgabe"},
		Status: domain.StatusPending, CreatedAt: created,
	}
}

func TestAnImportRunIsWrittenAndReadBackWhole(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	id, hub, mediaID := freshID(t), freshID(t), freshID(t)

	var found domain.Run
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := importRunRepo().Insert(ctx, importRunIn(tenantA, id, hub, mediaID)); err != nil {
			return err
		}
		claimed, err := importRunRepo().Claim(ctx, id, created)
		if err != nil || !claimed {
			t.Fatalf("claim: %v %v", claimed, err)
		}
		if err := importRunRepo().RecordProgress(ctx, id, backupdomain.Report{New: 3, Entities: map[string]int{"work_items": 3}}, map[string]int{"work_items": 3}); err != nil {
			return err
		}
		if err := importRunRepo().Finish(ctx, domain.Outcome{
			ID: id, Status: domain.StatusSucceeded, Report: backupdomain.Report{New: 4, Entities: map[string]int{"work_items": 3, "containers": 1}},
			Refused: []domain.Refusal{{Row: 7, Code: domain.CodeRowDateInvalid}}, FinishedAt: created,
		}); err != nil {
			return err
		}
		found, err = importRunRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if found.Status != domain.StatusSucceeded || found.Report.New != 4 || found.Report.Entities["containers"] != 1 || found.Mapping["title"] != "Aufgabe" {
		t.Errorf("read back: %+v", found)
	}
	if len(found.Refused) != 1 || found.Refused[0].Row != 7 || found.Progress["work_items"] != 3 {
		t.Errorf("refused/progress: %+v %+v", found.Refused, found.Progress)
	}
	if found.FinishedAt == nil || found.StartedAt == nil {
		t.Errorf("the stamps did not travel: %+v", found)
	}
	// A finished run refuses a second outcome.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return importRunRepo().Finish(ctx, domain.Outcome{ID: id, Status: domain.StatusFailed, FinishedAt: created})
	}); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a second outcome should conflict: %v", err)
	}
}

// Gate SG-3: every method of the port, from the other tenant.
func TestAnImportRunIsInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	id, hub, mediaID := freshID(t), freshID(t), freshID(t)
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return importRunRepo().Insert(ctx, importRunIn(tenantB, id, hub, mediaID))
	}); err != nil {
		t.Fatalf("seeding B's run: %v", err)
	}

	var find, progress, finish error
	var claimed bool
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, find = importRunRepo().Find(ctx, id)
		var err error
		if claimed, err = importRunRepo().Claim(ctx, id, created); err != nil {
			return err
		}
		progress = importRunRepo().RecordProgress(ctx, id, backupdomain.Report{}, nil)
		finish = importRunRepo().Finish(ctx, domain.Outcome{ID: id, Status: domain.StatusFailed, FinishedAt: created})
		return nil
	}); err != nil {
		t.Fatalf("from A: %v", err)
	}
	if !errors.Is(find, shared.ErrNotFound) {
		t.Errorf("tenant A found tenant B's import: %v", find)
	}
	if claimed {
		t.Error("tenant A claimed tenant B's import")
	}
	if !errors.Is(progress, shared.ErrConflict) || !errors.Is(finish, shared.ErrConflict) {
		t.Errorf("tenant A wrote tenant B's import: %v %v", progress, finish)
	}
	var status string
	if err := adminPool(ctx, t).QueryRow(ctx, `SELECT status FROM import_run WHERE id = $1`, id.String()).Scan(&status); err != nil || status != "PENDING" {
		t.Errorf("B's row moved: %q %v", status, err)
	}
}

// The whole path: a CSV staged as an import, the runner, and the collection it leaves under the
// hub - once, however often the same file is run.
func TestARunnerLandsACsvUnderTheHubOnce(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return containerRepo().Insert(ctx, containerIn(tenantA, authorA, hub, "Imports", "m"))
	}); err != nil {
		t.Fatalf("seeding the hub: %v", err)
	}

	file := []byte("title,due,labels,bucket\nEins,2026-09-20,rot;blau,Doing\nZwei,2026-09-21 10:00,rot,Done\nDrei,nope,,\n")
	store := storage.NewLocalStorage(t.TempDir())
	object, err := media.NewPendingObject(media.NewObjectInput{
		ID: freshID(t), TenantID: tenantA, FileName: "tasks.csv", DeclaredSize: int64(len(file)), SizeLimit: 1 << 20,
		Usage: media.UsageImport, CreatedBy: authorA, Now: created,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, storageport.Upload{Key: object.StorageKey, Content: bytes.NewReader(file), Size: int64(len(file)), ContentType: "text/csv"}); err != nil {
		t.Fatal(err)
	}
	sealed, err := object.Sealed("text/csv", int64(len(file)))
	if err != nil {
		t.Fatal(err)
	}
	runID := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := mediaRepo().Insert(ctx, object); err != nil {
			return err
		}
		if err := mediaRepo().Seal(ctx, sealed); err != nil {
			return err
		}
		run := importRunIn(tenantA, runID, hub, object.ID)
		run.Mapping = nil
		return importRunRepo().Insert(ctx, run)
	}); err != nil {
		t.Fatalf("staging: %v", err)
	}

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	runner := importservice.Runner{
		Runs: importRunRepo(), Objects: mediaRepo(), Store: store,
		Converters: map[domain.Kind]importrepo.Converter{domain.KindCSV: importadapter.CSV{}},
		Applier: backupservice.Applier{
			Restores: restoreRepo(), Import: postgres.NewBackupImportRepository(),
			Journal: postgres.NewDeletionJournalRepository(postgres.DefaultJournalBatch),
			Cipher:  crypto.NewStream(clockadapter.CryptoRandom{}), Objects: store,
			Epochs: postgres.NewEpochRepository(), UnitOfWork: unitOfWork,
			Clock: clock.Fixed(created), Batch: backupservice.DefaultRestoreBatch,
		},
		UnitOfWork: unitOfWork, Clock: clock.Fixed(created), MaxBytes: 1 << 20,
		SchemaVersion: "test", ProductVersion: "test",
	}
	if err := runner.Run(ctx, importservice.RunInput{ImportID: runID, TenantID: tenantA}); err != nil {
		t.Fatalf("the runner failed: %v", err)
	}

	var run domain.Run
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		found, err := importRunRepo().Find(ctx, runID)
		run = found
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if run.Status != domain.StatusSucceeded {
		t.Fatalf("the run did not succeed: %+v", run)
	}
	// One collection, two buckets, two labels, two entries, three links: eleven new rows; one
	// row refused by its date.
	if run.Report.New != 10 || run.Report.Entities["work_items"] != 2 || run.Report.Entities["containers"] != 1 {
		t.Errorf("report = %+v", run.Report)
	}
	if len(run.Refused) != 1 || run.Refused[0].Row != 4 {
		t.Errorf("refused = %+v", run.Refused)
	}
	var count int
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT count(*) FROM work_item w JOIN container c ON c.id = w.collection_id AND c.tenant_id = w.tenant_id WHERE c.parent_id = $1 AND w.tenant_id = $2`,
		hub.String(), tenantA.String()).Scan(&count); err != nil || count != 2 {
		t.Errorf("entries under the hub: %d %v", count, err)
	}
	var deleted bool
	if err := adminPool(ctx, t).QueryRow(ctx, `SELECT deleted_at IS NOT NULL FROM media_object WHERE id = $1`, object.ID.String()).Scan(&deleted); err != nil || !deleted {
		t.Errorf("the file was not marked for deletion: %v %v", deleted, err)
	}

	// The same file, a second run: the same identities, skipped.
	secondID := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		again, err := media.NewPendingObject(media.NewObjectInput{
			ID: freshID(t), TenantID: tenantA, FileName: "tasks.csv", DeclaredSize: int64(len(file)), SizeLimit: 1 << 20,
			Usage: media.UsageImport, CreatedBy: authorA, Now: created,
		})
		if err != nil {
			return err
		}
		if err := store.Put(ctx, storageport.Upload{Key: again.StorageKey, Content: bytes.NewReader(file), Size: int64(len(file)), ContentType: "text/csv"}); err != nil {
			return err
		}
		sealedAgain, _ := again.Sealed("text/csv", int64(len(file)))
		if err := mediaRepo().Insert(ctx, again); err != nil {
			return err
		}
		if err := mediaRepo().Seal(ctx, sealedAgain); err != nil {
			return err
		}
		run := importRunIn(tenantA, secondID, hub, again.ID)
		run.Mapping = nil
		return importRunRepo().Insert(ctx, run)
	}); err != nil {
		t.Fatal(err)
	}
	if err := runner.Run(ctx, importservice.RunInput{ImportID: secondID, TenantID: tenantA}); err != nil {
		t.Fatalf("the second runner failed: %v", err)
	}
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT count(*) FROM work_item w JOIN container c ON c.id = w.collection_id AND c.tenant_id = w.tenant_id WHERE c.parent_id = $1 AND w.tenant_id = $2`,
		hub.String(), tenantA.String()).Scan(&count); err != nil || count != 2 {
		// The same file into the same hub derives the same identities, and MERGE with skip
		// leaves every one of them as it is: the second import created nothing.
		t.Errorf("entries after the second run: %d %v", count, err)
	}
	var second domain.Run
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		found, err := importRunRepo().Find(ctx, secondID)
		second = found
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if second.Status != domain.StatusSucceeded || second.Report.New != 0 || second.Report.Skipped != 10 {
		t.Errorf("the second run's report = %+v", second.Report)
	}
}
