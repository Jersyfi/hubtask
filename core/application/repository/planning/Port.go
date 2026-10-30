// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package planning is the storage the planning module's use cases need (M2): a plan's entries as
// the scheduler reads them, a dependency's timing, an entry's scheduling mode, and a plan's
// baselines. Every method runs inside the caller's unit of work, under its tenant (rule 3).
package planning

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// PlanEntry is one entry of a plan: its dates, its place in the tree, and its mode.
type PlanEntry struct {
	ID           shared.ID
	ParentID     shared.ID
	CollectionID shared.ID
	HubID        shared.ID
	AssigneeID   shared.ID
	Title        string
	StartAt      *time.Time
	DueAt        *time.Time
	DueDateOnly  bool
	DueTimeZone  string
	Done         bool
	Version      int
	Mode         planning.Mode
	Milestone    bool
}

// EntrySchedule is an entry's mode and milestone mark; no row is MANUAL and no milestone.
type EntrySchedule struct {
	EntryID   shared.ID
	Mode      planning.Mode
	Milestone bool
	UpdatedAt time.Time
	Version   int
}

// Plans reads and writes what scheduling a plan needs.
type Plans interface {
	// Entries returns the plan's entries - a collection's, or a hub's collections' - that are
	// neither trashed nor archived, at most max.
	Entries(ctx context.Context, planID shared.ID, max int) ([]PlanEntry, error)
	// Schedules returns the stored modes of the entries among the identifiers.
	Schedules(ctx context.Context, entryIDs []shared.ID) (map[shared.ID]EntrySchedule, error)
	// SetSchedule writes an entry's mode and milestone mark.
	SetSchedule(ctx context.Context, schedule EntrySchedule) error
	// SetTiming writes a link's timing, or reports ErrNotFound.
	SetTiming(ctx context.Context, linkID shared.ID, timing planning.Timing) error
}

// Baselines stores a plan's baselines (PLN-04).
type Baselines interface {
	// Count is how many baselines a plan keeps.
	Count(ctx context.Context, planID shared.ID) (int, error)
	// Take writes the baseline and copies the plan's entries' dates into it at this moment, at
	// most max; it answers how many it copied.
	Take(ctx context.Context, baseline planning.Baseline, max int) (int, error)
	// List returns a plan's baselines without their entries, the newest first.
	List(ctx context.Context, planID shared.ID) ([]planning.Baseline, error)
	// Find returns a baseline with its entries, or ErrNotFound.
	Find(ctx context.Context, id shared.ID) (planning.Baseline, error)
	// Delete removes a baseline, or reports ErrNotFound.
	Delete(ctx context.Context, id shared.ID) error
}
