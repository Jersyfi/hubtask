// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"slices"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Role is what an account may do within a scope (domain-model.md §3.2, ADR-0005). The six values
// are the ones the database constrains (db/schema.sql, membership_role).
type Role string

const (
	RoleOwner       Role = "OWNER"
	RoleAdmin       Role = "ADMIN"
	RoleMember      Role = "MEMBER"
	RoleContributor Role = "CONTRIBUTOR"
	RoleViewer      Role = "VIEWER"
	RoleGuest       Role = "GUEST"
)

// roles are ordered from the most to the least powerful. The order is the definition of "the
// highest role along the path" - the one rule the whole inheritance rests on - so it lives in one
// place rather than in a comparison written out wherever two roles meet.
var roles = [...]Role{RoleOwner, RoleAdmin, RoleMember, RoleContributor, RoleViewer, RoleGuest}

// Roles returns every defined role, strongest first.
func Roles() []Role { return roles[:] }

// Valid reports whether the role is one of the defined ones.
func (r Role) Valid() bool { return slices.Contains(roles[:], r) }

// AtLeast reports whether this role is as powerful as the other one.
func (r Role) AtLeast(other Role) bool {
	mine, theirs := slices.Index(roles[:], r), slices.Index(roles[:], other)
	return mine >= 0 && theirs >= 0 && mine <= theirs
}

// ScopeType is the level a membership is granted at. The permission then applies downwards:
// tenant → hub → collection → item (domain-model.md §3.2).
type ScopeType string

const (
	ScopeTenant     ScopeType = "TENANT"
	ScopeHub        ScopeType = "HUB"
	ScopeCollection ScopeType = "COLLECTION"
	ScopeItem       ScopeType = "ITEM"
)

// Scope is one point on that path. The tenant scope carries no identifier: there is exactly one
// tenant per unit of work, and the database says the same with a NULL scope_id.
type Scope struct {
	Type ScopeType
	ID   shared.ID
}

// TenantScope is the top of every path.
func TenantScope() Scope { return Scope{Type: ScopeTenant} }

// HubScope and CollectionScope name the two container levels.
func HubScope(id shared.ID) Scope        { return Scope{Type: ScopeHub, ID: id} }
func CollectionScope(id shared.ID) Scope { return Scope{Type: ScopeCollection, ID: id} }

// Membership grants a role at a scope, to an account directly or through a group. Which of the
// two it was does not survive to here: the resolution asks what an account may do, and a right
// held through a group is not a lesser right (db/schema.sql, membership).
type Membership struct {
	AccountID shared.ID
	Scope     Scope
	Role      Role
}
