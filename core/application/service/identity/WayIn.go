// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// WayInNotices tells a person that a way into their account changed (ADR-0078 §6, identity.md §6).
// It queues inside the caller's transaction, so the notice exists exactly when the change
// committed. The notification context implements it; nil where the installation has no mail
// server, and the change is then only audited.
type WayInNotices interface {
	WayInChanged(ctx context.Context, tenantID, accountID shared.ID, change domain.WayInChange) error
}
