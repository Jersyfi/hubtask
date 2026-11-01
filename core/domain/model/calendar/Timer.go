// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package calendar

import (
	"regexp"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Purpose is what a timer is the clock of - a closed set, because each has its own handler when it
// elapses and the job's metrics label it. A purpose is added with its handler: an effectiveness
// check after a delay is a RELATIVE_DATE rule (automation.md §1.1), not a timer.
type Purpose string

const (
	PurposeSLA      Purpose = "SLA"
	PurposeApproval Purpose = "APPROVAL"
	// PurposePageReview counts down to the end of a page's verification (UC-DOC-12 check 4).
	PurposePageReview Purpose = "PAGE_REVIEW"
	// PurposeOLA counts down a team's goal on an entry; its subject is the goal's record, since
	// one entry carries the goals of several groups (UC-SVC-02 check 10).
	PurposeOLA Purpose = "OLA"
)

// Valid answers whether the purpose is one of the set.
func (p Purpose) Valid() bool {
	switch p {
	case PurposeSLA, PurposeApproval, PurposePageReview, PurposeOLA:
		return true
	}
	return false
}

// TimerState is where a timer stands.
type TimerState string

const (
	TimerRunning TimerState = "RUNNING"
	TimerPaused  TimerState = "PAUSED"
	TimerFired   TimerState = "FIRED"
	TimerStopped TimerState = "STOPPED"
)

// namePattern is a timer's name within its purpose and subject - the goal it measures
// (`first_response`, `resolution`), so one subject carries several clocks of one purpose.
var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,49}$`)

// MaxBudget bounds a timer: a year of working time is more than any SLA or approval asks.
const MaxBudget = 366 * 24 * time.Hour

// Timer is a durable clock of working time: a budget counted on a calendar, paused and resumed by
// what happens to its subject, firing once when the budget is spent. Durable because it is a row,
// not a sleeping goroutine - a restart, a deploy or another worker picks it up where it stood.
//
// The calendar is the one it was armed on (a Spec copy), so a calendar changed later moves no
// clock already running. While it runs, DueAt is the instant its budget is spent; paused, the
// working time spent so far is Consumed and there is no due moment.
type Timer struct {
	ID           shared.ID
	TenantID     shared.ID
	Purpose      Purpose
	SubjectID    shared.ID
	Name         string
	Calendar     BusinessCalendar
	Budget       time.Duration
	Consumed     time.Duration
	RunningSince *time.Time
	DueAt        *time.Time
	State        TimerState
	FiredAt      *time.Time
	Version      int
}

// Arm starts a timer at now.
func Arm(
	id, tenant shared.ID, purpose Purpose, subject shared.ID, name string, cal BusinessCalendar,
	budget time.Duration, now time.Time,
) (Timer, error) {
	if !purpose.Valid() {
		return Timer{}, shared.ErrValidation.WithDetail("timer.purpose_unknown")
	}
	if !namePattern.MatchString(name) {
		return Timer{}, shared.ErrValidation.WithDetail("timer.name_invalid")
	}
	if cal.IsZero() {
		return Timer{}, shared.ErrValidation.WithDetail("timer.calendar_missing")
	}
	if budget <= 0 || budget > MaxBudget {
		return Timer{}, shared.ErrValidation.WithDetail("timer.budget_invalid")
	}
	if subject.IsZero() {
		return Timer{}, shared.ErrValidation.WithDetail("timer.subject_missing")
	}
	timer := Timer{
		ID: id, TenantID: tenant, Purpose: purpose, SubjectID: subject, Name: name, Calendar: cal,
		Budget: budget, State: TimerPaused, Version: 1,
	}
	return timer.run(now), nil
}

// Rebuild makes a timer again from what its purpose recorded of it, for a clock whose row was
// lost - a restore carries no timers (backup-restore.md §8.4): consumed is the working time already
// spent, and running says whether it counts from now. A running one with nothing left is due at
// now, so the next pass fires it the way it would have fired.
func Rebuild(
	id, tenant shared.ID, purpose Purpose, subject shared.ID, name string, cal BusinessCalendar,
	budget, consumed time.Duration, running bool, now time.Time,
) (Timer, error) {
	timer, err := Arm(id, tenant, purpose, subject, name, cal, budget, now)
	if err != nil {
		return Timer{}, err
	}
	timer.Consumed = min(max(consumed, 0), budget)
	timer.RunningSince, timer.DueAt, timer.State = nil, nil, TimerPaused
	if running {
		return timer.run(now), nil
	}
	return timer, nil
}

func (t Timer) run(now time.Time) Timer {
	since := now
	due := t.Calendar.Add(now, t.Budget-t.Consumed)
	t.RunningSince, t.DueAt, t.State = &since, &due, TimerRunning
	return t
}

// Spent answers the working time used up to now.
func (t Timer) Spent(now time.Time) time.Duration {
	spent := t.Consumed
	if t.State == TimerRunning && t.RunningSince != nil {
		spent += t.Calendar.Between(*t.RunningSince, now)
	}
	return min(spent, t.Budget)
}

// Remaining answers the working time left at now.
func (t Timer) Remaining(now time.Time) time.Duration { return t.Budget - t.Spent(now) }

// Pause stops the clock, keeping what was spent. Pausing a paused timer changes nothing.
func (t Timer) Pause(now time.Time) (Timer, error) {
	switch t.State {
	case TimerPaused:
		return t, nil
	case TimerRunning:
		t.Consumed = t.Spent(now)
		t.RunningSince, t.DueAt, t.State = nil, nil, TimerPaused
		return t, nil
	}
	return Timer{}, shared.ErrConflict.WithDetail("timer.ended")
}

// Resume runs the clock again from now with what is left. Resuming a running timer changes
// nothing.
func (t Timer) Resume(now time.Time) (Timer, error) {
	switch t.State {
	case TimerRunning:
		return t, nil
	case TimerPaused:
		return t.run(now), nil
	}
	return Timer{}, shared.ErrConflict.WithDetail("timer.ended")
}

// Stop ends the timer before it fires - the subject reached what the clock was waiting for.
func (t Timer) Stop(now time.Time) (Timer, error) {
	switch t.State {
	case TimerStopped:
		return t, nil
	case TimerRunning, TimerPaused:
		t.Consumed = t.Spent(now)
		t.RunningSince, t.DueAt, t.State = nil, nil, TimerStopped
		return t, nil
	}
	return Timer{}, shared.ErrConflict.WithDetail("timer.ended")
}

// Fire ends a running timer whose due moment has come. It fires once: a timer not running, or not
// due yet, is refused, which is what keeps two workers from firing it twice.
func (t Timer) Fire(now time.Time) (Timer, error) {
	if t.State != TimerRunning || t.DueAt == nil || t.DueAt.After(now) {
		return Timer{}, shared.ErrConflict.WithDetail("timer.not_due")
	}
	fired := now
	t.Consumed, t.RunningSince, t.State, t.FiredAt = t.Budget, nil, TimerFired, &fired
	return t, nil
}
