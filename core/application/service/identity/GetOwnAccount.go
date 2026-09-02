// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	GetOwnAccountName = "GetOwnAccount"

	// AccountReadAction is the audit code of an attempted read. Declared even though an ordinary
	// read writes no entry: a *refused* read does, and it is recorded against the action that was
	// refused rather than against a generic "denied" (audit.md §4).
	AccountReadAction audit.Action = "account.read"
)

// GetOwnAccount answers "who am I", which is the one question about an account nobody could ask.
//
// `/accounts:invite` creates an account and `/accounts/{accountId}/preferences` writes to one, and
// between them a client never learns its own identifier - so the binding requirement that locale
// and time zone come from the account preference (`i18n-l10n.md` §2) had no way to be honoured.
// This is that read, and nothing more.
//
// Read-only throughout: the transaction may be served by a read replica (`multi-tenancy.md` §7),
// and a read that opened a write transaction would pin every sign-in to the primary.
type GetOwnAccount struct {
	Accounts   repository.Accounts
	UnitOfWork persistence.UnitOfWork
}

// Execute returns the account of the authenticated actor.
//
// There is no permission check and that is the decision, not an omission. Reading one's own
// account is not administering anybody: requiring the member-management permission for it would
// mean a viewer could not discover their own time zone, and there is nothing here to authorise
// against - the row is the caller's by definition. What *is* checked is the token scope, because a
// credential is a bound on its holder rather than a second identity (ADR-0005): a token minted
// without `accounts:read` may not read this, whoever holds it.
//
// The tenant boundary needs no check either, for a stronger reason than convenience: the actor's
// account id and the transaction's tenant come from the same authenticated context, and the row is
// found through the transaction wrapper that sets `app.tenant_id` (ADR-0010). An actor of another
// tenant does not resolve here because row level security does not return the row, which is what
// the cross-tenant test asserts rather than assumes.
func (h GetOwnAccount) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (domain.Account, error) {
	if err := actor.RequireScope(accountsRead); err != nil {
		return domain.Account{}, err
	}
	if actor.AccountID.IsZero() {
		// The system itself acts without an account - a scheduler run, a job. It has no "me" to
		// answer with, and inventing one would be worse than saying so.
		return domain.Account{}, shared.ErrForbidden.WithDetail("accounts.self_required")
	}

	var account domain.Account
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Accounts.Find(ctx, actor.AccountID)
		if err != nil {
			return err
		}
		account = found
		return nil
	})
	if err != nil {
		return domain.Account{}, err
	}
	return account, nil
}

// Descriptor registers the use case in all three channels.
func (h GetOwnAccount) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: GetOwnAccountName,
		Summary: "Reads the account the caller is signed in as, with the locale, time zone and " +
			"first day of the week it should be spoken to in. A service account gets the same " +
			"document rather than a refusal - it has an account row like anybody else.",
		SideEffects: "None. Reads only.",
		TokenScope:  accountsRead,
		ReadOnly:    true,
		// No input at all: the actor *is* the identifier, and a field for it would be a field a
		// caller could get wrong. Reading somebody else's account is a different use case with a
		// different permission, and it is not this one.
		Input: nil,
		Audit: usecase.AuditDeclaration{
			Action: AccountReadAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h GetOwnAccount) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	account, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	return accountOutput(account), nil
}
