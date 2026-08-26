// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"errors"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/recurrence"
)

// SchedulePass turns the schedules whose moment has come into backup jobs (E-05).
//
// One pass per scope, and the scope is what the caller opens it under: a tenant's poller runs it
// for that tenant, and the leader runs it for the instance-wide schedules that belong to no tenant.
// The pass itself does not know the difference, which is the point - what decides is the scope the
// transaction was opened with, and that is the one place in this system that may decide it.
type SchedulePass struct {
	Schedules  repository.Schedules
	Runs       repository.Runs
	Jobs       queue.Queue
	Expander   recurrence.Expander
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// Hold takes the row lock on the pass's own job, for the reason D-03's reminders take one: the pass
// decides when it next runs from the data, and a write committing between that read and the
// reschedule would find the row RUNNING - where the queue's conflict clause cannot pull a wake-up
// forward - and its schedule would wait for a wake-up nobody scheduled.
func (p SchedulePass) Hold(ctx context.Context, job queue.Job) error {
	return p.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		return p.Jobs.Hold(ctx, job)
	})
}

// PassResult is what one round did, and when the next is owed.
type PassResult struct {
	// Started is how many runs this round enqueued.
	Started int
	// NextDue is the earliest moment anything in scope owes a backup, and the zero time when
	// nothing does. It is what the poller reschedules itself to; a scope that owes nothing lets
	// its poller finish, and the next write re-seeds it.
	NextDue time.Time
}

// scheduleBatch bounds one round. A backlog of missed moments - a worker that was down for a week -
// becomes several rounds rather than a hundred jobs enqueued in one transaction.
const scheduleBatch = 50

// Run does one round for the scope the caller opens it under.
func (p SchedulePass) Run(ctx context.Context, scope persistence.Scope) (PassResult, error) {
	now := p.Clock.Now()
	var result PassResult

	err := p.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		due, err := p.Schedules.Due(ctx, now, scheduleBatch)
		if err != nil {
			return err
		}
		for _, schedule := range due {
			started, err := p.fire(ctx, schedule, now)
			if err != nil {
				return err
			}
			if started {
				result.Started++
			}
		}

		result.NextDue, err = p.Schedules.NextDue(ctx)
		return err
	})
	if err != nil {
		return PassResult{}, err
	}
	return result, nil
}

// fire enqueues one schedule's run and moves it on to its next moment.
//
// The schedule is advanced whether or not the run could be enqueued, and that is deliberate: a
// schedule that stayed on a moment it could not act on would try again on every round for ever,
// and a missed backup is a missed backup rather than a reason to stop backing up.
func (p SchedulePass) fire(
	ctx context.Context, schedule domain.Schedule, now time.Time,
) (bool, error) {
	occurrence := schedule.NextRunAt
	mode, err := p.modeOn(schedule, occurrence)
	if err != nil {
		return false, err
	}

	next, err := p.nextAfter(schedule, occurrence)
	if err != nil {
		return false, err
	}
	if err := p.Schedules.SetNextRun(ctx, schedule.ID, next); err != nil {
		return false, err
	}

	parentID, err := p.parentFor(ctx, schedule, mode)
	if err != nil {
		return false, err
	}
	if parentID.IsZero() && mode == domain.ModeIncremental {
		// Nothing to continue: the first run of an incremental schedule is a full one. Promoting
		// it is right here where it would be wrong for a caller who asked by hand - nobody is
		// waiting for an answer, and the alternative is a schedule that never produces anything.
		mode = domain.ModeFull
	}

	runID := p.IDs.NewID()
	_, err = p.Jobs.Enqueue(ctx, queue.Request{
		Kind:      queue.KindBackupRun,
		TenantID:  schedule.TenantID,
		DedupeKey: string(queue.KindBackupRun) + ":" + schedule.TargetID.String(),
		Payload: map[string]any{
			"run_id":        runID.String(),
			"target_id":     schedule.TargetID.String(),
			"schedule_id":   schedule.ID.String(),
			"mode":          string(mode),
			"parent_run_id": parentID.String(),
			"include_media": schedule.IncludeMedia,
			"include_audit": schedule.IncludeAudit,
			"trigger":       string(domain.TriggerSchedule),
		},
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

// modeOn decides whether this occurrence produces a full archive.
//
// `full_rrule` names days rather than instants, so the window asked for is the day around the
// occurrence in the schedule's own zone - which is what makes `FREQ=WEEKLY;BYDAY=SU` mean "the
// Sunday run" whatever hour the daily rule fires at.
func (p SchedulePass) modeOn(schedule domain.Schedule, occurrence time.Time) (domain.Mode, error) {
	if schedule.Mode == domain.ModeFull || schedule.FullRRULE == "" {
		return schedule.Mode, nil
	}
	zone, err := time.LoadLocation(schedule.TimeZone)
	if err != nil {
		return "", shared.ErrValidation.WithDetail(domain.CodeScheduleZoneUnknown).
			WithParams(map[string]string{"timezone": schedule.TimeZone})
	}

	days, err := p.Expander.Occurrences(recurrence.Rule{
		RRULE: schedule.FullRRULE, TimeZone: schedule.TimeZone, Start: schedule.Anchor(),
	}, occurrence.Add(-fullWindow), occurrence.Add(fullWindow), fullDayLimit)
	if err != nil {
		return "", ruleRefusal(err)
	}
	if schedule.IsFullOn(occurrence, days, zone) {
		return domain.ModeFull, nil
	}
	return domain.ModeIncremental, nil
}

const (
	// fullWindow is how far either side of an occurrence the promoting rule is expanded. A day and
	// a half, which covers the occurrence's own calendar day in every zone on earth and no more.
	fullWindow = 36 * time.Hour
	// fullDayLimit bounds that expansion. A rule dense enough to exceed it inside three days is
	// one nobody is reading.
	fullDayLimit = 64
)

// nextAfter is when the schedule fires after this occurrence, and the zero time when it never does.
func (p SchedulePass) nextAfter(schedule domain.Schedule, occurrence time.Time) (time.Time, error) {
	moments, err := p.Expander.Occurrences(recurrence.Rule{
		RRULE: schedule.RRULE, TimeZone: schedule.TimeZone, Start: schedule.Anchor(),
	}, occurrence, occurrence.Add(scheduleHorizon), 1)
	if err != nil {
		return time.Time{}, ruleRefusal(err)
	}
	if len(moments) == 0 {
		return time.Time{}, nil
	}
	return moments[0].UTC(), nil
}

// parentFor answers the archive an incremental continues, or nothing when there is none yet.
func (p SchedulePass) parentFor(
	ctx context.Context, schedule domain.Schedule, mode domain.Mode,
) (shared.ID, error) {
	if mode != domain.ModeIncremental {
		return "", nil
	}
	parent, err := p.Runs.LatestSuccessful(ctx, schedule.TargetID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	return parent.ID, nil
}

// SchedulePlans answers the plan a run was made under, which is what an expiry pass needs and the
// only thing it needs.
type SchedulePlans struct {
	Schedules  repository.Schedules
	UnitOfWork persistence.UnitOfWork
}

// Plan answers the retention plan and the zone its generations are counted in.
func (s SchedulePlans) Plan(
	ctx context.Context, tenantID, scheduleID shared.ID,
) (domain.Retention, string, error) {
	var schedule domain.Schedule
	err := s.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID},
		func(ctx context.Context) error {
			var err error
			schedule, err = s.Schedules.Find(ctx, scheduleID)
			return err
		})
	if err != nil {
		return domain.Retention{}, "", err
	}
	return schedule.Retention, schedule.TimeZone, nil
}
