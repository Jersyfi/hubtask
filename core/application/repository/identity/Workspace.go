// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
)

// Workspaces is the tenant's own row, read and written from inside the tenant (F4-01).
//
// Separate from the admin repository on purpose. That one enumerates workspaces and is the
// control plane's; this one never names a tenant, because row level security has already
// bound the transaction to exactly one - the same discipline `TenantPolicy` follows for the
// single switch it reads.
type Workspaces interface {
	// Find answers the workspace the transaction is bound to, or an error wrapping
	// shared.ErrNotFound where there is no row in scope.
	Find(ctx context.Context) (identity.Workspace, error)

	// Update writes the three columns and the settings keys this build models, leaving every
	// other key of the settings document where it is. It is guarded on the row version: two
	// administrators changing one workspace see each other rather than the last write winning.
	//
	// False means the guard did not hold - the row moved, or it is gone.
	Update(ctx context.Context, changed identity.Workspace, expectedVersion int, now time.Time) (bool, error)
}
