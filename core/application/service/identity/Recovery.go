// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	stepupport "github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// Recovery codes are the account's, not the factor's (milestone decision 5, SI-09).
//
// H-02 shipped them as part of the enrolment: ten codes shown once, replaced only by enrolling
// again - which meant disabling the factor first, with the password, and re-arming an authenticator
// that was working. Somebody who has burned eight of ten had no way to get ten back that did not
// involve taking their own second factor off for a minute.
//
// So they move. Their own route, their own audit action, and a count that is answered where somebody
// can act on it. The shape is also what makes them still right when a passkey is the second factor:
// the codes are the fallback for whatever the account holds, and nothing about them mentions TOTP.

const RegenerateRecoveryCodesName = "RegenerateRecoveryCodes"

// RecoveryRegeneratedAction is a fresh set. Notice rather than info: the old ten stopped working at
// that moment, and somebody reading the trail after being locked out needs to see it.
//
// Named in the second factor's family since SC-29; entries written before keep
// `mfa.recovery_regenerated`, and the trail finds both (audit.Renamed).
const RecoveryRegeneratedAction audit.Action = "auth.mfa_recovery_regenerated"

// RegenerateRecoveryCodes answers `POST /auth/mfa/recovery:regenerate`.
type RegenerateRecoveryCodes struct{ Writer SessionWriter }

// RegeneratedCodes is the single showing.
type RegeneratedCodes struct{ Codes []secret.Secret }

// Execute burns the old set and answers a new one.
//
// One transaction, because the two halves are one act: a set answered without the old one burned
// would be twenty live codes, and a set burned without a new one answered would lock somebody out of
// their own escape hatch.
func (h RegenerateRecoveryCodes) Execute(
	ctx context.Context, actor appshared.ActorContext, stepUpToken string,
) (RegeneratedCodes, error) {
	w := h.Writer
	if !actor.IsAuthenticated() || actor.AccountID.IsZero() {
		return RegeneratedCodes{}, shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	if w.Enrollments == nil || w.Recovery == nil {
		return RegeneratedCodes{}, shared.ErrNotFound.WithDetail("route.operation_not_available")
	}

	// The step-up, for the reason disabling the factor demands one: a fresh set of codes is a fresh
	// set of ways in, and a stolen tab must not be able to mint them.
	if err := w.requireStepUp(ctx, actor, stepUpToken); err != nil {
		return RegeneratedCodes{}, err
	}

	var account domain.Account
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := w.People.Find(ctx, actor.AccountID)
		account = found
		return err
	})
	if err != nil {
		return RegeneratedCodes{}, err
	}

	material, err := w.Entropy.Bytes(domain.RecoveryCodeCount * domain.RecoveryCodeBytes)
	if err != nil {
		return RegeneratedCodes{}, shared.ErrInternal.
			WithDetail("auth.session_unmintable").WithCause(err)
	}
	codes, err := domain.NewRecoveryCodes(material)
	if err != nil {
		return RegeneratedCodes{}, err
	}

	scope := persistence.Scope{TenantID: actor.TenantID, ActorID: actor.AccountID}
	err = w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		enrollment, err := w.Enrollments.Find(ctx, actor.AccountID)
		if err != nil {
			return err
		}
		if enrollment.ConfirmedAt.IsZero() {
			// An unconfirmed enrolment protects nobody, and its codes are the ones the enrolment
			// screen is still showing. Replacing them here would leave that screen lying.
			return shared.ErrConflict.WithDetail("auth.mfa_enrollment_pending")
		}

		now := w.Clock.Now()
		ids := make([]shared.ID, 0, len(codes))
		for range codes {
			ids = append(ids, w.IDs.NewID())
		}
		// Replace is one statement: the old set is gone in the same moment the new one lands.
		if err := w.Recovery.Replace(ctx, actor.AccountID, ids, codes, now); err != nil {
			return err
		}
		return w.recordMfaAudit(ctx, RecoveryRegeneratedAction, audit.SeverityNotice, account, now)
	})
	if err != nil {
		return RegeneratedCodes{}, err
	}

	answer := RegeneratedCodes{Codes: make([]secret.Secret, 0, len(codes))}
	for _, code := range codes {
		answer.Codes = append(answer.Codes, secret.New(code))
	}
	return answer, nil
}

// requireStepUp is the proof a privileged MFA operation demands. Its own method because two use
// cases now ask for it in the same words.
func (w SessionWriter) requireStepUp(
	ctx context.Context, actor appshared.ActorContext, token string,
) error {
	// Through the port's own Demand rather than by hand. Demand is what puts `params.methods` on
	// the refusal - the password where the account holds one, the code where a factor is armed -
	// and the contract promises that list at `POST /auth/step-up`: "a client builds its prompt from
	// that list, never from a guess". Written out here, the list was missing, so every client fell
	// back to the password and an account with an authenticator was asked for the wrong thing.
	return stepupport.Demand(
		ctx, StepUpVerifier{Writer: w}, actor.TenantID, actor.AccountID, token)
}

// Descriptor is the catalogue entry.
func (h RegenerateRecoveryCodes) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RegenerateRecoveryCodesName,
		Summary: "Replaces the ten recovery codes with ten new ones, behind a step-up. The old " +
			"set stops working in the same moment the new one is answered - one statement, " +
			"because a set answered without the old one burned would be twenty live codes. " +
			"Shown once, exactly as at enrolment, and stored only as hashes.",
		SideEffects: "Burns every live recovery code of the account, stores ten new hashes, and " +
			"writes an audit entry.",
		Input: []usecase.Field{
			{
				// Declared but not required, ChangePassword's reasoning: a caller without a proof
				// needs the `403` that carries `auth.step_up_required` rather than a `422` about
				// a missing field.
				Name: "step_up_token", Kind: usecase.KindString,
				Description: "The proof from `/auth/step-up`. It is consumed by this call, and " +
					"the call is refused without it.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: RecoveryRegeneratedAction, TargetType: accountTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A recovery code is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RegenerateRecoveryCodes) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	fresh, err := h.Execute(ctx, actor, in.String("step_up_token"))
	if err != nil {
		return nil, err
	}
	codes := make([]any, 0, len(fresh.Codes))
	for _, code := range fresh.Codes {
		codes = append(codes, code.Reveal())
	}
	return usecase.Output{"recovery_codes": codes}, nil
}
