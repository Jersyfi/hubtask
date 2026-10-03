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

// StepUpCommand carries the fresh proof - exactly one of the methods the account holds
// (ADR-0075 §1): the password, the authenticator's code, a recovery code, or the provider's return
// after StartProviderStepUp - its state and its authorization code, together.
type StepUpCommand struct {
	Password          secret.Secret
	Code              string
	RecoveryCode      secret.Secret
	State             secret.Secret
	AuthorizationCode string
}

// methodsGiven counts the proofs a command carries. One is a step-up; none is nothing to check, and
// two would be asking the server to choose which credential to believe. The provider's return is one
// proof in two halves, and half of it is none.
func (cmd StepUpCommand) methodsGiven() int {
	given := 0
	for _, present := range []bool{
		!cmd.Password.IsEmpty(), cmd.Code != "", !cmd.RecoveryCode.IsEmpty(), cmd.presentedProviderProof(),
	} {
		if present {
			given++
		}
	}
	if cmd.presentedProviderProof() && (cmd.State.IsEmpty() || cmd.AuthorizationCode == "") {
		return 0
	}
	return given
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
	if cmd.methodsGiven() != 1 {
		return StepUpGrant{}, shared.ErrValidation.
			WithDetail("auth.step_up_method_required").
			WithFields(shared.FieldError{Path: "/password", Code: "auth.step_up_method_required"})
	}

	// The session first: TokenID names one exactly when the actor signed in, and a proof that
	// could land nowhere is refused before any credential is examined.
	session, err := w.stepUpSession(ctx, actor)
	if err != nil {
		return StepUpGrant{}, err
	}

	method := domain.StepUpProvider
	if cmd.presentedProviderProof() {
		err = w.proveAtProvider(ctx, actor, session, cmd)
	} else {
		method, err = h.prove(ctx, actor, cmd)
	}
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
				Field: "method", Classification: audit.Open, To: methodAuditLabel(method),
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
	// A password the workspace switched off proves nothing here either (SC-24): the account proves
	// itself at its provider or with its factor. Asked before the proof's own transaction.
	if !cmd.Password.IsEmpty() {
		if err := w.passwordShut(ctx, actor.TenantID); err != nil {
			return "", err
		}
	}

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

		if !cmd.RecoveryCode.IsEmpty() {
			if err := w.burnRecoveryCode(ctx, actor, cmd.RecoveryCode, now); err != nil {
				return errors.Join(w.recordMfaFailure(ctx, subject, now), err)
			}
			method = domain.StepUpRecovery
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

// burnRecoveryCode spends one recovery code as the proof (ADR-0075 §1), exactly as the sign-in's
// second step spends one: burned in the statement that matches it, and the trail told how many are
// left - a run of these is what an account takeover looks like from the trail.
func (w SessionWriter) burnRecoveryCode(
	ctx context.Context, actor appshared.ActorContext, presented secret.Secret, now time.Time,
) error {
	if w.Recovery == nil {
		return shared.ErrUnauthenticated.WithDetail("auth.mfa_code_invalid")
	}
	burned, err := w.Recovery.Burn(ctx, actor.AccountID, presented.Reveal(), now)
	if err != nil {
		return err
	}
	if !burned {
		w.failure(ctx, FailureMfa)
		return shared.ErrUnauthenticated.WithDetail("auth.mfa_code_invalid")
	}
	left, err := w.Recovery.Remaining(ctx, actor.AccountID)
	if err != nil {
		return err
	}
	return w.recordRecoveryUse(ctx, domain.Account{
		ID: actor.AccountID, TenantID: actor.TenantID, DisplayName: actor.AccountName,
	}, left, now)
}

// methodAuditLabel answers the method as a fresh literal. A switch rather than a conversion,
// deliberately: the trail records *which kind* of proof was given, and a value that is
// provably a label from a closed set - not anything derived from a credential's flow - is what
// a scanner reading the taint should see too (CodeQL flags the constant's very name otherwise).
func methodAuditLabel(method domain.StepUpMethod) string {
	switch method {
	case domain.StepUpTotp:
		return "TOTP"
	case domain.StepUpRecovery:
		return "RECOVERY"
	case domain.StepUpProvider:
		return "PROVIDER"
	default:
		return "PASSWORD"
	}
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

// Methods names what this account can prove itself with: the password where it holds one, and the
// code where a factor is armed. An account that signs in only through a provider holds no password,
// so it is not asked for one (UC-ID-05 check 5) - and one with neither is named nothing, which the
// client says in a sentence rather than drawing a field nobody can fill. An unconfirmed enrolment is
// no factor - the person has not shown they hold the authenticator yet - so it is not offered either.
func (v StepUpVerifier) Methods(
	ctx context.Context, tenantID, accountID shared.ID,
) ([]stepupport.Method, error) {
	w := v.Writer
	methods := []stepupport.Method{}
	// Not offered where the workspace switched the password off (SC-24): a field for it would ask
	// for a proof the step-up refuses.
	passwordOpen := w.passwordShut(ctx, tenantID) == nil
	err := w.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID, ActorID: accountID},
		func(ctx context.Context) error {
			hash, err := w.Accounts.PasswordHashOf(ctx, accountID)
			if err != nil && !errors.Is(err, shared.ErrNotFound) {
				return err
			}
			if !hash.IsEmpty() && passwordOpen {
				methods = append(methods, stepupport.MethodPassword)
			}
			enrollment, err := w.Enrollments.Find(ctx, accountID)
			if err != nil && !errors.Is(err, shared.ErrNotFound) {
				return err
			}
			if err == nil && !enrollment.ConfirmedAt.IsZero() {
				methods = append(methods, stepupport.MethodTotp)
				// A recovery code where one is left: offering a method nobody can answer is the
				// prompt issue 544 was about.
				if w.Recovery != nil {
					left, err := w.Recovery.Remaining(ctx, accountID)
					if err != nil {
						return err
					}
					if left > 0 {
						methods = append(methods, stepupport.MethodRecovery)
					}
				}
			}
			// The provider last: a fresh sign-in there, where the account is connected to one that
			// is switched on for its workspace (ADR-0075 §2).
			_, connected, err := w.stepUpProvider(ctx, accountID)
			if err != nil {
				return err
			}
			if connected {
				methods = append(methods, stepupport.MethodProvider)
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return methods, nil
}

// Descriptor is the catalogue entry.
func (h StepUp) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: StepUpName,
		Summary: "Proves the caller afresh for one privileged action (security.md §5), with " +
			"exactly one method the account holds (ADR-0075): the password, the TOTP code where a " +
			"factor is armed, a recovery code, which the step-up consumes, or the state and code a " +
			"provider sent the browser back with after StartProviderStepUp. The proof lands on the current " +
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
			{
				Name: "recovery_code", Kind: usecase.KindString,
				Description: "One of the account's recovery codes, consumed by the step-up.",
			},
			{
				Name: "state", Kind: usecase.KindString,
				Description: "PROVIDER: the handle StartProviderStepUp minted, as the provider echoed it.",
			},
			{
				Name: "authorization_code", Kind: usecase.KindString,
				Description: "PROVIDER: the code the provider issued, presented together with the state.",
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
		Password:          secret.New(in.String("password")),
		Code:              in.String("code"),
		RecoveryCode:      secret.New(in.String("recovery_code")),
		State:             secret.New(in.String("state")),
		AuthorizationCode: in.String("authorization_code"),
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
