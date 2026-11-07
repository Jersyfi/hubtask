// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// Workflows stores a workspace's workflows and its scheme (UC-FLW-02 … 05). The shipped workflow
// has no row until a workspace first uses or changes it; the domain merges (work.NewWorkflows).
type Workflows interface {
	// List returns the workspace's rows.
	List(ctx context.Context) ([]work.Workflow, error)

	// Insert writes a new row; a key already taken comes back as `workflows.key_taken`.
	Insert(ctx context.Context, workflow work.Workflow) error

	// Update writes a row whole, or reports a version conflict.
	Update(ctx context.Context, workflow work.Workflow, expectedVersion int) error

	// Delete removes a workspace's own workflow no scheme names; one in use comes back as
	// `workflows.in_use`, a missing one as ErrNotFound.
	Delete(ctx context.Context, key string) error

	// Schemes returns every row of the scheme.
	Schemes(ctx context.Context) ([]work.WorkflowScheme, error)

	// SetScheme writes the workflow a container runs for a type, empty for every type.
	SetScheme(ctx context.Context, scheme work.WorkflowScheme, now time.Time) error

	// RemoveScheme removes a container's row for a type; a missing one is no error.
	RemoveScheme(ctx context.Context, containerID shared.ID, entryType string) error

	// StatusGroups counts the entries of the collections by type, status and completion.
	StatusGroups(ctx context.Context, collectionIDs []shared.ID) ([]work.StatusGroup, error)

	// MoveStatuses moves one group's entries and answers each moved entry's new version.
	MoveStatuses(ctx context.Context, move work.StatusMove, now time.Time) ([]MovedEntry, error)
}

// MovedEntry is an entry a publish moved, with the version the move gave it.
type MovedEntry struct {
	ID      shared.ID
	Version int
}
