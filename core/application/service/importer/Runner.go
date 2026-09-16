// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	repository "github.com/Jersyfi/hubtask/core/application/repository/importer"
	mediarepo "github.com/Jersyfi/hubtask/core/application/repository/media"
	"github.com/Jersyfi/hubtask/core/application/service/backup"
	backupdomain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/storage"
)

// Runner performs one import, end to end, for the job (P-08).
//
// Claim the run, read the file back from the object store, convert it, write the records as an
// archive into memory, apply that archive through the restore's applier in MERGE mode with skip,
// finish the run with the report, advance the synchronisation epoch, mark the file for deletion.
// Every step but the conversion is one the restore performs too, which is what "one ingestion
// path" means in code.
type Runner struct {
	Runs       repository.Runs
	Objects    mediarepo.Objects
	Store      storage.ObjectStore
	Converters map[domain.Kind]repository.Converter
	Applier    backup.Applier
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	// MaxBytes bounds the file read back: the upload limit, which the object cannot exceed, held
	// here again because a bound a caller has to remember is a bound somebody forgets.
	MaxBytes int64
	// SchemaVersion and ProductVersion stamp the archive's manifest, as a backup's are.
	SchemaVersion  string
	ProductVersion string
}

// RunInput names the import and the tenant the job runs for.
type RunInput struct {
	ImportID shared.ID
	TenantID shared.ID
	Report   func(float64)
}

// Run performs the import. An error the run itself produced is recorded on the run and answered
// as nil, so that the job finishes rather than retrying a file that will fail the same way; an
// error reaching the database is returned, so that the job retries.
func (r Runner) Run(ctx context.Context, in RunInput) error {
	scope := persistence.Scope{TenantID: in.TenantID}

	var run domain.Run
	var object mediaObject
	claimed := false
	err := r.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		found, err := r.Runs.Find(ctx, in.ImportID)
		if err != nil {
			return err
		}
		run = found
		if run.Status == domain.StatusSucceeded || run.Status == domain.StatusFailed {
			return nil
		}
		ok, err := r.Runs.Claim(ctx, in.ImportID, r.Clock.Now())
		if err != nil {
			return err
		}
		claimed = ok
		if !ok {
			return nil
		}
		stored, err := r.Objects.Find(ctx, run.MediaID)
		if err != nil {
			return err
		}
		object = mediaObject{key: stored.StorageKey, size: stored.ByteSize}
		return nil
	})
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}

	report, refused, applied := r.perform(ctx, in, run, object)
	return r.finish(ctx, scope, run, report, refused, applied)
}

type mediaObject struct {
	key  string
	size int64
}

// perform is the fallible middle: the file, the conversion, the archive, the apply. What it
// answers is the outcome the run records.
func (r Runner) perform(ctx context.Context, in RunInput, run domain.Run, object mediaObject) (backupdomain.Report, []domain.Refusal, error) {
	converter, ok := r.Converters[run.Kind]
	if !ok {
		return backupdomain.Report{}, nil, shared.ErrValidation.WithDetail(domain.CodeKindUnsupported)
	}
	if r.MaxBytes > 0 && object.size > r.MaxBytes {
		return backupdomain.Report{}, nil, shared.ErrValidation.WithDetail(domain.CodeFileNotKind)
	}
	stored, err := r.Store.Get(ctx, object.key)
	if err != nil {
		return backupdomain.Report{}, nil, err
	}
	defer func() { _ = stored.Content.Close() }()
	content := io.Reader(stored.Content)
	if r.MaxBytes > 0 {
		content = io.LimitReader(content, r.MaxBytes+1)
	}

	// The file is read whole, once: the digest that salts the identities needs every byte, and
	// the converters parse a document rather than a stream. Bounded by the upload limit above.
	raw, err := io.ReadAll(content)
	if err != nil {
		return backupdomain.Report{}, nil, err
	}
	if r.MaxBytes > 0 && int64(len(raw)) > r.MaxBytes {
		return backupdomain.Report{}, nil, shared.ErrValidation.WithDetail(domain.CodeFileNotKind)
	}
	digest := sha256.Sum256(raw)

	now := r.Clock.Now()
	result, err := converter.Convert(ctx, repository.Source{
		Content: bytes.NewReader(raw), Hub: run.HubID, Digest: hex.EncodeToString(digest[:]),
		Mapping: run.Mapping, Now: now, Actor: run.RequestedBy, Zone: run.Zone, Language: run.Language,
	})
	if err != nil {
		return backupdomain.Report{}, nil, err
	}

	store := newMemoryStore()
	prefix := "import-" + run.ID.String()
	_, err = archive.NewWriter(store, r.Applier.Cipher, nil).Write(ctx, archive.Request{
		ArchiveID: run.ID, Prefix: prefix,
		Scope:          archive.Scope{Kind: archive.ScopeTenant, ID: in.TenantID.String()},
		Mode:           archive.ModeFull,
		SnapshotAt:     now,
		SchemaVersion:  r.SchemaVersion,
		ProductVersion: r.ProductVersion,
		Encryption:     archive.Encryption{Mode: archive.EncryptionNone},
	}, recordSource{records: result.Records})
	if err != nil {
		return backupdomain.Report{}, result.Refused, err
	}

	report, err := r.Applier.Ingest(ctx, backup.IngestInput{
		TenantID: in.TenantID, RunID: run.ID, Store: store, Prefix: prefix,
		Progress: func(ctx context.Context, report backupdomain.Report, decided map[string]int) error {
			return r.Runs.RecordProgress(ctx, run.ID, report, decided)
		},
		Resume: backupdomain.Restore{Progress: run.Progress, Report: run.Report},
		Report: in.Report,
	})
	return report, result.Refused, err
}

// finish records the outcome, advances the epoch on success, and marks the file for deletion
// either way - a file somebody imported is not a file they attached.
func (r Runner) finish(ctx context.Context, scope persistence.Scope, run domain.Run, report backupdomain.Report, refused []domain.Refusal, applied error) error {
	status := domain.StatusSucceeded
	code := ""
	if applied != nil {
		status = domain.StatusFailed
		code = errorCode(applied)
	}
	err := r.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		if err := r.Runs.Finish(ctx, domain.Outcome{
			ID: run.ID, Status: status, Report: report, Refused: refused, ErrorCode: code, FinishedAt: r.Clock.Now(),
		}); err != nil {
			return err
		}
		if _, err := r.Objects.MarkDeleted(ctx, run.MediaID, r.Clock.Now()); err != nil && !errors.Is(err, shared.ErrNotFound) {
			return err
		}
		if status == domain.StatusSucceeded {
			return r.Applier.AdvanceEpoch(ctx)
		}
		return nil
	})
	if err != nil {
		return err
	}
	// A failure of the file is the run's outcome, recorded above; the job is done with it.
	if applied != nil && !isTransient(applied) {
		return nil
	}
	return applied
}

// errorCode is what the run records about a failure: the typed error's detail, or the category.
func errorCode(err error) string {
	var typed *shared.Error
	if errors.As(err, &typed) {
		if typed.DetailCode != "" {
			return typed.DetailCode
		}
		return typed.Code
	}
	return domain.CodeFileNotKind
}

// isTransient says whether a failure is the database or the store being away - worth a retry -
// rather than the file being what it is.
func isTransient(err error) bool {
	var typed *shared.Error
	if errors.As(err, &typed) {
		return typed.Category == shared.CategoryUnavailable || typed.Category == shared.CategoryInternal
	}
	return true
}

// recordSource hands the converter's records to the writer in the archive's entity order.
type recordSource struct {
	records map[string][]archive.Record
}

func (s recordSource) Records(_ context.Context, entity archive.Entity, _ time.Time, yield func(archive.Record) error) error {
	for _, record := range s.records[entity.Name] {
		if err := yield(record); err != nil {
			return err
		}
	}
	return nil
}
