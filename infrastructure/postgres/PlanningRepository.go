// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	repository "github.com/Jersyfi/hubtask/core/application/repository/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// PlanningRepository stores the planning module's rows (migration 0244, M2).
type PlanningRepository struct{}

// NewPlanningRepository builds the adapter.
func NewPlanningRepository() PlanningRepository { return PlanningRepository{} }

var (
	_ repository.Plans     = PlanningRepository{}
	_ repository.Baselines = PlanningBaselines{}
)

// Baselines answers the baselines' side of the adapter.
func (r PlanningRepository) Baselines() PlanningBaselines { return PlanningBaselines{} }

func planningError(err error, what string) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

// Entries returns the plan's entries.
func (r PlanningRepository) Entries(ctx context.Context, planID shared.ID, limit int) ([]repository.PlanEntry, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(planID)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G115: a bound the use case clamps
	rows, err := queries.ListPlanEntries(ctx, sqlc.ListPlanEntriesParams{PlanID: key, Max: int32(limit)})
	if err != nil {
		return nil, planningError(err, "reading a plan's entries")
	}
	out := make([]repository.PlanEntry, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		collection, err := idFrom(row.CollectionID)
		if err != nil {
			return nil, err
		}
		parent, err := optionalID(row.ParentID)
		if err != nil {
			return nil, err
		}
		hub, err := optionalID(row.HubID)
		if err != nil {
			return nil, err
		}
		assignee, err := optionalID(row.AssigneeID)
		if err != nil {
			return nil, err
		}
		entry := repository.PlanEntry{
			ID: id, ParentID: parent, CollectionID: collection, HubID: hub, AssigneeID: assignee,
			Title: row.Title, StartAt: optionalTime(row.StartAt), DueAt: optionalTime(row.DueAt), DueDateOnly: row.DueDateOnly,
			Done: row.IsCompleted, Version: int(row.Version), Mode: planning.Mode(row.Mode), Milestone: row.Milestone,
		}
		if row.DueTimeZone != nil {
			entry.DueTimeZone = *row.DueTimeZone
		}
		out = append(out, entry)
	}
	return out, nil
}

// Schedules returns the stored modes among the entries.
func (r PlanningRepository) Schedules(ctx context.Context, entryIDs []shared.ID) (map[shared.ID]repository.EntrySchedule, error) {
	out := map[shared.ID]repository.EntrySchedule{}
	if len(entryIDs) == 0 {
		return out, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := uuidsOf(entryIDs)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListEntrySchedules(ctx, keys)
	if err != nil {
		return nil, planningError(err, "reading entries' scheduling modes")
	}
	for _, row := range rows {
		id, err := idFrom(row.EntryID)
		if err != nil {
			return nil, err
		}
		out[id] = repository.EntrySchedule{
			EntryID: id, Mode: planning.Mode(row.Mode), Milestone: row.Milestone,
			UpdatedAt: timeFrom(row.UpdatedAt), Version: int(row.Version),
		}
	}
	return out, nil
}

// SetSchedule writes an entry's mode.
func (r PlanningRepository) SetSchedule(ctx context.Context, schedule repository.EntrySchedule) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(schedule.EntryID)
	if err != nil {
		return err
	}
	if err := queries.UpsertEntrySchedule(ctx, sqlc.UpsertEntryScheduleParams{
		EntryID: key, Mode: string(schedule.Mode), Milestone: schedule.Milestone,
		UpdatedAt: timestampOf(schedule.UpdatedAt),
	}); err != nil {
		return planningError(err, "writing an entry's scheduling mode")
	}
	return nil
}

// SetTiming writes a link's timing.
func (r PlanningRepository) SetTiming(ctx context.Context, linkID shared.ID, timing planning.Timing) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(linkID)
	if err != nil {
		return err
	}
	affected, err := queries.SetEntryLinkTiming(ctx, sqlc.SetEntryLinkTimingParams{
		ID: key, DependencyKind: string(timing.Kind), LagDays: int32(timing.LagDays), //nolint:gosec // G115: bounded by planning.MaxLagDays
	})
	if err != nil {
		return planningError(err, "writing a dependency's timing")
	}
	if affected == 0 {
		return shared.ErrNotFound.WithDetail("links.not_found")
	}
	return nil
}

// PlanningBaselines stores a plan's baselines.
type PlanningBaselines struct{}

// Count is how many baselines a plan keeps.
func (PlanningBaselines) Count(ctx context.Context, planID shared.ID) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	key, err := uuidOf(planID)
	if err != nil {
		return 0, err
	}
	count, err := queries.CountPlanBaselines(ctx, key)
	if err != nil {
		return 0, planningError(err, "counting a plan's baselines")
	}
	return int(count), nil
}

// Take writes the baseline and copies the plan into it.
func (PlanningBaselines) Take(ctx context.Context, baseline planning.Baseline, limit int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	id, err := uuidOf(baseline.ID)
	if err != nil {
		return 0, err
	}
	plan, err := uuidOf(baseline.PlanID)
	if err != nil {
		return 0, err
	}
	takenBy, err := optionalUUID(baseline.TakenBy)
	if err != nil {
		return 0, err
	}
	if err := queries.InsertPlanBaseline(ctx, sqlc.InsertPlanBaselineParams{
		ID: id, PlanID: plan, Name: baseline.Name, TakenAt: timestampOf(baseline.TakenAt), TakenBy: takenBy,
	}); err != nil {
		return 0, planningError(err, "writing a baseline")
	}
	//nolint:gosec // G115: a bound the use case clamps
	copied, err := queries.CopyPlanIntoBaseline(ctx, sqlc.CopyPlanIntoBaselineParams{BaselineID: id, PlanID: plan, Max: int32(limit)})
	if err != nil {
		return 0, planningError(err, "copying a plan into its baseline")
	}
	//nolint:gosec // G115: at most max
	if err := queries.SetPlanBaselineCount(ctx, sqlc.SetPlanBaselineCountParams{ID: id, EntryCount: int32(copied)}); err != nil {
		return 0, planningError(err, "counting a baseline's entries")
	}
	return int(copied), nil
}

// List returns a plan's baselines, the newest first.
func (PlanningBaselines) List(ctx context.Context, planID shared.ID) ([]planning.Baseline, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(planID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListPlanBaselines(ctx, key)
	if err != nil {
		return nil, planningError(err, "listing a plan's baselines")
	}
	out := make([]planning.Baseline, 0, len(rows))
	for _, row := range rows {
		baseline, err := baselineFrom(sqlc.FindPlanBaselineRow(row))
		if err != nil {
			return nil, err
		}
		out = append(out, baseline)
	}
	return out, nil
}

// Find returns a baseline with its entries.
func (PlanningBaselines) Find(ctx context.Context, id shared.ID) (planning.Baseline, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return planning.Baseline{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return planning.Baseline{}, err
	}
	row, err := queries.FindPlanBaseline(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return planning.Baseline{}, shared.ErrNotFound.WithDetail("baselines.not_found")
	}
	if err != nil {
		return planning.Baseline{}, planningError(err, "reading a baseline")
	}
	baseline, err := baselineFrom(row)
	if err != nil {
		return planning.Baseline{}, err
	}
	entries, err := queries.ListPlanBaselineEntries(ctx, key)
	if err != nil {
		return planning.Baseline{}, planningError(err, "reading a baseline's entries")
	}
	baseline.Entries = make([]planning.BaselineEntry, 0, len(entries))
	for _, e := range entries {
		entry, err := idFrom(e.EntryID)
		if err != nil {
			return planning.Baseline{}, err
		}
		baseline.Entries = append(baseline.Entries, planning.BaselineEntry{
			EntryID: entry, StartAt: optionalTime(e.StartAt), DueAt: optionalTime(e.DueAt),
		})
	}
	return baseline, nil
}

// Delete removes a baseline and its entries.
func (PlanningBaselines) Delete(ctx context.Context, id shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	affected, err := queries.DeletePlanBaseline(ctx, key)
	if err != nil {
		return planningError(err, "deleting a baseline")
	}
	if affected == 0 {
		return shared.ErrNotFound.WithDetail("baselines.not_found")
	}
	return nil
}

func baselineFrom(row sqlc.FindPlanBaselineRow) (planning.Baseline, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return planning.Baseline{}, err
	}
	plan, err := idFrom(row.PlanID)
	if err != nil {
		return planning.Baseline{}, err
	}
	takenBy, err := optionalID(row.TakenBy)
	if err != nil {
		return planning.Baseline{}, err
	}
	return planning.Baseline{
		ID: id, PlanID: plan, Name: row.Name, TakenAt: timeFrom(row.TakenAt), TakenBy: takenBy,
		EntryCount: int(row.EntryCount),
	}, nil
}
