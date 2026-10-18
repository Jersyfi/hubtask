// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"slices"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// WorkspaceHolder is the authoriser as the account link asks it: what an actor holds on the
// workspace itself, recording nothing (access.Service.HeldOnWorkspace).
type WorkspaceHolder interface {
	HeldOnWorkspace(ctx context.Context, actor appshared.ActorContext) ([]service.Permission, error)
}

// AccountLinks answers the provider's account link in force for a workspace.
type AccountLinks interface {
	AccountLink(ctx context.Context, tenantID shared.ID) (string, error)
}

// AccountLinkTeller decides who is told the provider's account link (multi-tenancy.md §4.2,
// UC-INS-13 check 11): the workspace's owners and administrators - whoever holds STRUCTURE on the
// workspace itself, which only OWNER and ADMIN do - and nobody else. One decision for every door
// that answers it: the manifest, and the refusal a paused workspace answers at sign-in.
type AccountLinkTeller struct {
	Held  WorkspaceHolder
	Links AccountLinks
}

// For answers the link for the actor, empty where none is in force or the actor is not told it.
func (t AccountLinkTeller) For(ctx context.Context, actor appshared.ActorContext) (string, error) {
	if t.Held == nil || t.Links == nil || !actor.IsAuthenticated() || actor.TenantID.IsZero() {
		return "", nil
	}
	held, err := t.Held.HeldOnWorkspace(ctx, actor)
	if err != nil || !slices.Contains(held, service.PermissionStructure) {
		return "", err
	}
	return t.Links.AccountLink(ctx, actor.TenantID)
}

// AccountLink answers the provider's account link in force for a workspace: the installation's,
// or its plan's where the installation leaves it open (multi-tenancy.md §4.2).
func (r SignInPolicyResolver) AccountLink(ctx context.Context, tenantID shared.ID) (string, error) {
	resolved, err := r.Resolve(ctx, tenantID)
	if err != nil {
		return "", err
	}
	return resolved.AccountLink, nil
}

// AccountLinkTold is what the sign-in asks of AccountLinkTeller.
type AccountLinkTold interface {
	For(ctx context.Context, actor appshared.ActorContext) (string, error)
}

// paused adds the provider's account link to the refusal a paused workspace answers - suspended or
// pending deletion - for an owner or administrator who has completed the proof (UC-INS-13 check
// 11): the page they meet says whom to ask, and only they are told where. Any other refusal, and a
// link that cannot be read, passes as it came: the paused sentence is owed either way, and a
// failed read must not turn it into a server error.
func (w SessionWriter) paused(ctx context.Context, tenantID, accountID shared.ID, refusal error) error {
	if w.AccountLinks == nil {
		return refusal
	}
	refused := shared.AsError(refusal)
	if refused == nil || (refused.DetailCode != "access.tenant_suspended" && refused.DetailCode != "access.tenant_pending_deletion") {
		return refusal
	}
	link, err := w.AccountLinks.For(ctx, appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantID, AccountID: accountID,
	})
	if err != nil || link == "" {
		return refusal
	}
	params := map[string]string{"account_url": link}
	for name, value := range refused.Params {
		params[name] = value
	}
	return refused.WithParams(params)
}

// passwordIsTheWholeProof answers whether a right password completes the proof for the account: no
// confirmed second factor is asked after it. Read as the challenge reads it; an unreadable answer
// is an error, which the caller answers with the plain refusal.
func (w SessionWriter) passwordIsTheWholeProof(
	ctx context.Context, scope persistence.Scope, account domain.Account, cmd SignInCommand,
) (bool, error) {
	if w.Enrollments == nil {
		return true, nil
	}
	verdict := SignInVerdict{}
	if w.Rule != nil {
		read, err := w.Rule.JudgeSignIn(ctx, scope.TenantID, account, cmd.Password)
		if err != nil {
			return false, err
		}
		verdict = read
	}
	var purpose domain.PendingPurpose
	if err := w.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		held, err := w.factorHeldAt(ctx, account.ID, cmd.Host)
		if err != nil {
			return err
		}
		purpose, err = w.pendingPurposeFor(ctx, account, verdict, held)
		return err
	}); err != nil {
		return false, err
	}
	return purpose != domain.PendingTotp, nil
}
