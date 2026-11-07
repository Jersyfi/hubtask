// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package realtime is the storage port of live documents (F-8, offline-sync.md §13). Every method
// runs inside the caller's transaction, under the tenant it names (rule 3).
package realtime

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/realtime"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Documents stores each document's update log and its snapshots.
type Documents interface {
	// Append writes the update as the document's next one, creating the document on its first,
	// and answers the place it was given. The head is locked for the transaction, so two writers
	// of one document are ordered and never share a place.
	Append(ctx context.Context, ref realtime.Ref, data []byte, author shared.ID, at time.Time) (int64, error)

	// Head answers where the document's log stands; a document never written stands at zero.
	Head(ctx context.Context, ref realtime.Ref) (realtime.Head, error)

	// Snapshot answers the latest snapshot; found false when the document has none.
	Snapshot(ctx context.Context, ref realtime.Ref) (realtime.Snapshot, bool, error)

	// Since answers the updates after a place, oldest first, at most limit.
	Since(ctx context.Context, ref realtime.Ref, after int64, limit int) ([]realtime.Update, error)

	// StoreSnapshot writes a snapshot and marks the updates it covers as compacted at the moment,
	// and the snapshot it replaces as superseded. Neither is removed here: the history behind a
	// snapshot goes by retention (DOCUMENT_HISTORY), never by the write that made it redundant.
	// False is a snapshot at or past this one stored meanwhile.
	StoreSnapshot(ctx context.Context, ref realtime.Ref, snapshot realtime.Snapshot) (bool, error)

	// Erase removes the document whole: its log, its snapshots and its head. The owning kind
	// calls it in the transaction that purges what the document belongs to.
	Erase(ctx context.Context, ref realtime.Ref) error
}

// History is the retention sweep's view of the store: the compacted updates and superseded
// snapshots older than a cutoff, counted and removed in batches (lifecycle.ExpiringRows).
type History interface {
	CountExpired(ctx context.Context, cutoff time.Time, ceiling int) (int, error)
	DeleteExpired(ctx context.Context, cutoff time.Time, batch int) (int, error)
}
