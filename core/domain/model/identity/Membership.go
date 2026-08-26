// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"slices"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Role is what an account may do within a scope (domain-model.md §3.2, ADR-0005). The seven values
// are the ones the database constrains (db/schema.sql, membership_role).
type Role string

const (
	RoleOwner       Role = "OWNER"
	RoleAdmin       Role = "ADMIN"
	RoleMember      Role = "MEMBER"
	RoleContributor Role = "CONTRIBUTOR"
	RoleViewer      Role = "VIEWER"
	RoleGuest       Role = "GUEST"
	// RoleAuditor reads the evidence and none of the work: the audit trail, and no container, no
	// entry, no comment (audit.md §5). It exists because the alternative, in practice, is giving
	// an auditor administrator rights - a permissions problem that arises precisely where
	// evidence is being demanded.
	RoleAuditor Role = "AUDITOR"
)

// roles are ordered from the most to the least powerful. The order is the definition of "the
// highest role along the path" - the one rule the whole inheritance rests on - so it lives in one
// place rather than in a comparison written out wherever two roles meet.
//
// AUDITOR is last, and it is the one value the order does not really describe. It carries a
// permission no other role has and lacks the one every other role has, so there is no place in a
// line where it belongs. Last is the harmless place: "the highest role along the path" can then
// never let it displace a content role held beside it, and what actually decides a permission is
// the union over the memberships rather than the maximum (core/domain/service.Allows).
var roles = [...]Role{
	RoleOwner, RoleAdmin, RoleMember, RoleContributor, RoleViewer, RoleGuest, RoleAuditor,
}

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

// ItemScope names one entry, the bottom of the path.
//
// It is what sharing an entry with somebody is (domain-model.md §3.2, C-04): a membership granted
// here reaches that entry and nothing else, so "shared items only" needs no mechanism of its own -
// it is the ordinary resolution asked about a path that ends at the entry. The scope type has been
// in the model since 0.1.0; until entries were on anybody's path there was nothing to grant it on.
func ItemScope(id shared.ID) Scope { return Scope{Type: ScopeItem, ID: id} }

// Membership grants a role at a scope, to an account directly or through a group. Which of the
// two it was does not survive to here: the resolution asks what an account may do, and a right
// held through a group is not a lesser right (db/schema.sql, membership).
type Membership struct {
	AccountID shared.ID
	Scope     Scope
	Role      Role
}

// Grant is a membership as it is stored: a role at a scope, held by an account or by a group.
//
// Membership is what the resolution reads and deliberately drops the distinction - a right held
// through a group is not a lesser right. Grant is what an administrator writes, where the
// distinction is the whole point: revoking a person's own membership must not silently take away
// what their team gives them.
type Grant struct {
	ID       shared.ID
	TenantID shared.ID
	// Exactly one of AccountID and GroupID is set. Both or neither is a programming error, and
	// NewGrant refuses it rather than leaving the database to.
	AccountID shared.ID
	GroupID   shared.ID
	Scope     Scope
	Role      Role
}

// NewGrant checks what the database constrains, before the database has to.
func NewGrant(id, tenantID, accountID, groupID shared.ID, scope Scope, role Role) (Grant, error) {
	switch {
	case id.IsZero() || tenantID.IsZero():
		return Grant{}, shared.ErrInternal.WithDetail("memberships.identity_incomplete")
	case accountID.IsZero() == groupID.IsZero():
		// Neither is a grant that applies to nobody; both is a grant with two subjects and no
		// answer to whose it is when one of them is removed.
		return Grant{}, shared.ErrValidation.WithDetail("memberships.subject_ambiguous")
	case !role.Valid():
		return Grant{}, shared.ErrValidation.
			WithDetail("memberships.role_unknown").
			WithParams(map[string]string{"value": string(role)})
	case !scope.Valid():
		return Grant{}, shared.ErrValidation.
			WithDetail("memberships.scope_invalid").
			WithParams(map[string]string{"value": string(scope.Type)})
	}
	return Grant{
		ID: id, TenantID: tenantID, AccountID: accountID, GroupID: groupID,
		Scope: scope, Role: role,
	}, nil
}

// Valid reports whether the scope is one the model defines, and whether it carries the identifier
// that level needs. The tenant scope has none - there is one tenant per unit of work - and every
// other level names the container or item it applies to.
func (s Scope) Valid() bool {
	switch s.Type {
	case ScopeTenant:
		return s.ID.IsZero()
	case ScopeHub, ScopeCollection, ScopeItem:
		return !s.ID.IsZero()
	default:
		return false
	}
}
