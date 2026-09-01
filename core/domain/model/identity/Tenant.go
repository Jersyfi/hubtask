// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import "github.com/Jersyfi/hubtask/core/domain/model/shared"

// TenantStatus is the workspace's standing in the lifecycle multi-tenancy.md §5 draws. It rides
// with every credential read (H-06): a suspension has to flip authentication itself, not each
// use case separately.
type TenantStatus string

const (
	TenantActive          TenantStatus = "ACTIVE"
	TenantSuspended       TenantStatus = "SUSPENDED"
	TenantPendingDeletion TenantStatus = "PENDING_DELETION"
)

// Verify decides whether requests of this workspace may proceed. Forbidden rather than
// unauthenticated: the credential is real and verified, the workspace is what stands still -
// and the holder is entitled to be told which (multi-tenancy.md §5, "the API responds
// 403 tenant_suspended").
func (s TenantStatus) Verify() error {
	switch s {
	case TenantActive:
		return nil
	case TenantSuspended:
		return shared.ErrForbidden.WithDetail("access.tenant_suspended")
	case TenantPendingDeletion:
		return shared.ErrForbidden.WithDetail("access.tenant_pending_deletion")
	default:
		// Fail closed: a standing this build does not know is not one it may wave through.
		return shared.ErrForbidden.WithDetail("access.tenant_suspended")
	}
}
