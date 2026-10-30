// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
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

// The issue tracker's and the wiki's imports against the real schema (UC-IMP-04, UC-IMP-05):
// every record the converters spell - entry types, a workflow and its scheme, fields and their
// values, history, attachments with their bytes, links, pages - is one the database takes, and the
// second run creates nothing.

func zipOf(t *testing.T, members map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range members {
		member, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := member.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// runImport stages the file as an import, runs the runner over it and answers the run.
func runImport(ctx context.Context, t *testing.T, store storage.LocalStorage, hub shared.ID, kind domain.Kind, file []byte) domain.Run {
	t.Helper()
	object, err := media.NewPendingObject(media.NewObjectInput{
		ID: freshID(t), TenantID: tenantA, FileName: "export.zip", DeclaredSize: int64(len(file)), SizeLimit: 1 << 24,
		Usage: media.UsageImport, CreatedBy: authorA, Now: created,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, storageport.Upload{Key: object.StorageKey, Content: bytes.NewReader(file), Size: int64(len(file)), ContentType: "application/zip"}); err != nil {
		t.Fatal(err)
	}
	sealed, err := object.Sealed("application/zip", int64(len(file)))
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
		run.Kind, run.Mapping, run.Zone = kind, nil, "Europe/Berlin"
		return importRunRepo().Insert(ctx, run)
	}); err != nil {
		t.Fatalf("staging: %v", err)
	}
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	runner := importservice.Runner{
		Runs: importRunRepo(), Objects: mediaRepo(), Store: store,
		Converters: map[domain.Kind]importrepo.Converter{
			domain.KindIssueTracker: importadapter.IssueTracker{}, domain.KindWiki: importadapter.Wiki{},
		},
		Applier: backupservice.Applier{
			Restores: restoreRepo(), Import: postgres.NewBackupImportRepository(),
			Journal: postgres.NewDeletionJournalRepository(postgres.DefaultJournalBatch),
			Cipher:  crypto.NewStream(clockadapter.CryptoRandom{}), Objects: store,
			Epochs: postgres.NewEpochRepository(), UnitOfWork: unitOfWork,
			Clock: clock.Fixed(created), Batch: backupservice.DefaultRestoreBatch,
		},
		UnitOfWork: unitOfWork, Clock: clock.Fixed(created), MaxBytes: 1 << 24,
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
	return run
}

func importedCount(ctx context.Context, t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := adminPool(ctx, t).QueryRow(ctx, query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func TestAnIssueTrackersProjectsLandWithTheirConfigurationOnce(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := containerRepo().Insert(ctx, containerIn(tenantA, authorA, hub, "Tracker", "a3"))
		return err
	}); err != nil {
		t.Fatalf("seeding the hub: %v", err)
	}
	answer, err := os.ReadFile("../../infrastructure/importer/testdata/issue-tracker.json")
	if err != nil {
		t.Fatal(err)
	}
	file := zipOf(t, map[string][]byte{"page-1.json": answer, "attachments/7001/trace.txt": []byte("panic: card declined twice\n")})
	store := storage.NewLocalStorage(t.TempDir())

	run := runImport(ctx, t, store, hub, domain.KindIssueTracker, file)
	if run.Status != domain.StatusSucceeded {
		t.Fatalf("the run did not succeed: %+v", run)
	}
	tenant := tenantA.String()
	entries := `SELECT count(*) FROM work_item w JOIN container c ON c.id = w.collection_id AND c.tenant_id = w.tenant_id WHERE c.parent_id = $1 AND w.tenant_id = $2`
	if n := importedCount(ctx, t, entries, hub.String(), tenant); n != 4 {
		t.Errorf("entries = %d, want 4", n)
	}
	if n := importedCount(ctx, t, `SELECT count(*) FROM work_item WHERE tenant_id = $1 AND title = 'Card payment fails on retry' AND entry_type = 'bug' AND status = 'done' AND is_completed`, tenant); n != 1 {
		t.Errorf("the bug's type, status and completion: %d", n)
	}
	if n := importedCount(ctx, t, `SELECT count(*) FROM workflow_scheme s JOIN workflow f ON f.tenant_id = s.tenant_id AND f.key = s.workflow_key WHERE s.tenant_id = $1 AND f.key = 'imported_shop' AND jsonb_array_length(f.published->'statuses') = 3`, tenant); n != 1 {
		t.Errorf("the workflow bound to the collection: %d", n)
	}
	if n := importedCount(ctx, t, `SELECT count(*) FROM entry_type WHERE tenant_id = $1 AND key IN ('epic','bug','story','sub_task')`, tenant); n != 4 {
		t.Errorf("entry types: %d", n)
	}
	// The typed projection the query language ranges over is written by the trigger.
	if n := importedCount(ctx, t, `SELECT count(*) FROM entry_field_value v JOIN custom_field_definition d ON d.id = v.definition_id WHERE v.tenant_id = $1 AND d.key = 'story_points' AND v.num = 3`, tenant); n != 1 {
		t.Errorf("a number field's typed value: %d", n)
	}
	if n := importedCount(ctx, t, `SELECT count(*) FROM activity_entry WHERE tenant_id = $1 AND verb = 'item.history_imported' AND change_set->'changed_by'->>'to' = 'Grace Hopper'`, tenant); n != 1 {
		t.Errorf("the imported history: %d", n)
	}
	if n := importedCount(ctx, t, `SELECT count(*) FROM entry_link WHERE tenant_id = $1 AND link_type IN ('blocks','implements')`, tenant); n != 2 {
		t.Errorf("links: %d", n)
	}
	var key string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT m.storage_key FROM media_object m JOIN item_attachment a ON a.media_id = m.id AND a.tenant_id = m.tenant_id WHERE m.tenant_id = $1 AND m.file_name = 'trace.txt' AND m.ref_count = 1`, tenant).Scan(&key); err != nil {
		t.Fatalf("the attachment: %v", err)
	}
	stored, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("the attachment's bytes: %v", err)
	}
	content, _ := io.ReadAll(stored.Content)
	_ = stored.Content.Close()
	if string(content) != "panic: card declined twice\n" {
		t.Errorf("bytes = %q", content)
	}

	again := runImport(ctx, t, store, hub, domain.KindIssueTracker, file)
	if again.Status != domain.StatusSucceeded || again.Report.New != 0 {
		t.Errorf("the second run's report = %+v", again.Report)
	}
	if n := importedCount(ctx, t, entries, hub.String(), tenant); n != 4 {
		t.Errorf("entries after the second run = %d", n)
	}
}

func TestAWikiSpaceLandsAsTheHubsPages(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := containerRepo().Insert(ctx, containerIn(tenantA, authorA, hub, "Handbook", "a4"))
		return err
	}); err != nil {
		t.Fatalf("seeding the hub: %v", err)
	}
	page := func(title, crumbs, content string) []byte {
		return []byte(`<html><head><title>Handbook : ` + title + `</title></head><body><ol id="breadcrumbs"><li><a href="index.html">Handbook</a></li>` + crumbs +
			`</ol><span id="title-text">Handbook : ` + title + `</span><div id="main-content">` + content + `</div></body></html>`)
	}
	file := zipOf(t, map[string][]byte{
		"H/index.html":     []byte(`<html><head><title>Handbook</title></head></html>`),
		"H/Start_1.html":   page("Start", "", `<p>See <a href="Details_2.html">details</a>.</p>`),
		"H/Details_2.html": page("Details", `<li><a href="Start_1.html">Start</a></li>`, `<h2>More</h2><p><img src="attachments/2/9.png"/></p>`),
		"H/attachments/2/9.png": []byte("\x89PNG\r\n\x1a\nthe picture"),
	})
	store := storage.NewLocalStorage(t.TempDir())
	run := runImport(ctx, t, store, hub, domain.KindWiki, file)
	if run.Status != domain.StatusSucceeded {
		t.Fatalf("the run did not succeed: %+v", run)
	}
	if n := importedCount(ctx, t, `SELECT count(*) FROM page c JOIN page p ON p.id = c.parent_id AND p.tenant_id = c.tenant_id WHERE c.tenant_id = $1 AND c.hub_id = $2 AND c.title = 'Details' AND p.title = 'Start'`, tenantA.String(), hub.String()); n != 1 {
		t.Errorf("the tree: %d", n)
	}
	if n := importedCount(ctx, t, `SELECT count(*) FROM page_version v JOIN page p ON p.id = v.page_id AND p.tenant_id = v.tenant_id WHERE p.hub_id = $1`, hub.String()); n != 2 {
		t.Errorf("versions: %d", n)
	}
	// The link to a page of the space is a page reference, which the reference trigger keeps.
	if n := importedCount(ctx, t, `SELECT count(*) FROM page WHERE hub_id = $1 AND body LIKE '%hubtask:page/%'`, hub.String()); n != 1 {
		t.Errorf("a page reference: %d", n)
	}
	// The picture is the page's file, its bytes stored, and drawn by the body (UC-IMP-05 check 5).
	if n := importedCount(ctx, t, `SELECT count(*) FROM page_media g JOIN page p ON p.id = g.page_id AND p.tenant_id = g.tenant_id
		JOIN media_object m ON m.id = g.media_id AND m.tenant_id = g.tenant_id
		WHERE p.hub_id = $1 AND p.title = 'Details' AND m.mime_type = 'image/png' AND m.storage_key <> ''
		  AND p.body LIKE '%hubtask:media/' || m.id::text || '%'`, hub.String()); n != 1 {
		t.Errorf("the page's picture: %d", n)
	}
}
