// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"

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
	RenewInvitationName = "RenewInvitation"

	// InvitationRenewedAction is the audit code: somebody's way into this workspace was issued
	// again, which is the same class of event as the invitation itself (audit.md §4).
	InvitationRenewedAction audit.Action = "account.invitation_renewed"
)

// RenewInvitationCommand names the invited account.
type RenewInvitationCommand struct {
	AccountID shared.ID
}

// RenewInvitation gives an invitation not yet accepted a new way to arrive: the mail again, or -
// where no mail goes out - a new link, answered once.
//
// It exists because the link is shown once. A link lost, or run out after its fortnight, would
// otherwise leave the address taken for good: inviting it again is refused (UC-ID-14 check 6) and
// nothing removes an account. The earlier link dies with the new token, at once without mail, at the
// new mail's delivery with it - the delivery mints, and the newest token wins.
type RenewInvitation struct {
	Accounts   repository.Accounts
	Authorizer Authorizer
	Notifier   Notifier
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	// Links hands the invitation over as a link where no mail goes out, as InviteAccount does.
	Links InvitationLinks
}

// Execute renews and answers the account, with the new link where it is handed over by hand.
func (h RenewInvitation) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd RenewInvitationCommand,
) (Invitation, error) {
	// Before the transaction, for InviteAccount's reason: a refusal's own trail entry must outlive it.
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     InvitationRenewedAction,
		TokenScope: membersWrite,
		TargetType: accountTarget,
		TargetID:   cmd.AccountID,
	}); err != nil {
		return Invitation{}, err
	}
	if err := needsAPerson(actor, h.Links); err != nil {
		return Invitation{}, err
	}

	var renewed Invitation
	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		// Read through the workspace's own transaction: an account of another workspace is not
		// found, the answer an identifier from elsewhere gets everywhere.
		invited, err := h.Accounts.Find(ctx, cmd.AccountID)
		if err != nil {
			return err
		}
		if invited.Status != domain.AccountInvited {
			return notInvited()
		}
		link, err := deliver(ctx, h.Links, h.Notifier, invited.TenantID, invited.ID, actor)
		if err != nil {
			return err
		}
		renewed = Invitation{Account: invited, Link: link}
		return h.recordAudit(ctx, invited, actor)
	})
	if err != nil {
		return Invitation{}, err
	}
	return renewed, nil
}

// notInvited answers a renewal of an account that is not waiting on an invitation: a person who
// arrived, a managed or service account, or one accepted a moment ago.
func notInvited() error {
	return shared.ErrConflict.WithDetail("accounts.not_invited")
}

func (h RenewInvitation) recordAudit(
	ctx context.Context, invited domain.Account, actor appshared.ActorContext,
) error {
	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   invited.TenantID,
		OccurredAt: h.Clock.Now(),
		Action:     InvitationRenewedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: accountTarget,
		TargetID:   invited.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		// How it went out, never the link: the trail outlives the credential and is read by people
		// who must not be able to accept somebody else's invitation (rule 10).
		Changes: audit.Changes(
			audit.Change{Field: "delivery", Classification: audit.Open, To: deliveryOf(h.Links)},
		),
	})
}

// Descriptor registers the use case in all three channels.
func (h RenewInvitation) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RenewInvitationName,
		Summary: "Renews an invitation that has not been accepted yet: the invitation mail is sent " +
			"again, or - where this installation sends no mail - a new invitation link is answered " +
			"once. The earlier link stops working.",
		SideEffects: "Queues the invitation message again or replaces the invitation's token, and " +
			"writes an audit entry.",
		TokenScope: membersWrite,
		Input: []usecase.Field{
			{
				Name: "account_id", Kind: usecase.KindID, Required: true,
				Description: "The invited account. It must not have accepted its invitation yet.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: InvitationRenewedAction, TargetType: accountTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RenewInvitation) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	accountID, err := in.ID("account_id")
	if err != nil {
		return nil, err
	}
	renewed, err := h.Execute(ctx, actor, RenewInvitationCommand{AccountID: accountID})
	if err != nil {
		return nil, err
	}
	return invitationOutput(renewed), nil
}
