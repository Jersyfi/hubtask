// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// CustomRole is a role a workspace defines itself from the permission catalogue (UC-ID-26,
// domain-model.md §3.2): a stable key, a name its people chose, and the permissions it carries.
// The seven built-in roles stay the shipped scheme beside it.
//
// The permissions are names from the catalogue (core/domain/service.PermissionCatalogue); this
// package does not know the catalogue, so a role is held to it where it is defined
// (service.DefineRole), not here.
type CustomRole struct {
	ID       shared.ID
	TenantID shared.ID
	// Key is the stable name the configuration, a workflow step and a form's `actor.roles` use.
	// Lower case, so it can never be mistaken for one of the seven.
	Key string
	// Name is user content (rule 10).
	Name        string
	Permissions []string
}

// MaxCustomRoleNameLength bounds a role's name, as a label's.
const MaxCustomRoleNameLength = 120

var customRoleKey = regexp.MustCompile(`^[a-z][a-z0-9_]{1,62}$`)

// ValidCustomRoleKey reports whether a key may name a custom role.
func ValidCustomRoleKey(key string) bool { return customRoleKey.MatchString(key) }

// NewCustomRole checks the shape of a role; the permissions are checked by the caller against the
// catalogue, and arrive here sorted and without duplicates from it.
func NewCustomRole(id, tenantID shared.ID, key, name string, permissions []string) (CustomRole, error) {
	name = strings.TrimSpace(name)
	switch {
	case id.IsZero() || tenantID.IsZero():
		return CustomRole{}, shared.ErrInternal.WithDetail("roles.identity_incomplete")
	case !ValidCustomRoleKey(key):
		return CustomRole{}, shared.ErrValidation.WithDetail("roles.key_invalid").
			WithParams(map[string]string{"key": key})
	case name == "" || utf8.RuneCountInString(name) > MaxCustomRoleNameLength || strings.ContainsAny(name, "\r\n"):
		return CustomRole{}, shared.ErrValidation.WithDetail("roles.name_invalid")
	}
	held := slices.Clone(permissions)
	slices.Sort(held)
	return CustomRole{
		ID: id, TenantID: tenantID, Key: key, Name: name, Permissions: slices.Compact(held),
	}, nil
}

// CustomGrant is a custom role at a scope, held by an account or by a group: Grant's counterpart
// for a role the workspace defined.
type CustomGrant struct {
	ID        shared.ID
	TenantID  shared.ID
	AccountID shared.ID
	GroupID   shared.ID
	Scope     Scope
	RoleID    shared.ID
}

// NewCustomGrant checks what the database constrains, before the database has to.
func NewCustomGrant(id, tenantID, accountID, groupID shared.ID, scope Scope, roleID shared.ID) (CustomGrant, error) {
	switch {
	case id.IsZero() || tenantID.IsZero() || roleID.IsZero():
		return CustomGrant{}, shared.ErrInternal.WithDetail("memberships.identity_incomplete")
	case accountID.IsZero() == groupID.IsZero():
		return CustomGrant{}, shared.ErrValidation.WithDetail("memberships.subject_ambiguous")
	case !scope.Valid() || scope.Type == ScopeItem:
		// A custom role is a role over places; sharing one entry stays a built-in role's.
		return CustomGrant{}, shared.ErrValidation.
			WithDetail("memberships.scope_invalid").
			WithParams(map[string]string{"value": string(scope.Type)})
	}
	return CustomGrant{
		ID: id, TenantID: tenantID, AccountID: accountID, GroupID: groupID, Scope: scope, RoleID: roleID,
	}, nil
}
