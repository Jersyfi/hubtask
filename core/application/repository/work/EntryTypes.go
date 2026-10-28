// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// EntryTypes stores a workspace's type registry (UC-FLW-01): its own types and its rows for the
// system types it changed. The system types it never changed have no row; the domain merges
// (work.NewEntryTypes).
type EntryTypes interface {
	// List returns the workspace's rows, archived ones included.
	List(ctx context.Context) ([]work.EntryType, error)

	// Find returns the row of a key, or ErrNotFound when the workspace holds none.
	Find(ctx context.Context, key string) (work.EntryType, error)

	// Insert writes a new row. A key already taken comes back as `entry_types.key_taken`, from the
	// unique index.
	Insert(ctx context.Context, entryType work.EntryType) error

	// Update writes a row whole, or reports a version conflict.
	Update(ctx context.Context, entryType work.EntryType, expectedVersion int) error

	// Delete removes a workspace's own type that no entry carries, trashed ones included; one in
	// use comes back as `entry_types.in_use`, a missing one as ErrNotFound.
	Delete(ctx context.Context, key string) error

	// InUse reports whether any entry carries the type.
	InUse(ctx context.Context, key string) (bool, error)

	// SetOf writes an entry's type - empty for its level's system type - or reports a version
	// conflict.
	SetOf(ctx context.Context, item work.WorkItem, expectedVersion int) error
}
