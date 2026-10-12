// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"math"
	"strconv"
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
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	RequestAddressName = "RequestAddress"

	// AddressRequestedAction is the audit code: a link to confirm an address went out for a
	// managed account. Nothing on the account changed yet.
	AddressRequestedAction audit.Action = "account.address_requested"
)

// RequestAddress sends a managed account's new address its confirmation (UC-ID-20 check 7,
// ADR-0074 §6). The person may ask for themselves, and so may whoever manages the workspace's
// people; the address becomes the account's only when somebody holding that mailbox follows the
// link (ConfirmAddress).
//
// Whether the address already belongs to somebody here is not said: a managed person could
// otherwise learn which addresses have accounts, which a member's view of another deliberately
// withholds. The confirmation answers it, to whoever holds the mailbox.
type RequestAddress struct {
	Requests   repository.AddressRequests
	Authorizer Authorizer
	Notifier   Notifier
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	// Domains brings the address's domain to its ASCII form, an invitation's rule.
	Domains text.DomainEncoder
	// Door answers whether the password is a way into the workspace. Where it is not, an address
	// would take the person's one way in: an account found by name that holds an address is refused
	// the password (identity.md §4.1), and no provider need know the address (§13). Nil reads as
	// open, as a writer without a rule does.
	Door PasswordDoor
}

// Execute writes the request and queues its mail, answering until when the link confirms.
func (h RequestAddress) Execute(
	ctx context.Context, actor appshared.ActorContext, accountID shared.ID, email string,
) (time.Time, error) {
	if err := h.authorize(ctx, actor, accountID); err != nil {
		return time.Time{}, err
	}
	address, err := domain.NewAddress(email, h.Domains)
	if err != nil {
		return time.Time{}, err
	}
	// Read before the transaction: the rule's resolution runs its own, and a nested one is what the
	// unit of work refuses.
	passwordOpen := true
	if h.Door != nil {
		open, _, err := h.Door.PasswordOpen(ctx, actor.TenantID)
		if err != nil {
			return time.Time{}, err
		}
		passwordOpen = open
	}

	var expires time.Time
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		// The account's row is held from here on: a second request for it waits, and then reads
		// the first one's moment.
		account, err := h.Requests.LockAccount(ctx, accountID)
		if err != nil {
			return err
		}
		if !account.OnlyThePasswordReaches() {
			return notManaged()
		}
		if !passwordOpen {
			return shared.ErrConflict.WithDetail("accounts.address_needs_password")
		}
		now := h.Clock.Now()
		latest, err := h.Requests.LatestRequest(ctx, accountID)
		if err != nil {
			return err
		}
		if wait := domain.AddressRequestWait(latest, now); wait > 0 {
			return shared.ErrRateLimited.WithDetail("accounts.address_requested_recently").
				WithParams(map[string]string{
					"retry_after_seconds": strconv.Itoa(int(math.Ceil(wait.Seconds()))),
				})
		}
		if err := h.Requests.Supersede(ctx, accountID, now); err != nil {
			return err
		}
		request := domain.PendingCredential{
			ID: h.IDs.NewID(), TenantID: actor.TenantID, AccountID: accountID,
			Purpose: domain.PendingAddress, CreatedAt: now.UTC(),
			ExpiresAt: now.Add(domain.AddressLifetime).UTC(),
		}
		if err := h.Requests.Insert(ctx, request, address); err != nil {
			return err
		}
		if _, err := h.Notifier.Enqueue(ctx, queue.Request{
			Kind:     queue.KindAddressConfirmationEmail,
			TenantID: actor.TenantID,
			// One job per request: an earlier job still waiting finds its request superseded and
			// sends nothing, so keying on the account would drop the newest mail instead.
			DedupeKey: request.ID.String(),
			// Identifiers only (rule 10): the address is on the request's row.
			Payload: map[string]any{"request_id": request.ID.String()},
		}); err != nil {
			return err
		}
		expires = request.ExpiresAt
		return h.recordAudit(ctx, actor, accountID, address, now)
	})
	if err != nil {
		return time.Time{}, err
	}
	return expires, nil
}

// authorize lets a person ask for their own account, and asks the workspace's people permission of
// anybody asking for somebody else's.
func (h RequestAddress) authorize(ctx context.Context, actor appshared.ActorContext, accountID shared.ID) error {
	if !actor.AccountID.IsZero() && accountID == actor.AccountID {
		return actor.RequireScope(accountsWrite)
	}
	return h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     AddressRequestedAction,
		TokenScope: membersWrite,
		TargetType: accountTarget,
		TargetID:   accountID,
	})
}

func (h RequestAddress) recordAudit(
	ctx context.Context, actor appshared.ActorContext, accountID shared.ID, address string, at time.Time,
) error {
	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: at,
		Action:     AddressRequestedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: accountTarget,
		TargetID:   accountID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			// Sensitive, as an invitation's address is.
			audit.Change{Field: "email", Classification: audit.Sensitive, To: address},
		),
	})
}

// Descriptor registers the use case in all three channels.
func (h RequestAddress) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RequestAddressName,
		Summary: "Sends a link to a mail address that a managed account - a sign-in name and no " +
			"address - is to take. Nothing changes until somebody holding that mailbox follows the " +
			"link; then the account is an ordinary one and keeps its sign-in name. The person may " +
			"ask for themselves, and whoever manages the workspace's people for anybody. Refused " +
			"for an account that has an address; a new request replaces the earlier link, and one " +
			"within ten minutes of the last is refused.",
		SideEffects: "Writes the request and queues the confirmation mail; writes an audit entry.",
		TokenScope:  accountsWrite,
		Input: []usecase.Field{
			{
				Name: "account_id", Kind: usecase.KindID, Required: true,
				Description: "The managed account; the caller's own, or anybody's for an administrator.",
			},
			{
				Name: "email", Kind: usecase.KindString, Required: true,
				Description: "The address to confirm.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: AddressRequestedAction, TargetType: accountTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "An address is not an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RequestAddress) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	accountID, err := in.ID("account_id")
	if err != nil {
		return nil, err
	}
	expires, err := h.Execute(ctx, actor, accountID, in.String("email"))
	if err != nil {
		return nil, err
	}
	return usecase.Output{"expires_at": expires}, nil
}
