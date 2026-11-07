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
	"github.com/Jersyfi/hubtask/core/port/stepup"
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
	// ByLink hands the invitation over by link also where mail is sent (*Copy invitation link*).
	ByLink bool
	// StepUpToken is the fresh proof a link handed over instead of a mail demands.
	StepUpToken string
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
	// Held is what the invited account holds - the roles its invitation gives - for the ceiling on
	// a link.
	Held       repository.HeldRoles
	Notifier   Notifier
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	// Links hands the invitation over as a link where no mail goes out, as InviteAccount does.
	Links InvitationLinks
	// People holds the renewal to the people limit: a link that ran out had freed its place.
	People PeopleLimit
	// HandOver mints a link where mail is sent, for a renewal asked by link; Jobs cancels the
	// mail still queued so its token never replaces the link (identity.md §12, "Never both");
	// StepUp judges the fresh proof it demands. Unwired, a renewal by link where mail is sent is
	// refused.
	HandOver InvitationLinks
	Jobs     repository.InvitationJobs
	StepUp   stepup.Verifier
}

// linkInsteadOfMail is a renewal by link where this installation sends mail.
func (h RenewInvitation) linkInsteadOfMail(cmd RenewInvitationCommand) bool {
	return cmd.ByLink && !h.Links.handsOver()
}

// Execute renews and answers the account, with the new link where it is handed over by hand.
func (h RenewInvitation) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd RenewInvitationCommand,
) (Invitation, error) {
	// Before the transaction, for InviteAccount's reason: a refusal's own trail entry must outlive it.
	request := access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     InvitationRenewedAction,
		TokenScope: membersWrite,
		TargetType: accountTarget,
		TargetID:   cmd.AccountID,
	}
	if err := h.Authorizer.Authorize(ctx, actor, request); err != nil {
		return Invitation{}, err
	}
	links := h.Links
	instead := h.linkInsteadOfMail(cmd)
	if instead {
		if !h.HandOver.handsOver() || h.Jobs == nil {
			return Invitation{}, shared.ErrInternal.WithDetail("accounts.invitation_link_unavailable")
		}
		// The link is answered to the caller rather than mailed to the person.
		if err := stepup.Demand(ctx, h.StepUp, actor.TenantID, actor.AccountID, cmd.StepUpToken); err != nil {
			return Invitation{}, err
		}
		links = h.HandOver
	}
	if err := needsAPerson(actor, links); err != nil {
		return Invitation{}, err
	}
	if links.handsOver() {
		if err := h.holdTheLink(ctx, actor, request); err != nil {
			return Invitation{}, err
		}
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
		// The actor's workspace, not the row's: the account repository's read does not carry the
		// tenant, and the row was found inside this workspace's transaction anyway.
		// Asked before the new link goes out: a link that ran out had freed its place.
		if h.People != nil {
			if err := h.People.Renewal(ctx, actor.TenantID.String(), invited.ID.String()); err != nil {
				return err
			}
		}
		if instead {
			// Before the token: a delivery under way rolls back at its next write, its token with it.
			if _, err := h.Jobs.CancelOpen(ctx, invited.ID, h.Clock.Now()); err != nil {
				return err
			}
		}
		link, err := deliver(ctx, links, h.Notifier, actor.TenantID, invited.ID, actor)
		if err != nil {
			return err
		}
		renewed = Invitation{Account: invited, Link: link}
		return h.recordAudit(ctx, invited, actor, deliveryOf(links))
	})
	if err != nil {
		return Invitation{}, err
	}
	return renewed, nil
}

// holdTheLink holds a renewal by link to the role ceiling: the link is answered to the actor, who
// could accept it and arrive as the invited account with the roles its invitation gives. A mail
// goes to the invited address instead, and is no way in for the actor.
//
// The account is read once here, in a transaction that has ended before the ceiling may record a
// refusal; one not waiting on an invitation is answered so first, and the renewal reads it again.
func (h RenewInvitation) holdTheLink(ctx context.Context, actor appshared.ActorContext, request access.Request) error {
	ceiling := wayInCeiling{Held: h.Held, Authorizer: h.Authorizer}
	var held []domain.Role
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		invited, err := h.Accounts.Find(ctx, request.TargetID)
		if err != nil {
			return err
		}
		if invited.Status != domain.AccountInvited {
			return notInvited()
		}
		held, err = ceiling.heldBy(ctx, invited.ID)
		return err
	})
	if err != nil {
		return err
	}
	return ceiling.hold(ctx, actor, request, held)
}

// notInvited answers a renewal of an account that is not waiting on an invitation: a person who
// arrived, a managed or service account, or one accepted a moment ago.
func notInvited() error {
	return shared.ErrConflict.WithDetail("accounts.not_invited")
}

func (h RenewInvitation) recordAudit(
	ctx context.Context, invited domain.Account, actor appshared.ActorContext, delivery string,
) error {
	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
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
			audit.Change{Field: "delivery", Classification: audit.Open, To: delivery},
		),
	})
}

// Descriptor registers the use case in all three channels.
func (h RenewInvitation) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       RenewInvitationName,
		Permission: service.PermissionManageMembers,
		Summary: "Renews an invitation that has not been accepted yet: the invitation mail is sent " +
			"again, or - where this installation sends no mail, or delivery LINK is asked - a new " +
			"invitation link is answered once. The earlier link stops working. A link instead of " +
			"a mail cancels the queued mail and needs a fresh proof.",
		SideEffects: "Queues the invitation message again or replaces the invitation's token (and " +
			"then cancels a queued invitation mail), and writes an audit entry.",
		TokenScope: membersWrite,
		Input: []usecase.Field{
			{
				Name: "account_id", Kind: usecase.KindID, Required: true,
				Description: "The invited account. It must not have accepted its invitation yet.",
			},
			{
				Name: "delivery", Kind: usecase.KindString,
				Description: "LINK answers a new invitation link to hand over, also where this " +
					"installation sends mail. Omitted, it goes out the installation's own way.",
			},
			{
				Name: "step_up_token", Kind: usecase.KindString,
				Description: "The fresh proof a link instead of a mail demands.",
			},
		},
		StepUp: "handing an invitation over by link where mail is sent",
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
	delivery := in.String("delivery")
	if delivery != "" && delivery != DeliveryLink {
		return nil, shared.ErrValidation.WithDetail("accounts.delivery_unknown").
			WithParams(map[string]string{"delivery": delivery}).
			WithFields(shared.FieldError{Path: "/delivery", Code: "accounts.delivery_unknown"})
	}
	renewed, err := h.Execute(ctx, actor, RenewInvitationCommand{
		AccountID: accountID, ByLink: delivery == DeliveryLink, StepUpToken: in.String("step_up_token"),
	})
	if err != nil {
		return nil, err
	}
	return invitationOutput(renewed), nil
}
