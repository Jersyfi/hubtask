// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"slices"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ListRolesName       = "ListRoles"
	CreateRoleName      = "CreateRole"
	ChangeRoleName      = "ChangeRole"
	DeleteRoleName      = "DeleteRole"
	GrantRoleName       = "GrantRole"
	RevokeRoleGrantName = "RevokeRoleGrant"
	ExplainAccessName   = "ExplainAccess"
	roleTarget          = "custom_role"

	RoleCreatedAction audit.Action = "role.created"
	RoleChangedAction audit.Action = "role.changed"
	RoleDeletedAction audit.Action = "role.deleted"
	// RoleReadAction names the reads in a refusal's entry.
	RoleReadAction audit.Action = "role.read"

	// maxRoleGrants bounds the grants one listing answers; a workspace holds far fewer.
	maxRoleGrants = 2000
)

// Roles is what the role use cases share: the store, the decision point, the trail.
type Roles struct {
	Store       repository.CustomRoles
	Authorizer  Authorizer
	Held        HeldReader
	Revocations Revoker
	Audit       audit.Sink
	UnitOfWork  persistence.UnitOfWork
	Clock       clock.Clock
	IDs         clock.IDGenerator
}

// HeldReader answers what an account holds along a path: the authorisation service's HeldBy.
type HeldReader interface {
	HeldBy(ctx context.Context, actor appshared.ActorContext, accountID shared.ID, path []domain.Scope) (access.Held, error)
}

// ShippedRole is one of the seven, with what it carries.
type ShippedRole struct {
	Role        domain.Role
	Permissions []service.Permission
}

// RoleCatalogue is what the *Roles* screen and *who may do what* read (A11): the catalogue, the
// shipped scheme, the workspace's own roles and where they are held.
type RoleCatalogue struct {
	Catalogue []service.CatalogueEntry
	Shipped   []ShippedRole
	Custom    []repository.CustomRoleRow
	Grants    []repository.CustomGrantRow
}

// ListRoles answers the catalogue, the shipped scheme and the workspace's own roles.
type ListRoles struct{ Roles Roles }

func (h ListRoles) Execute(ctx context.Context, actor appshared.ActorContext) (RoleCatalogue, error) {
	if err := h.Roles.Authorizer.Authorize(ctx, actor, access.Request{
		Permission:  service.PermissionManageMembers,
		Alternative: service.PermissionReadConfiguration,
		Path:        []domain.Scope{domain.TenantScope()},
		Action:      RoleReadAction,
		TokenScope:  membersRead,
		TargetType:  roleTarget,
	}); err != nil {
		return RoleCatalogue{}, err
	}
	out := RoleCatalogue{Catalogue: service.PermissionCatalogue()}
	for _, role := range domain.Roles() {
		out.Shipped = append(out.Shipped, ShippedRole{Role: role, Permissions: service.PermissionsOf(role)})
	}
	err := h.Roles.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if out.Custom, err = h.Roles.Store.List(ctx); err != nil {
			return err
		}
		out.Grants, err = h.Roles.Store.Grants(ctx, maxRoleGrants)
		return err
	})
	return out, err
}

func (h ListRoles) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListRolesName,
		Summary: "Lists the permission catalogue, the seven shipped roles with what each carries, the " +
			"workspace's own roles, and where its own roles are held. What the roles screen and the " +
			"who-may-do-what matrix are drawn from.",
		SideEffects: "None. Reads only.",
		TokenScope:  membersRead,
		ReadOnly:    true,
		Permission:  service.PermissionManageMembers,
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			catalogue, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			return RoleCatalogueOutput(catalogue), nil
		}),
	}
}

// RoleCatalogueOutput is the answer every door shares.
func RoleCatalogueOutput(c RoleCatalogue) usecase.Output {
	catalogue := make([]map[string]any, 0, len(c.Catalogue))
	for _, entry := range c.Catalogue {
		catalogue = append(catalogue, map[string]any{
			"permission": string(entry.Permission), "kind": string(entry.Kind), "group": entry.Group,
			"shipped": roleStrings(entry.Shipped),
		})
	}
	shipped := make([]map[string]any, 0, len(c.Shipped))
	for _, role := range c.Shipped {
		shipped = append(shipped, map[string]any{"role": string(role.Role), "permissions": permissionStrings(role.Permissions)})
	}
	custom := make([]map[string]any, 0, len(c.Custom))
	for _, row := range c.Custom {
		custom = append(custom, CustomRoleOutput(row))
	}
	grants := make([]map[string]any, 0, len(c.Grants))
	for _, row := range c.Grants {
		grants = append(grants, customGrantOutput(row))
	}
	return usecase.Output{"catalogue": catalogue, "shipped": shipped, "custom": custom, "grants": grants}
}

// CustomRoleOutput is one custom role as every door answers it.
func CustomRoleOutput(row repository.CustomRoleRow) map[string]any {
	return map[string]any{
		"id": row.Role.ID.String(), "key": row.Role.Key, "name": row.Role.Name,
		"permissions": slices.Clone(row.Role.Permissions), "version": row.Version,
		"created_at": row.CreatedAt, "updated_at": row.UpdatedAt,
	}
}

func customGrantOutput(row repository.CustomGrantRow) map[string]any {
	out := map[string]any{
		"id": row.Grant.ID.String(), "role": row.RoleKey, "scope_type": string(row.Grant.Scope.Type),
	}
	if !row.Grant.AccountID.IsZero() {
		out["account_id"] = row.Grant.AccountID.String()
	}
	if !row.Grant.GroupID.IsZero() {
		out["group_id"] = row.Grant.GroupID.String()
	}
	if !row.Grant.Scope.ID.IsZero() {
		out["scope_id"] = row.Grant.Scope.ID.String()
	}
	return out
}

// RoleCommand is a role as a writer names it.
type RoleCommand struct {
	Key         string
	Name        string
	Permissions []string
	// ExpectedVersion is the version a change was made against; ignored on create.
	ExpectedVersion int
}

// CreateRole defines a role from the catalogue (UC-ID-26 check 1). Whoever defines it must hold
// every permission it carries on the workspace (D-05).
type CreateRole struct{ Roles Roles }

func (h CreateRole) Execute(ctx context.Context, actor appshared.ActorContext, cmd RoleCommand) (repository.CustomRoleRow, error) {
	if slices.Contains(roleNames(), cmd.Key) {
		return repository.CustomRoleRow{}, shared.ErrValidation.WithDetail("roles.key_invalid").
			WithParams(map[string]string{"key": cmd.Key})
	}
	role, err := service.DefineRole(h.Roles.IDs.NewID(), actor.TenantID, cmd.Key, cmd.Name, cmd.Permissions)
	if err != nil {
		return repository.CustomRoleRow{}, err
	}
	if err := h.Roles.authorizeDefinition(ctx, actor, RoleCreatedAction, role.ID, role.Permissions); err != nil {
		return repository.CustomRoleRow{}, err
	}
	now := h.Roles.Clock.Now()
	err = h.Roles.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Roles.Store.Insert(ctx, role, now); err != nil {
			return err
		}
		return h.Roles.recordRole(ctx, actor, RoleCreatedAction, role, nil, now)
	})
	if err != nil {
		return repository.CustomRoleRow{}, err
	}
	return repository.CustomRoleRow{Role: role, CreatedAt: now, UpdatedAt: now, Version: 1}, nil
}

func (h CreateRole) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateRoleName,
		Summary: "Defines a role of the workspace's own from the permission catalogue: a lower-case key " +
			"that never spells a shipped role, a name, and the permissions it carries. Whoever " +
			"defines it must hold every one of them on the workspace.",
		SideEffects: "Writes the role and an audit entry.",
		TokenScope:  membersWrite,
		Permission:  service.PermissionManageMembers,
		Input:       roleInput(false),
		Audit: usecase.AuditDeclaration{
			Action: RoleCreatedAction, TargetType: roleTarget, Severity: audit.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			cmd, err := roleCommand(in)
			if err != nil {
				return nil, err
			}
			row, err := h.Execute(ctx, actor, cmd)
			if err != nil {
				return nil, err
			}
			return CustomRoleOutput(row), nil
		}),
	}
}

// ChangeRole renames a role and changes what it carries. Every permission added or taken away must
// be one the writer holds: taking a permission away from its holders is handing it out undone.
type ChangeRole struct{ Roles Roles }

func (h ChangeRole) Execute(ctx context.Context, actor appshared.ActorContext, cmd RoleCommand) (repository.CustomRoleRow, error) {
	current, err := h.Roles.find(ctx, actor, cmd.Key)
	if err != nil {
		return repository.CustomRoleRow{}, err
	}
	role, err := service.DefineRole(current.Role.ID, actor.TenantID, cmd.Key, cmd.Name, cmd.Permissions)
	if err != nil {
		return repository.CustomRoleRow{}, err
	}
	if err := h.Roles.authorizeDefinition(ctx, actor, RoleChangedAction, role.ID,
		changedPermissions(current.Role.Permissions, role.Permissions)); err != nil {
		return repository.CustomRoleRow{}, err
	}
	version := cmd.ExpectedVersion
	if version == 0 {
		version = current.Version
	}
	now := h.Roles.Clock.Now()
	err = h.Roles.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		written, err := h.Roles.Store.Update(ctx, role, version, now)
		if err != nil {
			return err
		}
		if !written {
			return shared.ErrVersionConflict.WithDetail("roles.version_conflict").
				WithParams(map[string]string{"key": role.Key})
		}
		return h.Roles.recordRole(ctx, actor, RoleChangedAction, role, current.Role.Permissions, now)
	})
	if err != nil {
		return repository.CustomRoleRow{}, err
	}
	return repository.CustomRoleRow{Role: role, CreatedAt: current.CreatedAt, UpdatedAt: now, Version: version + 1}, nil
}

func (h ChangeRole) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ChangeRoleName,
		Summary: "Renames one of the workspace's own roles and changes what it carries; it applies to " +
			"everybody who holds it from the next request. Every permission added or taken away " +
			"must be one the writer holds on the workspace.",
		SideEffects: "Writes the role and an audit entry.",
		TokenScope:  membersWrite,
		Permission:  service.PermissionManageMembers,
		Input:       roleInput(true),
		Audit: usecase.AuditDeclaration{
			Action: RoleChangedAction, TargetType: roleTarget, Severity: audit.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			cmd, err := roleCommand(in)
			if err != nil {
				return nil, err
			}
			row, err := h.Execute(ctx, actor, cmd)
			if err != nil {
				return nil, err
			}
			return CustomRoleOutput(row), nil
		}),
	}
}

// DeleteRole removes a role and every grant of it; whoever held it loses it on the next request,
// and their devices are told.
type DeleteRole struct{ Roles Roles }

func (h DeleteRole) Execute(ctx context.Context, actor appshared.ActorContext, key string) error {
	current, err := h.Roles.find(ctx, actor, key)
	if err != nil {
		return err
	}
	if err := h.Roles.authorizeDefinition(ctx, actor, RoleDeletedAction, current.Role.ID, current.Role.Permissions); err != nil {
		return err
	}
	now := h.Roles.Clock.Now()
	return h.Roles.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		grants, err := h.Roles.Store.Grants(ctx, maxRoleGrants)
		if err != nil {
			return err
		}
		var losses []access.Loss
		for _, grant := range grants {
			if grant.Grant.RoleID != current.Role.ID || h.Roles.Revocations == nil {
				continue
			}
			loss, err := h.Roles.Revocations.AfterGrantRevoked(ctx, asGrant(grant.Grant))
			if err != nil {
				return err
			}
			losses = append(losses, loss)
		}
		removed, err := h.Roles.Store.Delete(ctx, current.Role.ID)
		if err != nil {
			return err
		}
		if !removed {
			return shared.ErrNotFound.WithDetail("roles.not_found").WithParams(map[string]string{"key": key})
		}
		if h.Roles.Revocations != nil && len(losses) > 0 {
			if err := h.Roles.Revocations.Announce(ctx, actor.TenantID, losses...); err != nil {
				return err
			}
		}
		return h.Roles.recordRole(ctx, actor, RoleDeletedAction, current.Role, current.Role.Permissions, now)
	})
}

func (h DeleteRole) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DeleteRoleName,
		Summary: "Removes one of the workspace's own roles and every grant of it. Whoever held it loses " +
			"what it carried on the next request; nothing they wrote is touched.",
		SideEffects: "Removes the role and its grants, tells the holders' devices, and writes an audit entry.",
		TokenScope:  membersWrite,
		Destructive: true,
		Permission:  service.PermissionManageMembers,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The role's key."},
		},
		Audit: usecase.AuditDeclaration{
			Action: RoleDeletedAction, TargetType: roleTarget, Severity: audit.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := h.Execute(ctx, actor, in.String("key")); err != nil {
				return nil, err
			}
			return usecase.Output{}, nil
		}),
	}
}

// GrantRoleCommand is the input, typed.
type GrantRoleCommand struct {
	Role      string
	AccountID shared.ID
	GroupID   shared.ID
	Scope     domain.Scope
}

// GrantRole gives an account or a group one of the workspace's own roles at a place (UC-ID-26
// check 2). The granter must hold MANAGE_MEMBERS there and every permission the role carries.
type GrantRole struct {
	Roles Roles
	// Grants checks the subject and the private hub as a built-in grant does.
	Grants GrantMembership
}

func (h GrantRole) Execute(ctx context.Context, actor appshared.ActorContext, cmd GrantRoleCommand) (repository.CustomGrantRow, error) {
	role, err := h.Roles.find(ctx, actor, cmd.Role)
	if err != nil {
		return repository.CustomGrantRow{}, err
	}
	grant, err := domain.NewCustomGrant(h.Roles.IDs.NewID(), actor.TenantID, cmd.AccountID, cmd.GroupID, cmd.Scope, role.Role.ID)
	if err != nil {
		return repository.CustomGrantRow{}, err
	}
	if err := h.Roles.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       pathTo(grant.Scope),
		Action:     MembershipGrantedAction,
		TokenScope: membersWrite,
		TargetType: membershipTarget,
		TargetID:   grant.Scope.ID,
		HandsOut:   role.Role.Permissions,
	}); err != nil {
		return repository.CustomGrantRow{}, err
	}
	now := h.Roles.Clock.Now()
	err = h.Roles.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		like := asGrant(grant)
		if err := h.Grants.subjectExists(ctx, like); err != nil {
			return err
		}
		if err := h.Grants.noGroupInPrivateHub(ctx, like); err != nil {
			return err
		}
		created, err := h.Roles.Store.Grant(ctx, grant, now)
		if err != nil || !created {
			return err
		}
		return h.Roles.Audit.Append(ctx, customGrantEntry(ctx, MembershipGrantedAction, grant, role.Role.Key, actor, now))
	})
	if err != nil {
		return repository.CustomGrantRow{}, err
	}
	return repository.CustomGrantRow{Grant: grant, RoleKey: role.Role.Key}, nil
}

func (h GrantRole) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: GrantRoleName,
		Summary: "Gives an account or a group one of the workspace's own roles at the workspace, a hub " +
			"or a collection. It applies downwards as a shipped role does. The granter must hold " +
			"every permission the role carries there.",
		SideEffects: "Writes the grant and an audit entry.",
		TokenScope:  membersWrite,
		Permission:  service.PermissionManageMembers,
		Input: []usecase.Field{
			{Name: "role", Kind: usecase.KindString, Required: true, Description: "The custom role's key."},
			{
				Name: "scope_type", Kind: usecase.KindString, Required: true,
				Enum:        []string{string(domain.ScopeTenant), string(domain.ScopeHub), string(domain.ScopeCollection)},
				Description: "TENANT for the whole workspace; otherwise the level the scope identifier names.",
			},
			{Name: "scope_id", Kind: usecase.KindID, Description: "The hub or collection. Omitted for the workspace."},
			{Name: "account_id", Kind: usecase.KindID, Description: "Who gets the role. Exactly one of account_id and group_id."},
			{Name: "group_id", Kind: usecase.KindID, Description: "The group that gets it. Exactly one of account_id and group_id."},
		},
		Audit: usecase.AuditDeclaration{
			Action: MembershipGrantedAction, TargetType: membershipTarget, Severity: audit.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			cmd := GrantRoleCommand{Role: in.String("role")}
			var err error
			if cmd.AccountID, err = in.ID("account_id"); err != nil {
				return nil, err
			}
			if cmd.GroupID, err = in.ID("group_id"); err != nil {
				return nil, err
			}
			scopeID, err := in.ID("scope_id")
			if err != nil {
				return nil, err
			}
			cmd.Scope = domain.Scope{Type: domain.ScopeType(in.String("scope_type")), ID: scopeID}
			row, err := h.Execute(ctx, actor, cmd)
			if err != nil {
				return nil, err
			}
			return customGrantOutput(row), nil
		}),
	}
}

// RevokeRoleGrant takes one grant of a custom role away; it needs what giving it needed.
type RevokeRoleGrant struct{ Roles Roles }

func (h RevokeRoleGrant) Execute(ctx context.Context, actor appshared.ActorContext, grantID shared.ID) error {
	if grantID.IsZero() {
		return shared.ErrValidation.WithDetail("memberships.identifier_required")
	}
	var grant repository.CustomGrantRow
	var role repository.CustomRoleRow
	err := h.Roles.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if grant, err = h.Roles.Store.FindGrant(ctx, grantID); err != nil {
			return err
		}
		role, err = h.Roles.Store.Find(ctx, grant.RoleKey)
		return err
	})
	if err != nil {
		return err
	}
	if err := h.Roles.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       pathTo(grant.Grant.Scope),
		Action:     MembershipRevokedAction,
		TokenScope: membersWrite,
		TargetType: membershipTarget,
		TargetID:   grant.Grant.ID,
		HandsOut:   role.Role.Permissions,
	}); err != nil {
		return err
	}
	now := h.Roles.Clock.Now()
	return h.Roles.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var loss access.Loss
		if h.Roles.Revocations != nil {
			if loss, err = h.Roles.Revocations.AfterGrantRevoked(ctx, asGrant(grant.Grant)); err != nil {
				return err
			}
		}
		removed, err := h.Roles.Store.Revoke(ctx, grant.Grant.ID)
		if err != nil {
			return err
		}
		if !removed {
			return shared.ErrNotFound.WithDetail("memberships.not_found").
				WithParams(map[string]string{"membership_id": grant.Grant.ID.String()})
		}
		if h.Roles.Revocations != nil {
			if err := h.Roles.Revocations.Announce(ctx, actor.TenantID, loss); err != nil {
				return err
			}
		}
		return h.Roles.Audit.Append(ctx, customGrantEntry(ctx, MembershipRevokedAction, grant.Grant, grant.RoleKey, actor, now))
	})
}

func (h RevokeRoleGrant) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        RevokeRoleGrantName,
		Summary:     "Takes one grant of a workspace's own role away. It stops applying on the next request.",
		SideEffects: "Removes the grant, tells the holder's devices, and writes an audit entry.",
		TokenScope:  membersWrite,
		Destructive: true,
		Permission:  service.PermissionManageMembers,
		Input: []usecase.Field{
			{Name: "grant_id", Kind: usecase.KindID, Required: true, Description: "The grant to remove."},
		},
		Audit: usecase.AuditDeclaration{
			Action: MembershipRevokedAction, TargetType: membershipTarget, Severity: audit.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("grant_id")
			if err != nil {
				return nil, err
			}
			if err := h.Execute(ctx, actor, id); err != nil {
				return nil, err
			}
			return usecase.Output{}, nil
		}),
	}
}

// AccessExplained is what *view as* and *who may do what* show for one person at one place (A6,
// A11): the roles that count there and where each is held, the permissions they carry, and - when
// one is asked about - whether the person holds it.
type AccessExplained struct {
	AccountID   shared.ID
	Scope       domain.Scope
	Held        access.Held
	Asked       service.Permission
	Holds       bool
	HeldThrough []access.Source
}

// ExplainAccess answers what a person holds at a place and why (A6, A11). A person may always ask
// about themselves; about somebody else, it takes MANAGE_MEMBERS or READ_CONFIGURATION on the
// workspace, as reading who holds what does.
type ExplainAccess struct{ Roles Roles }

func (h ExplainAccess) Execute(
	ctx context.Context, actor appshared.ActorContext, accountID shared.ID, scope domain.Scope, asked string,
) (AccessExplained, error) {
	if accountID.IsZero() {
		accountID = actor.AccountID
	}
	if !scope.Valid() {
		return AccessExplained{}, shared.ErrValidation.WithDetail("memberships.scope_invalid").
			WithParams(map[string]string{"value": string(scope.Type)})
	}
	var permission service.Permission
	if asked != "" {
		var err error
		if permission, err = service.ParsePermission(asked); err != nil {
			return AccessExplained{}, err
		}
	}
	if accountID != actor.AccountID {
		if err := h.Roles.Authorizer.Authorize(ctx, actor, access.Request{
			Permission:  service.PermissionManageMembers,
			Alternative: service.PermissionReadConfiguration,
			Path:        []domain.Scope{domain.TenantScope()},
			Action:      RoleReadAction,
			TokenScope:  membersRead,
			TargetType:  "account",
			TargetID:    accountID,
		}); err != nil {
			return AccessExplained{}, err
		}
	} else if !actor.IsAuthenticated() {
		return AccessExplained{}, shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	var held access.Held
	err := h.Roles.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		held, err = h.Roles.Held.HeldBy(ctx, actor, accountID, pathTo(scope))
		return err
	})
	if err != nil {
		return AccessExplained{}, err
	}
	out := AccessExplained{AccountID: accountID, Scope: scope, Held: held, Asked: permission}
	if permission != "" {
		for _, source := range held.Sources {
			if source.Carries(permission) {
				out.HeldThrough = append(out.HeldThrough, source)
			}
		}
		out.Holds = len(out.HeldThrough) > 0
	}
	return out, nil
}

func (h ExplainAccess) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ExplainAccessName,
		Summary: "Answers what a person holds at the workspace, a hub or a collection: the roles that " +
			"count there and where each is held, and every permission they carry. Given a permission, " +
			"also whether the person holds it and through which role. A person may ask about " +
			"themselves; about somebody else it takes MANAGE_MEMBERS or READ_CONFIGURATION.",
		SideEffects: "None. Reads only.",
		TokenScope:  membersRead,
		ReadOnly:    true,
		Permission:  service.PermissionManageMembers,
		Input: []usecase.Field{
			{Name: "account_id", Kind: usecase.KindID, Description: "The person; omitted for the caller."},
			{
				Name: "scope_type", Kind: usecase.KindString, Required: true,
				Enum:        []string{string(domain.ScopeTenant), string(domain.ScopeHub), string(domain.ScopeCollection)},
				Description: "TENANT for the workspace; otherwise the level the scope identifier names.",
			},
			{Name: "scope_id", Kind: usecase.KindID, Description: "The hub or collection. Omitted for the workspace."},
			{Name: "permission", Kind: usecase.KindString, Description: "A permission of the catalogue to ask about."},
		},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			accountID, err := in.ID("account_id")
			if err != nil {
				return nil, err
			}
			scopeID, err := in.ID("scope_id")
			if err != nil {
				return nil, err
			}
			scope := domain.Scope{Type: domain.ScopeType(in.String("scope_type")), ID: scopeID}
			explained, err := h.Execute(ctx, actor, accountID, scope, in.String("permission"))
			if err != nil {
				return nil, err
			}
			return AccessExplainedOutput(explained), nil
		}),
	}
}

// AccessExplainedOutput is the answer every door shares.
func AccessExplainedOutput(e AccessExplained) usecase.Output {
	sources := func(list []access.Source) []map[string]any {
		out := make([]map[string]any, 0, len(list))
		for _, source := range list {
			one := map[string]any{"role": source.Role, "custom": source.Custom, "scope_type": string(source.Scope.Type)}
			if !source.Scope.ID.IsZero() {
				one["scope_id"] = source.Scope.ID.String()
			}
			out = append(out, one)
		}
		return out
	}
	out := usecase.Output{
		"account_id":  e.AccountID.String(),
		"scope_type":  string(e.Scope.Type),
		"roles":       slices.Clone(e.Held.Roles),
		"permissions": permissionStrings(e.Held.Permissions),
		"sources":     sources(e.Held.Sources),
		"requester":   e.Held.Requester,
	}
	if !e.Scope.ID.IsZero() {
		out["scope_id"] = e.Scope.ID.String()
	}
	if e.Asked != "" {
		out["asked"] = string(e.Asked)
		out["holds"] = e.Holds
		out["held_through"] = sources(e.HeldThrough)
	}
	return out
}

func (r Roles) find(ctx context.Context, actor appshared.ActorContext, key string) (repository.CustomRoleRow, error) {
	var row repository.CustomRoleRow
	err := r.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		row, err = r.Store.Find(ctx, key)
		return err
	})
	if errors.Is(err, shared.ErrNotFound) {
		return repository.CustomRoleRow{}, shared.ErrNotFound.WithDetail("roles.not_found").
			WithParams(map[string]string{"key": key})
	}
	return row, err
}

// authorizeDefinition asks MANAGE_MEMBERS on the workspace and every permission handed out or
// taken back (D-05).
func (r Roles) authorizeDefinition(
	ctx context.Context, actor appshared.ActorContext, action audit.Action, roleID shared.ID, permissions []string,
) error {
	return r.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     action,
		TokenScope: membersWrite,
		TargetType: roleTarget,
		TargetID:   roleID,
		HandsOut:   permissions,
	})
}

// recordRole writes the trail's entry. The key and the permissions are codes; the name is user
// content and stays out (rule 10).
func (r Roles) recordRole(
	ctx context.Context, actor appshared.ActorContext, action audit.Action, role domain.CustomRole,
	before []string, at time.Time,
) error {
	changes := []audit.Change{
		{Field: "key", Classification: audit.Open, To: role.Key},
		{Field: "permissions", Classification: audit.Open, From: nilIfEmpty(before), To: slices.Clone(role.Permissions)},
	}
	if action == RoleDeletedAction {
		changes[1] = audit.Change{Field: "permissions", Classification: audit.Open, From: slices.Clone(before)}
	}
	return r.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: at,
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: roleTarget,
		TargetID:   role.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    audit.Changes(changes...),
	})
}

func customGrantEntry(
	ctx context.Context, action audit.Action, grant domain.CustomGrant, key string, actor appshared.ActorContext, at time.Time,
) audit.Entry {
	subject, subjectID := "account", grant.AccountID
	if grant.AccountID.IsZero() {
		subject, subjectID = "group", grant.GroupID
	}
	var scopeID any
	if !grant.Scope.ID.IsZero() {
		scopeID = grant.Scope.ID.String()
	}
	// The fields of a built-in grant's entry, so an access review reads both as one shape; the
	// role is the custom role's key.
	changes := audit.Changes(
		audit.Change{Field: "subject_type", Classification: audit.Open, To: subject},
		audit.Change{Field: "subject_id", Classification: audit.Open, To: subjectID.String()},
		audit.Change{Field: "scope_type", Classification: audit.Open, To: string(grant.Scope.Type)},
		audit.Change{Field: "scope_id", Classification: audit.Open, To: scopeID},
		audit.Change{Field: "role", Classification: audit.Open, To: key},
		audit.Change{Field: "custom_role", Classification: audit.Open, To: true},
	)
	return audit.Entry{
		TenantID:   grant.TenantID,
		OccurredAt: at,
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: membershipTarget,
		TargetID:   grant.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    changes,
	}
}

// asGrant is a custom grant in the shape the subject checks and the revocation read: they ask who
// and where, never which role.
func asGrant(grant domain.CustomGrant) domain.Grant {
	return domain.Grant{
		ID: grant.ID, TenantID: grant.TenantID, AccountID: grant.AccountID, GroupID: grant.GroupID, Scope: grant.Scope,
	}
}

// changedPermissions is every permission added or taken away.
func changedPermissions(before, after []string) []string {
	changed := []string{}
	for _, p := range after {
		if !slices.Contains(before, p) {
			changed = append(changed, p)
		}
	}
	for _, p := range before {
		if !slices.Contains(after, p) {
			changed = append(changed, p)
		}
	}
	return changed
}

func roleInput(change bool) []usecase.Field {
	fields := []usecase.Field{
		{Name: "key", Kind: usecase.KindString, Required: true, Description: "Lower case, `^[a-z][a-z0-9_]{1,62}$`; fixed once defined."},
		{Name: "name", Kind: usecase.KindString, Required: true, Description: "Up to 120 characters, on one line."},
		{Name: "permissions", Kind: usecase.KindList, Required: true, Description: "Names from the permission catalogue."},
	}
	if change {
		fields = append(fields, usecase.Field{
			Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version the change was made against.",
		})
	}
	return fields
}

func roleCommand(in usecase.Input) (RoleCommand, error) {
	permissions, err := in.StringList("permissions")
	if err != nil {
		return RoleCommand{}, err
	}
	return RoleCommand{
		Key: in.String("key"), Name: in.String("name"), Permissions: permissions,
		ExpectedVersion: in.Int("expected_version"),
	}, nil
}

func roleStrings(roles []domain.Role) []string {
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		out = append(out, string(role))
	}
	return out
}

func permissionStrings(permissions []service.Permission) []string {
	out := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		out = append(out, string(permission))
	}
	return out
}

func nilIfEmpty(list []string) any {
	if len(list) == 0 {
		return nil
	}
	return slices.Clone(list)
}
