// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package work declares what the work management use cases need from storage.
//
// Every method runs inside a unit of work, and what it can see is decided by the tenant that unit
// was opened with - never by a condition in the query (ADR-0010). A repository here therefore
// never takes a tenant parameter: a parameter can be forgotten, and row level security cannot.
package work

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// Containers stores hubs and collections.
type Containers interface {
	// Find returns the container, or ErrNotFound if it does not exist *for this tenant*. The two
	// cases are deliberately one answer: telling a caller that an identifier exists elsewhere
	// would confirm the existence of another tenant's data (multi-tenancy.md §2).
	Find(ctx context.Context, id shared.ID) (work.Container, error)

	// LastOrderKey returns the highest rank among the containers directly under parentID, or the
	// empty string when there are none. An empty parentID means the hubs, which sit under nothing.
	//
	// Trashed containers count. Their rank is still occupied - a restore has to land where it
	// was, and reusing the key would put two containers in the same place.
	LastOrderKey(ctx context.Context, parentID shared.ID) (string, error)

	// Insert writes a new container.
	//
	// A name that is already taken at this level comes back as a conflict with the detail code
	// `containers.name_taken`, translated from the unique index rather than checked beforehand: a
	// check followed by an insert is two statements with a gap between them, and two requests
	// arriving in that gap both pass the check (multi-tenancy.md §2.1).
	Insert(ctx context.Context, container work.Container) error
}
