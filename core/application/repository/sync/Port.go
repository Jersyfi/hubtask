// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package sync declares what offline clients need recorded about a change.
//
// The change log is deliberately not the event outbox (offline-sync.md §10). The outbox carries
// business integration events outwards - CloudEvents, versioned, a public contract - while the
// change log carries state deltas to clients. Different recipients, different retention,
// different compatibility commitments; mixing them would damage both.
package sync

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Operation is what happened to an entity from a synchronising client's point of view.
type Operation string

const (
	// Upsert covers creation and change alike: a client that has the entity applies the payload,
	// one that does not adds it. Two operations would make the client decide which case it is in,
	// and it is exactly the client that cannot know.
	Upsert Operation = "UPSERT"
	Delete Operation = "DELETE"
	// AccessRevoked tells a device to delete its local copy because the actor may no longer see
	// the entity. Distinct from a deletion: the entity still exists, just not for this client
	// (offline-sync.md §6).
	AccessRevoked Operation = "ACCESS_REVOKED"
)

// Change is one entry of the log. The sequence number is assigned by the database, because it has
// to be gapless per tenant and monotonic - it is the cursor `:pull` pages on, and a value chosen
// in the application would leave holes whenever a transaction rolled back.
type Change struct {
	TenantID shared.ID
	Entity   string
	EntityID shared.ID
	Op       Operation
	// ContainerID is the visibility filter a pull applies: a device that subscribes to one hub
	// gets the changes below it and nothing else. For a container it is the container itself.
	ContainerID shared.ID
	ActorID     shared.ID
	// DeviceID is the device that caused the change, so that the device which made it can skip
	// its own echo. Empty for a change made through the API rather than through a sync push.
	DeviceID shared.ID
	// HLC orders the change against concurrent ones on other devices (offline-sync.md §4.1).
	HLC shared.HLC
	// Payload is the changed fields. Nil on a deletion - there is nothing left to describe, and a
	// tombstone carries no content by design.
	Payload map[string]any
}

// Recorded is one entry as it comes back out: the change, and the position it holds.
//
// The position is not on Change, because a writer does not have one - `seq` is the database's, and
// it is assigned when the row lands. A type that carried it on the way in would be a type with a
// field every writer has to leave empty and hope nobody reads.
type Recorded struct {
	Change
	// Seq is the position in the log, and the cursor a reader resumes from. Monotonic per tenant
	// and sparse: the identity is table-wide, so a gap between two of one tenant's entries is
	// somebody else's entry rather than one of theirs that went missing.
	Seq int64
	// OccurredAt is when the change was recorded. Not the cursor - ADR-0021 is explicit that a
	// timestamp is never one - but what a client shows and what an operator reads.
	OccurredAt time.Time
}

// ChangeLog records what a client has to be told about.
type ChangeLog interface {
	// Record writes one change inside the caller's transaction. A change that reached the tables
	// but not the log would be invisible to every offline client until something else touched the
	// same row - which is a data loss that looks like a caching bug.
	Record(ctx context.Context, change Change) error
}

// Changes reads the log back. The half `:pull` and the stream share: the same records, the same
// order, the same cursor - which is what makes the stream an accelerator rather than a second
// source of truth (ADR-0021).
type Changes interface {
	// After returns up to batch entries past the cursor, oldest first. A short page is the end of
	// the log rather than the end of a page: there is no `has_more` to compute, because the cursor
	// of the last entry is the answer to "what next".
	After(ctx context.Context, after int64, batch int) ([]Recorded, error)

	// Latest is where the log stands now, and where a client with no cursor starts. Zero for a
	// workspace nothing has happened in.
	Latest(ctx context.Context) (int64, error)
}
