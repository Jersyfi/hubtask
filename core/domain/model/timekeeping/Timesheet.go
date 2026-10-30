// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// SheetState is where a person's week stands (UC-TIM-02, UC-TIM-03).
type SheetState string

const (
	// SheetOpen: the person writes it. A week nobody handed in is open.
	SheetOpen SheetState = "OPEN"
	// SheetSubmitted: handed in, waiting for an approver; its records no longer change.
	SheetSubmitted SheetState = "SUBMITTED"
	// SheetApproved: approved; its records never change again, a correction goes into an open week.
	SheetApproved SheetState = "APPROVED"
	// SheetRejected: sent back with a reason; the person changes it and hands it in again.
	SheetRejected SheetState = "REJECTED"
)

// Valid reports whether the state is one of the four.
func (s SheetState) Valid() bool {
	return s == SheetOpen || s == SheetSubmitted || s == SheetApproved || s == SheetRejected
}

// Writable reports whether the week's records may change: an open or a sent-back week.
func (s SheetState) Writable() bool { return s == "" || s == SheetOpen || s == SheetRejected }

// MaxReasonLength is a rejection's reason, in characters.
const MaxReasonLength = 500

// Timesheet is one person's week from Monday.
type Timesheet struct {
	// ID is the week's row, empty for a week nobody has handed in yet.
	ID        shared.ID
	AccountID shared.ID
	WeekStart Day
	State     SheetState
	// Minutes are the week's total when it was handed in, kept for the approver's list.
	Minutes     int
	SubmittedAt *time.Time
	DecidedAt   *time.Time
	DecidedBy   shared.ID
	// Reason is why an approver sent it back: a person's words.
	Reason  string
	Version int
}

// OpenSheet is the week nobody has handed in yet.
func OpenSheet(account shared.ID, week Day) Timesheet {
	return Timesheet{AccountID: account, WeekStart: WeekOf(week), State: SheetOpen}
}

// Submit hands the week in with its total; an empty week is not handed in.
func (t Timesheet) Submit(minutes int, now time.Time) (Timesheet, error) {
	if !t.State.Writable() {
		return t, shared.ErrConflict.WithDetail("timesheets.not_open")
	}
	if minutes <= 0 {
		return t, shared.ErrValidation.WithDetail("timesheets.empty")
	}
	t.State, t.Minutes, t.SubmittedAt = SheetSubmitted, minutes, &now
	t.DecidedAt, t.DecidedBy, t.Reason = nil, "", ""
	return t, nil
}

// Withdraw takes a handed-in week back before anybody decided.
func (t Timesheet) Withdraw() (Timesheet, error) {
	if t.State != SheetSubmitted {
		return t, shared.ErrConflict.WithDetail("timesheets.not_submitted")
	}
	t.State, t.SubmittedAt = SheetOpen, nil
	return t, nil
}

// Approve approves a handed-in week; nobody approves their own (four eyes).
func (t Timesheet) Approve(approver shared.ID, now time.Time) (Timesheet, error) {
	if err := t.decidable(approver); err != nil {
		return t, err
	}
	t.State, t.DecidedAt, t.DecidedBy, t.Reason = SheetApproved, &now, approver, ""
	return t, nil
}

// Reject sends a handed-in week back with a reason.
func (t Timesheet) Reject(approver shared.ID, reason string, now time.Time) (Timesheet, error) {
	if err := t.decidable(approver); err != nil {
		return t, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return t, invalid("/reason", "timesheets.reason_required")
	}
	if utf8.RuneCountInString(reason) > MaxReasonLength {
		return t, shared.ErrValidation.WithDetail("timesheets.reason_too_long").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxReasonLength)}).
			WithFields(shared.FieldError{Path: "/reason", Code: "timesheets.reason_too_long"})
	}
	t.State, t.DecidedAt, t.DecidedBy, t.Reason = SheetRejected, &now, approver, reason
	return t, nil
}

func (t Timesheet) decidable(approver shared.ID) error {
	if t.State != SheetSubmitted {
		return shared.ErrConflict.WithDetail("timesheets.not_submitted")
	}
	if approver == t.AccountID {
		return shared.ErrForbidden.WithDetail("timesheets.own_week")
	}
	return nil
}

// Visibility is what a lead sees of other people's time (M7, the works council's option).
type Visibility string

const (
	// VisibilityFull: an approver sees each person's records and minutes.
	VisibilityFull Visibility = "FULL"
	// VisibilityAggregated: a report over other people sums them and never names one; a person's
	// records are seen only by that person, and by an approver for the week they decide.
	VisibilityAggregated Visibility = "AGGREGATED"
)

// Valid reports whether the visibility is one of the two.
func (v Visibility) Valid() bool { return v == VisibilityFull || v == VisibilityAggregated }

// Policy is the workspace's rules for time: the period that is locked for everybody and what a
// lead sees. A workspace that set nothing has no locked period and full visibility.
type Policy struct {
	// LockedThrough is the last locked day: no record on or before it is written, changed or
	// removed, whatever its week's state.
	LockedThrough *Day
	Visibility    Visibility
	UpdatedAt     time.Time
	UpdatedBy     shared.ID
	Version       int
}

// DefaultPolicy is a workspace's policy before anybody set one.
func DefaultPolicy() Policy { return Policy{Visibility: VisibilityFull} }

// Locked reports whether a day lies in the locked period.
func (p Policy) Locked(d Day) bool { return p.LockedThrough != nil && d <= *p.LockedThrough }

// MayWrite answers whether a record on a day may be written, changed or removed given its week's
// state: not in the locked period and not in a handed-in or approved week.
func (p Policy) MayWrite(d Day, week SheetState) error {
	if p.Locked(d) {
		return shared.ErrConflict.WithDetail("time.period_locked").
			WithParams(map[string]string{"locked_through": p.LockedThrough.String()})
	}
	if !week.Writable() {
		return shared.ErrConflict.WithDetail("time.week_not_open").
			WithParams(map[string]string{"state": string(week)})
	}
	return nil
}

// WorkType is a kind of work a workspace records time as: development, meeting, support.
type WorkType struct {
	Key  string
	Name string
	// Billable is the flag a new record of this kind starts with.
	Billable bool
	Archived bool
	Position int
	Version  int
}

// The bounds of the work types.
const (
	MaxWorkTypes      = 100
	MaxWorkTypeName   = 80
	workTypeKeyMaxLen = 40
)

var workTypeKey = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

// CheckWorkType holds a kind of work to its bounds: a key of lowercase letters, digits and
// underscores, a one-line name of at most MaxWorkTypeName characters.
func CheckWorkType(w WorkType) error {
	if !workTypeKey.MatchString(w.Key) || len(w.Key) > workTypeKeyMaxLen {
		return invalid("/key", "work_types.key_invalid")
	}
	name := strings.TrimSpace(w.Name)
	if name == "" {
		return invalid("/name", "work_types.name_required")
	}
	if utf8.RuneCountInString(name) > MaxWorkTypeName || strings.ContainsAny(name, "\n\r\t") {
		return shared.ErrValidation.WithDetail("work_types.name_invalid").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxWorkTypeName)}).
			WithFields(shared.FieldError{Path: "/name", Code: "work_types.name_invalid"})
	}
	return nil
}

// Timer is a person's running clock: at most one per person, the same on every device.
type Timer struct {
	AccountID shared.ID
	EntryID   shared.ID
	WorkType  string
	Note      string
	Billable  bool
	StartedAt time.Time
	Version   int
}

// Stop turns the timer into a record of the day it started in the person's zone; a timer that ran
// longer than a day is refused, to be recorded by hand.
func (t Timer) Stop(id shared.ID, now time.Time, zone *time.Location) (Record, error) {
	minutes, err := Span(t.StartedAt, now)
	if err != nil {
		if now.After(t.StartedAt) {
			return Record{}, shared.ErrValidation.WithDetail("timers.ran_too_long").
				WithParams(map[string]string{"maximum": strconv.Itoa(MaxMinutes)})
		}
		return Record{}, shared.ErrValidation.WithDetail("timers.not_started")
	}
	started := t.StartedAt
	record := Record{
		ID: id, AccountID: t.AccountID, EntryID: t.EntryID, WorkType: t.WorkType,
		Day: planningDay(started, zone), StartedAt: &started, Minutes: minutes, Note: t.Note,
		Billable: t.Billable, Source: SourceTimer, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	return record, record.Check()
}
