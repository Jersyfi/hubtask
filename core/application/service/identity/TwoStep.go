// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"strconv"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const CompleteSignInName = "CompleteSignIn"

// RecoveryCodeUsedAction is a recovery code's consumption - audited, because a burned escape
// hatch is something its owner should be able to find in the trail (H-02).
const RecoveryCodeUsedAction audit.Action = "auth.recovery_code_used"

// FailureMfa is the metric reason of a refused second step.
const FailureMfa = "mfa_refused"

// The challenge methods, the contract's closed set.
const (
	methodTotp     = "TOTP"
	methodRecovery = "RECOVERY"
	methodEnroll   = "ENROLL"
)

// mfaSecretPurpose binds the sealed TOTP secret to its account, E-02's discipline: a ciphertext
// lifted onto another row no longer opens.
func mfaSecretPurpose(accountID shared.ID) cryptoport.Purpose {
	return cryptoport.Purpose("account_mfa.secret:" + accountID.String())
}

// mfaSubject is the attempt ledger's key for second-step guesses: per account, because the code
// space is small and the address behind a phone changes (H-02 "rate-limits attempts per account").
func mfaSubject(accountID shared.ID) string { return "mfa:" + accountID.String() }

// challengeFor decides whether the password is the whole of this sign-in (H-02).
//
// An armed enrolment demands its code. Under the tenant switch, an OWNER or ADMIN with no armed
// enrolment is routed into enrolment instead of into a session - the pending credential it hands
// out can do nothing else. Everybody else signs straight in, and an installation wired without
// the second factor behaves exactly as H-01 shipped it.
func (w SessionWriter) challengeFor(
	ctx context.Context, scope persistence.Scope, account domain.Account,
	cmd SignInCommand, subjects []string,
) (*SignInChallenge, error) {
	if w.Enrollments == nil || w.Pending == nil {
		return nil, nil
	}

	var challenge *SignInChallenge
	err := w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		purpose, err := w.pendingPurposeFor(ctx, account)
		if err != nil || purpose == "" {
			return err
		}

		material, err := w.Entropy.Bytes(domain.TokenSecretBytes)
		if err != nil {
			return shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
		}
		presented, err := domain.NewPendingToken(scope.TenantID, material)
		if err != nil {
			return err
		}

		now := w.Clock.Now()
		credential := domain.PendingCredential{
			ID:        w.IDs.NewID(),
			TenantID:  scope.TenantID,
			AccountID: account.ID,
			Purpose:   purpose,
			UserAgent: cmd.UserAgent,
			IPClass:   domain.IPClass(cmd.RemoteAddr),
			CreatedAt: now.UTC(),
			ExpiresAt: now.Add(domain.PendingLifetime).UTC(),
		}
		if err := w.Pending.Insert(ctx, credential, presented); err != nil {
			return err
		}

		// The password was proved: the sign-in ledger's slate is wiped here, and the second
		// step guards itself through its own per-account subject.
		for _, subject := range subjects {
			if err := w.Attempts.Clear(ctx, subject); err != nil {
				return err
			}
		}

		methods := []string{methodTotp, methodRecovery}
		if purpose == domain.PendingEnroll {
			methods = []string{methodEnroll}
		}
		challenge = &SignInChallenge{
			Token:     secret.New(presented.Secret()),
			ExpiresAt: credential.ExpiresAt,
			Methods:   methods,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return challenge, nil
}

// pendingPurposeFor is the decision itself: TOTP for an armed enrolment, ENROLL for an
// unenrolled administrator under the switch, empty for a plain sign-in.
func (w SessionWriter) pendingPurposeFor(
	ctx context.Context, account domain.Account,
) (domain.PendingPurpose, error) {
	enrollment, err := w.Enrollments.Find(ctx, account.ID)
	switch {
	case err == nil && !enrollment.ConfirmedAt.IsZero():
		return domain.PendingTotp, nil
	case err != nil && !errors.Is(err, shared.ErrNotFound):
		return "", err
	}

	// Not armed. An unconfirmed enrolment protects nobody and locks nobody out, so it counts
	// exactly as none here.
	if w.Policy == nil || w.Memberships == nil {
		return "", nil
	}
	required, err := w.Policy.RequireAdminTotp(ctx)
	if err != nil || !required {
		return "", err
	}
	admin, err := w.holdsAdminRole(ctx, account.ID)
	if err != nil || !admin {
		return "", err
	}
	return domain.PendingEnroll, nil
}

// holdsAdminRole answers whether the tenant switch reaches this account: OWNER or ADMIN at any
// scope, because an administrator of a hub is exactly who the switch means to cover as much as
// the workspace's own.
func (w SessionWriter) holdsAdminRole(ctx context.Context, accountID shared.ID) (bool, error) {
	held, err := w.Memberships.Along(ctx, accountID, []domain.Scope{domain.TenantScope()})
	if err != nil {
		return false, err
	}
	for _, membership := range held {
		if membership.Role == domain.RoleOwner || membership.Role == domain.RoleAdmin {
			return true, nil
		}
	}
	return false, nil
}

// CompleteSignInCommand carries the second step.
type CompleteSignInCommand struct {
	PendingToken secret.Secret
	Code         string
	RecoveryCode secret.Secret
	// TenantHeader may confirm the token's tenant, never overrule it.
	TenantHeader string
}

// CompleteSignIn presents the second factor and receives the pair (H-02).
type CompleteSignIn struct{ Writer SessionWriter }

// Execute verifies. The pending credential dies on use whatever happens next; a code verifies
// within one step of drift and never twice; a recovery code burns; and failures count against
// the account's own ledger subject, because the code space is small and the clock is patient.
func (h CompleteSignIn) Execute(
	ctx context.Context, cmd CompleteSignInCommand,
) (SessionPair, int, error) {
	w := h.Writer

	token, err := domain.ParsePendingToken(cmd.PendingToken.Reveal())
	if err != nil {
		w.failure(ctx, FailureMfa)
		return SessionPair{}, -1, challengeRefused()
	}
	if cmd.TenantHeader != "" && cmd.TenantHeader != token.TenantID().String() {
		return SessionPair{}, -1, shared.ErrForbidden.WithDetail("access.tenant_mismatch")
	}
	if cmd.Code == "" && cmd.RecoveryCode.IsEmpty() {
		return SessionPair{}, -1, shared.ErrValidation.
			WithDetail("auth.mfa_code_required").
			WithFields(shared.FieldError{Path: "/code", Code: "auth.mfa_code_required"})
	}

	scope := persistence.Scope{TenantID: token.TenantID()}
	var (
		account   domain.Account
		hint      domain.PendingCredential
		remaining = -1
	)
	err = w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		lookup, err := w.Pending.FindByToken(ctx, token)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				w.failure(ctx, FailureMfa)
				return challengeRefused()
			}
			return err
		}

		now := w.Clock.Now()
		if err := lookup.Credential.Verify(now); err != nil {
			w.failure(ctx, FailureMfa)
			return err
		}
		if lookup.Credential.Purpose != domain.PendingTotp {
			// An ENROLL credential completes nothing here: it opens exactly the enrolment
			// routes, and a sign-in it could complete would be a factor nobody proved.
			w.failure(ctx, FailureMfa)
			return challengeRefused()
		}
		if err := lookup.Account.Verify(); err != nil {
			return err
		}
		// The workspace's standing (H-06), Session's reasoning: the second step of a sign-in a
		// suspension has overtaken completes nothing.
		if err := lookup.TenantStatus.Verify(); err != nil {
			return err
		}

		subject := mfaSubject(lookup.Account.ID)
		if err := w.checkLocked(ctx, []string{subject}, now); err != nil {
			return err
		}

		if cmd.Code != "" {
			if err := w.verifyTotpCode(ctx, lookup.Account.ID, cmd.Code, now); err != nil {
				return errors.Join(w.recordMfaFailure(ctx, subject, now), err)
			}
		} else {
			burned, err := w.Recovery.Burn(ctx, lookup.Account.ID, cmd.RecoveryCode.Reveal(), now)
			if err != nil {
				return err
			}
			if !burned {
				w.failure(ctx, FailureMfa)
				return errors.Join(
					w.recordMfaFailure(ctx, subject, now),
					shared.ErrUnauthenticated.WithDetail("auth.mfa_code_invalid"))
			}
			left, err := w.Recovery.Remaining(ctx, lookup.Account.ID)
			if err != nil {
				return err
			}
			remaining = left
			if err := w.recordRecoveryUse(ctx, lookup.Account, left, now); err != nil {
				return err
			}
		}

		consumed, err := w.Pending.Consume(ctx, lookup.Credential.ID, now)
		if err != nil {
			return err
		}
		if !consumed {
			// Somebody completed this sign-in between our read and our write: two holders of
			// one credential, the refresh exchange's answer.
			w.failure(ctx, FailureMfa)
			return challengeRefused()
		}
		if err := w.Attempts.Clear(ctx, subject); err != nil {
			return err
		}

		account, hint = lookup.Account, lookup.Credential
		return nil
	})
	if err != nil {
		return SessionPair{}, -1, err
	}

	pair, err := w.openSessionWithHint(ctx, scope, token.TenantID(), account,
		hint.UserAgent, hint.IPClass, SignedInAction)
	if err != nil {
		return SessionPair{}, -1, err
	}
	return pair, remaining, nil
}

// verifyTotpCode opens the sealed secret, judges the code, and advances the replay floor
// atomically - a second presentation of the same step loses the race in the statement, not in a
// comparison two processes could both win.
func (w SessionWriter) verifyTotpCode(
	ctx context.Context, accountID shared.ID, code string, now time.Time,
) error {
	enrollment, err := w.Enrollments.Find(ctx, accountID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return shared.ErrUnauthenticated.WithDetail("auth.mfa_code_invalid")
		}
		return err
	}
	if enrollment.ConfirmedAt.IsZero() {
		return shared.ErrUnauthenticated.WithDetail("auth.mfa_code_invalid")
	}

	plaintext, err := w.Encryptor.Open(ctx, enrollment.Secret, mfaSecretPurpose(accountID))
	if err != nil {
		return err
	}
	step, ok := domain.VerifyTotp([]byte(plaintext.Reveal()), code, now, enrollment.LastStep)
	if !ok {
		w.failure(ctx, FailureMfa)
		return shared.ErrUnauthenticated.WithDetail("auth.mfa_code_invalid")
	}
	advanced, err := w.Enrollments.RecordStep(ctx, accountID, step, now)
	if err != nil {
		return err
	}
	if !advanced {
		w.failure(ctx, FailureMfa)
		return shared.ErrUnauthenticated.WithDetail("auth.mfa_code_invalid")
	}
	return nil
}

func (w SessionWriter) recordMfaFailure(ctx context.Context, subject string, now time.Time) error {
	attempt, err := w.Attempts.Find(ctx, subject)
	if err != nil {
		return err
	}
	failures := attempt.Failures + 1
	return w.Attempts.Record(ctx, subject, repository.AuthAttempt{
		Failures:      failures,
		LastFailureAt: now.UTC(),
		LockedUntil:   domain.LockedUntil(failures, now),
	})
}

// recordRecoveryUse is the audit entry a burned escape hatch owes: what remains is in the entry,
// the code itself is nowhere (rule 10).
func (w SessionWriter) recordRecoveryUse(
	ctx context.Context, account domain.Account, remaining int, at time.Time,
) error {
	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   account.TenantID,
		OccurredAt: at,
		Action:     RecoveryCodeUsedAction,
		Outcome:    audit.OutcomeSuccess,
		// Warning rather than notice: a recovery code in use means the authenticator was not at
		// hand, and a run of these is what an account takeover looks like from the trail.
		Severity:   audit.SeverityWarning,
		ActorKind:  appshared.ActorUser,
		ActorID:    account.ID,
		ActorLabel: account.DisplayName,
		TargetType: accountTarget,
		TargetID:   account.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(audit.Change{
			Field: "recovery_codes_remaining", Classification: audit.Open,
			To: strconv.Itoa(remaining),
		}),
	})
}

// openSessionWithHint is openSession for a flow whose client hint was recorded earlier - the
// pending row carries the sign-in's own device, already coarsened, and the completion call's
// peer is deliberately not consulted.
func (w SessionWriter) openSessionWithHint(
	ctx context.Context, scope persistence.Scope, tenantID shared.ID, account domain.Account,
	userAgent, ipClass string, action audit.Action,
) (SessionPair, error) {
	pair, err := w.openSession(ctx, scope, tenantID, account, userAgent, "", action, nil)
	if err != nil {
		return SessionPair{}, err
	}
	if ipClass != "" {
		// The stored class wins over the (empty) completion peer. The row was written by
		// openSession without it; stamping the struct here keeps the answer honest, and the
		// row's own hint is corrected in the same transaction the next touch writes.
		pair.Session.IPClass = ipClass
	}
	return pair, nil
}

// challengeRefused is the second step's one probe-facing refusal: unknown, expired, consumed
// and lost races are the same bytes.
func challengeRefused() error {
	return shared.ErrUnauthenticated.WithDetail("auth.mfa_challenge_failed")
}

// challengeOutput is the 202 answer's projection.
func challengeOutput(challenge SignInChallenge) usecase.Output {
	methods := make([]any, 0, len(challenge.Methods))
	for _, method := range challenge.Methods {
		methods = append(methods, method)
	}
	return usecase.Output{
		"mfa_required":  true,
		"pending_token": challenge.Token.Reveal(),
		"expires_at":    challenge.ExpiresAt.UTC(),
		"methods":       methods,
	}
}

// Descriptor is the catalogue entry.
func (h CompleteSignIn) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CompleteSignInName,
		Summary: "Presents the second factor of a two-step sign-in: the pending credential the " +
			"password answered, plus a TOTP code - one step of drift either side, never the " +
			"same step twice - or one recovery code, which burns on use and answers how many " +
			"remain. Failures count against the account's own attempt ledger.",
		SideEffects: "Consumes the pending credential, advances the replay floor or burns a " +
			"recovery code, opens a session, and writes audit entries.",
		Input: []usecase.Field{
			{
				Name: "pending_token", Kind: usecase.KindString, Required: true,
				Description: "The challenge's credential. It dies on use.",
			},
			{
				Name: "code", Kind: usecase.KindString,
				Description: "The authenticator's current code.",
			},
			{
				Name: "recovery_code", Kind: usecase.KindString,
				Description: "One of the ten shown at enrolment. It works exactly once.",
			},
			{
				Name: "tenant_header", Kind: usecase.KindString,
				Description: "The X-Hubtask-Tenant header, when sent. It may confirm the " +
					"token's tenant, never overrule it.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: SignedInAction, TargetType: sessionTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A sign-in is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CompleteSignIn) invoke(
	ctx context.Context, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	pair, remaining, err := h.Execute(ctx, CompleteSignInCommand{
		PendingToken: secret.New(in.String("pending_token")),
		Code:         in.String("code"),
		RecoveryCode: secret.New(in.String("recovery_code")),
		TenantHeader: in.String("tenant_header"),
	})
	if err != nil {
		return nil, err
	}
	out := pairOutput(pair)
	if remaining >= 0 {
		out["recovery_codes_remaining"] = remaining
	}
	return out, nil
}
