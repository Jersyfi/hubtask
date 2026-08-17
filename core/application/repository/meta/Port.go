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
}
