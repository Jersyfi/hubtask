// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package lifecycle declares what the end of data's life needs stored: the instructions not to
// delete, and the record of what was deleted anyway.
package lifecycle

import (
	"context"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
)

// LegalHolds reads the instructions that override every retention decision (data-retention.md §4.1).
type LegalHolds interface {
	// Active returns the holds in force for this tenant, released ones left out.
	//
	// The whole set rather than a question per target. A purge run judges a batch of a thousand
	// rows, and a query per row would be a thousand round trips for an answer that is the same
	// every time - the holds of a tenant are few and change rarely, and deciding against them in
	// the domain is what keeps the rule readable (lifecycle.Holds.Blocking).
	Active(ctx context.Context) (domain.Holds, error)
}

// Removals records what a hard delete leaves behind.
//
// One method writing both records rather than two that a caller has to remember to pair. They are
// not independently useful: a journal entry without a tombstone lets a device that was offline
// recreate the row on its next push, and a tombstone without a journal entry lets a restore from
// backup bring it back. Either on its own is the orphan the completeness rule forbids (ADR-0020 §6).
type Removals interface {
	// Record writes a journal entry and a tombstone for each removal, inside the caller's
	// transaction - so what was removed and the record of its removal commit together.
	//
	// Both are idempotent on the row's identity: a run that dies after writing them and is picked
	// up again writes the same records rather than failing, which is what makes the retention job
	// safe to retry at all (ADR-0008, at-least-once).
	//
	// The removals may span both tables; the adapter groups them. Grouping here rather than at every
	// call site is what keeps a purge of a hub - containers and entries in one act - a single call.
	Record(ctx context.Context, removals []domain.Removal, deletedAt, purgeAfter time.Time) error
}
