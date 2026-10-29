// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// CustomRoleRow is a custom role as stored, with its version for the optimistic write.
type CustomRoleRow struct {
	Role      identity.CustomRole
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int
}

// CustomGrantRow is a custom role held at a place, with the role's key.
type CustomGrantRow struct {
	Grant   identity.CustomGrant
	RoleKey string
}

// CustomRoles stores the workspace's own roles and where they are held (UC-ID-26). The tenant
// boundary is the transaction's (ADR-0010); what an account holds of them reaches the decision
// through Memberships.Along.
type CustomRoles interface {
	// List answers every custom role, by key.
	List(ctx context.Context) ([]CustomRoleRow, error)
	// Find answers one by key, or an error wrapping shared.ErrNotFound.
	Find(ctx context.Context, key string) (CustomRoleRow, error)
	Insert(ctx context.Context, role identity.CustomRole, at time.Time) error
	// Update writes the name and permissions held to the version read; false is a version moved on.
	Update(ctx context.Context, role identity.CustomRole, version int, at time.Time) (bool, error)
	// Delete removes the role and every grant of it; false is nothing there.
	Delete(ctx context.Context, roleID shared.ID) (bool, error)
	// Grant records the grant; created false is the subject holding the role there already.
	Grant(ctx context.Context, grant identity.CustomGrant, at time.Time) (created bool, err error)
	// FindGrant answers one grant, or an error wrapping shared.ErrNotFound.
	FindGrant(ctx context.Context, grantID shared.ID) (CustomGrantRow, error)
	// Revoke removes one grant; false is nothing there.
	Revoke(ctx context.Context, grantID shared.ID) (bool, error)
	// Grants answers every grant in the workspace, at most limit of them.
	Grants(ctx context.Context, limit int) ([]CustomGrantRow, error)
}
