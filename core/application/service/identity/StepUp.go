// SPDX-License-Identifier: BUSL-1.1
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
	"github.com/Jersyfi/hubtask/core/port/persistence"
	stepupport "github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const StepUpName = "StepUp"

// StepUpAction is the proof itself, audited with its method and never its credential (H-03).
const StepUpAction audit.Action = "auth.step_up"

// stepUpSubject is the attempt ledger's key for step-up guesses, mfaSubject's reasoning.
func stepUpSubject(accountID shared.ID) string { return "stepup:" + accountID.String() }

// StepUpCommand carries the fresh proof: the password, or the code where a factor is armed.
type StepUpCommand struct {
	Password secret.Secret
	Code     string
}

// StepUpGrant is what a successful proof answers: the token the one privileged action will
// consume, and until when it could.
type StepUpGrant struct {
	Token     secret.Secret
	ExpiresAt time.Time
	Method    domain.StepUpMethod
}

// StepUp is the fresh re-authentication a privileged action demands (H-03, security.md §5).
type StepUp struct{ Writer SessionWriter }

// Execute proves. Only a session may step up - the proof is recorded on it, and a personal
// access token has no session and no person at the keyboard to ask.
func (h StepUp) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd StepUpCommand,
) (StepUpGrant, error) {
	w := h.Writer
	if !actor.IsAuthenticated() || actor.AccountID.IsZero() {
		return StepUpGrant{}, shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	if (cmd.Password.IsEmpty() && cmd.Code == "") || (!cmd.Password.IsEmpty() && cmd.Code != "") {
		return StepUpGrant{}, shared.ErrValidation.
			WithDetail("auth.step_up_method_required").
			WithFields(shared.FieldError{Path: "/password", Code: "auth.step_up_method_required"})
	}

	// The session first: TokenID names one exactly when the actor signed in, and a proof that
	// could land nowhere is refused before any credential is examined.
	var session domain.Session
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		credential, err := w.Sessions.FindForAuth(ctx, actor.TokenID)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return shared.ErrForbidden.WithDetail("auth.step_up_session_required")
			}
			return err
		}
		if credential.Session.AccountID != actor.AccountID {
			return shared.ErrForbidden.WithDetail("auth.step_up_session_required")
		}
		session = credential.Session
		return nil
	})
	if err != nil {
		return StepUpGrant{}, err
	}
	if err := session.Verify(w.Clock.Now()); err != nil {
		return StepUpGrant{}, err
	}

	method, err := h.prove(ctx, actor, cmd)
	if err != nil {
		return StepUpGrant{}, err
	}

	material, err := w.Entropy.Bytes(domain.TokenSecretBytes)
	if err != nil {
		return StepUpGrant{}, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}
	presented, err := domain.NewStepUpToken(actor.TenantID, material)
	if err != nil {
		return StepUpGrant{}, err
	}

	var grant StepUpGrant
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()
		landed, err := w.StepUps.Record(ctx, session.ID, actor.AccountID, presented, method, now)
		if err != nil {
			return err
		}
		if !landed {
			return shared.ErrForbidden.WithDetail("auth.step_up_session_required")
		}
		if err := w.Attempts.Clear(ctx, stepUpSubject(actor.AccountID)); err != nil {
			return err
		}

		if err := w.Audit.Append(ctx, audit.Entry{
			TenantID:   actor.TenantID,
			OccurredAt: now,
			Action:     StepUpAction,
			Outcome:    audit.OutcomeSuccess,
			Severity:   audit.SeverityNotice,
			ActorKind:  actor.Kind,
			ActorID:    actor.AccountID,
			ActorLabel: actor.AccountName,
			TargetType: sessionTarget,
			TargetID:   session.ID,
			Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(audit.Change{
				Field: "method", Classification: audit.Open, To: string(method),
			}),
		}); err != nil {
			return err
		}

		grant = StepUpGrant{
			Token:     secret.New(presented.Secret()),
			ExpiresAt: now.Add(w.stepUpWindow()).UTC(),
			Method:    method,
		}
		return nil
	})
	if err != nil {
		return StepUpGrant{}, err
	}
	return grant, nil
}

// prove is the re-authentication itself, under the same ledger discipline every credential
// check runs: locked first, the decoy where nothing can verify, failures on the account's own
// subject.
func (h StepUp) prove(
	ctx context.Context, actor appshared.ActorContext, cmd StepUpCommand,
) (domain.StepUpMethod, error) {
	w := h.Writer
	subject := stepUpSubject(actor.AccountID)

	var method domain.StepUpMethod
	err := w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()
		if err := w.checkLocked(ctx, []string{subject}, now); err != nil {
			return err
		}

		if cmd.Code != "" {
			if err := w.verifyTotpCode(ctx, actor.AccountID, cmd.Code, now); err != nil {
				return errors.Join(w.recordMfaFailure(ctx, subject, now), err)
			}
			method = domain.StepUpTotp
			return nil
		}

		stored, err := w.Accounts.PasswordHashOf(ctx, actor.AccountID)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return err
		}
		verified := false
		if err == nil && !stored.IsEmpty() {
			ok, verifyErr := w.Passwords.Verify(stored.Reveal(), cmd.Password)
			if verifyErr != nil {
				return verifyErr
			}
			verified = ok
		} else {
			w.Passwords.VerifyDecoy(cmd.Password)
		}
		if !verified {
			w.failure(ctx, FailureWrongCredential)
			return errors.Join(w.recordMfaFailure(ctx, subject, now), domain.ErrSignInFailed())
		}
		method = domain.StepUpPassword
		return nil
	})
	if err != nil {
		return "", err
	}
	return method, nil
}

// stepUpWindow is the configured validity, with a floor that keeps a zero-value writer usable in
// tests: the composition root always passes the configured value.
func (w SessionWriter) stepUpWindow() time.Duration {
	if w.StepUpWindow > 0 {
		return w.StepUpWindow
	}
	return 5 * time.Minute
}

// StepUpVerifier is the port's implementation (H-03): the seam E-06 cut, filled without changing
// shape. Available is finally true; Satisfied judges and burns the proof in one statement.
type StepUpVerifier struct{ Writer SessionWriter }

var _ stepupport.Verifier = StepUpVerifier{}

// Available reports yes: this installation can ask anybody with a session to prove themselves
// again, which since H-01 is everybody who signs in.
func (v StepUpVerifier) Available() bool { return true }

// Satisfied consumes. Unknown, foreign, stale, already-burned and expired-session proofs are one
// false: which of them applies is not for the holder of a stolen token to learn.
func (v StepUpVerifier) Satisfied(
	ctx context.Context, accountID shared.ID, token string,
) (bool, error) {
	w := v.Writer
	parsed, err := domain.ParseStepUpToken(token)
	if err != nil {
		return false, nil //nolint:nilerr // a malformed proof is "not proved", not a failure
	}

	satisfied := false
	err = w.UnitOfWork.Within(ctx, persistence.Scope{TenantID: parsed.TenantID()},
		func(ctx context.Context) error {
			now := w.Clock.Now()
			_, consumed, err := w.StepUps.Consume(
				ctx, parsed, accountID, now.Add(-w.stepUpWindow()), now)
			satisfied = consumed
			return err
		})
	if err != nil {
		return false, err
	}
	return satisfied, nil
}

// Descriptor is the catalogue entry.
func (h StepUp) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: StepUpName,
		Summary: "Proves the caller afresh for one privileged action (security.md §5): the " +
			"password, or the TOTP code where a factor is armed. The proof lands on the current " +
			"session, is valid for a short window, and is consumed by the one action it is " +
			"presented to - a second privileged action needs a second proof.",
		SideEffects: "Records the proof on the session, writes an audit entry naming the method, " +
			"and answers a token once.",
		Input: []usecase.Field{
			{
				Name: "password", Kind: usecase.KindString,
				Description: "The caller's password. One of the two, never both.",
			},
			{
				Name: "code", Kind: usecase.KindString,
				Description: "The authenticator's current code, where a factor is armed.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: StepUpAction, TargetType: sessionTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A proof is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h StepUp) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	grant, err := h.Execute(ctx, actor, StepUpCommand{
		Password: secret.New(in.String("password")),
		Code:     in.String("code"),
	})
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"step_up_token": grant.Token.Reveal(),
		"expires_at":    grant.ExpiresAt.UTC(),
		"method":        string(grant.Method),
	}, nil
}
