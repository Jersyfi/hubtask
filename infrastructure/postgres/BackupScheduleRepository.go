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

// BackupScheduleRepository stores what runs when (E-05).
//
// `next_run_at` is a stored decision rather than a rule expanded on every read. Expanding an RRULE
// costs a library call per schedule, and a poller that did it on every wake-up would do it for
// every schedule that is not due; the value is written by the pass that last ran, which is the
// shape D-03's reminders already use.
type BackupScheduleRepository struct{}

func NewBackupScheduleRepository() BackupScheduleRepository { return BackupScheduleRepository{} }

var _ repository.Schedules = BackupScheduleRepository{}

// Insert writes a schedule with the moment it is next due already decided.
func (r BackupScheduleRepository) Insert(
	ctx context.Context, schedule domain.Schedule, nextRunAt time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(schedule.ID)
	if err != nil {
		return err
	}
	targetID, err := uuidOf(schedule.TargetID)
	if err != nil {
		return err
	}
	tenantID, err := optionalUUID(schedule.TenantID)
	if err != nil {
		return err
	}
	scopeID, err := optionalUUID(schedule.ScopeID)
	if err != nil {
		return err
	}
	retention, err := json.Marshal(retentionRow{
		KeepLast: schedule.Retention.KeepLast, KeepDaily: schedule.Retention.KeepDaily,
		KeepWeekly: schedule.Retention.KeepWeekly, KeepMonthly: schedule.Retention.KeepMonthly,
		KeepYearly: schedule.Retention.KeepYearly, MinKeep: schedule.Retention.MinKeep,
	})
	if err != nil {
		return shared.ErrInternal.WithDetail("backup.schedule_unserialisable").WithCause(err)
	}

	err = queries.InsertBackupSchedule(ctx, sqlc.InsertBackupScheduleParams{
		ID: id, TargetID: targetID, TenantID: tenantID,
		ScopeKind: string(schedule.Scope), ScopeID: scopeID,
		Rrule: schedule.RRULE, TimeZone: schedule.TimeZone, Mode: string(schedule.Mode),
		FullRrule:    optionalText(schedule.FullRRULE),
		IncludeMedia: schedule.IncludeMedia, IncludeAudit: schedule.IncludeAudit,
		Retention: retention, NotifyOn: notificationsOf(schedule.NotifyOn),
		NextRunAt: optionalTimestamp(timeOrNil(nextRunAt)), CreatedAt: timestampOf(schedule.CreatedAt),
	})
	if err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing backup schedule %s: %w", schedule.ID, err))
	}
	return nil
}

// List answers the schedules visible in the caller's scope.
func (r BackupScheduleRepository) List(ctx context.Context) ([]domain.Schedule, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListBackupSchedules(ctx)
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing backup schedules: %w", err))
	}
	return schedulesOf(rows)
}

// Find answers one schedule.
func (r BackupScheduleRepository) Find(ctx context.Context, id shared.ID) (domain.Schedule, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Schedule{}, err
	}
	scheduleID, err := uuidOf(id)
	if err != nil {
		return domain.Schedule{}, err
	}

	row, err := queries.FindBackupSchedule(ctx, scheduleID)
	if err != nil {
		if IsNoRows(err) {
			return domain.Schedule{}, shared.ErrNotFound.WithDetail(domain.CodeScheduleNotFound)
		}
		return domain.Schedule{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading backup schedule %s: %w", id, err))
	}
	return scheduleOf(row)
}

// Due answers the schedules whose moment has come.
func (r BackupScheduleRepository) Due(
	ctx context.Context, now time.Time, batch int,
) ([]domain.Schedule, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	if batch <= 0 || batch > maxExportBatch {
		batch = 100
	}

	rows, err := queries.DueBackupSchedules(ctx, sqlc.DueBackupSchedulesParams{
		Now: timestampOf(now), Batch: int32(batch),
	})
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the due backup schedules: %w", err))
	}
	return schedulesOf(rows)
}

// NextDue is the earliest moment anything in scope is owed.
func (r BackupScheduleRepository) NextDue(ctx context.Context) (time.Time, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return time.Time{}, err
	}
	due, err := queries.NextBackupScheduleDue(ctx)
	if err != nil {
		if IsNoRows(err) {
			return time.Time{}, nil
		}
		return time.Time{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the next backup moment: %w", err))
	}
	if !due.Valid {
		return time.Time{}, nil
	}
	return due.Time.UTC(), nil
}

// SetNextRun records when the schedule is next owed.
func (r BackupScheduleRepository) SetNextRun(
	ctx context.Context, id shared.ID, nextRunAt time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	scheduleID, err := uuidOf(id)
	if err != nil {
		return err
	}

	err = queries.SetBackupScheduleNextRun(ctx, sqlc.SetBackupScheduleNextRunParams{
		NextRunAt: optionalTimestamp(timeOrNil(nextRunAt)), ID: scheduleID,
	})
	if err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("rescheduling backup schedule %s: %w", id, err))
	}
	return nil
}

// retentionRow is the plan as the jsonb column holds it, in the names the contract uses.
type retentionRow struct {
	KeepLast    int `json:"keep_last"`
	KeepDaily   int `json:"keep_daily"`
	KeepWeekly  int `json:"keep_weekly"`
	KeepMonthly int `json:"keep_monthly"`
	KeepYearly  int `json:"keep_yearly"`
	MinKeep     int `json:"min_keep"`
}

// The three read statements answer the table's own row, because they select the table's own
// columns. One mapper serves all of them; a second would be a second place for a column to go
// missing.
func schedulesOf(rows []sqlc.BackupSchedule) ([]domain.Schedule, error) {
	schedules := make([]domain.Schedule, 0, len(rows))
	for _, row := range rows {
		schedule, err := scheduleOf(row)
		if err != nil {
			return nil, err
		}
		schedules = append(schedules, schedule)
	}
	return schedules, nil
}

func scheduleOf(row sqlc.BackupSchedule) (domain.Schedule, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return domain.Schedule{}, err
	}
	targetID, err := idFrom(row.TargetID)
	if err != nil {
		return domain.Schedule{}, err
	}
	tenantID, err := optionalID(row.TenantID)
	if err != nil {
		return domain.Schedule{}, err
	}
	scopeID, err := optionalID(row.ScopeID)
	if err != nil {
		return domain.Schedule{}, err
	}

	var plan retentionRow
	if len(row.Retention) > 0 {
		if err := json.Unmarshal(row.Retention, &plan); err != nil {
			return domain.Schedule{}, shared.ErrInternal.
				WithDetail("backup.schedule_unreadable").WithCause(err)
		}
	}

	schedule := domain.Schedule{
		ID: id, TargetID: targetID, TenantID: tenantID,
		Scope: domain.ScheduleScope(row.ScopeKind), ScopeID: scopeID,
		RRULE: row.Rrule, TimeZone: row.TimeZone, Mode: domain.Mode(row.Mode),
		IncludeMedia: row.IncludeMedia, IncludeAudit: row.IncludeAudit,
		Retention: domain.Retention{
			KeepLast: plan.KeepLast, KeepDaily: plan.KeepDaily, KeepWeekly: plan.KeepWeekly,
			KeepMonthly: plan.KeepMonthly, KeepYearly: plan.KeepYearly, MinKeep: plan.MinKeep,
		},
		Enabled: row.Enabled, CreatedAt: row.CreatedAt.Time.UTC(), Version: int(row.Version),
	}
	if row.FullRrule != nil {
		schedule.FullRRULE = *row.FullRrule
	}
	for _, occasion := range row.NotifyOn {
		schedule.NotifyOn = append(schedule.NotifyOn, domain.Notification(occasion))
	}
	if row.NextRunAt.Valid {
		schedule.NextRunAt = row.NextRunAt.Time.UTC()
	}
	return schedule, nil
}

func notificationsOf(occasions []domain.Notification) []string {
	out := make([]string, 0, len(occasions))
	for _, occasion := range occasions {
		out = append(out, string(occasion))
	}
	return out
}

// timeOrNil turns the zero time into the absence a nullable column wants.
func timeOrNil(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}
