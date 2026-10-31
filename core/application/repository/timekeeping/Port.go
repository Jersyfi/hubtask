// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package timekeeping is the time module's storage port (M7): kinds of work, time records with the
// place of their entry, running timers, a person's weeks, the workspace's time policy, and the
// resources and costs - capacity, absences, cost rates and budgets.
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
	Records(ctx context.Context, f Filter, limit int) ([]Placed, error)
	// Lines answers the filter's records summed per person, entry, kind, day and billable flag, at
	// most max.
	Lines(ctx context.Context, f Filter, limit int) ([]domain.Line, error)
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
	Sheets(ctx context.Context, account shared.ID, state domain.SheetState, from, to domain.Day, limit int) ([]domain.Timesheet, error)
	// PutSheet writes a week: a new row when expected is 0, else held to the version read.
	PutSheet(ctx context.Context, id shared.ID, s domain.Timesheet, expected int, at time.Time) error

	// Policy answers the workspace's time policy; a workspace that set none has the default.
	Policy(ctx context.Context) (domain.Policy, error)
	// PutPolicy writes the policy: a new row when expected is 0, else held to the version read.
	PutPolicy(ctx context.Context, id shared.ID, p domain.Policy, expected int) error
}

// Resources reads and writes a person's capacity and absences, the cost rates and the budgets of
// hubs and collections (migration 0247), inside the caller's transaction.
type Resources interface {
	// Capacities answers the capacities of the people named, or of everybody for none, at most max.
	Capacities(ctx context.Context, accounts []shared.ID, limit int) ([]domain.Capacity, error)
	// PutCapacity writes a person's capacity from its day on; id is used for a new one.
	PutCapacity(ctx context.Context, id shared.ID, c domain.Capacity, by shared.ID, at time.Time) error
	// RemoveCapacity removes the capacity a person has from a day, and reports whether one was.
	RemoveCapacity(ctx context.Context, account shared.ID, from domain.Day) (bool, error)

	// Absences answers the absences sharing a day with a range, of one person or everybody, at
	// most max.
	Absences(ctx context.Context, account shared.ID, from, to domain.Day, limit int) ([]domain.Absence, error)
	FindAbsence(ctx context.Context, id shared.ID) (domain.Absence, error)
	// PutAbsence writes an absence: a new row when expected is 0, else held to the version read.
	PutAbsence(ctx context.Context, a domain.Absence, expected int, by shared.ID, at time.Time) error
	// RemoveAbsence removes an absence held to the version read.
	RemoveAbsence(ctx context.Context, id shared.ID, expected int) error

	// Rates answers the workspace's cost rates, at most max.
	Rates(ctx context.Context, limit int) ([]domain.CostRate, error)
	FindRate(ctx context.Context, id shared.ID) (domain.CostRate, error)
	// PutRate writes a rate: a new row when expected is 0, else held to the version read.
	PutRate(ctx context.Context, r domain.CostRate, expected int, by shared.ID, at time.Time) error
	// RemoveRate removes a rate held to the version read.
	RemoveRate(ctx context.Context, id shared.ID, expected int) error

	// BillingRates answers the billing rates of the plans named, at most max.
	BillingRates(ctx context.Context, plans []shared.ID, limit int) ([]domain.BillingRate, error)
	FindBillingRate(ctx context.Context, id shared.ID) (domain.BillingRate, error)
	// PutBillingRate writes a billing rate: a new row when expected is 0, else held to the version read.
	PutBillingRate(ctx context.Context, r domain.BillingRate, expected int, by shared.ID, at time.Time) error
	// RemoveBillingRate removes a billing rate held to the version read.
	RemoveBillingRate(ctx context.Context, id shared.ID, expected int) error

	// Budget answers a plan's budget, and whether it has one.
	Budget(ctx context.Context, plan shared.ID) (domain.Budget, bool, error)
	// PutBudget writes a plan's budget: a new row when expected is 0, else held to the version read.
	PutBudget(ctx context.Context, id shared.ID, b domain.Budget, expected int, by shared.ID, at time.Time) error
	// RemoveBudget removes a plan's budget held to the version read.
	RemoveBudget(ctx context.Context, plan shared.ID, expected int) error
}
