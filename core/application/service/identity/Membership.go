// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"

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
	GrantMembershipName  = "GrantMembership"
	RevokeMembershipName = "RevokeMembership"
	membershipTarget     = "membership"

	// The two audit codes an access review reads first: who was given a way in, and who lost one.
	MembershipGrantedAction audit.Action = "membership.granted"
	MembershipRevokedAction audit.Action = "membership.revoked"
)

// GrantMembershipCommand is the input, typed.
type GrantMembershipCommand struct {
	AccountID shared.ID
	GroupID   shared.ID
	Scope     domain.Scope
	Role      domain.Role
}

// GrantMembership gives an account or a group a role at a scope.
//
// The role applies downwards from that scope - a role at a hub applies to its collections and their
// items - and the resolution takes the highest role along the path (domain-model.md §3.2). Which is
// why the permission is checked at the scope being granted rather than at the tenant: an
// administrator of one hub may hand out roles inside it and nowhere else.
type GrantMembership struct {
	Grants     repository.MembershipGrants
	Accounts   repository.Accounts
	Groups     repository.Groups
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// Execute records the grant and returns it.
func (h GrantMembership) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd GrantMembershipCommand,
) (domain.Grant, error) {
	grant, err := domain.NewGrant(h.IDs.NewID(), actor.TenantID, cmd.AccountID, cmd.GroupID, cmd.Scope, cmd.Role)
	if err != nil {
		return domain.Grant{}, err
	}

	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       pathTo(grant.Scope),
		Action:     MembershipGrantedAction,
		TokenScope: membersWrite,
		TargetType: membershipTarget,
		TargetID:   grant.Scope.ID,
	}); err != nil {
		return domain.Grant{}, err
	}

	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		// The subject is looked up so that granting to somebody who is not here says so, rather
		// than surfacing as a foreign key violation from three layers down.
		if err := h.subjectExists(ctx, grant); err != nil {
			return err
		}
		if err := h.Grants.Grant(ctx, grant); err != nil {
			return err
		}
		return h.recordAudit(ctx, grant, actor)
	})
	if err != nil {
		return domain.Grant{}, err
	}
	return grant, nil
}

func (h GrantMembership) subjectExists(ctx context.Context, grant domain.Grant) error {
	if !grant.AccountID.IsZero() {
		if _, err := h.Accounts.Find(ctx, grant.AccountID); err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return shared.ErrNotFound.
					WithDetail("accounts.not_found").
					WithParams(map[string]string{"account_id": grant.AccountID.String()})
			}
			return err
		}
		return nil
	}
	if _, err := h.Groups.Find(ctx, grant.GroupID); err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return shared.ErrNotFound.
				WithDetail("groups.not_found").
				WithParams(map[string]string{"group_id": grant.GroupID.String()})
		}
		return err
	}
	return nil
}

func (h GrantMembership) recordAudit(
	ctx context.Context, grant domain.Grant, actor appshared.ActorContext,
) error {
	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   grant.TenantID,
		OccurredAt: h.Clock.Now(),
		Action:     MembershipGrantedAction,
		Outcome:    audit.OutcomeSuccess,
		// Notice, like an invitation: this is the entry an access review looks for.
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: membershipTarget,
		TargetID:   grant.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    grantChanges(grant),
	})
}

// grantChanges is what both entries record. Everything about a grant is a code or an identifier -
// none of it is user content, and all of it is what a review needs to read (audit.md §2).
func grantChanges(grant domain.Grant) map[string]any {
	subject, subjectID := "account", grant.AccountID
	if grant.AccountID.IsZero() {
		subject, subjectID = "group", grant.GroupID
	}

	var scopeID any
	if !grant.Scope.ID.IsZero() {
		scopeID = grant.Scope.ID.String()
	}

	return audit.Changes(
		audit.Change{Field: "subject_type", Classification: audit.Open, To: subject},
		audit.Change{Field: "subject_id", Classification: audit.Open, To: subjectID.String()},
		audit.Change{Field: "scope_type", Classification: audit.Open, To: string(grant.Scope.Type)},
		audit.Change{Field: "scope_id", Classification: audit.Open, To: scopeID},
		audit.Change{Field: "role", Classification: audit.Open, To: string(grant.Role)},
	)
}

// pathTo is the path the authorisation walks: the tenant, then the scope itself when it is not the
// tenant. A grant at a hub is decided by what the actor holds at that hub or above it.
func pathTo(scope domain.Scope) []domain.Scope {
	path := []domain.Scope{domain.TenantScope()}
	if scope.Type != domain.ScopeTenant {
		path = append(path, scope)
	}
	return path
}

// Descriptor registers the use case in all three channels.
func (h GrantMembership) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: GrantMembershipName,
		Summary: "Gives an account or a group a role at a scope. The role applies downwards: a " +
			"role at a hub applies to its collections and their items, and the strongest role " +
			"along the path is the one that counts.",
		SideEffects: "Writes the membership and an audit entry.",
		TokenScope:  membersWrite,
		Input: []usecase.Field{
			{
				Name: "scope_type", Kind: usecase.KindString, Required: true,
				Enum: []string{string(domain.ScopeTenant), string(domain.ScopeHub),
					string(domain.ScopeCollection), string(domain.ScopeItem)},
				Description: "TENANT for the whole workspace; otherwise the level the scope identifier names.",
			},
			{
				Name: "role", Kind: usecase.KindString, Required: true,
				Enum:        roleNames(),
				Description: "OWNER, ADMIN, MEMBER, CONTRIBUTOR, VIEWER or GUEST, strongest first.",
			},
			{
				Name: "account_id", Kind: usecase.KindID,
				Description: "Who gets the role. Exactly one of account_id and group_id.",
			},
			{
				Name: "group_id", Kind: usecase.KindID,
				Description: "The group that gets the role. Exactly one of account_id and group_id.",
			},
			{
				Name: "scope_id", Kind: usecase.KindID,
				Description: "The hub, collection or item the role applies to. Omitted for the whole workspace.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: MembershipGrantedAction, TargetType: membershipTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h GrantMembership) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	accountID, err := in.ID("account_id")
	if err != nil {
		return nil, err
	}
	groupID, err := in.ID("group_id")
	if err != nil {
		return nil, err
	}
	scopeID, err := in.ID("scope_id")
	if err != nil {
		return nil, err
	}

	grant, err := h.Execute(ctx, actor, GrantMembershipCommand{
		AccountID: accountID,
		GroupID:   groupID,
		Scope:     domain.Scope{Type: domain.ScopeType(in.String("scope_type")), ID: scopeID},
		Role:      domain.Role(in.String("role")),
	})
	if err != nil {
		return nil, err
	}
	return grantOutput(grant), nil
}

func grantOutput(grant domain.Grant) usecase.Output {
	out := usecase.Output{
		"id":         grant.ID.String(),
		"scope_type": string(grant.Scope.Type),
		"role":       string(grant.Role),
	}
	if !grant.AccountID.IsZero() {
		out["account_id"] = grant.AccountID.String()
	}
	if !grant.GroupID.IsZero() {
		out["group_id"] = grant.GroupID.String()
	}
	if !grant.Scope.ID.IsZero() {
		out["scope_id"] = grant.Scope.ID.String()
	}
	return out
}

func roleNames() []string {
	roles := domain.Roles()
	names := make([]string, 0, len(roles))
	for _, role := range roles {
		names = append(names, string(role))
	}
	return names
}

// RevokeMembershipCommand is the input, typed.
type RevokeMembershipCommand struct {
	MembershipID shared.ID
}

// RevokeMembership takes a role away.
//
// It takes effect on the next request, because the resolution reads the table rather than a cache
// (ADR-0005) - there is nothing to invalidate and no window in which a revoked role still works.
// What it does not touch is what the account holds through a group: that is the group's membership,
// and revoking it there would take it from everybody.
type RevokeMembership struct {
	Grants     repository.MembershipGrants
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Execute removes the membership.
func (h RevokeMembership) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd RevokeMembershipCommand,
) error {
	if cmd.MembershipID.IsZero() {
		return shared.ErrValidation.WithDetail("memberships.identifier_required")
	}

	// Read before authorising, because the permission is decided at the scope the membership is
	// at, and this is the only way to learn what that scope is. The read is in its own
	// transaction: a refusal must not roll back the audit entry that records it (audit.md §7).
	var grant domain.Grant
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Grants.Find(ctx, cmd.MembershipID)
		grant = found
		return err
	})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return shared.ErrNotFound.
				WithDetail("memberships.not_found").
				WithParams(map[string]string{"membership_id": cmd.MembershipID.String()})
		}
		return err
	}

	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       pathTo(grant.Scope),
		Action:     MembershipRevokedAction,
		TokenScope: membersWrite,
		TargetType: membershipTarget,
		TargetID:   grant.ID,
	}); err != nil {
		return err
	}

	return h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		removed, err := h.Grants.Revoke(ctx, grant.ID)
		if err != nil {
			return err
		}
		if !removed {
			// It was there a moment ago and is not now. Reporting success would tell the caller
			// their revocation did something it did not.
			return shared.ErrNotFound.
				WithDetail("memberships.not_found").
				WithParams(map[string]string{"membership_id": grant.ID.String()})
		}
		return h.recordAudit(ctx, grant, actor)
	})
}

func (h RevokeMembership) recordAudit(
	ctx context.Context, grant domain.Grant, actor appshared.ActorContext,
) error {
	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   grant.TenantID,
		OccurredAt: h.Clock.Now(),
		Action:     MembershipRevokedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: membershipTarget,
		TargetID:   grant.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		// The same fields as the grant, so that the two entries of one access review read as a
		// pair rather than as two shapes.
		Changes: grantChanges(grant),
	})
}

// Descriptor registers the use case in all three channels.
func (h RevokeMembership) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RevokeMembershipName,
		Summary: "Takes a role away. It stops applying on the next request. What the account " +
			"holds through a group is unaffected - that is the group's membership.",
		SideEffects: "Removes the membership and writes an audit entry.",
		TokenScope:  membersWrite,
		Destructive: true,
		Input: []usecase.Field{
			{
				Name: "membership_id", Kind: usecase.KindID, Required: true,
				Description: "The membership to remove.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: MembershipRevokedAction, TargetType: membershipTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RevokeMembership) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	membershipID, err := in.ID("membership_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, RevokeMembershipCommand{MembershipID: membershipID}); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}
