// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// The walk's time rows (offline-sync.md §3.1): a person's own records and timer, and the
// workspace's kinds of work and policy, read as the module's own repository reads them.

// OwnTimeRecords pages one person's time records.
func (SnapshotRepository) OwnTimeRecords(ctx context.Context, account, after shared.ID, batch int) ([]domain.Record, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	holder, err := uuidOf(account)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotOwnTimeRecords(ctx, sqlc.SnapshotOwnTimeRecordsParams{
		AccountID: holder, After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("time records", err)
	}
	return mapRows(rows, func(row sqlc.SnapshotOwnTimeRecordsRow) (domain.Record, error) {
		return timeRecordFrom(row.ID, row.AccountID, row.EntryID, row.Corrects, row.WorkType, row.Day, row.StartedAt,
			row.Minutes, row.Note, row.Billable, row.Source, row.CreatedAt, row.UpdatedAt, row.Version)
	})
}

// OwnTimer reads one person's running timer.
func (SnapshotRepository) OwnTimer(ctx context.Context, account shared.ID) (domain.Timer, bool, error) {
	return TimeRepository{}.Timer(ctx, account)
}

// TimeWorkTypes reads the workspace's kinds of work.
func (SnapshotRepository) TimeWorkTypes(ctx context.Context) ([]domain.WorkType, error) {
	return TimeRepository{}.WorkTypes(ctx)
}

// TimePolicy reads the workspace's time policy.
func (SnapshotRepository) TimePolicy(ctx context.Context) (domain.Policy, error) {
	return TimeRepository{}.Policy(ctx)
}

// OwnTimesheets pages one person's weeks.
func (SnapshotRepository) OwnTimesheets(ctx context.Context, account, after shared.ID, batch int) ([]domain.Timesheet, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	holder, err := uuidOf(account)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotOwnTimesheets(ctx, sqlc.SnapshotOwnTimesheetsParams{
		AccountID: holder, After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("weeks", err)
	}
	return mapRows(rows, func(r sqlc.SnapshotOwnTimesheetsRow) (domain.Timesheet, error) {
		return sheetFrom(r.ID, r.AccountID, r.WeekStart, r.State, r.Minutes, r.SubmittedAt, r.DecidedAt, r.DecidedBy, r.Reason, r.Version)
	})
}

// OwnCapacities reads one person's capacities.
func (SnapshotRepository) OwnCapacities(ctx context.Context, account shared.ID, max int) ([]domain.Capacity, error) {
	return TimeResourceRepository{}.Capacities(ctx, []shared.ID{account}, max)
}

// TimeAbsences pages the workspace's absences.
func (SnapshotRepository) TimeAbsences(ctx context.Context, after shared.ID, batch int) ([]domain.Absence, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotTimeAbsences(ctx, sqlc.SnapshotTimeAbsencesParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("absences", err)
	}
	return mapRows(rows, func(r sqlc.SnapshotTimeAbsencesRow) (domain.Absence, error) {
		return absenceFrom(r.ID, r.AccountID, r.FromDay, r.ToDay, r.Kind, r.Half, r.Version)
	})
}

// TimeCostRates reads the workspace's cost rates.
func (SnapshotRepository) TimeCostRates(ctx context.Context, max int) ([]domain.CostRate, error) {
	return TimeResourceRepository{}.Rates(ctx, max)
}

// TimeBudgets pages the hubs' and collections' budgets by their plan.
func (SnapshotRepository) TimeBudgets(ctx context.Context, after shared.ID, batch int) ([]domain.Budget, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotTimeBudgets(ctx, sqlc.SnapshotTimeBudgetsParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("budgets", err)
	}
	return mapRows(rows, func(r sqlc.SnapshotTimeBudgetsRow) (domain.Budget, error) {
		plan, err := idFrom(r.PlanID)
		if err != nil {
			return domain.Budget{}, err
		}
		return domain.Budget{PlanID: plan, Minutes: r.Minutes, AmountMinor: r.AmountMinor, Currency: r.Currency, Version: int(r.Version)}, nil
	})
}
