// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package service

import (
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
)

// Permission is one column of the role matrix in domain-model.md §3.2.
//
// The matrix has qualifiers the columns cannot express: a contributor writes only what is
// assigned to them, a member manages only their own automation rules, a guest sees only what was
// shared with them. Those are narrowings the individual use case applies on top of the
// permission, and each is named where it is enforced. They are deliberately not folded in here -
// a permission that sometimes means "all items" and sometimes "one item" is a permission nobody
// can reason about.
type Permission string

const (
	// PermissionRead is seeing containers and items at all.
	PermissionRead Permission = "READ"
	// PermissionWriteItems is creating and changing items.
	PermissionWriteItems Permission = "WRITE_ITEMS"
	// PermissionStructure is changing the shape of the workspace: hubs, collections, buckets,
	// labels, custom fields. Creating a container is this one.
	PermissionStructure Permission = "STRUCTURE"
	// PermissionManageMembers is granting and revoking access.
	PermissionManageMembers Permission = "MANAGE_MEMBERS"
	// PermissionAutomation is creating and running automation rules.
	PermissionAutomation Permission = "AUTOMATION"
	// PermissionDeleteContainer is deleting a hub or a collection - the one thing an
	// administrator cannot do, because it takes a subtree with it.
	PermissionDeleteContainer Permission = "DELETE_CONTAINER"
)

// rolePermissions is the matrix of domain-model.md §3.2, written out rather than derived from the
// role order. Derivation would be shorter and wrong: an administrator ranks above a member and
// still may not delete a container, which is the point of the row.
var rolePermissions = map[identity.Role][]Permission{
	identity.RoleOwner: {
		PermissionRead, PermissionWriteItems, PermissionStructure,
		PermissionManageMembers, PermissionAutomation, PermissionDeleteContainer,
	},
	identity.RoleAdmin: {
		PermissionRead, PermissionWriteItems, PermissionStructure,
		PermissionManageMembers, PermissionAutomation,
	},
	identity.RoleMember:      {PermissionRead, PermissionWriteItems, PermissionAutomation},
	identity.RoleContributor: {PermissionRead, PermissionWriteItems},
	identity.RoleViewer:      {PermissionRead},
	identity.RoleGuest:       {PermissionRead},
}

// RoleAllows reports whether the role carries the permission.
func RoleAllows(role identity.Role, permission Permission) bool {
	for _, granted := range rolePermissions[role] {
		if granted == permission {
			return true
		}
	}
	return false
}

// EffectiveRole is the highest role the account holds anywhere along the path, and reports
// whether it holds one at all.
//
// The path runs from the tenant downwards: [tenant, hub, collection]. Inheritance is downwards
// only - a role on one collection says nothing about its neighbour - and the highest wins, so a
// tenant administrator does not lose anything by also being a viewer somewhere.
//
// Memberships that name a scope outside the path are ignored rather than refused: the caller
// passes what it read, and filtering here means the query may be generous without the answer
// being wrong.
func EffectiveRole(memberships []identity.Membership, path []identity.Scope) (identity.Role, bool) {
	var best identity.Role
	found := false

	for _, membership := range memberships {
		if !membership.Role.Valid() || !onPath(membership.Scope, path) {
			continue
		}
		if !found || membership.Role.AtLeast(best) {
			best, found = membership.Role, true
		}
	}
	return best, found
}

func onPath(scope identity.Scope, path []identity.Scope) bool {
	for _, step := range path {
		if step.Type == scope.Type && step.ID == scope.ID {
			return true
		}
	}
	return false
}

// Allows is the whole decision in one call: does what this account holds along this path carry
// this permission?
func Allows(memberships []identity.Membership, path []identity.Scope, permission Permission) bool {
	role, found := EffectiveRole(memberships, path)
	return found && RoleAllows(role, permission)
}
