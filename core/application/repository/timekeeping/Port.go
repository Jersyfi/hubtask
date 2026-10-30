// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package timekeeping is the time module's storage port (M7): kinds of work, time records with the
// place of their entry, running timers, a person's weeks and the workspace's time policy.
package timekeeping

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
)

// Filter narrows records to a range of days and, where set, one person, entry, collection, hub
// and billable flag.
type Filter struct {
	From         domain.Day
	To           domain.Day
	AccountID    shared.ID
	EntryID      shared.ID
	CollectionID shared.ID
	HubID        shared.ID
	Billable     *bool
}

// Placed is a record with where its entry lies now, and its title; a record on no entry, or on an
// entry purged since, has no place.
type Placed struct {
	domain.Record
	CollectionID shared.ID
	HubID        shared.ID
	EntryTitle   string
}

// Store reads and writes the time module's rows, inside the caller's transaction.
type Store interface {
	WorkTypes(ctx context.Context) ([]domain.WorkType, error)
	// PutWorkType writes a kind of work by its key; id is used for a new one.
	PutWorkType(ctx context.Context, id shared.ID, w domain.WorkType, at time.Time) error

	InsertRecord(ctx context.Context, r domain.Record) error
	// UpdateRecord writes a record held to the version read.
	UpdateRecord(ctx context.Context, r domain.Record, expected int) error
	// DeleteRecord removes a record held to the version read.
	DeleteRecord(ctx context.Context, id shared.ID, expected int) error
	FindRecord(ctx context.Context, id shared.ID) (domain.Record, error)
	// Records answers the filter's records, the earliest day first, at most max.
	Records(ctx context.Context, f Filter, max int) ([]Placed, error)
	// Lines answers the filter's records summed per person, entry, kind, day and billable flag, at
	// most max.
	Lines(ctx context.Context, f Filter, max int) ([]domain.Line, error)
	// MinutesOn answers everybody's recorded minutes on each entry.
	MinutesOn(ctx context.Context, entries []shared.ID) (map[shared.ID]int, error)
	// WeekMinutes answers a person's minutes in a range of days.
	WeekMinutes(ctx context.Context, account shared.ID, from, to domain.Day) (int, error)

	// Timer answers a person's running timer, and whether one runs.
	Timer(ctx context.Context, account shared.ID) (domain.Timer, bool, error)
	// StartTimer writes a running timer; one already running answers timers.running.
	StartTimer(ctx context.Context, id shared.ID, t domain.Timer) error
	// RemoveTimer stops a person's timer, and reports whether one ran.
	RemoveTimer(ctx context.Context, account shared.ID) (bool, error)

	// Sheet answers a person's week, and whether it was ever handed in.
	Sheet(ctx context.Context, account shared.ID, week domain.Day) (domain.Timesheet, bool, error)
	// Sheets answers the weeks in a state (every state for ""), of one person or everybody, from a
	// first Monday to a last, at most max.
	Sheets(ctx context.Context, account shared.ID, state domain.SheetState, from, to domain.Day, max int) ([]domain.Timesheet, error)
	// PutSheet writes a week: a new row when expected is 0, else held to the version read.
	PutSheet(ctx context.Context, id shared.ID, s domain.Timesheet, expected int, at time.Time) error

	// Policy answers the workspace's time policy; a workspace that set none has the default.
	Policy(ctx context.Context) (domain.Policy, error)
	// PutPolicy writes the policy: a new row when expected is 0, else held to the version read.
	PutPolicy(ctx context.Context, id shared.ID, p domain.Policy, expected int) error
}
