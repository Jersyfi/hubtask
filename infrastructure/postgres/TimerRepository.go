// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/calendar"
	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// TimerRepository stores durable timers (migration 0229).
type TimerRepository struct{}

func NewTimerRepository() TimerRepository { return TimerRepository{} }

var _ repository.Timers = TimerRepository{}

// calendarDocument is the stored copy of a timer's calendar.
type calendarDocument struct {
	Zone     string      `json:"zone"`
	Week     [7][][2]int `json:"week"`
	Holidays []string    `json:"holidays,omitempty"`
}

func calendarJSON(cal calendar.BusinessCalendar) ([]byte, error) {
	spec := cal.Spec()
	doc := calendarDocument{Zone: spec.Zone}
	for day, spans := range spec.Week {
		doc.Week[day] = make([][2]int, 0, len(spans))
		for _, span := range spans {
			doc.Week[day] = append(doc.Week[day], [2]int{span.From, span.To})
		}
	}
	for _, date := range spec.Holidays {
		doc.Holidays = append(doc.Holidays, fmt.Sprintf("%04d-%02d-%02d", date.Year, date.Month, date.Day))
	}
	return json.Marshal(doc)
}

func calendarFromJSON(raw []byte) (calendar.BusinessCalendar, error) {
	var doc calendarDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return calendar.BusinessCalendar{}, err
	}
	spec := calendar.Spec{Zone: doc.Zone}
	for day, spans := range doc.Week {
		for _, span := range spans {
			spec.Week[day] = append(spec.Week[day], calendar.Span{From: span[0], To: span[1]})
		}
	}
	for _, text := range doc.Holidays {
		day, err := time.Parse("2006-01-02", text)
		if err != nil {
			return calendar.BusinessCalendar{}, err
		}
		spec.Holidays = append(spec.Holidays, calendar.Date{Year: day.Year(), Month: day.Month(), Day: day.Day()})
	}
	return calendar.New(spec)
}

func timestampOrNull(at *time.Time) pgtype.Timestamptz {
	if at == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *at, Valid: true}
}

func (TimerRepository) Insert(ctx context.Context, timer calendar.Timer, now time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(timer.ID)
	if err != nil {
		return err
	}
	tenant, err := uuidOf(timer.TenantID)
	if err != nil {
		return err
	}
	subject, err := uuidOf(timer.SubjectID)
	if err != nil {
		return err
	}
	doc, err := calendarJSON(timer.Calendar)
	if err != nil {
		return shared.ErrInternal.WithDetail("timer.calendar_unwritable").WithCause(err)
	}
	if err := queries.InsertTimer(ctx, sqlc.InsertTimerParams{
		ID: id, TenantID: tenant, Purpose: string(timer.Purpose), SubjectID: subject, Name: timer.Name,
		Calendar: doc, BudgetSeconds: int64(timer.Budget / time.Second),
		ConsumedSeconds: int64(timer.Consumed / time.Second),
		RunningSince:    timestampOrNull(timer.RunningSince), DueAt: timestampOrNull(timer.DueAt),
		State: string(timer.State), FiredAt: timestampOrNull(timer.FiredAt),
		Now: pgtype.Timestamptz{Time: now, Valid: true},
	}); err != nil {
		if isUniqueViolation(err) {
			return shared.ErrConflict.WithDetail("timer.already_running")
		}
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the timer %s: %w", timer.ID, err))
	}
	return nil
}

func (TimerRepository) FindLive(
	ctx context.Context, purpose calendar.Purpose, subject shared.ID, name string,
) (calendar.Timer, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return calendar.Timer{}, err
	}
	subjectID, err := uuidOf(subject)
	if err != nil {
		return calendar.Timer{}, err
	}
	row, err := queries.FindLiveTimer(ctx, sqlc.FindLiveTimerParams{
		Purpose: string(purpose), SubjectID: subjectID, Name: name,
	})
	if err != nil {
		if IsNoRows(err) {
			return calendar.Timer{}, shared.ErrNotFound
		}
		return calendar.Timer{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading a live timer: %w", err))
	}
	return timerFrom(sqlc.ListDueTimersRow(row))
}

func (TimerRepository) Update(ctx context.Context, timer calendar.Timer, expectedVersion int, now time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(timer.ID)
	if err != nil {
		return err
	}
	affected, err := queries.UpdateTimer(ctx, sqlc.UpdateTimerParams{
		ID: id, ExpectedVersion: int32(expectedVersion), //nolint:gosec // a version is far below 2^31
		ConsumedSeconds: int64(timer.Consumed / time.Second),
		RunningSince:    timestampOrNull(timer.RunningSince), DueAt: timestampOrNull(timer.DueAt),
		State: string(timer.State), FiredAt: timestampOrNull(timer.FiredAt),
		Now: pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("updating the timer %s: %w", timer.ID, err))
	}
	if affected == 0 {
		// The row moved on, or - through row level security - was never this tenant's: one
		// answer for both (multi-tenancy.md §2).
		return shared.ErrVersionConflict.WithDetail("timer.version_conflict")
	}
	return nil
}

func (TimerRepository) ClaimDue(ctx context.Context, now time.Time, limit int) ([]calendar.Timer, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListDueTimers(ctx, sqlc.ListDueTimersParams{
		Now: pgtype.Timestamptz{Time: now, Valid: true}, BatchSize: int32(limit), //nolint:gosec // a batch size is small
	})
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("claiming due timers: %w", err))
	}
	timers := make([]calendar.Timer, 0, len(rows))
	for _, row := range rows {
		timer, err := timerFrom(row)
		if err != nil {
			return nil, err
		}
		timers = append(timers, timer)
	}
	return timers, nil
}

func (TimerRepository) NextDue(ctx context.Context) (*time.Time, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	next, err := queries.NextTimerDue(ctx)
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the next timer: %w", err))
	}
	return optionalTime(next), nil
}

func timerFrom(row sqlc.ListDueTimersRow) (calendar.Timer, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return calendar.Timer{}, err
	}
	tenant, err := idFrom(row.TenantID)
	if err != nil {
		return calendar.Timer{}, err
	}
	subject, err := idFrom(row.SubjectID)
	if err != nil {
		return calendar.Timer{}, err
	}
	cal, err := calendarFromJSON(row.Calendar)
	if err != nil {
		return calendar.Timer{}, shared.ErrInternal.WithDetail("timer.calendar_unreadable").
			WithCause(fmt.Errorf("the calendar of timer %s: %w", id, err))
	}
	return calendar.Timer{
		ID: id, TenantID: tenant, Purpose: calendar.Purpose(row.Purpose), SubjectID: subject,
		Name: row.Name, Calendar: cal,
		Budget:       time.Duration(row.BudgetSeconds) * time.Second,
		Consumed:     time.Duration(row.ConsumedSeconds) * time.Second,
		RunningSince: optionalTime(row.RunningSince), DueAt: optionalTime(row.DueAt),
		State: calendar.TimerState(row.State), FiredAt: optionalTime(row.FiredAt),
		Version: int(row.Version),
	}, nil
}
