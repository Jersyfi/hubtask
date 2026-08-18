// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package meta declares what the self-description needs from storage.
package meta

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// CapabilityProfiles reads the profile that applies per item type: a tenant's own override where
// it has one, the system default otherwise.
//
// The choice is made in the query rather than here, so there is no second place where "which one
// wins" could be decided differently (db/queries/Meta.sql).
type CapabilityProfiles interface {
	// List returns one profile per item type, in a stable order.
	//
	// It runs inside the caller's transaction, which decides what it sees: a tenant's scope shows
	// the system defaults plus that tenant's overrides, an installation scope shows the defaults
	// alone. Neither can see another tenant's overrides - that is row level security, not a
	// condition in the query (ADR-0010).
	List(ctx context.Context) ([]work.CapabilityProfile, error)

	// ListSystem returns the system defaults, ignoring any tenant override.
	//
	// They are what bounds a narrowing: a tenant may take capabilities, children and depth away,
	// never add them (domain-model.md §2). The distinction matters for one question in
	// particular - which types sit directly under a collection. That is derived from which types
	// nothing accepts as a child, and read off a narrowed set it gives the wrong answer: a tenant
	// that removes a task's children would thereby promote the work package to a top level it was
	// never allowed to sit at, which is a widening produced by a narrowing.
	ListSystem(ctx context.Context) ([]work.CapabilityProfile, error)
}
