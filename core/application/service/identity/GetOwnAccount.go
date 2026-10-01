// SPDX-License-Identifier: BUSL-1.1
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
	// Recovery answers how many of the ten escape hatches are left (SI-09). The contract has
	// carried that number at sign-in since H-02 and no client had ever read it, which is the
	// smaller half of the problem - the larger half was that it was answered at the one moment
	// nobody can act on it. Here it is beside the account, where the screen that makes new ones is.
	// Nil answers nothing, which is what an installation wired without the second factor does.
	Recovery repository.RecoveryCodes
	// Enrollments answers the one question no read in the contract answered: whether this account
	// holds a second factor at all.
	//
	// The security screen said so in its own comment and worked around it - it offered enrolment
	// and let the server refuse one that was already armed. That is survivable for enrolment and
	// wrong for everything beside it. Without this, a screen cannot tell an account with no
	// authenticator from one whose codes have all been spent, so it showed somebody with no
	// authenticator a red "none left" and offered them two actions the server would refuse.
	Enrollments repository.MfaEnrollments
	// Password answers whether the account holds a password at all (UC-ID-05 check 5): one that
	// signs in only through a provider has none, and a screen offering to change it would offer a
	// control the server cannot honour. Nil answers true, the shape of every account before
	// providers existed.
	Password PasswordHolder
}

// PasswordHolder answers whether the signed-in account holds a password.
type PasswordHolder interface {
	HasPassword(ctx context.Context, actor appshared.ActorContext) (bool, error)
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
// OwnAccount is the account plus the two things about it that are not on the row: whether a second
// factor is armed, and how many recovery codes are left. Two further values rather than fields on
// the domain type, because both are projections of other tables and an account is not the place to
// cache one.
type OwnAccount struct {
	Account domain.Account
	// HasSecondFactor is whether an *armed* enrolment stands. An enrolment begun and never
	// confirmed protects nobody and locks nobody out, so it counts as none here - the same reading
	// the sign-in path takes.
	HasSecondFactor bool
	// HasPassword is whether the account holds a password, so that nothing offers to change one or
	// asks for one as a proof where there is none.
	HasPassword bool
	// RecoveryCodesRemaining is -1 where there is nothing to count: an installation wired without
	// the second factor, or an account that holds none. Its codes are not "zero left", they are a
	// thing that does not exist yet, and a screen told zero sends somebody to make codes the server
	// would refuse to make. Where a factor is armed, zero is answered as zero, because there zero
	// is the number to act on.
	RecoveryCodesRemaining int
}

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

// ExecuteWithRecovery is Execute plus the count. Two methods rather than one changed signature,
// because every other caller of Execute wants the account and nothing else.
func (h GetOwnAccount) ExecuteWithRecovery(
	ctx context.Context, actor appshared.ActorContext,
) (OwnAccount, error) {
	account, err := h.Execute(ctx, actor)
	if err != nil {
		return OwnAccount{}, err
	}

	answer := OwnAccount{Account: account, RecoveryCodesRemaining: -1, HasPassword: true}
	if h.Password != nil {
		held, err := h.Password.HasPassword(ctx, actor)
		if err != nil {
			return OwnAccount{}, err
		}
		answer.HasPassword = held
	}
	if h.Recovery == nil || h.Enrollments == nil {
		return answer, nil
	}
	err = h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		enrollment, err := h.Enrollments.Find(ctx, actor.AccountID)
		switch {
		case errors.Is(err, shared.ErrNotFound):
			return nil
		case err != nil:
			return err
		case enrollment.ConfirmedAt.IsZero():
			// Begun and not armed. The codes shown on the enrolment screen are still the ones
			// that count, and this screen has nothing to say about them yet.
			return nil
		}
		answer.HasSecondFactor = true

		remaining, err := h.Recovery.Remaining(ctx, actor.AccountID)
		if err != nil {
			return err
		}
		answer.RecoveryCodesRemaining = remaining
		return nil
	})
	if err != nil {
		return OwnAccount{}, err
	}
	return answer, nil
}

func (h GetOwnAccount) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	own, err := h.ExecuteWithRecovery(ctx, actor)
	if err != nil {
		return nil, err
	}
	out := accountOutput(own.Account)
	out["has_second_factor"] = own.HasSecondFactor
	out["has_password"] = own.HasPassword
	if own.RecoveryCodesRemaining >= 0 {
		out["recovery_codes_remaining"] = own.RecoveryCodesRemaining
	}
	return out, nil
}
