// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	service "github.com/Jersyfi/hubtask/core/application/service/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/storage"
)

var snapshotAt = time.Date(2026, 8, 26, 3, 0, 0, 0, time.UTC)

// exportDouble is the repository, as a table a test writes by hand.
type exportDouble struct {
	rows       map[string][]repository.Row
	tombstones map[string][]repository.Tombstone
	locations  map[string]repository.MediaLocation
	askedSince map[string]time.Time
	failure    error
}

func newExport() *exportDouble {
	return &exportDouble{
		rows:       map[string][]repository.Row{},
		tombstones: map[string][]repository.Tombstone{},
		locations:  map[string]repository.MediaLocation{},
		askedSince: map[string]time.Time{},
	}
}

func (e *exportDouble) Rows(_ context.Context, table string, since time.Time, yield func(repository.Row) error) error {
	e.askedSince[table] = since
	if e.failure != nil {
		return e.failure
	}
	for _, row := range e.rows[table] {
		if err := yield(row); err != nil {
			return err
		}
	}
	return nil
}

func (e *exportDouble) Tombstones(_ context.Context, table string, since time.Time, yield func(repository.Tombstone) error) error {
	if since.IsZero() {
		return nil
	}
	for _, marker := range e.tombstones[table] {
		if err := yield(marker); err != nil {
			return err
		}
	}
	return nil
}

func (e *exportDouble) MediaLocation(_ context.Context, checksum string) (repository.MediaLocation, error) {
	location, found := e.locations[checksum]
	if !found {
		return repository.MediaLocation{}, shared.ErrNotFound
	}
	return location, nil
}

var _ repository.Export = (*exportDouble)(nil)

type objectDouble struct {
	content map[string]string
	asked   []string
}

func (o *objectDouble) Put(context.Context, storage.Upload) error { return nil }
func (o *objectDouble) Delete(context.Context, string) error      { return nil }

func (o *objectDouble) Get(_ context.Context, key string) (storage.Object, error) {
	o.asked = append(o.asked, key)
	content, found := o.content[key]
	if !found {
		return storage.Object{}, shared.ErrNotFound
	}
	return storage.Object{
		Content: io.NopCloser(strings.NewReader(content)), Size: int64(len(content)),
	}, nil
}

var _ storage.ObjectStore = (*objectDouble)(nil)

func collect(t *testing.T, source archive.Source, entity string) []archive.Record {
	t.Helper()
	found, ok := archive.FindEntity(entity)
	if !ok {
		t.Fatalf("no entity %s", entity)
	}
	var records []archive.Record
	err := source.Records(t.Context(), found, time.Time{}, func(r archive.Record) error {
		records = append(records, r)
		return nil
	})
	if err != nil {
		t.Fatalf("records: %v", err)
	}
	return records
}

func TestRowsBecomeUpsertsAndMarkersBecomeTombstones(t *testing.T) {
	export := newExport()
	export.rows["work_item"] = []repository.Row{
		{ID: "w1", ChangedAt: snapshotAt.Add(-time.Hour), Data: map[string]any{"state": "OPEN"}},
	}
	export.tombstones["work_item"] = []repository.Tombstone{
		{ID: "w2", DeletedAt: snapshotAt.Add(-30 * time.Minute)},
	}
	source := service.ExportSource{Export: export, SnapshotAt: snapshotAt}
	items, _ := archive.FindEntity("work_items")

	var records []archive.Record
	err := source.Records(t.Context(), items, snapshotAt.Add(-2*time.Hour), func(r archive.Record) error {
		records = append(records, r)
		return nil
	})
	if err != nil {
		t.Fatalf("records: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("records: %+v", records)
	}
	if records[0].Op != archive.OpUpsert || records[0].ID != "w1" {
		t.Fatalf("first record: %+v", records[0])
	}
	if records[1].Op != archive.OpDelete || records[1].ID != "w2" || records[1].Data != nil {
		t.Fatalf("second record: %+v", records[1])
	}
	for _, record := range records {
		if err := record.Validate(); err != nil {
			t.Fatalf("%s: %v", record.ID, err)
		}
	}
}

// A full archive is the whole truth and has nothing to deny, so it carries no markers.
func TestAFullArchiveCarriesNoTombstones(t *testing.T) {
	export := newExport()
	export.rows["label"] = []repository.Row{{ID: "l1", Data: map[string]any{"colour_token": "label.red"}}}
	export.tombstones["label"] = []repository.Tombstone{{ID: "l2", DeletedAt: snapshotAt}}

	records := collect(t, service.ExportSource{Export: export, SnapshotAt: snapshotAt}, "labels")
	for _, record := range records {
		if record.Op == archive.OpDelete {
			t.Fatalf("a full archive carried a tombstone: %+v", record)
		}
	}
}

// A row from a table that cannot date a change is stamped with the archive's own instant. It has
// to carry a time - a restore orders by it - and the honest one is when the archive was taken.
func TestARowThatCannotDateItselfIsStampedWithTheSnapshot(t *testing.T) {
	export := newExport()
	export.rows["item_label"] = []repository.Row{{ID: "w1/l1", Data: map[string]any{}}}

	records := collect(t, service.ExportSource{Export: export, SnapshotAt: snapshotAt}, "item_labels")
	if len(records) != 1 {
		t.Fatalf("records: %+v", records)
	}
	if !records[0].UpdatedAt.Equal(snapshotAt) {
		t.Fatalf("stamped %v, want the snapshot %v", records[0].UpdatedAt, snapshotAt)
	}
	if err := records[0].Validate(); err != nil {
		t.Fatalf("a row with no fields but its key was refused: %v", err)
	}
}

// The media row is the one that carries a blob reference, and only when the upload finished.
func TestOnlyAFinishedMediumCarriesItsBytes(t *testing.T) {
	digest := strings.Repeat("ab", 32)
	export := newExport()
	export.rows["media_object"] = []repository.Row{
		{ID: "m1", Data: map[string]any{
			"checksum": digest, "status": "READY", "byte_size": float64(12), "deleted_at": nil}},
		{ID: "m2", Data: map[string]any{
			"checksum": digest, "status": "PENDING", "byte_size": float64(12)}},
		{ID: "m3", Data: map[string]any{"status": "READY", "byte_size": float64(12)}},
		{ID: "m4", Data: map[string]any{
			"checksum": digest, "status": "READY", "byte_size": float64(12),
			"deleted_at": "2026-08-25T00:00:00Z"}},
	}

	records := collect(t, service.ExportSource{Export: export, SnapshotAt: snapshotAt}, "media_objects")
	if len(records) != 4 {
		t.Fatalf("every row is carried, bytes or no bytes: %+v", records)
	}
	if len(records[0].Blobs) != 1 || records[0].Blobs[0].Digest != digest {
		t.Fatalf("the finished upload carries no reference: %+v", records[0])
	}
	for _, record := range records[1:] {
		if len(record.Blobs) != 0 {
			t.Fatalf("%s carries bytes it should not: %+v", record.ID, record.Blobs)
		}
	}
}

// The archive addresses a medium by the SHA-256 of its content; the storage key is this
// installation's and stays here.
func TestAMediumIsFoundByItsContentAddress(t *testing.T) {
	digest := strings.Repeat("cd", 32)
	export := newExport()
	export.locations[digest] = repository.MediaLocation{StorageKey: "tenants/a/media/xyz", Bytes: 5}
	objects := &objectDouble{content: map[string]string{"tenants/a/media/xyz": "bytes"}}

	media := service.ExportMedia{Export: export, Objects: objects}
	stream, err := media.Open(t.Context(), digest)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = stream.Close() }()

	content, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(content) != "bytes" {
		t.Fatalf("content %q", content)
	}
	if len(objects.asked) != 1 || objects.asked[0] != "tenants/a/media/xyz" {
		t.Fatalf("the object store was asked for %v", objects.asked)
	}
}

// The row says the bytes are there and the bucket says they are not. That stops the run rather
// than being skipped: an archive missing an attachment nobody noticed is one whose restore is a
// surprise.
func TestBytesTheBucketNoLongerHasStopTheRun(t *testing.T) {
	digest := strings.Repeat("ef", 32)
	export := newExport()
	export.locations[digest] = repository.MediaLocation{StorageKey: "gone"}

	media := service.ExportMedia{Export: export, Objects: &objectDouble{content: map[string]string{}}}
	_, err := media.Open(t.Context(), digest)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("missing bytes: %v", err)
	}
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != archive.CodeMediaMissing {
		t.Fatalf("detail code: %v", err)
	}
}

func TestTheSourcePassesThePeriodOn(t *testing.T) {
	export := newExport()
	since := snapshotAt.Add(-24 * time.Hour)
	source := service.ExportSource{Export: export, SnapshotAt: snapshotAt}
	items, _ := archive.FindEntity("comments")

	if err := source.Records(t.Context(), items, since, func(archive.Record) error { return nil }); err != nil {
		t.Fatalf("records: %v", err)
	}
	if asked := export.askedSince["comment"]; !asked.Equal(since) {
		t.Fatalf("asked from %v, want %v", asked, since)
	}
}

func TestARepositoryFailureIsNotSwallowed(t *testing.T) {
	export := newExport()
	export.failure = shared.ErrUnavailable.WithDetail("postgres.query_failed")
	source := service.ExportSource{Export: export, SnapshotAt: snapshotAt}
	items, _ := archive.FindEntity("containers")

	err := source.Records(t.Context(), items, time.Time{}, func(archive.Record) error { return nil })
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("the failure was swallowed: %v", err)
	}
}
