// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// BackupRunRepository stores what happened (E-05).
type BackupRunRepository struct{}

func NewBackupRunRepository() BackupRunRepository { return BackupRunRepository{} }

var _ repository.Runs = BackupRunRepository{}

// Start writes the run and answers whether it got the target.
//
// The lock §5 asks for is the statement, not a check this method ran a moment earlier: the insert
// carries `WHERE NOT EXISTS (... status = 'RUNNING')`, so a second run at the same target writes
// nothing and finds out by getting no row back. A check followed by an insert would have a gap
// between them wide enough for exactly the thing it is meant to prevent.
func (r BackupRunRepository) Start(ctx context.Context, run domain.Run) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(run.ID)
	if err != nil {
		return false, err
	}
	targetID, err := uuidOf(run.TargetID)
	if err != nil {
		return false, err
	}
	scheduleID, err := optionalUUID(run.ScheduleID)
	if err != nil {
		return false, err
	}
	tenantID, err := optionalUUID(run.TenantID)
	if err != nil {
		return false, err
	}
	parentID, err := optionalUUID(run.ParentRunID)
	if err != nil {
		return false, err
	}

	affected, err := queries.InsertBackupRun(ctx, sqlc.InsertBackupRunParams{
		ID: id, ScheduleID: scheduleID, TargetID: targetID, TenantID: tenantID,
		ParentRunID: parentID, Trigger: string(run.Trigger), Mode: string(run.Mode),
		SnapshotAt: optionalTimestamp(timeOrNil(run.SnapshotAt)),
		StartedAt:  timestampOf(run.StartedAt),
	})
	if err != nil {
		return false, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("starting backup run %s: %w", run.ID, err))
	}
	if affected == 1 {
		return true, nil
	}

	// Nothing was written, and there are two reasons for that. Either another run holds the
	// target, or this run's own row is already there - the attempt that takes over after a worker
	// died is the same run, and it has to be able to carry on rather than be locked out by
	// itself (BK-7). Reading the row is what tells the two apart.
	existing, err := r.Find(ctx, run.ID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return existing.Status == domain.RunRunning, nil
}

// Find answers one run.
func (r BackupRunRepository) Find(ctx context.Context, id shared.ID) (domain.Run, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Run{}, err
	}
	runID, err := uuidOf(id)
	if err != nil {
		return domain.Run{}, err
	}

	row, err := queries.FindBackupRun(ctx, runID)
	if err != nil {
		if IsNoRows(err) {
			return domain.Run{}, shared.ErrNotFound.WithDetail(domain.CodeRunNotFound)
		}
		return domain.Run{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading backup run %s: %w", id, err))
	}
	return runOf(runRow(row))
}

// Finish records how a run ended and what it left behind.
func (r BackupRunRepository) Finish(ctx context.Context, outcome domain.Outcome) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(outcome.ID)
	if err != nil {
		return err
	}

	affected, err := queries.FinishBackupRun(ctx, sqlc.FinishBackupRunParams{
		Status:      string(outcome.Status),
		ArchivePath: optionalText(outcome.ArchivePath),
		Manifest:    outcome.Manifest,
		SizeBytes:   optionalBytes(outcome.SizeBytes),
		ItemCount:   optionalRows(outcome.ItemCount),
		MediaCount:  optionalRows(outcome.MediaCount),
		Checksum:    optionalText(outcome.Checksum),
		SnapshotAt:  optionalTimestamp(timeOrNil(outcome.SnapshotAt)),
		FinishedAt:  timestampOf(outcome.FinishedAt),
		ErrorCode:   optionalText(outcome.ErrorCode),
		ID:          id,
	})
	if err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("finishing backup run %s: %w", outcome.ID, err))
	}
	if affected == 0 {
		// The run is no longer RUNNING: somebody cancelled it, or a worker that fell behind is
		// writing an outcome for work another one has already redone. A conflict rather than a
		// silent overwrite - a cancelled run must stay cancelled.
		return shared.ErrConflict.WithDetail(domain.CodeRunNotRunning).
			WithParams(map[string]string{"run_id": outcome.ID.String()})
	}
	return nil
}

// LatestSuccessful is the archive an incremental continues.
func (r BackupRunRepository) LatestSuccessful(
	ctx context.Context, targetID shared.ID,
) (domain.Run, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Run{}, err
	}
	id, err := uuidOf(targetID)
	if err != nil {
		return domain.Run{}, err
	}

	row, err := queries.LatestSuccessfulBackupRun(ctx, id)
	if err != nil {
		if IsNoRows(err) {
			return domain.Run{}, shared.ErrNotFound.WithDetail(domain.CodeNoParentArchive)
		}
		return domain.Run{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the last successful run at target %s: %w", targetID, err))
	}
	return runOf(runRow(row))
}

// RecordVerification writes down what `:verify` found.
func (r BackupRunRepository) RecordVerification(
	ctx context.Context, id shared.ID, at time.Time, ok bool,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	runID, err := uuidOf(id)
	if err != nil {
		return err
	}

	affected, err := queries.RecordBackupVerification(ctx, sqlc.RecordBackupVerificationParams{
		VerifiedAt: timestampOf(at), VerifyOk: &ok, ID: runID,
	})
	if err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording the verification of run %s: %w", id, err))
	}
	if affected == 0 {
		return shared.ErrNotFound.WithDetail(domain.CodeRunNotFound)
	}
	return nil
}

// SetExpiry records when the generation plan expects an archive to go.
func (r BackupRunRepository) SetExpiry(
	ctx context.Context, id shared.ID, expiresAt time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	runID, err := uuidOf(id)
	if err != nil {
		return err
	}

	err = queries.SetBackupRunExpiry(ctx, sqlc.SetBackupRunExpiryParams{
		ExpiresAt: optionalTimestamp(timeOrNil(expiresAt)), ID: runID,
	})
	if err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording the expiry of run %s: %w", id, err))
	}
	return nil
}

// MarkExpired moves a run whose archive has been deleted to EXPIRED.
func (r BackupRunRepository) MarkExpired(ctx context.Context, id shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	runID, err := uuidOf(id)
	if err != nil {
		return err
	}

	if err := queries.ExpireBackupRun(ctx, runID); err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("expiring run %s: %w", id, err))
	}
	return nil
}

// LastSuccessPerTarget is the number alert A-12 watches.
func (r BackupRunRepository) LastSuccessPerTarget(
	ctx context.Context,
) (map[shared.ID]time.Time, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.LastSuccessfulBackupPerTarget(ctx)
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the last successful backups: %w", err))
	}

	answer := make(map[shared.ID]time.Time, len(rows))
	for _, row := range rows {
		targetID, err := idFrom(row.TargetID)
		if err != nil {
			return nil, err
		}
		if row.LastSuccessAt.Valid {
			answer[targetID] = row.LastSuccessAt.Time.UTC()
		}
	}
	return answer, nil
}

// runRow is the shape both read statements answer: the table's columns without the manifest, which
// is a copy of a file and has no business in a resource.
type runRow struct {
	ID          pgtype.UUID
	ScheduleID  pgtype.UUID
	TargetID    pgtype.UUID
	TenantID    pgtype.UUID
	ParentRunID pgtype.UUID
	Trigger     string
	Mode        string
	Status      string
	ArchivePath *string
	SizeBytes   *int64
	ItemCount   *int32
	MediaCount  *int32
	Checksum    *string
	SnapshotAt  pgtype.Timestamptz
	StartedAt   pgtype.Timestamptz
	FinishedAt  pgtype.Timestamptz
	ErrorCode   *string
	ExpiresAt   pgtype.Timestamptz
	VerifiedAt  pgtype.Timestamptz
	VerifyOk    *bool
}

func runOf(row runRow) (domain.Run, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return domain.Run{}, err
	}
	targetID, err := idFrom(row.TargetID)
	if err != nil {
		return domain.Run{}, err
	}
	scheduleID, err := optionalID(row.ScheduleID)
	if err != nil {
		return domain.Run{}, err
	}
	tenantID, err := optionalID(row.TenantID)
	if err != nil {
		return domain.Run{}, err
	}
	parentID, err := optionalID(row.ParentRunID)
	if err != nil {
		return domain.Run{}, err
	}

	run := domain.Run{
		ID: id, ScheduleID: scheduleID, TargetID: targetID, TenantID: tenantID,
		ParentRunID: parentID, Trigger: domain.Trigger(row.Trigger), Mode: domain.Mode(row.Mode),
		Status: domain.RunStatus(row.Status), StartedAt: row.StartedAt.Time.UTC(),
		VerifyOK: row.VerifyOk,
	}
	if row.ArchivePath != nil {
		run.ArchivePath = *row.ArchivePath
	}
	if row.SizeBytes != nil {
		run.SizeBytes = *row.SizeBytes
	}
	if row.ItemCount != nil {
		run.ItemCount = int(*row.ItemCount)
	}
	if row.MediaCount != nil {
		run.MediaCount = int(*row.MediaCount)
	}
	if row.Checksum != nil {
		run.Checksum = *row.Checksum
	}
	if row.ErrorCode != nil {
		run.ErrorCode = *row.ErrorCode
	}
	for at, into := range map[pgtype.Timestamptz]*time.Time{
		row.SnapshotAt: &run.SnapshotAt, row.FinishedAt: &run.FinishedAt,
		row.ExpiresAt: &run.ExpiresAt, row.VerifiedAt: &run.VerifiedAt,
	} {
		if at.Valid {
			*into = at.Time.UTC()
		}
	}
	return run, nil
}

func optionalBytes(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}

func optionalRows(value int) *int32 {
	if value == 0 {
		return nil
	}
	narrowed := int32(value) //nolint:gosec // G115: counts of rows in one archive, bounded by the export
	return &narrowed
}
