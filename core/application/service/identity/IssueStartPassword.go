// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
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
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const (
	IssueStartPasswordName = "IssueStartPassword"

	// StartPasswordIssuedAction is the audit code: somebody else replaced this person's password
	// (ADR-0074 §4). A warning, as every password change is; the start password is nowhere in it.
	StartPasswordIssuedAction audit.Action = "account.start_password_issued"
)

// SessionEnder ends every session of an account - the one method of the session store a renewal
// needs.
type SessionEnder interface {
	RevokeAll(ctx context.Context, accountID shared.ID, at time.Time) (int, error)
}

// StartPasswordIssued is the account and its new start password - shown here and never again.
type StartPasswordIssued struct {
	Account       domain.Account
	StartPassword secret.Secret
}

// IssueStartPassword helps a managed person back in who forgot their password (UC-ID-20 check 5):
// the one exception to "an administrator does not recover an account", and only for an account no
// mailbox could recover (ADR-0074 §4, NG-weaker-recovery).
//
// A fresh proof of the administrator, the trail, every session of the account ended and every
// unfinished sign-in step spent; the second factor and the recovery codes stay, so the next sign-in
// still asks for them before it asks for a new password. Personal access tokens stay, as a password
// change leaves them: the renewal answers a forgotten password, not a stolen token. The attempt
// ledger is not cleared - a guesser must not have it reset through an administrator.
type IssueStartPassword struct {
	Accounts       repository.Accounts
	StartPasswords repository.StartPasswords
	Sessions       SessionEnder
	Authorizer     Authorizer
	StepUp         stepup.Verifier
	Passwords      cryptoport.PasswordHasher
	Entropy        clock.Entropy
	Audit          audit.Sink
	UnitOfWork     persistence.UnitOfWork
	Clock          clock.Clock
}

// Execute renews the start password and answers it once.
func (h IssueStartPassword) Execute(
	ctx context.Context, actor appshared.ActorContext, accountID shared.ID, stepUpToken string,
) (StartPasswordIssued, error) {
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     StartPasswordIssuedAction,
		TokenScope: membersWrite,
		TargetType: accountTarget,
		TargetID:   accountID,
	}); err != nil {
		return StartPasswordIssued{}, err
	}
	// A rule's run reads no answer: the new start password would reach nobody, and the person
	// would be locked out by the very act meant to let them back in.
	if !actor.RunsForRule.IsZero() {
		return StartPasswordIssued{}, shared.ErrForbidden.WithDetail("accounts.managed_needs_a_person")
	}

	var account domain.Account
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		read, err := h.Accounts.Find(ctx, accountID)
		account = read
		return err
	})
	if err != nil {
		return StartPasswordIssued{}, err
	}
	// Before the proof is demanded, so that a refused target does not burn it.
	if !account.OnlyThePasswordReaches() {
		return StartPasswordIssued{}, notManaged()
	}
	if err := stepup.Demand(ctx, h.StepUp, actor.TenantID, actor.AccountID, stepUpToken); err != nil {
		return StartPasswordIssued{}, err
	}

	start, hash, err := drawStartPassword(h.Entropy, h.Passwords)
	if err != nil {
		return StartPasswordIssued{}, err
	}

	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := h.Clock.Now()
		renewed, err := h.StartPasswords.IssueStartPassword(ctx, account.ID, hash, now)
		if err != nil {
			return err
		}
		if !renewed {
			// An address was confirmed between the read and this write: the account recovers
			// through its mailbox now.
			return notManaged()
		}
		ended, err := h.Sessions.RevokeAll(ctx, account.ID, now)
		if err != nil {
			return err
		}
		if _, err := h.StartPasswords.SpendPending(ctx, account.ID, now); err != nil {
			return err
		}
		return h.recordAudit(ctx, actor, account, ended, now)
	})
	if err != nil {
		return StartPasswordIssued{}, err
	}
	return StartPasswordIssued{Account: account, StartPassword: start}, nil
}

// notManaged is the answer for every account a new start password does not exist for: one with an
// address recovers through its mailbox (NG-weaker-recovery), and nobody else signs in by name.
func notManaged() error {
	return shared.ErrConflict.WithDetail("accounts.not_managed")
}

func (h IssueStartPassword) recordAudit(
	ctx context.Context, actor appshared.ActorContext, account domain.Account, ended int, at time.Time,
) error {
	return h.Audit.Append(ctx, audit.Entry{
		// The actor's: the account read by identifier carries no tenant of its own.
		TenantID:   actor.TenantID,
		OccurredAt: at,
		Action:     StartPasswordIssuedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityWarning,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: accountTarget,
		TargetID:   account.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "sessions_ended", Classification: audit.Open, To: strconv.Itoa(ended)},
		),
	})
}

// Descriptor registers the use case in all three channels.
func (h IssueStartPassword) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: IssueStartPasswordName,
		Summary: "Gives a managed account - a sign-in name and no mail address - a new start " +
			"password when its person forgot theirs, answered once. Every session of the account " +
			"ends; the next sign-in asks for the second factor where one is set up, then for a new " +
			"password. Refused for any account with an address, which recovers through its mailbox, " +
			"and for an act an automation rule runs, since nobody would receive the password.",
		SideEffects: "Replaces the password's hash, ends every session and unfinished sign-in of " +
			"the account, and writes an audit entry. Nothing keeps the start password but its hash.",
		TokenScope: membersWrite,
		Input: []usecase.Field{
			{
				Name: "account_id", Kind: usecase.KindID, Required: true,
				Description: "The managed account.",
			},
			{
				// Declared but not required, ChangePassword's reasoning: a caller without a proof
				// needs the 403 that names the methods rather than a 422 about a missing field.
				Name: "step_up_token", Kind: usecase.KindString,
				Description: "The fresh proof this demands. It is consumed by this call.",
			},
		},
		StepUp: "every new start password",
		Audit: usecase.AuditDeclaration{
			Action: StartPasswordIssuedAction, TargetType: accountTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A password is not an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h IssueStartPassword) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	accountID, err := in.ID("account_id")
	if err != nil {
		return nil, err
	}
	issued, err := h.Execute(ctx, actor, accountID, in.String("step_up_token"))
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"account":        accountOutput(issued.Account),
		"start_password": issued.StartPassword.Reveal(),
	}, nil
}
