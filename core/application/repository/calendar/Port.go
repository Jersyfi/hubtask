// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package calendar is the storage port of durable timers (domain-model.md §3.7). Every method
// runs inside the caller's transaction, under the tenant it names (rule 3).
package calendar

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Timers stores durable timers.
type Timers interface {
	// Insert writes a timer that was just armed; a live timer of the same purpose, subject and
	// name already there is a conflict (`timer.already_running`).
	Insert(ctx context.Context, timer calendar.Timer, now time.Time) error

	// FindLive answers the running or paused timer of one purpose, subject and name, locked for
	// the transaction; shared.ErrNotFound when there is none.
	FindLive(ctx context.Context, purpose calendar.Purpose, subject shared.ID, name string) (calendar.Timer, error)

	// Update writes the timer's new state where its version is still expectedVersion.
	Update(ctx context.Context, timer calendar.Timer, expectedVersion int, now time.Time) error

	// ClaimDue takes the tenant's running timers whose due moment has come, oldest first and at
	// most limit, locking them so that overlapping passes take disjoint sets.
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]calendar.Timer, error)

	// NextDue answers the tenant's next due moment; nil when no timer runs.
	NextDue(ctx context.Context) (*time.Time, error)
}
