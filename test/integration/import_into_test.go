// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"bytes"
	"context"
	"testing"

	importrepo "github.com/Jersyfi/hubtask/core/application/repository/importer"
	backupservice "github.com/Jersyfi/hubtask/core/application/service/backup"
	importservice "github.com/Jersyfi/hubtask/core/application/service/importer"
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

// An import into an existing collection against the real boundary: the collection's own column
// and label are met by the name its unique index compares - accents and case aside - rather than
// refused as a second one, nothing is created under the hub, and the same file twice lands
// nothing the second time.

// stageIntoCollection writes the file as a confirmed import upload and the run that lands it in
// the collection.
func stageIntoCollection(ctx context.Context, t *testing.T, store storage.LocalStorage, hub, collection shared.ID, file []byte) shared.ID {
	t.Helper()
	object, err := media.NewPendingObject(media.NewObjectInput{
		ID: freshID(t), TenantID: tenantA, FileName: "errands.csv", DeclaredSize: int64(len(file)), SizeLimit: 1 << 20,
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
		run.Mapping, run.CollectionID = nil, collection
		return importRunRepo().Insert(ctx, run)
	}); err != nil {
		t.Fatalf("staging: %v", err)
	}
	return runID
}

func TestARunnerLandsACsvInAnExistingCollectionOnce(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub, collection := freshID(t), freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := containerRepo().Insert(ctx, containerIn(tenantA, authorA, hub, "Imports into", "a3")); err != nil {
			return err
		}
		if err := containerRepo().Insert(ctx, collectionIn(tenantA, authorA, collection, hub, "Inbox", "a0")); err != nil {
			return err
		}
		if err := bucketRepo().Insert(ctx, bucketIn(tenantA, collection, freshID(t), "doing", "a5")); err != nil {
			return err
		}
		return labelRepo().Insert(ctx, labelIn(tenantA, collection, freshID(t), "Grün"))
	}); err != nil {
		t.Fatalf("seeding the collection: %v", err)
	}

	store := storage.NewLocalStorage(t.TempDir())
	file := []byte("title,labels,bucket,collection\nEins,grun;rot,Doing,Work\nZwei,,Done,Home\n")
	runID := stageIntoCollection(ctx, t, store, hub, collection, file)

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	runner := importservice.Runner{
		Runs: importRunRepo(), Objects: mediaRepo(), Store: store,
		Converters: map[domain.Kind]importrepo.Converter{domain.KindCSV: importadapter.CSV{}},
		Containers: containerRepo(), Buckets: bucketRepo(), Labels: labelRepo(), Items: itemRepo(),
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
	if run.Status != domain.StatusSucceeded || run.CollectionID != collection {
		t.Fatalf("the run = %+v", run)
	}

	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := adminPool(ctx, t).QueryRow(ctx, query, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(`SELECT count(*) FROM work_item WHERE collection_id = $1 AND tenant_id = $2`, collection.String(), tenantA.String()); n != 2 {
		t.Errorf("entries in the collection: %d, want 2", n)
	}
	if n := count(`SELECT count(*) FROM container WHERE parent_id = $1 AND tenant_id = $2`, hub.String(), tenantA.String()); n != 1 {
		t.Errorf("collections under the hub: %d, want the one that was there", n)
	}
	// *Doing* is *doing*, *grun* is *Grün*: one new column, three new labels (*rot* and the lists).
	if n := count(`SELECT count(*) FROM bucket WHERE collection_id = $1 AND tenant_id = $2`, collection.String(), tenantA.String()); n != 2 {
		t.Errorf("columns: %d, want 2", n)
	}
	if n := count(`SELECT count(*) FROM label WHERE collection_id = $1 AND tenant_id = $2`, collection.String(), tenantA.String()); n != 4 {
		t.Errorf("labels: %d, want 4", n)
	}
	if n := count(`SELECT count(*) FROM item_label il JOIN label l ON l.id = il.label_id AND l.tenant_id = il.tenant_id WHERE l.collection_id = $1 AND l.name = 'Grün' AND il.tenant_id = $2`, collection.String(), tenantA.String()); n != 1 {
		t.Errorf("the entry labelled *grun* carries the collection's *Grün*: %d", n)
	}

	// The same file again: the same identities, nothing new.
	secondID := stageIntoCollection(ctx, t, store, hub, collection, file)
	if err := runner.Run(ctx, importservice.RunInput{ImportID: secondID, TenantID: tenantA}); err != nil {
		t.Fatalf("the second runner failed: %v", err)
	}
	var second domain.Run
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		found, err := importRunRepo().Find(ctx, secondID)
		second = found
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if second.Status != domain.StatusSucceeded || second.Report.New != 0 {
		t.Errorf("the second run = %+v", second)
	}
	if n := count(`SELECT count(*) FROM work_item WHERE collection_id = $1 AND tenant_id = $2`, collection.String(), tenantA.String()); n != 2 {
		t.Errorf("entries after the second run: %d, want 2", n)
	}
}
