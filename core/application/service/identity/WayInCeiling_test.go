// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// heldRoles is what each account holds anywhere in the workspace, as the ceiling reads it.
type heldRoles map[shared.ID][]domain.Role

func (h heldRoles) HeldAnywhere(_ context.Context, accountID shared.ID) ([]domain.Role, error) {
	return h[accountID], nil
}

var _ repository.HeldRoles = heldRoles(nil)

// ceilingCase is one row of the matrix every door is tested against (identity.md §13): the actor's
// own role on the workspace, what the target holds, and whether the new way in is issued.
type ceilingCase struct {
	name   string
	own    domain.Role
	held   []domain.Role
	issued bool
}

var ceilingCases = []ceilingCase{
	{"an administrator, for an owner", domain.RoleAdmin, []domain.Role{domain.RoleOwner}, false},
	{"an administrator, for an owner of one hub who is a member elsewhere", domain.RoleAdmin,
		[]domain.Role{domain.RoleMember, domain.RoleOwner}, false},
	{"an administrator, for a member", domain.RoleAdmin, []domain.Role{domain.RoleMember}, true},
	{"an administrator, for another administrator", domain.RoleAdmin, []domain.Role{domain.RoleAdmin}, true},
	{"an administrator, for an auditor", domain.RoleAdmin, []domain.Role{domain.RoleAuditor}, true},
	{"an administrator, for an account holding nothing", domain.RoleAdmin, nil, true},
	{"an owner, for an owner", domain.RoleOwner, []domain.Role{domain.RoleOwner}, true},
	{"an owner, for a member", domain.RoleOwner, []domain.Role{domain.RoleMember}, true},
}
