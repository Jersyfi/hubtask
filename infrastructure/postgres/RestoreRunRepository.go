// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// RestoreRunRepository stores what a restore did (E-06).
type RestoreRunRepository struct{}

func NewRestoreRunRepository() RestoreRunRepository { return RestoreRunRepository{} }

var _ repository.Restores = RestoreRunRepository{}

// Insert writes the accepted restore.
//
// The tenant of the row is `current_tenant_id()` - the tenant that asked - and the tenant being
// restored *into* is a column of its own. They differ only for NEW_TENANT, whose target did not
// exist when the restore was asked for, and filing the row under that one would make it invisible
// to the person holding the `result_url` (migration 0034).
func (r RestoreRunRepository) Insert(ctx context.Context, restore domain.Restore) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(restore.ID)
	if err != nil {
		return err
	}
	targetID, err := uuidOf(restore.TargetID)
	if err != nil {
		return err
	}
	into, err := optionalUUID(restore.TenantID)
	if err != nil {
		return err
	}
	requestedBy, err := uuidOf(restore.RequestedBy)
	if err != nil {
		return err
	}
	selection, err := encodeSelection(restore.Selection)
	if err != nil {
		return err
	}

	err = queries.InsertRestoreRun(ctx, sqlc.InsertRestoreRunParams{
		ID: id, TargetID: targetID, SourceArchive: restore.SourceArchive,
		TargetTenantID: into, Mode: string(restore.Mode),
		ConflictRule: string(restore.ConflictRule), Selection: selection,
		DryRun: restore.DryRun, RequestedBy: requestedBy,
	})
	if err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the restore run: %w", err))
	}
	return nil
}

// Find answers one restore.
func (r RestoreRunRepository) Find(ctx context.Context, id shared.ID) (domain.Restore, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Restore{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.Restore{}, err
	}

	row, err := queries.FindRestoreRun(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return domain.Restore{}, shared.ErrNotFound.WithDetail(domain.CodeRestoreNotFound).
				WithParams(map[string]string{"restore_id": id.String()})
		}
		return domain.Restore{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the restore run: %w", err))
	}
	return restoreFrom(row)
}

// Claim moves the run to RUNNING and answers whether it got the tenant.
func (r RestoreRunRepository) Claim(ctx context.Context, id shared.ID, at time.Time) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return false, err
	}

	affected, err := queries.ClaimRestoreRun(ctx, sqlc.ClaimRestoreRunParams{
		ID: key, StartedAt: timestampOf(at),
	})
	if err != nil {
		return false, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("claiming the restore run: %w", err))
	}
	return affected > 0, nil
}

// Finish records how a restore ended.
func (r RestoreRunRepository) Finish(ctx context.Context, outcome domain.RestoreOutcome) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(outcome.ID)
	if err != nil {
		return err
	}
	safety, err := optionalUUID(outcome.SafetyRunID)
	if err != nil {
		return err
	}
	report, err := json.Marshal(reportRow(outcome.Report))
	if err != nil {
		return shared.Internalf("postgres: a restore report could not be encoded: %w", err)
	}

	affected, err := queries.FinishRestoreRun(ctx, sqlc.FinishRestoreRunParams{
		ID: id, Status: string(outcome.Status), Report: report,
		SafetyBackupRunID: safety, FinishedAt: timestampOf(outcome.FinishedAt),
		ErrorCode: optionalText(outcome.ErrorCode),
	})
	if err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("finishing the restore run: %w", err))
	}
	if affected == 0 {
		// The run is no longer going. A cancelled restore stays cancelled, and an outcome written
		// over one would be a report of work nobody let finish.
		return shared.ErrConflict.WithDetail(domain.CodeRestoreNotRunning).
			WithParams(map[string]string{"restore_id": outcome.ID.String()})
	}
	return nil
}

// RecordSafetyCopy writes down the backup taken before a destructive mode.
func (r RestoreRunRepository) RecordSafetyCopy(ctx context.Context, id, backupRunID shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	copyOf, err := uuidOf(backupRunID)
	if err != nil {
		return err
	}

	affected, err := queries.RecordRestoreSafetyCopy(ctx, sqlc.RecordRestoreSafetyCopyParams{
		ID: key, SafetyBackupRunID: copyOf,
	})
	if err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording the safety copy: %w", err))
	}
	if affected == 0 {
		return shared.ErrNotFound.WithDetail(domain.CodeRestoreNotFound).
			WithParams(map[string]string{"restore_id": id.String()})
	}
	return nil
}

// InProgress reports whether this tenant already has a restore going.
func (r RestoreRunRepository) InProgress(ctx context.Context) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	running, err := queries.RestoreInProgress(ctx)
	if err != nil {
		return false, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("asking whether a restore is running: %w", err))
	}
	return running, nil
}

// selectionRow is how a SELECTIVE restore's choice is stored: identifiers, in the vocabulary the
// contract uses, so that a row read back years later needs no code to interpret.
type selectionRow struct {
	ContainerIDs []string `json:"container_ids,omitempty"`
	ItemIDs      []string `json:"item_ids,omitempty"`
}

func encodeSelection(selection domain.Selection) ([]byte, error) {
	if selection.Empty() {
		return nil, nil
	}
	row := selectionRow{}
	for _, id := range selection.ContainerIDs {
		row.ContainerIDs = append(row.ContainerIDs, id.String())
	}
	for _, id := range selection.ItemIDs {
		row.ItemIDs = append(row.ItemIDs, id.String())
	}
	encoded, err := json.Marshal(row)
	if err != nil {
		return nil, shared.Internalf("postgres: a restore selection could not be encoded: %w", err)
	}
	return encoded, nil
}

func decodeSelection(raw []byte) (domain.Selection, error) {
	if len(raw) == 0 {
		return domain.Selection{}, nil
	}
	var row selectionRow
	if err := json.Unmarshal(raw, &row); err != nil {
		return domain.Selection{}, shared.Internalf("postgres: a restore selection could not be read: %w", err)
	}
	var selection domain.Selection
	for _, raw := range row.ContainerIDs {
		id, err := shared.ParseID(raw)
		if err != nil {
			return domain.Selection{}, err
		}
		selection.ContainerIDs = append(selection.ContainerIDs, id)
	}
	for _, raw := range row.ItemIDs {
		id, err := shared.ParseID(raw)
		if err != nil {
			return domain.Selection{}, err
		}
		selection.ItemIDs = append(selection.ItemIDs, id)
	}
	return selection, nil
}

// reportRow is the report as the column holds it. The names are the contract's, so that the stored
// document and the response are the same words - a report an operator reads out of the database
// during an incident should not need a translation table.
type reportRow struct {
	New         int            `json:"new"`
	Overwritten int            `json:"overwritten"`
	Skipped     int            `json:"skipped"`
	Duplicated  int            `json:"duplicated"`
	Conflicts   int            `json:"conflicts"`
	Withheld    map[string]int `json:"withheld,omitempty"`
	Media       int            `json:"media"`
	Entities    map[string]int `json:"entities,omitempty"`
}

func decodeReport(raw []byte) (domain.Report, error) {
	if len(raw) == 0 {
		return domain.Report{}, nil
	}
	var row reportRow
	if err := json.Unmarshal(raw, &row); err != nil {
		return domain.Report{}, shared.Internalf("postgres: a restore report could not be read: %w", err)
	}
	return domain.Report(row), nil
}

func restoreFrom(row sqlc.FindRestoreRunRow) (domain.Restore, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return domain.Restore{}, err
	}
	targetID, err := idFrom(row.TargetID)
	if err != nil {
		return domain.Restore{}, err
	}
	into, err := optionalID(row.TargetTenantID)
	if err != nil {
		return domain.Restore{}, err
	}
	safety, err := optionalID(row.SafetyBackupRunID)
	if err != nil {
		return domain.Restore{}, err
	}
	requestedBy, err := optionalID(row.RequestedBy)
	if err != nil {
		return domain.Restore{}, err
	}
	approvedBy, err := optionalID(row.ApprovedBy)
	if err != nil {
		return domain.Restore{}, err
	}
	selection, err := decodeSelection(row.Selection)
	if err != nil {
		return domain.Restore{}, err
	}
	report, err := decodeReport(row.Report)
	if err != nil {
		return domain.Restore{}, err
	}

	return domain.Restore{
		ID: id, TargetID: targetID, TenantID: into, SourceArchive: row.SourceArchive,
		Mode: domain.RestoreMode(row.Mode), ConflictRule: domain.ConflictRule(row.ConflictRule),
		Selection: selection, DryRun: row.DryRun, SafetyRunID: safety,
		Status: domain.RestoreStatus(row.Status), Report: report,
		RequestedBy: requestedBy, ApprovedBy: approvedBy,
		StartedAt: timeFrom(row.StartedAt), FinishedAt: timeFrom(row.FinishedAt),
		ErrorCode: stringFrom(row.ErrorCode),
	}, nil
}
