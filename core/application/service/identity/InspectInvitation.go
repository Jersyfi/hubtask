// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// InspectInvitationName is the catalogue name of the invitation card's first read.
const InspectInvitationName = "InspectInvitation"

// InspectInvitationCommand carries the token the invitation card holds.
type InspectInvitationCommand struct {
	Token secret.Secret
	// TenantHeader may confirm the token's tenant, never overrule it.
	TenantHeader string
}

// InvitationSummary is what the card names before anything is typed (UC-ID-07 check 1).
type InvitationSummary struct {
	WorkspaceName string
	Email         string
}

// InspectInvitation names the workspace and the address an invitation is for.
//
// **Only for a token that would redeem.** Whoever holds the token holds the invitation - the same
// person the mail or the handed-over link reached - so naming its workspace and address to them
// tells them nothing they could not learn by redeeming it. Every other token gets the redemption's
// one refusal, counted against the same ledger, so this read is no oracle the redemption is not.
// Nothing is spent: the invitation is exactly as redeemable afterwards.
type InspectInvitation struct{ Writer SessionWriter }

// Execute reads the invitation.
func (h InspectInvitation) Execute(ctx context.Context, cmd InspectInvitationCommand) (InvitationSummary, error) {
	w := h.Writer
	token, err := domain.ParseRedemptionToken(cmd.Token.Reveal())
	if err != nil {
		w.failure(ctx, FailureRedemption)
		return InvitationSummary{}, redemptionRefused()
	}
	if cmd.TenantHeader != "" && cmd.TenantHeader != token.TenantID().String() {
		return InvitationSummary{}, shared.ErrForbidden.WithDetail("access.tenant_mismatch")
	}

	var summary InvitationSummary
	err = w.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: token.TenantID()},
		func(ctx context.Context) error {
			found, err := w.Accounts.FindByRedemptionToken(ctx, token)
			if errors.Is(err, shared.ErrNotFound) {
				return redemptionRefused()
			}
			if err != nil {
				return err
			}
			if !found.ExpiresAt.After(w.Clock.Now()) || found.Account.Status != domain.AccountInvited {
				return redemptionRefused()
			}
			summary = InvitationSummary{WorkspaceName: found.TenantName, Email: found.Account.Email}
			return nil
		})
	if err != nil {
		if shared.AsError(err) != nil && shared.AsError(err).DetailCode == "auth.redemption_failed" {
			w.failure(ctx, FailureRedemption)
		}
		return InvitationSummary{}, err
	}
	return summary, nil
}

// Descriptor is the catalogue entry.
func (h InspectInvitation) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: InspectInvitationName,
		Summary: "Names the workspace and the address an invitation is for, so the invitation " +
			"card can say them before anything is typed. Only for a token that would redeem; " +
			"every other one is the redemption's one refusal. Nothing is spent.",
		SideEffects: "None - a read; a refused token counts against the redemption's ledger.",
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "token", Kind: usecase.KindString, Required: true,
				Description: "The redemption token from the invitation. Read, not spent."},
			{Name: "tenant_header", Kind: usecase.KindString,
				Description: "The X-Hubtask-Tenant header, when sent. It may confirm the " +
					"token's tenant, never overrule it."},
		},
		// None: a read of an invitation by its own holder changes nothing, and its refusals count in
		// the redemption's metric, as the sign-in rules' read records nothing either.
		Audit: usecase.AuditDeclaration{},
		Activity: usecase.ActivityDeclaration{
			Exempt: "An invitation is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h InspectInvitation) invoke(
	ctx context.Context, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	summary, err := h.Execute(ctx, InspectInvitationCommand{
		Token:        secret.New(in.String("token")),
		TenantHeader: in.String("tenant_header"),
	})
	if err != nil {
		return nil, err
	}
	return usecase.Output{"workspace_name": summary.WorkspaceName, "email": summary.Email}, nil
}
