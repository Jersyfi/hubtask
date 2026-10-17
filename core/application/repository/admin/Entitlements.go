// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/entitlement"
)

// Entitlements reads what a workspace's plan leaves out (multi-tenancy.md §4.2).
//
// The workspace is the one the transaction is bound to, so the reader joins whatever transaction
// its caller already runs in: the engine's, the dispatcher's, a use case's. A workspace on no plan
// reads an empty set - every feature included.
type Entitlements interface {
	LeftOut(ctx context.Context) (entitlement.LeftOut, error)
}
