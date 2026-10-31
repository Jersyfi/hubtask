// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/timekeeping"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// TimeRepository stores the time module's rows (migration 0246, M7).
type TimeRepository struct{}

var _ repository.Store = TimeRepository{}

// NewTimeRepository answers the adapter.
func NewTimeRepository() TimeRepository { return TimeRepository{} }

func timeError(err error, what string) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

func dayDate(d domain.Day) pgtype.Date { return planDate(d) }

func dateDay(v pgtype.Date) domain.Day { return planDay(v) }

func optionalDayDate(d *domain.Day) pgtype.Date {
	if d == nil {
		return pgtype.Date{}
	}
	return dayDate(*d)
}

// WorkTypes answers the workspace's kinds of work.
func (TimeRepository) WorkTypes(ctx context.Context) ([]domain.WorkType, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListTimeWorkTypes(ctx)
	if err != nil {
		return nil, timeError(err, "reading the kinds of work")
	}
	out := make([]domain.WorkType, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.WorkType{
			Key: r.Key, Name: r.Name, Billable: r.Billable, Archived: r.Archived,
			Position: int(r.Position), Version: int(r.Version),
		})
	}
	return out, nil
}

// PutWorkType writes a kind of work by its key.
func (TimeRepository) PutWorkType(ctx context.Context, id shared.ID, w domain.WorkType, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	//nolint:gosec // G115: a position is bounded by MaxWorkTypes
	if err := queries.UpsertTimeWorkType(ctx, sqlc.UpsertTimeWorkTypeParams{
		ID: key, Key: w.Key, Name: w.Name, Billable: w.Billable, Archived: w.Archived,
		Position: int32(w.Position), At: timestampOf(at),
	}); err != nil {
		return timeError(err, "writing a kind of work")
	}
	return nil
}

// InsertRecord writes a new record.
func (TimeRepository) InsertRecord(ctx context.Context, r domain.Record) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(r.ID)
	if err != nil {
		return err
	}
	account, err := uuidOf(r.AccountID)
	if err != nil {
		return err
	}
	entry, err := optionalUUID(r.EntryID)
	if err != nil {
		return err
	}
	corrects, err := optionalUUID(r.Corrects)
	if err != nil {
		return err
	}
	//nolint:gosec // G115: minutes are bounded by MaxMinutes
	if err := queries.InsertTimeRecord(ctx, sqlc.InsertTimeRecordParams{
		ID: id, AccountID: account, EntryID: entry, WorkType: r.WorkType, Day: dayDate(r.Day),
		StartedAt: optionalTimestamp(r.StartedAt), Minutes: int32(r.Minutes), Note: r.Note,
		Billable: r.Billable, Source: string(r.Source), Corrects: corrects, CreatedAt: timestampOf(r.CreatedAt),
	}); err != nil {
		return timeError(err, "writing a time record")
	}
	return nil
}

// UpdateRecord writes a record held to the version read.
func (TimeRepository) UpdateRecord(ctx context.Context, r domain.Record, expected int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(r.ID)
	if err != nil {
		return err
	}
	entry, err := optionalUUID(r.EntryID)
	if err != nil {
		return err
	}
	//nolint:gosec // G115: minutes are bounded by MaxMinutes, a version by the row
	affected, err := queries.UpdateTimeRecord(ctx, sqlc.UpdateTimeRecordParams{
		ID: id, EntryID: entry, WorkType: r.WorkType, Day: dayDate(r.Day), StartedAt: optionalTimestamp(r.StartedAt),
		Minutes: int32(r.Minutes), Note: r.Note, Billable: r.Billable, UpdatedAt: timestampOf(r.UpdatedAt),
		ExpectedVersion: int32(expected),
	})
	if err != nil {
		return timeError(err, "writing a time record")
	}
	if affected == 0 {
		return recordConflict(r.ID, expected)
	}
	return nil
}

func recordConflict(id shared.ID, expected int) error {
	return shared.ErrVersionConflict.WithDetail("time.version_conflict").
		WithParams(map[string]string{"record_id": id.String(), "expected_version": strconv.Itoa(expected)})
}

// DeleteRecord removes a record held to the version read.
func (TimeRepository) DeleteRecord(ctx context.Context, id shared.ID, expected int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	//nolint:gosec // G115: a version is bounded by the row
	affected, err := queries.DeleteTimeRecord(ctx, sqlc.DeleteTimeRecordParams{ID: key, ExpectedVersion: int32(expected)})
	if err != nil {
		return timeError(err, "removing a time record")
	}
	if affected == 0 {
		return recordConflict(id, expected)
	}
	return nil
}

// FindRecord answers one record.
func (TimeRepository) FindRecord(ctx context.Context, id shared.ID) (domain.Record, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Record{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.Record{}, err
	}
	row, err := queries.FindTimeRecord(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Record{}, shared.ErrNotFound.WithDetail("time.record_not_found")
	}
	if err != nil {
		return domain.Record{}, timeError(err, "reading a time record")
	}
	return timeRecordFrom(row.ID, row.AccountID, row.EntryID, row.Corrects, row.WorkType, row.Day, row.StartedAt,
		row.Minutes, row.Note, row.Billable, row.Source, row.CreatedAt, row.UpdatedAt, row.Version)
}

func timeRecordFrom(
	idValue, accountValue, entryValue, correctsValue pgtype.UUID, workType string, day pgtype.Date,
	started pgtype.Timestamptz, minutes int32, note string, billable bool, source string,
	created, updated pgtype.Timestamptz, version int32,
) (domain.Record, error) {
	id, err := idFrom(idValue)
	if err != nil {
		return domain.Record{}, err
	}
	account, err := idFrom(accountValue)
	if err != nil {
		return domain.Record{}, err
	}
	entry, err := optionalID(entryValue)
	if err != nil {
		return domain.Record{}, err
	}
	corrects, err := optionalID(correctsValue)
	if err != nil {
		return domain.Record{}, err
	}
	return domain.Record{
		ID: id, AccountID: account, EntryID: entry, WorkType: workType, Day: dateDay(day),
		StartedAt: optionalTime(started), Minutes: int(minutes), Note: note, Billable: billable,
		Source: domain.Source(source), Corrects: corrects, CreatedAt: timeFrom(created),
		UpdatedAt: timeFrom(updated), Version: int(version),
	}, nil
}

type filterValues struct {
	account, entry, collection, hub pgtype.UUID
}

func filterOf(f repository.Filter) (filterValues, error) {
	var (
		v   filterValues
		err error
	)
	if v.account, err = optionalUUID(f.AccountID); err != nil {
		return v, err
	}
	if v.entry, err = optionalUUID(f.EntryID); err != nil {
		return v, err
	}
	if v.collection, err = optionalUUID(f.CollectionID); err != nil {
		return v, err
	}
	v.hub, err = optionalUUID(f.HubID)
	return v, err
}

// Records answers the filter's records with their entry's place.
func (TimeRepository) Records(ctx context.Context, f repository.Filter, limit int) ([]repository.Placed, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	v, err := filterOf(f)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G115: max is bounded by the use case
	rows, err := queries.ListTimeRecords(ctx, sqlc.ListTimeRecordsParams{
		FromDay: dayDate(f.From), ToDay: dayDate(f.To), AccountID: v.account, EntryID: v.entry,
		CollectionID: v.collection, HubID: v.hub, Billable: f.Billable, Max: int32(limit),
	})
	if err != nil {
		return nil, timeError(err, "reading time records")
	}
	out := make([]repository.Placed, 0, len(rows))
	for _, r := range rows {
		record, err := timeRecordFrom(r.ID, r.AccountID, r.EntryID, r.Corrects, r.WorkType, r.Day, r.StartedAt,
			r.Minutes, r.Note, r.Billable, r.Source, r.CreatedAt, r.UpdatedAt, r.Version)
		if err != nil {
			return nil, err
		}
		collection, err := optionalID(r.CollectionID)
		if err != nil {
			return nil, err
		}
		hub, err := optionalID(r.HubID)
		if err != nil {
			return nil, err
		}
		out = append(out, repository.Placed{Record: record, CollectionID: collection, HubID: hub, EntryTitle: r.EntryTitle})
	}
	return out, nil
}

// Lines answers the filter's records summed.
func (TimeRepository) Lines(ctx context.Context, f repository.Filter, limit int) ([]domain.Line, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	v, err := filterOf(f)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G115: max is bounded by the use case
	rows, err := queries.SumTimeLines(ctx, sqlc.SumTimeLinesParams{
		FromDay: dayDate(f.From), ToDay: dayDate(f.To), AccountID: v.account, CollectionID: v.collection,
		HubID: v.hub, Billable: f.Billable, Max: int32(limit),
	})
	if err != nil {
		return nil, timeError(err, "summing time records")
	}
	out := make([]domain.Line, 0, len(rows))
	for _, r := range rows {
		account, err := idFrom(r.AccountID)
		if err != nil {
			return nil, err
		}
		entry, err := optionalID(r.EntryID)
		if err != nil {
			return nil, err
		}
		collection, err := optionalID(r.CollectionID)
		if err != nil {
			return nil, err
		}
		hub, err := optionalID(r.HubID)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.Line{
			AccountID: account, EntryID: entry, CollectionID: collection, HubID: hub, WorkType: r.WorkType,
			Day: dateDay(r.Day), Billable: r.Billable, Minutes: int(r.Minutes), Records: int(r.Records),
		})
	}
	return out, nil
}

// MinutesOn answers everybody's minutes on each entry.
func (TimeRepository) MinutesOn(ctx context.Context, entries []shared.ID) (map[shared.ID]int, error) {
	out := map[shared.ID]int{}
	if len(entries) == 0 {
		return out, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := uuidsOf(entries)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SumTimeOnEntries(ctx, keys)
	if err != nil {
		return nil, timeError(err, "summing time on entries")
	}
	for _, r := range rows {
		id, err := idFrom(r.EntryID)
		if err != nil {
			return nil, err
		}
		out[id] = int(r.Minutes)
	}
	return out, nil
}

// WeekMinutes answers a person's minutes in a range of days.
func (TimeRepository) WeekMinutes(ctx context.Context, account shared.ID, from, to domain.Day) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	key, err := uuidOf(account)
	if err != nil {
		return 0, err
	}
	minutes, err := queries.SumTimeOfWeek(ctx, sqlc.SumTimeOfWeekParams{AccountID: key, FromDay: dayDate(from), ToDay: dayDate(to)})
	if err != nil {
		return 0, timeError(err, "summing a week")
	}
	return int(minutes), nil
}

// Timer answers a person's running timer.
func (TimeRepository) Timer(ctx context.Context, account shared.ID) (domain.Timer, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Timer{}, false, err
	}
	key, err := uuidOf(account)
	if err != nil {
		return domain.Timer{}, false, err
	}
	row, err := queries.FindTimeTimer(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Timer{}, false, nil
	}
	if err != nil {
		return domain.Timer{}, false, timeError(err, "reading a timer")
	}
	entry, err := optionalID(row.EntryID)
	if err != nil {
		return domain.Timer{}, false, err
	}
	return domain.Timer{
		AccountID: account, EntryID: entry, WorkType: row.WorkType, Note: row.Note, Billable: row.Billable,
		StartedAt: timeFrom(row.StartedAt), Version: int(row.Version),
	}, true, nil
}

// StartTimer writes a running timer.
func (TimeRepository) StartTimer(ctx context.Context, id shared.ID, t domain.Timer) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	account, err := uuidOf(t.AccountID)
	if err != nil {
		return err
	}
	entry, err := optionalUUID(t.EntryID)
	if err != nil {
		return err
	}
	err = queries.InsertTimeTimer(ctx, sqlc.InsertTimeTimerParams{
		ID: key, AccountID: account, EntryID: entry, WorkType: t.WorkType, Note: t.Note, Billable: t.Billable,
		StartedAt: timestampOf(t.StartedAt),
	})
	if isUniqueViolation(err) {
		return shared.ErrConflict.WithDetail("timers.running")
	}
	if err != nil {
		return timeError(err, "starting a timer")
	}
	return nil
}

// RemoveTimer stops a person's timer.
func (TimeRepository) RemoveTimer(ctx context.Context, account shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(account)
	if err != nil {
		return false, err
	}
	affected, err := queries.DeleteTimeTimer(ctx, key)
	if err != nil {
		return false, timeError(err, "stopping a timer")
	}
	return affected > 0, nil
}

func sheetFrom(
	idValue, accountValue pgtype.UUID, week pgtype.Date, state string, minutes int32, submitted, decided pgtype.Timestamptz,
	decidedByValue pgtype.UUID, reason string, version int32,
) (domain.Timesheet, error) {
	account, err := idFrom(accountValue)
	if err != nil {
		return domain.Timesheet{}, err
	}
	decidedBy, err := optionalID(decidedByValue)
	if err != nil {
		return domain.Timesheet{}, err
	}
	id, err := idFrom(idValue)
	if err != nil {
		return domain.Timesheet{}, err
	}
	return domain.Timesheet{
		ID: id, AccountID: account, WeekStart: dateDay(week), State: domain.SheetState(state), Minutes: int(minutes),
		SubmittedAt: optionalTime(submitted), DecidedAt: optionalTime(decided), DecidedBy: decidedBy,
		Reason: reason, Version: int(version),
	}, nil
}

// Sheet answers a person's week.
func (TimeRepository) Sheet(ctx context.Context, account shared.ID, week domain.Day) (domain.Timesheet, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Timesheet{}, false, err
	}
	key, err := uuidOf(account)
	if err != nil {
		return domain.Timesheet{}, false, err
	}
	row, err := queries.FindTimesheet(ctx, sqlc.FindTimesheetParams{AccountID: key, WeekStart: dayDate(domain.WeekOf(week))})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.OpenSheet(account, week), false, nil
	}
	if err != nil {
		return domain.Timesheet{}, false, timeError(err, "reading a week")
	}
	sheet, err := sheetFrom(row.ID, row.AccountID, row.WeekStart, row.State, row.Minutes, row.SubmittedAt, row.DecidedAt,
		row.DecidedBy, row.Reason, row.Version)
	return sheet, err == nil, err
}

// Sheets answers weeks in a state.
func (TimeRepository) Sheets(
	ctx context.Context, account shared.ID, state domain.SheetState, from, to domain.Day, limit int,
) ([]domain.Timesheet, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := optionalUUID(account)
	if err != nil {
		return nil, err
	}
	//nolint:gosec // G115: max is bounded by the use case
	rows, err := queries.ListTimesheets(ctx, sqlc.ListTimesheetsParams{
		State: string(state), AccountID: key, FromDay: dayDate(from), ToDay: dayDate(to), Max: int32(limit),
	})
	if err != nil {
		return nil, timeError(err, "reading weeks")
	}
	out := make([]domain.Timesheet, 0, len(rows))
	for _, r := range rows {
		sheet, err := sheetFrom(r.ID, r.AccountID, r.WeekStart, r.State, r.Minutes, r.SubmittedAt, r.DecidedAt,
			r.DecidedBy, r.Reason, r.Version)
		if err != nil {
			return nil, err
		}
		out = append(out, sheet)
	}
	return out, nil
}

// PutSheet writes a week.
func (TimeRepository) PutSheet(ctx context.Context, id shared.ID, s domain.Timesheet, expected int, at time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	account, err := uuidOf(s.AccountID)
	if err != nil {
		return err
	}
	decidedBy, err := optionalUUID(s.DecidedBy)
	if err != nil {
		return err
	}
	if expected == 0 {
		key, err := uuidOf(id)
		if err != nil {
			return err
		}
		//nolint:gosec // G115: a week's minutes are bounded by seven days
		err = queries.InsertTimesheet(ctx, sqlc.InsertTimesheetParams{
			ID: key, AccountID: account, WeekStart: dayDate(s.WeekStart), State: string(s.State),
			Minutes: int32(s.Minutes), SubmittedAt: optionalTimestamp(s.SubmittedAt),
			DecidedAt: optionalTimestamp(s.DecidedAt), DecidedBy: decidedBy, Reason: s.Reason, UpdatedAt: timestampOf(at),
		})
		if isUniqueViolation(err) {
			return sheetConflict(s, expected)
		}
		if err != nil {
			return timeError(err, "writing a week")
		}
		return nil
	}
	//nolint:gosec // G115: a week's minutes are bounded by seven days, a version by the row
	affected, err := queries.UpdateTimesheet(ctx, sqlc.UpdateTimesheetParams{
		AccountID: account, WeekStart: dayDate(s.WeekStart), State: string(s.State), Minutes: int32(s.Minutes),
		SubmittedAt: optionalTimestamp(s.SubmittedAt), DecidedAt: optionalTimestamp(s.DecidedAt),
		DecidedBy: decidedBy, Reason: s.Reason, UpdatedAt: timestampOf(at), ExpectedVersion: int32(expected),
	})
	if err != nil {
		return timeError(err, "writing a week")
	}
	if affected == 0 {
		return sheetConflict(s, expected)
	}
	return nil
}

func sheetConflict(s domain.Timesheet, expected int) error {
	return shared.ErrVersionConflict.WithDetail("timesheets.version_conflict").
		WithParams(map[string]string{"week": s.WeekStart.String(), "expected_version": strconv.Itoa(expected)})
}

// Policy answers the workspace's time policy.
func (TimeRepository) Policy(ctx context.Context) (domain.Policy, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Policy{}, err
	}
	row, err := queries.FindTimePolicy(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DefaultPolicy(), nil
	}
	if err != nil {
		return domain.Policy{}, timeError(err, "reading the time policy")
	}
	by, err := optionalID(row.UpdatedBy)
	if err != nil {
		return domain.Policy{}, err
	}
	policy := domain.Policy{
		Visibility: domain.Visibility(row.Visibility), UpdatedAt: timeFrom(row.UpdatedAt), UpdatedBy: by,
		Version: int(row.Version),
	}
	if row.LockedThrough.Valid {
		d := dateDay(row.LockedThrough)
		policy.LockedThrough = &d
	}
	return policy, nil
}

// PutPolicy writes the policy.
func (TimeRepository) PutPolicy(ctx context.Context, id shared.ID, p domain.Policy, expected int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	by, err := optionalUUID(p.UpdatedBy)
	if err != nil {
		return err
	}
	conflict := shared.ErrVersionConflict.WithDetail("time.policy_version_conflict").
		WithParams(map[string]string{"expected_version": strconv.Itoa(expected)})
	if expected == 0 {
		key, err := uuidOf(id)
		if err != nil {
			return err
		}
		err = queries.InsertTimePolicy(ctx, sqlc.InsertTimePolicyParams{
			ID: key, LockedThrough: optionalDayDate(p.LockedThrough), Visibility: string(p.Visibility),
			UpdatedAt: timestampOf(p.UpdatedAt), UpdatedBy: by,
		})
		if isUniqueViolation(err) {
			return conflict
		}
		if err != nil {
			return timeError(err, "writing the time policy")
		}
		return nil
	}
	//nolint:gosec // G115: a version is bounded by the row
	affected, err := queries.UpdateTimePolicy(ctx, sqlc.UpdateTimePolicyParams{
		LockedThrough: optionalDayDate(p.LockedThrough), Visibility: string(p.Visibility),
		UpdatedAt: timestampOf(p.UpdatedAt), UpdatedBy: by, ExpectedVersion: int32(expected),
	})
	if err != nil {
		return timeError(err, "writing the time policy")
	}
	if affected == 0 {
		return conflict
	}
	return nil
}
