// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/importer"
	backup "github.com/Jersyfi/hubtask/core/domain/model/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// ImportRunRepository stores what an import asked and did (P-08). The shape is the restore run's:
// the applier writes the same report and the same progress into it, because the applying is the
// restore's.
type ImportRunRepository struct{}

func NewImportRunRepository() ImportRunRepository { return ImportRunRepository{} }

var _ repository.Runs = ImportRunRepository{}

// Insert writes the accepted run. The tenant is current_tenant_id(): the transaction's.
func (r ImportRunRepository) Insert(ctx context.Context, run domain.Run) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(run.ID)
	if err != nil {
		return err
	}
	requestedBy, err := uuidOf(run.RequestedBy)
	if err != nil {
		return err
	}
	mediaID, err := uuidOf(run.MediaID)
	if err != nil {
		return err
	}
	hubID, err := uuidOf(run.HubID)
	if err != nil {
		return err
	}
	var mapping []byte
	if len(run.Mapping) > 0 {
		mapping, err = json.Marshal(run.Mapping)
		if err != nil {
			return shared.Internalf("postgres: an import mapping could not be encoded: %w", err)
		}
	}
	err = queries.InsertImportRun(ctx, sqlc.InsertImportRunParams{
		ID: id, RequestedBy: requestedBy, Kind: string(run.Kind), MediaID: mediaID, HubID: hubID, Mapping: mapping,
		TimeZone: optionalText(run.Zone), Language: optionalText(run.Language),
	})
	if err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the import run: %w", err))
	}
	return nil
}

// Find answers one run.
func (r ImportRunRepository) Find(ctx context.Context, id shared.ID) (domain.Run, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Run{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.Run{}, err
	}
	row, err := queries.FindImportRun(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return domain.Run{}, shared.ErrNotFound.WithDetail(domain.CodeNotFound).
				WithParams(map[string]string{"import_id": id.String()})
		}
		return domain.Run{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the import run: %w", err))
	}
	return importRunFrom(row)
}

// Claim moves the run to RUNNING; a run already RUNNING claims itself again.
func (r ImportRunRepository) Claim(ctx context.Context, id shared.ID, at time.Time) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return false, err
	}
	affected, err := queries.ClaimImportRun(ctx, sqlc.ClaimImportRunParams{ID: key, StartedAt: timestampOf(at)})
	if err != nil {
		return false, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("claiming the import run: %w", err))
	}
	return affected == 1, nil
}

// RecordProgress writes how far the applier got, and the report so far.
func (r ImportRunRepository) RecordProgress(
	ctx context.Context, id shared.ID, report backup.Report, progress map[string]int,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	encodedReport, err := json.Marshal(reportRow(report))
	if err != nil {
		return shared.Internalf("postgres: an import report could not be encoded: %w", err)
	}
	encodedProgress, err := json.Marshal(progress)
	if err != nil {
		return shared.Internalf("postgres: an import's progress could not be encoded: %w", err)
	}
	affected, err := queries.RecordImportProgress(ctx, sqlc.RecordImportProgressParams{
		ID: key, Report: encodedReport, Progress: encodedProgress,
	})
	if err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording an import's progress: %w", err))
	}
	if affected == 0 {
		return shared.ErrConflict.WithDetail(domain.CodeRunNotRunning).
			WithParams(map[string]string{"import_id": id.String()})
	}
	return nil
}

// Finish records how the run ended.
func (r ImportRunRepository) Finish(ctx context.Context, outcome domain.Outcome) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(outcome.ID)
	if err != nil {
		return err
	}
	var report []byte
	if !isEmptyReport(outcome.Report) {
		report, err = json.Marshal(reportRow(outcome.Report))
		if err != nil {
			return shared.Internalf("postgres: an import report could not be encoded: %w", err)
		}
	}
	var refused []byte
	if len(outcome.Refused) > 0 {
		refused, err = json.Marshal(refusalRows(outcome.Refused))
		if err != nil {
			return shared.Internalf("postgres: an import's refusals could not be encoded: %w", err)
		}
	}
	affected, err := queries.FinishImportRun(ctx, sqlc.FinishImportRunParams{
		ID: id, Status: string(outcome.Status), Report: report, Refused: refused,
		ErrorCode: optionalText(outcome.ErrorCode), FinishedAt: timestampOf(outcome.FinishedAt),
	})
	if err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("finishing the import run: %w", err))
	}
	if affected == 0 {
		return shared.ErrConflict.WithDetail(domain.CodeRunNotRunning).
			WithParams(map[string]string{"import_id": outcome.ID.String()})
	}
	return nil
}

type refusalRow struct {
	Row  int    `json:"row"`
	Code string `json:"code"`
}

func refusalRows(refused []domain.Refusal) []refusalRow {
	rows := make([]refusalRow, 0, len(refused))
	for _, r := range refused {
		rows = append(rows, refusalRow{Row: r.Row, Code: r.Code})
	}
	return rows
}

func importRunFrom(row sqlc.ImportRun) (domain.Run, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return domain.Run{}, err
	}
	tenantID, err := idFrom(row.TenantID)
	if err != nil {
		return domain.Run{}, err
	}
	requestedBy, err := idFrom(row.RequestedBy)
	if err != nil {
		return domain.Run{}, err
	}
	mediaID, err := idFrom(row.MediaID)
	if err != nil {
		return domain.Run{}, err
	}
	hubID, err := idFrom(row.HubID)
	if err != nil {
		return domain.Run{}, err
	}
	report, err := decodeReport(row.Report)
	if err != nil {
		return domain.Run{}, err
	}
	progress, err := decodeProgress(row.Progress)
	if err != nil {
		return domain.Run{}, err
	}
	run := domain.Run{
		ID: id, TenantID: tenantID, RequestedBy: requestedBy, Kind: domain.Kind(row.Kind),
		MediaID: mediaID, HubID: hubID, Status: domain.Status(row.Status), Report: report,
		Progress: progress, CreatedAt: row.CreatedAt.Time,
		StartedAt: optionalTime(row.StartedAt), FinishedAt: optionalTime(row.FinishedAt),
	}
	if row.ErrorCode != nil {
		run.ErrorCode = *row.ErrorCode
	}
	if row.TimeZone != nil {
		run.Zone = *row.TimeZone
	}
	if row.Language != nil {
		run.Language = *row.Language
	}
	if len(row.Mapping) > 0 {
		if err := json.Unmarshal(row.Mapping, &run.Mapping); err != nil {
			return domain.Run{}, shared.Internalf("postgres: an import mapping could not be read: %w", err)
		}
	}
	if len(row.Refused) > 0 {
		var rows []refusalRow
		if err := json.Unmarshal(row.Refused, &rows); err != nil {
			return domain.Run{}, shared.Internalf("postgres: an import's refusals could not be read: %w", err)
		}
		for _, r := range rows {
			run.Refused = append(run.Refused, domain.Refusal{Row: r.Row, Code: r.Code})
		}
	}
	return run, nil
}
