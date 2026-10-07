// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// Replacing the authenticator: a new phone, a lost app. Before this the only way was to turn
// the factor off and set one up again - a moment without a factor, which a workspace that requires
// one forbids. Now the new secret waits beside the armed one, and a code from the new app confirms a
// swap that happens in one statement: the old factor and its codes work until it, and not after.

const (
	StartAuthenticatorReplacementName   = "StartAuthenticatorReplacement"
	ConfirmAuthenticatorReplacementName = "ConfirmAuthenticatorReplacement"
)

// The audit codes of a replacement. The swap is its own action rather than an enrolment's, because
// "the authenticator was exchanged" is what a reader of the trail looks for after a lost phone.
const (
	MfaReplacementStartedAction audit.Action = "auth.mfa_replacement_started"
	MfaReplacedAction           audit.Action = "auth.mfa_replaced"
)

// replacementStepUp is the descriptor's sentence for the demand: replacing the factor is the same
// power as removing it.
const replacementStepUp = "replacing the second factor"

// MintedReplacement is the new secret's single showing: for typing, for the QR, and until when it can
// be confirmed. Nothing is armed yet.
type MintedReplacement struct {
	Secret    secret.Secret
	URI       secret.Secret
	ExpiresAt time.Time
}

// StartAuthenticatorReplacementCommand carries the step-up's proof.
type StartAuthenticatorReplacementCommand struct {
	StepUpToken string
}

// StartAuthenticatorReplacement puts a new secret beside the armed factor.
type StartAuthenticatorReplacement struct{ Writer SessionWriter }

// Execute begins a replacement. Whether there is anything to replace is asked before the proof, so a
// person without a factor is told so without spending the proof they just gave.
func (h StartAuthenticatorReplacement) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd StartAuthenticatorReplacementCommand,
) (MintedReplacement, error) {
	w := h.Writer
	session, err := w.stepUpSession(ctx, actor)
	if err != nil {
		return MintedReplacement{}, err
	}
	caller, err := w.resolveMfaCaller(ctx, actor, secret.Secret{})
	if err != nil {
		return MintedReplacement{}, err
	}
	if caller.account.Kind != domain.AccountUser {
		return MintedReplacement{}, shared.ErrForbidden.WithDetail("auth.mfa_credential_required")
	}
	if err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return w.requireArmed(ctx, actor.AccountID)
	}); err != nil {
		return MintedReplacement{}, err
	}
	if err := w.requireStepUp(ctx, actor, cmd.StepUpToken); err != nil {
		return MintedReplacement{}, err
	}

	material, err := w.Entropy.Bytes(domain.TotpSecretBytes)
	if err != nil {
		return MintedReplacement{}, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}
	var minted MintedReplacement
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		sealed, err := w.Encryptor.Seal(ctx, secret.New(string(material)), mfaSecretPurpose(actor.AccountID))
		if err != nil {
			return err
		}
		now := w.Clock.Now()
		expiresAt := now.Add(domain.MfaReplacementLifetime).UTC()
		started, err := w.Enrollments.StartReplacement(ctx, actor.AccountID, sealed, session.ID, expiresAt, now)
		if err != nil {
			return err
		}
		if !started {
			// Disabled between the read and this write: nothing armed is nothing to replace.
			return shared.ErrConflict.WithDetail("auth.mfa_not_enrolled")
		}
		minted = MintedReplacement{
			Secret:    secret.New(domain.TotpSecretBase32(material)),
			URI:       secret.New(domain.TotpProvisioningURI(w.issuerLabel(), caller.account.Email, material)),
			ExpiresAt: expiresAt,
		}
		return w.recordMfaAudit(ctx, MfaReplacementStartedAction, audit.SeverityInfo, caller.account, now)
	})
	if err != nil {
		return MintedReplacement{}, err
	}
	return minted, nil
}

// requireArmed refuses an account without an armed factor: setting one up is enrolment's.
func (w SessionWriter) requireArmed(ctx context.Context, accountID shared.ID) error {
	enrollment, err := w.Enrollments.Find(ctx, accountID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return shared.ErrConflict.WithDetail("auth.mfa_not_enrolled")
		}
		return err
	}
	if enrollment.ConfirmedAt.IsZero() {
		return shared.ErrConflict.WithDetail("auth.mfa_not_enrolled")
	}
	return nil
}

// ConfirmAuthenticatorReplacementCommand carries the new authenticator's code.
type ConfirmAuthenticatorReplacementCommand struct {
	Code string
}

// ConfirmAuthenticatorReplacement proves the new authenticator holds the new secret and swaps.
type ConfirmAuthenticatorReplacement struct{ Writer SessionWriter }

// Execute confirms and swaps, and answers the ten new recovery codes for the only time.
//
// The code is checked before anything is written, and a wrong one is recorded in a transaction of
// its own - the refusal must land on the ledger even though the confirmation did not, as at every
// second-factor door (settleRefusal).
func (h ConfirmAuthenticatorReplacement) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ConfirmAuthenticatorReplacementCommand,
) ([]secret.Secret, error) {
	w := h.Writer
	session, err := w.stepUpSession(ctx, actor)
	if err != nil {
		return nil, err
	}
	scope := actor.PersistenceScope()
	subject := mfaSubject(actor.AccountID)

	var (
		step     int64
		expected cryptoport.Sealed
		verified bool
	)
	err = w.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		now := w.Clock.Now()
		if err := w.checkLocked(ctx, []string{subject}, now); err != nil {
			return err
		}
		enrollment, err := w.Enrollments.Find(ctx, actor.AccountID)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return err
		}
		if err != nil || enrollment.Replacement == nil || enrollment.ReplacementSession != session.ID ||
			!now.Before(enrollment.ReplacementExpiresAt) {
			return shared.ErrNotFound.WithDetail("auth.mfa_replacement_unknown")
		}
		plaintext, err := w.Encryptor.Open(ctx, *enrollment.Replacement, mfaSecretPurpose(actor.AccountID))
		if err != nil {
			return err
		}
		accepted, ok := domain.VerifyTotp([]byte(plaintext.Reveal()), cmd.Code, now, 0)
		if !ok {
			return nil
		}
		// The very secret verified is the one the swap may arm: the statement compares it.
		verified, step, expected = true, accepted, *enrollment.Replacement
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !verified {
		w.failure(ctx, FailureMfa)
		if err := w.recordFailure(ctx, scope, []string{subject}); err != nil {
			return nil, err
		}
		return nil, shared.ErrUnauthenticated.WithDetail("auth.mfa_code_invalid")
	}

	material, err := w.Entropy.Bytes(domain.RecoveryCodeCount * domain.RecoveryCodeBytes)
	if err != nil {
		return nil, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}
	codes, err := domain.NewRecoveryCodes(material)
	if err != nil {
		return nil, err
	}

	err = w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		now := w.Clock.Now()
		swapped, err := w.Enrollments.SwapReplacement(ctx, actor.AccountID, session.ID, expected, step, now)
		if err != nil {
			return err
		}
		if !swapped {
			// Begun again, lapsed or disabled since the code was checked: nothing verified is armed.
			return shared.ErrNotFound.WithDetail("auth.mfa_replacement_unknown")
		}
		ids := make([]shared.ID, 0, len(codes))
		for range codes {
			ids = append(ids, w.IDs.NewID())
		}
		if err := w.Recovery.Replace(ctx, actor.AccountID, ids, codes, now); err != nil {
			return err
		}
		if err := w.Attempts.Clear(ctx, subject); err != nil {
			return err
		}
		return w.recordMfaAudit(ctx, MfaReplacedAction, audit.SeverityNotice, ownAccount(actor), now)
	})
	if err != nil {
		return nil, err
	}

	answered := make([]secret.Secret, 0, len(codes))
	for _, code := range codes {
		answered = append(answered, secret.New(code))
	}
	return answered, nil
}

// Descriptor is the catalogue entry.
func (h StartAuthenticatorReplacement) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: StartAuthenticatorReplacementName,
		Summary: "Begins replacing the authenticator: a new secret, kept beside the armed " +
			"one as a second, unconfirmed enrolment. Nothing changes yet - the armed factor and its " +
			"recovery codes keep working until the replacement is confirmed. Behind a step-up with " +
			"whatever the account holds; bound to the session that began it, for ten minutes.",
		SideEffects: "Seals a new secret beside the armed one and writes an audit entry. Answers the " +
			"secret for the only time.",
		Input: []usecase.Field{
			{
				// Not required: an absent proof has to reach the use case, which answers the 403
				// carrying `auth.step_up_required` and the account's methods.
				Name: "step_up_token", Kind: usecase.KindString,
				Description: "The step-up's proof: the X-Hubtask-Step-Up header.",
			},
		},
		StepUp: replacementStepUp,
		Audit: usecase.AuditDeclaration{
			Action: MfaReplacementStartedAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "An account is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h StartAuthenticatorReplacement) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	minted, err := h.Execute(ctx, actor, StartAuthenticatorReplacementCommand{
		StepUpToken: in.String("step_up_token"),
	})
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"secret":      minted.Secret.Reveal(),
		"otpauth_uri": minted.URI.Reveal(),
		"expires_at":  minted.ExpiresAt,
	}, nil
}

// Descriptor is the catalogue entry.
func (h ConfirmAuthenticatorReplacement) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ConfirmAuthenticatorReplacementName,
		Summary: "Confirms the new authenticator with one of its codes and swaps it in, in one " +
			"statement: the new secret becomes the armed one and the old stops working, and ten " +
			"new recovery codes replace the old ones - answered for the only time. Only the " +
			"session that began the replacement confirms it, inside its window.",
		SideEffects: "Swaps the armed secret, replaces the recovery codes and writes an audit entry.",
		Input: []usecase.Field{
			{
				Name: "code", Kind: usecase.KindString, Required: true,
				Description: "The current code of the new authenticator.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: MfaReplacedAction, TargetType: accountTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "An account is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ConfirmAuthenticatorReplacement) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	codes, err := h.Execute(ctx, actor, ConfirmAuthenticatorReplacementCommand{Code: in.String("code")})
	if err != nil {
		return nil, err
	}
	// A list of values, the shape every list in an output takes: MCP and automation serialise it,
	// and the REST adapter reads it the way it reads the regenerated codes.
	answered := make([]any, 0, len(codes))
	for _, code := range codes {
		answered = append(answered, code.Reveal())
	}
	return usecase.Output{"recovery_codes": answered}, nil
}
