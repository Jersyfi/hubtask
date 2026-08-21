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
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	InviteAccountName = "InviteAccount"
	accountTarget     = "account"
	membersWrite      = "members:write"

	// AccountInvitedAction is the audit code. Adding somebody to a workspace is the event an
	// auditor looks for first when asking how a person got access (audit.md §2).
	AccountInvitedAction audit.Action = "account.invited"
)

// Notifier is how an invitation reaches the person. It is the queue, not an email adapter: an
// unreachable mail server must not fail the request that invited somebody, and the job is what
// makes the send survive a restart (ADR-0008).
type Notifier interface {
	Enqueue(ctx context.Context, request queue.Request) error
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
// be arranged for it straight away and none of them work until the invitation is accepted - which
// needs the sign-in flow and arrives with it (security.md §5). Until then this is an administrator
// preparing a seat, and the audit trail records it as exactly that.
type InviteAccount struct {
	Accounts   repository.Accounts
	Authorizer Authorizer
	Notifier   Notifier
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// Execute invites the account and returns it.
func (h InviteAccount) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd InviteAccountCommand,
) (domain.Account, error) {
	// Before the transaction: a refusal writes its own audit entry, and one written inside this
	// transaction would roll back with the refusal (audit.md §7).
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     AccountInvitedAction,
		TokenScope: membersWrite,
		TargetType: accountTarget,
	}); err != nil {
		return domain.Account{}, err
	}

	invited, err := domain.Invite(h.IDs.NewID(), actor.TenantID, cmd.Email, cmd.DisplayName)
	if err != nil {
		return domain.Account{}, err
	}

	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		// Asked before inserting, so that the common mistake - inviting somebody who is already
		// here - is a clear answer rather than a unique constraint surfacing as a conflict.
		// The insert still carries the constraint: two administrators inviting the same address
		// at the same moment both pass this check, and only one may win.
		existing, err := h.Accounts.FindByEmail(ctx, invited.Email)
		switch {
		case err == nil:
			return shared.ErrConflict.
				WithDetail("accounts.email_taken").
				WithParams(map[string]string{"account_id": existing.ID.String()})
		case !errors.Is(err, shared.ErrNotFound):
			return err
		}

		if err := h.Accounts.Insert(ctx, invited); err != nil {
			return err
		}
		if err := h.notify(ctx, invited, actor); err != nil {
			return err
		}
		return h.recordAudit(ctx, invited, actor)
	})
	if err != nil {
		return domain.Account{}, err
	}
	return invited, nil
}

// notify queues the invitation message inside the same transaction as the account.
//
// The two belong together: an invitation nobody was told about is a seat that stays empty, and a
// message about an account that was never created is worse. The queue is transactional for exactly
// this (ADR-0008), and the delivery itself is somebody else's problem at somebody else's time.
func (h InviteAccount) notify(
	ctx context.Context, invited domain.Account, actor appshared.ActorContext,
) error {
	return h.Notifier.Enqueue(ctx, queue.Request{
		Kind:     queue.KindInvitationEmail,
		TenantID: invited.TenantID,
		// One pending invitation message per account. A retried request must not queue a second.
		DedupeKey: invited.ID.String(),
		Payload: map[string]any{
			// Identifiers only. The address is in the row the job will read, and a payload is a
			// place personal data would sit unencrypted in a table nothing cleans (rule 10).
			"account_id": invited.ID.String(),
			"invited_by": actor.AccountID.String(),
		},
	})
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
		// of event a review looks for (audit.md §2).
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
			// the address readable in a table that outlives the account (ADR-0018, audit.md §2).
			audit.Change{Field: "email", Classification: audit.Sensitive, To: invited.Email},
			audit.Change{Field: "status", Classification: audit.Open, To: string(invited.Status)},
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
		SideEffects: "Writes the account, queues the invitation message, and writes an audit entry.",
		TokenScope:  membersWrite,
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
	account, err := h.Execute(ctx, actor, InviteAccountCommand{
		Email:       in.String("email"),
		DisplayName: in.String("display_name"),
	})
	if err != nil {
		return nil, err
	}
	return accountOutput(account), nil
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
		"email":      account.Email,
		"locale":     account.Locale,
		"time_zone":  account.TimeZone,
		"week_start": account.WeekStart,
	} {
		if value != "" {
			out[field] = value
		}
	}
	return out
}
