// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// PlanningSprints stores sprints, their entries and estimates (migration 0245, M2).
type PlanningSprints struct{}

var _ repository.Sprints = PlanningSprints{}

// Sprints answers the sprints' side of the adapter.
func (r PlanningRepository) Sprints() PlanningSprints { return PlanningSprints{} }

// planDate carries a planning day through a `date` column as its midnight UTC.
func planDate(day planning.Day) pgtype.Date {
	return pgtype.Date{Time: day.In(time.UTC), Valid: true}
}

func planDay(value pgtype.Date) planning.Day {
	return planning.DayOf(value.Time, time.UTC)
}

// Insert writes a planned sprint.
func (PlanningSprints) Insert(ctx context.Context, s planning.Sprint) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(s.ID)
	if err != nil {
		return err
	}
	collection, err := uuidOf(s.CollectionID)
	if err != nil {
		return err
	}
	createdBy, err := optionalUUID(s.CreatedBy)
	if err != nil {
		return err
	}
	if err := queries.InsertSprint(ctx, sqlc.InsertSprintParams{
		ID: id, CollectionID: collection, Name: s.Name, Goal: s.Goal, StartOn: planDate(s.StartOn),
		EndOn: planDate(s.EndOn), Unit: string(s.Unit), CreatedAt: timestampOf(s.CreatedAt), CreatedBy: createdBy,
	}); err != nil {
		return planningError(err, "writing a sprint")
	}
	return nil
}

// Update writes a sprint held to the version read.
func (PlanningSprints) Update(ctx context.Context, s planning.Sprint, expected int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(s.ID)
	if err != nil {
		return err
	}
	//nolint:gosec // G115: counts bounded by MaxSprintEntries, a version by the row
	affected, err := queries.UpdateSprint(ctx, sqlc.UpdateSprintParams{
		ID: id, Name: s.Name, Goal: s.Goal, StartOn: planDate(s.StartOn), EndOn: planDate(s.EndOn),
		State: string(s.State), Unit: string(s.Unit),
		CommittedValue: s.CommittedValue, CommittedCount: int32(s.CommittedCount),
		CompletedValue: s.CompletedValue, CompletedCount: int32(s.CompletedCount),
		StartedAt: optionalTimestamp(s.StartedAt), ClosedAt: optionalTimestamp(s.ClosedAt),
		UpdatedAt: timestampOf(s.UpdatedAt), ExpectedVersion: int32(expected),
	})
	if isUniqueViolation(err) {
		return shared.ErrConflict.WithDetail("sprints.another_running")
	}
	if err != nil {
		return planningError(err, "writing a sprint")
	}
	if affected == 0 {
		return shared.ErrVersionConflict.WithDetail("sprints.version_conflict").
			WithParams(map[string]string{"sprint_id": s.ID.String(), "expected_version": strconv.Itoa(expected)})
	}
	return nil
}

// Find returns a sprint.
func (PlanningSprints) Find(ctx context.Context, id shared.ID) (planning.Sprint, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return planning.Sprint{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return planning.Sprint{}, err
	}
	row, err := queries.FindSprint(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return planning.Sprint{}, shared.ErrNotFound.WithDetail("sprints.not_found")
	}
	if err != nil {
		return planning.Sprint{}, planningError(err, "reading a sprint")
	}
	return sprintFrom(row)
}

// List returns a collection's sprints, the latest first.
func (PlanningSprints) List(ctx context.Context, collectionID shared.ID, state planning.SprintState, limit int) ([]planning.Sprint, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(collectionID)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G115: a bound the use case clamps
	rows, err := queries.ListSprints(ctx, sqlc.ListSprintsParams{CollectionID: key, State: string(state), Max: int32(limit)})
	if err != nil {
		return nil, planningError(err, "listing sprints")
	}
	out := make([]planning.Sprint, 0, len(rows))
	for _, row := range rows {
		s, err := sprintFrom(sqlc.FindSprintRow(row))
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// Delete removes a sprint and its memberships.
func (PlanningSprints) Delete(ctx context.Context, id shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	affected, err := queries.DeleteSprint(ctx, key)
	if err != nil {
		return planningError(err, "deleting a sprint")
	}
	if affected == 0 {
		return shared.ErrNotFound.WithDetail("sprints.not_found")
	}
	return nil
}

// Entries returns a sprint's memberships.
func (PlanningSprints) Entries(ctx context.Context, sprintID shared.ID, limit int) ([]repository.SprintMember, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(sprintID)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G115: a bound the use case clamps
	rows, err := queries.ListSprintEntries(ctx, sqlc.ListSprintEntriesParams{SprintID: key, Max: int32(limit)})
	if err != nil {
		return nil, planningError(err, "reading a sprint's entries")
	}
	out := make([]repository.SprintMember, 0, len(rows))
	for _, row := range rows {
		entry, err := idFrom(row.EntryID)
		if err != nil {
			return nil, err
		}
		collection, err := idFrom(row.CollectionID)
		if err != nil {
			return nil, err
		}
		hub, err := optionalID(row.HubID)
		if err != nil {
			return nil, err
		}
		member := repository.SprintMember{
			SprintEntry: planning.SprintEntry{
				SprintID: sprintID, EntryID: entry, AddedAt: timeFrom(row.AddedAt), RemovedAt: optionalTime(row.RemovedAt),
				Committed: row.Committed, Done: row.IsCompleted, CompletedAt: optionalTime(row.CompletedAt),
			},
			CollectionID: collection, HubID: hub,
		}
		if row.Outcome != nil {
			member.Outcome = planning.SprintOutcome(*row.Outcome)
		}
		out = append(out, member)
	}
	return out, nil
}

// OpenMemberships answers the open sprint each entry is in.
func (PlanningSprints) OpenMemberships(ctx context.Context, entryIDs []shared.ID) ([]repository.OpenMembership, error) {
	if len(entryIDs) == 0 {
		return nil, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := uuidsOf(entryIDs)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListOpenSprintMemberships(ctx, keys)
	if err != nil {
		return nil, planningError(err, "reading entries' sprints")
	}
	out := make([]repository.OpenMembership, 0, len(rows))
	for _, row := range rows {
		entry, err := idFrom(row.EntryID)
		if err != nil {
			return nil, err
		}
		sprint, err := idFrom(row.SprintID)
		if err != nil {
			return nil, err
		}
		out = append(out, repository.OpenMembership{EntryID: entry, SprintID: sprint})
	}
	return out, nil
}

// Add puts an entry into a sprint.
func (PlanningSprints) Add(ctx context.Context, sprintID, entryID shared.ID, at time.Time, committed bool) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	sprint, err := uuidOf(sprintID)
	if err != nil {
		return err
	}
	entry, err := uuidOf(entryID)
	if err != nil {
		return err
	}
	err = queries.AddSprintEntry(ctx, sqlc.AddSprintEntryParams{SprintID: sprint, EntryID: entry, AddedAt: timestampOf(at), Committed: committed})
	if isUniqueViolation(err) {
		return shared.ErrConflict.WithDetail("sprints.entry_in_another_sprint")
	}
	if err != nil {
		return planningError(err, "adding an entry to a sprint")
	}
	return nil
}

// Remove takes an entry out of a sprint.
func (PlanningSprints) Remove(ctx context.Context, sprintID, entryID shared.ID, at time.Time) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	sprint, err := uuidOf(sprintID)
	if err != nil {
		return false, err
	}
	entry, err := uuidOf(entryID)
	if err != nil {
		return false, err
	}
	affected, err := queries.RemoveSprintEntry(ctx, sqlc.RemoveSprintEntryParams{SprintID: sprint, EntryID: entry, RemovedAt: timestampOf(at)})
	if err != nil {
		return false, planningError(err, "removing an entry from a sprint")
	}
	return affected > 0, nil
}

// Commit marks a sprint's open memberships as committed.
func (PlanningSprints) Commit(ctx context.Context, sprintID shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	sprint, err := uuidOf(sprintID)
	if err != nil {
		return err
	}
	if err := queries.CommitSprintEntries(ctx, sprint); err != nil {
		return planningError(err, "committing a sprint's entries")
	}
	return nil
}

// SetOutcomes records what became of a closed sprint's entries.
func (PlanningSprints) SetOutcomes(ctx context.Context, sprintID shared.ID, outcome planning.SprintOutcome, entryIDs []shared.ID) error {
	if len(entryIDs) == 0 {
		return nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	sprint, err := uuidOf(sprintID)
	if err != nil {
		return err
	}
	keys, err := uuidsOf(entryIDs)
	if err != nil {
		return err
	}
	code := string(outcome)
	if err := queries.SetSprintEntryOutcomes(ctx, sqlc.SetSprintEntryOutcomesParams{SprintID: sprint, Outcome: &code, EntryIds: keys}); err != nil {
		return planningError(err, "closing a sprint's entries")
	}
	return nil
}

// SetEstimate writes an entry's estimate.
func (PlanningSprints) SetEstimate(ctx context.Context, e planning.Estimate) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	entry, err := uuidOf(e.EntryID)
	if err != nil {
		return err
	}
	by, err := optionalUUID(e.UpdatedBy)
	if err != nil {
		return err
	}
	var size *string
	if e.Size != "" {
		size = &e.Size
	}
	if err := queries.UpsertEntryEstimate(ctx, sqlc.UpsertEntryEstimateParams{
		EntryID: entry, Unit: string(e.Unit), Value: e.Value, Size: size, UpdatedAt: timestampOf(e.UpdatedAt), UpdatedBy: by,
	}); err != nil {
		return planningError(err, "writing an estimate")
	}
	return nil
}

// ClearEstimate removes an entry's estimate.
func (PlanningSprints) ClearEstimate(ctx context.Context, entryID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	entry, err := uuidOf(entryID)
	if err != nil {
		return false, err
	}
	affected, err := queries.DeleteEntryEstimate(ctx, entry)
	if err != nil {
		return false, planningError(err, "removing an estimate")
	}
	return affected > 0, nil
}

// Estimates returns the estimates among the entries.
func (PlanningSprints) Estimates(ctx context.Context, entryIDs []shared.ID) (map[shared.ID]planning.Estimate, error) {
	out := map[shared.ID]planning.Estimate{}
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
	rows, err := queries.ListEntryEstimates(ctx, keys)
	if err != nil {
		return nil, planningError(err, "reading estimates")
	}
	for _, row := range rows {
		id, err := idFrom(row.EntryID)
		if err != nil {
			return nil, err
		}
		by, err := optionalID(row.UpdatedBy)
		if err != nil {
			return nil, err
		}
		e := planning.Estimate{
			EntryID: id, Unit: planning.EstimateUnit(row.Unit), Value: row.Value,
			UpdatedAt: timeFrom(row.UpdatedAt), UpdatedBy: by, Version: int(row.Version),
		}
		if row.Size != nil {
			e.Size = *row.Size
		}
		out[id] = e
	}
	return out, nil
}

func sprintFrom(row sqlc.FindSprintRow) (planning.Sprint, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return planning.Sprint{}, err
	}
	collection, err := idFrom(row.CollectionID)
	if err != nil {
		return planning.Sprint{}, err
	}
	createdBy, err := optionalID(row.CreatedBy)
	if err != nil {
		return planning.Sprint{}, err
	}
	return planning.Sprint{
		ID: id, CollectionID: collection, Name: row.Name, Goal: row.Goal,
		StartOn: planDay(row.StartOn), EndOn: planDay(row.EndOn), State: planning.SprintState(row.State),
		Unit: planning.EstimateUnit(row.Unit), CommittedValue: row.CommittedValue, CommittedCount: int(row.CommittedCount),
		CompletedValue: row.CompletedValue, CompletedCount: int(row.CompletedCount),
		StartedAt: optionalTime(row.StartedAt), ClosedAt: optionalTime(row.ClosedAt),
		CreatedAt: timeFrom(row.CreatedAt), CreatedBy: createdBy, UpdatedAt: timeFrom(row.UpdatedAt), Version: int(row.Version),
	}, nil
}
