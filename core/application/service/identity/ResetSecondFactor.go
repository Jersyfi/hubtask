// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
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
	"github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ResetSecondFactorName = "ResetSecondFactor"

	// SecondFactorResetAction is the audit code: somebody else removed this person's authenticator
	// app and recovery codes (fifth answers 6). A warning, as every change to a way in made by
	// another person is.
	SecondFactorResetAction audit.Action = "account.second_factor_reset"
)

// PendingSpender spends every unfinished sign-in step of an account.
type PendingSpender interface {
	SpendPending(ctx context.Context, accountID shared.ID, at time.Time) (int, error)
}

// ResetSecondFactor lets a person back in who lost both their authenticator app and their
// recovery codes (fifth answers 6, identity.md §9): an administrator removes the app and the codes,
// and the next sign-in leads into enrolling a new one wherever the workspace demands a factor - no
// sign-in skips the factor (P-02), it only starts it again.
//
// The same holds as for a new start password: `MANAGE_MEMBERS`, the administrator's own fresh
// proof, the role ceiling (an administrator never resets an owner, so an owner's factor is reset
// only by another owner), and never one's own - that is DisableTotp, behind one's own proof.
// Every session and unfinished sign-in of the account ends, the person is mailed, the trail holds
// it. Passkeys stay: they are not what was lost, and the person removes one from their own
// settings.
type ResetSecondFactor struct {
	Accounts    repository.Accounts
	Enrollments repository.MfaEnrollments
	Recovery    repository.RecoveryCodes
	// Keys holds the account's row while the factor goes, as DisableTotp does. Nil holds nothing.
	Keys       FactorKeys
	Sessions   SessionEnder
	Pending    PendingSpender
	Authorizer Authorizer
	Held       repository.HeldRoles
	StepUp     stepup.Verifier
	// Notices mails the person (ADR-0078 §6). Nil where no mail goes out: then only audited.
	Notices    WayInNotices
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Execute resets the account's second factor.
func (h ResetSecondFactor) Execute(
	ctx context.Context, actor appshared.ActorContext, accountID shared.ID, stepUpToken string,
) error {
	request := access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     SecondFactorResetAction,
		TokenScope: membersWrite,
		TargetType: accountTarget,
		TargetID:   accountID,
	}
	if err := h.Authorizer.Authorize(ctx, actor, request); err != nil {
		return err
	}
	// A rule's run is nobody who could be asked for the proof, and taking away somebody's factor
	// is not an act to schedule.
	if !actor.RunsForRule.IsZero() {
		return shared.ErrForbidden.WithDetail("accounts.factor_reset_needs_a_person")
	}
	if accountID == actor.AccountID {
		return shared.ErrForbidden.WithDetail("accounts.factor_reset_own")
	}

	ceiling := wayInCeiling{Held: h.Held, Authorizer: h.Authorizer}
	var account domain.Account
	var held []domain.Role
	var enrolled bool
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		read, err := h.Accounts.Find(ctx, accountID)
		if err != nil {
			return err
		}
		account = read
		if held, err = ceiling.heldBy(ctx, accountID); err != nil {
			return err
		}
		enrolled, err = h.hasApp(ctx, accountID)
		return err
	})
	if err != nil {
		return err
	}
	// All before the proof is demanded, so that a refused target does not burn it.
	if account.Kind == domain.AccountServiceAccount {
		return shared.ErrConflict.WithDetail("auth.mfa_not_enrolled")
	}
	if !enrolled {
		return shared.ErrConflict.WithDetail("auth.mfa_not_enrolled")
	}
	if err := ceiling.hold(ctx, actor, request, held); err != nil {
		return err
	}
	if err := stepup.Demand(ctx, h.StepUp, actor.TenantID, actor.AccountID, stepUpToken); err != nil {
		return err
	}

	var at time.Time
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if h.Keys != nil {
			if err := h.Keys.LockAccount(ctx, account.ID); err != nil {
				return err
			}
		}
		at = h.Clock.Now()
		removed, err := h.Enrollments.Disable(ctx, account.ID)
		if err != nil {
			return err
		}
		if !removed {
			// The person removed it themselves meanwhile.
			return shared.ErrConflict.WithDetail("auth.mfa_not_enrolled")
		}
		// Every code goes, also where a passkey would back them: the codes are what was lost, and
		// whoever finds them must not hold a way past the factor.
		if err := h.Recovery.Replace(ctx, account.ID, nil, nil, at); err != nil {
			return err
		}
		ended, err := h.Sessions.RevokeAll(ctx, account.ID, at)
		if err != nil {
			return err
		}
		if _, err := h.Pending.SpendPending(ctx, account.ID, at); err != nil {
			return err
		}
		return h.recordAudit(ctx, actor, account, ended, at)
	})
	if err != nil {
		return err
	}
	if h.Notices != nil {
		// After the commit: a mail about a reset that rolled back would tell of nothing. A notice
		// that cannot be queued does not undo the reset; the trail holds it.
		_ = h.Notices.WayInChanged(ctx, actor.TenantID, account.ID,
			domain.WayInChange{Event: domain.WayInSecondFactorReset, At: at})
	}
	return nil
}

// hasApp is whether an armed authenticator app stands: an enrolment begun and never confirmed is
// no factor, and resetting it would tell the person of a change that changed nothing.
func (h ResetSecondFactor) hasApp(ctx context.Context, accountID shared.ID) (bool, error) {
	enrollment, err := h.Enrollments.Find(ctx, accountID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return !enrollment.ConfirmedAt.IsZero(), nil
}

func (h ResetSecondFactor) recordAudit(
	ctx context.Context, actor appshared.ActorContext, account domain.Account, ended int, at time.Time,
) error {
	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: at,
		Action:     SecondFactorResetAction,
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
func (h ResetSecondFactor) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ResetSecondFactorName,
		Permission: service.PermissionManageMembers,
		Summary: "Resets a person's second factor when they lost both their authenticator app and " +
			"their recovery codes: the app and every code are removed, every session of the account " +
			"ends, and the next sign-in leads into setting up a new app wherever the workspace " +
			"requires a factor. Held to the role ceiling, so an owner's factor is reset only by " +
			"another owner; never one's own, and never by an automation rule.",
		SideEffects: "Removes the authenticator app and the recovery codes, ends every session and " +
			"unfinished sign-in of the account, mails the person and writes an audit entry. " +
			"Passkeys stay.",
		TokenScope: membersWrite,
		Input: []usecase.Field{
			{
				Name: "account_id", Kind: usecase.KindID, Required: true,
				Description: "The person whose second factor is reset.",
			},
			{
				Name: "step_up_token", Kind: usecase.KindString,
				Description: "The fresh proof this demands. It is consumed by this call.",
			},
		},
		StepUp: "every reset of somebody's second factor",
		Audit: usecase.AuditDeclaration{
			Action: SecondFactorResetAction, TargetType: accountTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A second factor is not an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ResetSecondFactor) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	accountID, err := in.ID("account_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, accountID, in.String("step_up_token")); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}
