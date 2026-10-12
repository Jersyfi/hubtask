// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ConfirmAddressName = "ConfirmAddress"

	// AddressConfirmedAction is the audit code: a managed account took an address and is an
	// ordinary account from now on (ADR-0074 §6).
	AddressConfirmedAction audit.Action = "account.address_confirmed"
)

// ConfirmAddress spends the mailed link and gives the account its address (UC-ID-20 check 7).
//
// Holding the link proves the mailbox and nothing more: it signs nobody in. From then on the
// account recovers through that mailbox like any other - and a new start password no longer
// exists for it - while its sign-in name keeps working as a second identifier.
type ConfirmAddress struct {
	Requests   repository.AddressRequests
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// ConfirmAddressCommand carries the link's token and the tenant header, when sent.
type ConfirmAddressCommand struct {
	Token        string
	TenantHeader string
}

// Execute confirms. Unknown, spent, replaced, expired and foreign links are one refusal, as is a
// link whose account took an address or was erased meanwhile.
func (h ConfirmAddress) Execute(ctx context.Context, cmd ConfirmAddressCommand) error {
	token, err := domain.ParsePendingToken(cmd.Token)
	if err != nil {
		return addressLinkInvalid()
	}
	if cmd.TenantHeader != "" && cmd.TenantHeader != token.TenantID().String() {
		return shared.ErrForbidden.WithDetail("access.tenant_mismatch")
	}
	tenantID := token.TenantID()

	return h.UnitOfWork.Within(ctx, persistence.Scope{TenantID: tenantID}, func(ctx context.Context) error {
		lookup, err := h.Requests.FindByToken(ctx, token)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return addressLinkInvalid()
			}
			return err
		}
		now := h.Clock.Now()
		if lookup.Credential.Purpose != domain.PendingAddress || lookup.Credential.Verify(now) != nil {
			return addressLinkInvalid()
		}
		if err := lookup.TenantStatus.Verify(); err != nil {
			return err
		}
		spent, err := h.Requests.Consume(ctx, lookup.Credential.ID, now)
		if err != nil {
			return err
		}
		if !spent {
			return addressLinkInvalid()
		}
		// An address another account holds is refused here by the database; the transaction rolls
		// back, and the link stays for once the address is free.
		confirmed, err := h.Requests.Confirm(ctx, lookup.Credential.AccountID, lookup.Address, now)
		if err != nil {
			return err
		}
		if !confirmed {
			return addressLinkInvalid()
		}
		return h.Audit.Append(ctx, audit.Entry{
			TenantID:   tenantID,
			OccurredAt: now,
			Action:     AddressConfirmedAction,
			Outcome:    audit.OutcomeSuccess,
			Severity:   audit.SeverityNotice,
			// The mailbox's holder acts for the account the link names; nobody is signed in.
			ActorKind:  shared.ActorUser,
			ActorID:    lookup.Credential.AccountID,
			TargetType: accountTarget,
			TargetID:   lookup.Credential.AccountID,
			Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(
				audit.Change{Field: "email", Classification: audit.Sensitive, To: lookup.Address},
			),
		})
	})
}

// addressLinkInvalid is the confirmation's one probe-facing refusal.
func addressLinkInvalid() error {
	return shared.ErrUnauthenticated.WithDetail("auth.address_link_invalid")
}

// Descriptor registers the use case in all three channels.
func (h ConfirmAddress) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ConfirmAddressName,
		Summary: "Spends the link mailed to a managed account's new address and makes the address " +
			"the account's: from then on it is an ordinary account that keeps its sign-in name. " +
			"The link proves the mailbox and signs nobody in; it works once and for a day, and " +
			"unknown, spent, replaced and expired links are one refusal.",
		SideEffects: "Spends the link, writes the address onto the account, and writes an audit entry.",
		Input: []usecase.Field{
			{
				Name: "token", Kind: usecase.KindString, Required: true,
				Description: "The token from the confirmation mail. It dies on use.",
			},
			{
				Name: "tenant_header", Kind: usecase.KindString,
				Description: "The X-Hubtask-Tenant header, when sent. It may confirm the " +
					"token's tenant, never overrule it.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: AddressConfirmedAction, TargetType: accountTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "An address is not an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ConfirmAddress) invoke(
	ctx context.Context, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	if err := h.Execute(ctx, ConfirmAddressCommand{
		Token: in.String("token"), TenantHeader: in.String("tenant_header"),
	}); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}
