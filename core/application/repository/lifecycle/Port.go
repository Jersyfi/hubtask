// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package lifecycle declares what the end of data's life needs stored: the instructions not to
// delete, and the record of what was deleted anyway.
package lifecycle

import (
	"context"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
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

// ExpiredItem is one entry whose time in the trash is up, with what deciding its removal needs.
//
// A projection rather than the aggregate: a purge does not read an entry in order to change it, and
// loading the whole of one - the notes, the completion, the labels - for a row that is about to stop
// existing would be work done to throw away.
type ExpiredItem struct {
	ID   shared.ID
	Type work.ItemType
	// Path is the chain of entries above it, which is what a legal hold placed higher up is judged
	// against, and what a purge event carries so a consumer can clean up below it (I-W2).
	Path         string
	CollectionID shared.ID
	// HubID is the hub of the collection. A hold placed on a hub has to reach an entry three levels
	// below it, and the entry alone does not know which hub it is under.
	HubID     shared.ID
	DeletedAt time.Time
}

// ExpiredContainer is one hub or collection whose time in the trash is up.
type ExpiredContainer struct {
	ID   shared.ID
	Type work.ContainerType
	// ParentID is the hub a collection sits in, and empty for a hub.
	ParentID  shared.ID
	DeletedAt time.Time
}

// Expired reads what may now be removed for good.
//
// Its own interface rather than methods on the work repositories, because the question is the
// lifecycle context's: "what is past its period" is not something a board or a list ever asks, and a
// port that mixed them would let a read path reach a statement written for a deletion.
type Expired interface {
	// Items returns entries deleted before the cutoff, deepest first, at most batchSize of them.
	//
	// Deepest first because a purge works a subtree from the bottom up (data-retention.md §4.6): a
	// parent removed before its children would take them through the foreign key, and rows removed
	// by a cascade nobody counted leave no journal entry and no tombstone behind.
	//
	// The cutoff is an instant rather than a period, so that one run reads the clock once - two
	// readings would let a long batch use two different definitions of "expired".
	Items(ctx context.Context, cutoff time.Time, batchSize int) ([]ExpiredItem, error)

	// Containers returns hubs and collections deleted before the cutoff, collections before the hubs
	// that hold them - which is the order `container.parent_id` being ON DELETE RESTRICT insists on.
	Containers(ctx context.Context, cutoff time.Time, batchSize int) ([]ExpiredContainer, error)
}
