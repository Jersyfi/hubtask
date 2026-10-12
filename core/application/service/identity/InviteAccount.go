// SPDX-License-Identifier: Apache-2.0
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
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const (
	InviteAccountName = "InviteAccount"
	accountTarget     = "account"
	membersWrite      = "members:write"

	// AccountInvitedAction is the audit code. Adding somebody to a workspace is the event an
	// auditor looks for first when asking how a person got access (audit.md §4).
	AccountInvitedAction audit.Action = "account.invited"

	// DeliveryMail and DeliveryLink are how an invitation reached the person, as the trail records
	// it: by the installation's mail, or by a link the inviter was shown once and handed over.
	DeliveryMail = "MAIL"
	DeliveryLink = "LINK"
)

// InvitationLinks is how an installation that sends no mail hands an invitation over: the token
// is minted at once, in the invitation's own transaction, and the link answered once.
//
// The zero value mails, which is the installation with a mail server. Set from the composition
// root on exactly the condition `/meta/capabilities` answers `features.mail` false
// (environment.MailConfig.Configured), so the screen's promise and this behaviour cannot part.
type InvitationLinks struct {
	// Minter stores the token's hash and answers its plaintext. Nil mails the invitation.
	Minter RedemptionMinter
	// BaseURL is the base the mail's link names; the link answered here is that same link.
	BaseURL string
}

// RedemptionMinter mints an invitation's redemption token - MintRedemptionToken.
type RedemptionMinter interface {
	MintRedemptionToken(ctx context.Context, tenantID, accountID shared.ID) (secret.Secret, error)
}

// handsOver reports whether invitations travel as a link the inviter is shown, not by mail.
func (l InvitationLinks) handsOver() bool { return l.Minter != nil }

// Invitation is what inviting answers: the account, and - where no mail goes out - the link that
// accepts it, which exists nowhere else.
type Invitation struct {
	Account domain.Account
	// Link is empty where the invitation went by mail.
	Link secret.Secret
}

// Notifier is how an invitation reaches the person. It is the queue, not an email adapter: an
// unreachable mail server must not fail the request that invited somebody, and the job is what
// makes the send survive a restart (ADR-0008).
type Notifier interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// Authorizer is the slice of the authorisation service these use cases need.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// InviteAccountCommand is the input, typed.
type InviteAccountCommand struct {
	Email       string
	DisplayName string
}

// InviteAccount adds a person to the workspace.
//
// What it does not do is let them in. The account is created in INVITED status, so permissions can
// be arranged for it straight away and none of them work until the invitation is accepted
// (identity.md §12). Until then this is an administrator preparing a seat, and the audit trail
// records it as exactly that.
type InviteAccount struct {
	Accounts   repository.Accounts
	Authorizer Authorizer
	Notifier   Notifier
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	// Domains brings the address's domain to its ASCII form before it is stored or compared, so
	// that two spellings of one mailbox are one row (i18n-l10n.md §7).
	Domains text.DomainEncoder
	// Text brings the display name to normal form C on the way in (i18n-l10n.md §5).
	Text text.Normalizer
	// Links hands the invitation over as a link where no mail goes out; the zero value mails it.
	Links InvitationLinks
}

// Execute invites the account and returns it, with the link where it is handed over by hand.
func (h InviteAccount) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd InviteAccountCommand,
) (Invitation, error) {
	// Before the transaction: a refusal writes its own audit entry, and one written inside this
	// transaction would roll back with the refusal (audit.md §7).
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     AccountInvitedAction,
		TokenScope: membersWrite,
		TargetType: accountTarget,
	}); err != nil {
		return Invitation{}, err
	}
	if err := needsAPerson(actor, h.Links); err != nil {
		return Invitation{}, err
	}

	invited, err := domain.Invite(h.IDs.NewID(), actor.TenantID, cmd.Email, cmd.DisplayName, h.Domains, h.Text)
	if err != nil {
		return Invitation{}, err
	}

	var link secret.Secret
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		// Asked before inserting, so that the common mistake - inviting somebody who is already
		// here - is a clear answer rather than a unique constraint surfacing as a conflict.
		// The insert still carries the constraint: two administrators inviting the same address
		// at the same moment both pass this check, and only one may win.
		existing, err := h.Accounts.FindByEmail(ctx, invited.Email)
		switch {
		case err == nil:
			return alreadyHere(existing)
		case !errors.Is(err, shared.ErrNotFound):
			return err
		}

		if err := h.Accounts.Insert(ctx, invited); err != nil {
			return err
		}
		link, err = deliver(ctx, h.Links, h.Notifier, invited.TenantID, invited.ID, actor)
		if err != nil {
			return err
		}
		return h.recordAudit(ctx, invited, actor)
	})
	if err != nil {
		return Invitation{}, err
	}
	return Invitation{Account: invited, Link: link}, nil
}

// alreadyHere is check 6's answer (UC-ID-14): an address that has an account here is not invited a
// second time, and an invitation still waiting is named as one - the inviter's next step there is
// a new link or mail for it, not a second account.
func alreadyHere(existing domain.Account) error {
	code := "accounts.email_taken"
	if existing.Status == domain.AccountInvited {
		code = "accounts.already_invited"
	}
	return shared.ErrConflict.WithDetail(code).
		WithParams(map[string]string{"account_id": existing.ID.String()})
}

// needsAPerson refuses a rule's run where the invitation would travel as a link: a run reads no
// answer, so the link would reach nobody and the address would wait on an invitation nobody holds.
// With mail the run invites like anybody else.
func needsAPerson(actor appshared.ActorContext, links InvitationLinks) error {
	if links.handsOver() && !actor.RunsForRule.IsZero() {
		return shared.ErrForbidden.WithDetail("accounts.invitation_needs_a_person")
	}
	return nil
}

// deliver sends the invitation on its way, inside the caller's transaction: the mail queued, or
// the token minted and its link answered.
//
// Never both. The mail's delivery mints a token of its own on every attempt and the newest wins,
// so a mail queued beside a handed-over link would kill the link at its first retry.
func deliver(
	ctx context.Context, links InvitationLinks, notifier Notifier,
	tenantID, accountID shared.ID, actor appshared.ActorContext,
) (secret.Secret, error) {
	if !links.handsOver() {
		_, err := notifier.Enqueue(ctx, queue.Request{
			Kind:     queue.KindInvitationEmail,
			TenantID: tenantID,
			// One pending invitation message per account. A retried request must not queue a second.
			DedupeKey: accountID.String(),
			Payload: map[string]any{
				// Identifiers only. The address is in the row the job will read, and a payload is a
				// place personal data would sit unencrypted in a table nothing cleans (rule 10).
				"account_id": accountID.String(),
				"invited_by": actor.AccountID.String(),
			},
		})
		return secret.Secret{}, err
	}
	token, err := links.Minter.MintRedemptionToken(ctx, tenantID, accountID)
	if err != nil {
		return secret.Secret{}, err
	}
	link := domain.InvitationLink(links.BaseURL, token.Reveal())
	if link == "" {
		// A token minted and no link to carry it is an invitation nobody can accept: the account
		// left waiting, or the installation's base address missing - which is its operator's to fix.
		return secret.Secret{}, shared.ErrInternal.WithDetail("accounts.invitation_link_unavailable")
	}
	return secret.New(link), nil
}

// deliveryOf is the trail's word for how the invitation went out.
func deliveryOf(links InvitationLinks) string {
	if links.handsOver() {
		return DeliveryLink
	}
	return DeliveryMail
}

func (h InviteAccount) recordAudit(
	ctx context.Context, invited domain.Account, actor appshared.ActorContext,
) error {
	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   invited.TenantID,
		OccurredAt: h.Clock.Now(),
		Action:     AccountInvitedAction,
		Outcome:    audit.OutcomeSuccess,
		// Notice rather than info: somebody now has a way into this workspace, which is the class
		// of event a review looks for (audit.md §4).
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: accountTarget,
		TargetID:   invited.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			// Sensitive, so the trail carries a hash rather than the address. An auditor needs
			// to see that an invitation went somewhere and to compare two entries; neither needs
			// the address readable in a table that outlives the account (ADR-0018, audit.md §4).
			audit.Change{Field: "email", Classification: audit.Sensitive, To: invited.Email},
			audit.Change{Field: "status", Classification: audit.Open, To: string(invited.Status)},
			audit.Change{Field: "delivery", Classification: audit.Open, To: deliveryOf(h.Links)},
		),
	})
}

// Descriptor registers the use case in all three channels.
func (h InviteAccount) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: InviteAccountName,
		Summary: "Invites a person into this workspace by email address. The account is created " +
			"immediately and cannot act until the invitation is accepted, so roles may be granted " +
			"to it straight away.",
		SideEffects: "Writes the account and an audit entry, and queues the invitation message - or, where " +
			"this installation sends no mail, mints the invitation's link and answers it once instead.",
		TokenScope: membersWrite,
		Input: []usecase.Field{
			{
				Name: "email", Kind: usecase.KindString, Required: true,
				Description: "The address to invite. Unique within the workspace, compared lower case.",
			},
			{
				Name: "display_name", Kind: usecase.KindString,
				Description: "What to show beside this person's actions. Defaults to the local part of the address.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: AccountInvitedAction, TargetType: accountTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h InviteAccount) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	invitation, err := h.Execute(ctx, actor, InviteAccountCommand{
		Email:       in.String("email"),
		DisplayName: in.String("display_name"),
	})
	if err != nil {
		return nil, err
	}
	return invitationOutput(invitation), nil
}

// invitationOutput is the account, and the link where it is handed over: the one appearance of
// that link anywhere (T-18's "shown once").
func invitationOutput(invitation Invitation) usecase.Output {
	out := accountOutput(invitation.Account)
	if !invitation.Link.IsEmpty() {
		out["invitation_link"] = invitation.Link.Reveal()
	}
	return out
}

// accountOutput is the wire shape of an account, shared by every use case that returns one.
func accountOutput(account domain.Account) usecase.Output {
	out := usecase.Output{
		"id":           account.ID.String(),
		"kind":         string(account.Kind),
		"display_name": account.DisplayName,
		"status":       string(account.Status),
	}
	// Absent rather than empty: a client distinguishes "inherits the workspace default" from "set
	// to nothing", and only one of those is a thing a person chose.
	for field, value := range map[string]string{
		"email":        account.Email,
		"sign_in_name": account.SignInName,
		"locale":       account.Locale,
		"time_zone":    account.TimeZone,
		"week_start":   account.WeekStart,
	} {
		if value != "" {
			out[field] = value
		}
	}
	// The moments: a boolean and an instant, absent while the default applies - the same
	// distinction as above, kept in the types the contract gives them.
	if account.Celebrations != nil {
		out["celebrations"] = *account.Celebrations
	}
	if account.OnboardingCompletedAt != nil {
		out["onboarding_completed_at"] = *account.OnboardingCompletedAt
	}
	return out
}
