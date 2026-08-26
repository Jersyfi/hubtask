// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/storage"
)

// ExportSource is the tenant, as the archive writer reads it (E-05).
//
// It is the one place the two vocabularies meet: the repository answers in table names because
// that is what the schema and the deletion markers are written in, and the archive asks in entity
// names because that is what a restore in another version reads. Everything either side of this
// type stays in its own language.
type ExportSource struct {
	Export repository.Export
	// SnapshotAt is the instant the export's snapshot was taken, and it is what a row that cannot
	// date itself is stamped with.
	//
	// Not the process clock and not a zero value: a record has to carry a time - a restore orders
	// by it - and the honest time for "this row was seen in this archive, and its table cannot say
	// when it last changed" is the instant the archive represents.
	SnapshotAt time.Time
}

var _ archive.Source = ExportSource{}

// Records hands the writer one entity: the rows first, then the deletions of the same period.
//
// The deletions come after the rows rather than interleaved with them, and they may name rows the
// same file has just carried as upserts - a row created and deleted between two runs appears in
// both halves. That is not a contradiction to resolve here: within one entity a later line
// supersedes an earlier one, which the archive format says in as many words, so the deletion wins
// by being last. Sorting the two halves together would cost a pass over both and decide the same
// thing.
func (s ExportSource) Records(
	ctx context.Context, entity archive.Entity, since time.Time, yield func(archive.Record) error,
) error {
	err := s.Export.Rows(ctx, entity.Table, since, func(row repository.Row) error {
		changedAt := row.ChangedAt
		if changedAt.IsZero() {
			changedAt = s.SnapshotAt
		}
		record := archive.Record{
			ID: row.ID, Op: archive.OpUpsert, UpdatedAt: changedAt, Data: row.Data,
		}
		if blob, carries := blobOf(entity, row); carries {
			record.Blobs = []archive.Blob{blob}
		}
		return yield(record)
	})
	if err != nil {
		return err
	}

	return s.Export.Tombstones(ctx, entity.Table, since, func(marker repository.Tombstone) error {
		return yield(archive.Record{
			ID: marker.ID, Op: archive.OpDelete, UpdatedAt: marker.DeletedAt,
		})
	})
}

// blobOf answers the medium a row refers to, if it refers to one at all.
//
// Only `media_object` does, and only when the upload finished: a PENDING row is one whose bytes
// were never read back and judged (C-06), and one with no recorded checksum has no content address
// for the archive to store it under. Both keep their row and lose their bytes, which is the honest
// outcome - a restore then knows the attachment existed and that its content is gone, rather than
// finding a reference to a file nothing wrote.
func blobOf(entity archive.Entity, row repository.Row) (archive.Blob, bool) {
	if entity.Table != mediaObjectTable {
		return archive.Blob{}, false
	}
	checksum, isText := row.Data["checksum"].(string)
	if !isText || checksum == "" {
		return archive.Blob{}, false
	}
	if status, _ := row.Data["status"].(string); status != mediaReady {
		return archive.Blob{}, false
	}
	if deleted, present := row.Data["deleted_at"]; present && deleted != nil {
		return archive.Blob{}, false
	}
	size, isNumber := row.Data["byte_size"].(float64)
	if !isNumber {
		return archive.Blob{}, false
	}
	return archive.Blob{Digest: checksum, Bytes: int64(size)}, true
}

const (
	mediaObjectTable = "media_object"
	mediaReady       = "READY"
)

// ExportMedia is the object store, addressed the way an archive addresses things: by the SHA-256
// of the content rather than by the key it happens to be stored under.
//
// The indirection is what makes the archive portable. A storage key is this installation's - it
// carries a tenant, a prefix and whatever the bucket layout was that year - and an archive that
// stored bytes under one would be an archive only this installation could read back.
type ExportMedia struct {
	Export  repository.Export
	Objects storage.ObjectStore
}

var _ archive.Media = ExportMedia{}

// Open answers the medium's content.
func (m ExportMedia) Open(ctx context.Context, digest string) (io.ReadCloser, error) {
	location, err := m.Export.MediaLocation(ctx, digest)
	if err != nil {
		return nil, err
	}
	object, err := m.Objects.Get(ctx, location.StorageKey)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			// The row says the bytes are there and the bucket says they are not. That is worth
			// stopping the run for rather than skipping: an archive missing an attachment nobody
			// noticed is one whose restore is a surprise, and the reconciliation job (C-06) is
			// what fixes the row.
			return nil, shared.ErrNotFound.WithDetail(archive.CodeMediaMissing).
				WithParams(map[string]string{"sha256": digest}).WithCause(err)
		}
		return nil, err
	}
	return object.Content, nil
}
