// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package goal is the storage port of the goals and insight module (M8): periods, goals with
// their linked entries and check-ins, portfolios with their updates, and dashboards. Every method
// runs inside the caller's unit of work, which names the tenant (rule 3).
package goal

import (
	"context"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// GoalFilter narrows a list of goals; a zero field narrows nothing.
type GoalFilter struct {
	PeriodID shared.ID
	ParentID shared.ID
	OwnerID  shared.ID
	IDs      []shared.ID
	Max      int
}

// Store keeps the module's rows. A write held to a version answers a version conflict when the
// row moved on, and not found when it is gone.
type Store interface {
	Periods(ctx context.Context, max int) ([]domain.Period, error)
	Period(ctx context.Context, id shared.ID) (domain.Period, error)
	InsertPeriod(ctx context.Context, p domain.Period, by shared.ID, at time.Time) error
	UpdatePeriod(ctx context.Context, p domain.Period, expected int, at time.Time) error
	// ClosePeriod closes an open period and grades every goal it names, once.
	ClosePeriod(ctx context.Context, id shared.ID, expected int, grades map[shared.ID]float64, by shared.ID, at time.Time) error

	Goals(ctx context.Context, filter GoalFilter) ([]domain.Goal, error)
	Goal(ctx context.Context, id shared.ID) (domain.Goal, error)
	InsertGoal(ctx context.Context, g domain.Goal, by shared.ID, at time.Time) error
	UpdateGoal(ctx context.Context, g domain.Goal, expected int, by shared.ID, at time.Time) error
	// DeleteGoal removes a goal, its links and check-ins, and hands the goals under it to its parent.
	DeleteGoal(ctx context.Context, g domain.Goal, expected int) error

	// LinkedEntries answers the entries linked to each goal.
	LinkedEntries(ctx context.Context, goals []shared.ID) (map[shared.ID][]shared.ID, error)
	GoalsOfEntry(ctx context.Context, entry shared.ID, max int) ([]shared.ID, error)
	LinkEntries(ctx context.Context, goal shared.ID, entries []shared.ID, by shared.ID, at time.Time) error
	UnlinkEntry(ctx context.Context, goal, entry shared.ID) (bool, error)

	AddCheckIn(ctx context.Context, c domain.CheckIn) error
	CheckIns(ctx context.Context, goal shared.ID, max int) ([]domain.CheckIn, error)

	Portfolios(ctx context.Context, max int) ([]domain.Portfolio, error)
	Portfolio(ctx context.Context, id shared.ID) (domain.Portfolio, error)
	InsertPortfolio(ctx context.Context, p domain.Portfolio, by shared.ID, at time.Time) error
	// UpdatePortfolio writes the portfolio and replaces its projects.
	UpdatePortfolio(ctx context.Context, p domain.Portfolio, expected int, by shared.ID, at time.Time) error
	// DeletePortfolio removes a portfolio and its updates and hands its portfolios to its parent.
	DeletePortfolio(ctx context.Context, p domain.Portfolio, expected int) error
	AddStatusUpdate(ctx context.Context, u domain.StatusUpdate) error
	StatusUpdates(ctx context.Context, portfolio shared.ID, max int) ([]domain.StatusUpdate, error)

	Dashboards(ctx context.Context, max int) ([]domain.Dashboard, error)
	Dashboard(ctx context.Context, id shared.ID) (domain.Dashboard, error)
	InsertDashboard(ctx context.Context, d domain.Dashboard, at time.Time) error
	UpdateDashboard(ctx context.Context, d domain.Dashboard, expected int, by shared.ID, at time.Time) error
	DeleteDashboard(ctx context.Context, id shared.ID, expected int) error
}
