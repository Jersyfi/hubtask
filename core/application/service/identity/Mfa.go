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
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const (
	EnrollTotpName  = "EnrollTotp"
	ConfirmTotpName = "ConfirmTotp"
	DisableTotpName = "DisableTotp"
)

// The audit codes of the second factor's lifecycle. Arming and disarming a factor are the class
// of event a review looks for (audit.md §2, security.md §5).
const (
	MfaEnrollmentStartedAction audit.Action = "auth.mfa_enrollment_started"
	MfaEnabledAction           audit.Action = "auth.mfa_enabled"
	MfaDisabledAction          audit.Action = "auth.mfa_disabled"
)

// mfaCaller is who an MFA operation acts for: a signed-in person, or the pending credential of
// an enforcement sign-in - resolved once, because the answer decides both the account and
// whether a confirmation also opens a session.
type mfaCaller struct {
	account  domain.Account
	tenantID shared.ID
	// pending is set for the enforcement flow; its consumption is the confirmation's business.
	pending domain.PendingCredential
}

func (c mfaCaller) enforcementFlow() bool { return !c.pending.ID.IsZero() }

// resolveMfaCaller admits the two callers and nobody else. The pending credential must carry the
// ENROLL purpose: a TOTP credential belongs to a person who already holds an armed factor, and
// letting it enrol would let a stolen password replace the very factor it is stopped by.
func (w SessionWriter) resolveMfaCaller(
	ctx context.Context, actor appshared.ActorContext, pendingToken secret.Secret,
) (mfaCaller, error) {
	if actor.IsAuthenticated() {
		if actor.AccountID.IsZero() {
			return mfaCaller{}, shared.ErrForbidden.WithDetail("access.token_owner_required")
		}
		// Read rather than reconstructed from the actor: the provisioning URI labels the
		// account by its address, and the actor deliberately does not carry one.
		var account domain.Account
		err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(),
			func(ctx context.Context) error {
				found, err := w.People.Find(ctx, actor.AccountID)
				account = found
				return err
			})
		if err != nil {
			return mfaCaller{}, err
		}
		if err := account.Verify(); err != nil {
			return mfaCaller{}, err
		}
		account.TenantID = actor.TenantID
		return mfaCaller{account: account, tenantID: actor.TenantID}, nil
	}

	if pendingToken.IsEmpty() {
		return mfaCaller{}, shared.ErrUnauthenticated.WithDetail("auth.mfa_credential_required")
	}
	token, err := domain.ParsePendingToken(pendingToken.Reveal())
	if err != nil {
		return mfaCaller{}, challengeRefused()
	}

	var caller mfaCaller
	err = w.UnitOfWork.Within(ctx, persistence.Scope{TenantID: token.TenantID()},
		func(ctx context.Context) error {
			lookup, err := w.Pending.FindByToken(ctx, token)
			if err != nil {
				if errors.Is(err, shared.ErrNotFound) {
					return challengeRefused()
				}
				return err
			}
			now := w.Clock.Now()
			if err := lookup.Credential.Verify(now); err != nil {
				return err
			}
			if lookup.Credential.Purpose != domain.PendingEnroll {
				return challengeRefused()
			}
			if err := lookup.Account.Verify(); err != nil {
				return err
			}
			caller = mfaCaller{
				account:  lookup.Account,
				tenantID: token.TenantID(),
				pending:  lookup.Credential,
			}
			return nil
		})
	if err != nil {
		return mfaCaller{}, err
	}
	return caller, nil
}

// MintedEnrollment is the single showing: the secret for typing, the URI for the QR, the ten
// codes. Everything in masking wrappers, MintedToken's reasoning (T-18).
type MintedEnrollment struct {
	Secret        secret.Secret
	URI           secret.Secret
	RecoveryCodes []secret.Secret
}

// EnrollTotpCommand carries the one optional credential.
type EnrollTotpCommand struct {
	PendingToken secret.Secret
}

// EnrollTotp begins the enrolment (H-02): mints the secret, seals it, mints the codes, and
// answers all of it for the only time. Nothing is armed yet.
type EnrollTotp struct{ Writer SessionWriter }

// Execute enrols. Enrolling again before confirming replaces the unconfirmed secret; enrolling
// while armed is refused - disable first, with the password.
func (h EnrollTotp) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd EnrollTotpCommand,
) (MintedEnrollment, error) {
	w := h.Writer

	caller, err := w.resolveMfaCaller(ctx, actor, cmd.PendingToken)
	if err != nil {
		return MintedEnrollment{}, err
	}
	if caller.account.Kind != domain.AccountUser {
		// A machine holds tokens, not authenticators.
		return MintedEnrollment{}, shared.ErrForbidden.WithDetail("auth.mfa_credential_required")
	}

	secretMaterial, err := w.Entropy.Bytes(domain.TotpSecretBytes)
	if err != nil {
		return MintedEnrollment{}, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}
	codeMaterial, err := w.Entropy.Bytes(domain.RecoveryCodeCount * domain.RecoveryCodeBytes)
	if err != nil {
		return MintedEnrollment{}, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}
	codes, err := domain.NewRecoveryCodes(codeMaterial)
	if err != nil {
		return MintedEnrollment{}, err
	}

	scope := persistence.Scope{TenantID: caller.tenantID}
	err = w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		sealed, err := w.Encryptor.Seal(ctx,
			secret.New(string(secretMaterial)), mfaSecretPurpose(caller.account.ID))
		if err != nil {
			return err
		}

		now := w.Clock.Now()
		fresh, err := w.Enrollments.Upsert(ctx, caller.account.ID, sealed, now)
		if err != nil {
			return err
		}
		if !fresh {
			return shared.ErrConflict.WithDetail("auth.mfa_already_armed")
		}

		ids := make([]shared.ID, 0, len(codes))
		for range codes {
			ids = append(ids, w.IDs.NewID())
		}
		if err := w.Recovery.Replace(ctx, caller.account.ID, ids, codes, now); err != nil {
			return err
		}

		return w.recordMfaAudit(ctx, MfaEnrollmentStartedAction, audit.SeverityInfo,
			caller.account, now)
	})
	if err != nil {
		return MintedEnrollment{}, err
	}

	minted := MintedEnrollment{
		Secret: secret.New(domain.TotpSecretBase32(secretMaterial)),
		URI: secret.New(domain.TotpProvisioningURI(
			w.issuerLabel(), caller.account.Email, secretMaterial)),
	}
	for _, code := range codes {
		minted.RecoveryCodes = append(minted.RecoveryCodes, secret.New(code))
	}
	return minted, nil
}

func (w SessionWriter) issuerLabel() string {
	if w.Issuer != "" {
		return w.Issuer
	}
	return "Hubtask"
}

// ConfirmTotpCommand carries the first code and, for the enforcement flow, the credential.
type ConfirmTotpCommand struct {
	PendingToken secret.Secret
	Code         string
}

// ConfirmedEnrollment is the confirmation's answer: armed, and - when a pending credential
// completed a sign-in here - the pair.
type ConfirmedEnrollment struct {
	Pair *SessionPair
}

// ConfirmTotp arms the enrolment (H-02): the caller proves the authenticator holds the secret.
type ConfirmTotp struct{ Writer SessionWriter }

// Execute confirms. For the enforcement flow the confirmation also opens the session - both
// factors are proved by now, and a second round trip through sign-in would teach nothing.
func (h ConfirmTotp) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ConfirmTotpCommand,
) (ConfirmedEnrollment, error) {
	w := h.Writer

	caller, err := w.resolveMfaCaller(ctx, actor, cmd.PendingToken)
	if err != nil {
		return ConfirmedEnrollment{}, err
	}

	scope := persistence.Scope{TenantID: caller.tenantID}
	err = w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		now := w.Clock.Now()
		subject := mfaSubject(caller.account.ID)
		if err := w.checkLocked(ctx, []string{subject}, now); err != nil {
			return err
		}

		enrollment, err := w.Enrollments.Find(ctx, caller.account.ID)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return shared.ErrConflict.WithDetail("auth.mfa_not_enrolled")
			}
			return err
		}
		if !enrollment.ConfirmedAt.IsZero() {
			return shared.ErrConflict.WithDetail("auth.mfa_already_armed")
		}

		plaintext, err := w.Encryptor.Open(ctx, enrollment.Secret, mfaSecretPurpose(caller.account.ID))
		if err != nil {
			return err
		}
		step, ok := domain.VerifyTotp([]byte(plaintext.Reveal()), cmd.Code, now, 0)
		if !ok {
			w.failure(ctx, FailureMfa)
			return errors.Join(
				w.recordMfaFailure(ctx, subject, now),
				shared.ErrUnauthenticated.WithDetail("auth.mfa_code_invalid"))
		}

		armed, err := w.Enrollments.Confirm(ctx, caller.account.ID, step, now)
		if err != nil {
			return err
		}
		if !armed {
			return shared.ErrConflict.WithDetail("auth.mfa_already_armed")
		}
		if err := w.Attempts.Clear(ctx, subject); err != nil {
			return err
		}

		if caller.enforcementFlow() {
			consumed, err := w.Pending.Consume(ctx, caller.pending.ID, now)
			if err != nil {
				return err
			}
			if !consumed {
				return challengeRefused()
			}
		}

		return w.recordMfaAudit(ctx, MfaEnabledAction, audit.SeverityNotice, caller.account, now)
	})
	if err != nil {
		return ConfirmedEnrollment{}, err
	}

	if !caller.enforcementFlow() {
		return ConfirmedEnrollment{}, nil
	}
	pair, err := w.openSessionWithHint(ctx, scope, caller.tenantID, caller.account,
		caller.pending.UserAgent, caller.pending.IPClass, SignedInAction)
	if err != nil {
		return ConfirmedEnrollment{}, err
	}
	return ConfirmedEnrollment{Pair: &pair}, nil
}

// DisableTotpCommand carries the fresh password.
type DisableTotpCommand struct {
	Password secret.Secret
}

// DisableTotp removes the factor (H-02): the one case where "recently signed in" is not enough,
// because a stolen session removing the second factor is the attack.
type DisableTotp struct{ Writer SessionWriter }

// Execute disables, behind the fresh password and outside the tenant switch's reach.
func (h DisableTotp) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd DisableTotpCommand,
) error {
	w := h.Writer
	if !actor.IsAuthenticated() || actor.AccountID.IsZero() {
		return shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}

	// The hash is read in one transaction and verified outside it, sign-in's reasoning: Argon2id
	// is deliberately slow, and a connection held through it would let a burst drain the pool.
	var stored secret.Secret
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		hash, err := w.Accounts.PasswordHashOf(ctx, actor.AccountID)
		stored = hash
		return err
	})
	if err != nil {
		return err
	}

	verified := false
	if !stored.IsEmpty() {
		ok, err := w.Passwords.Verify(stored.Reveal(), cmd.Password)
		if err != nil {
			return err
		}
		verified = ok
	} else {
		// An account that signs in some other way holds no password to prove afresh; the decoy
		// keeps the refusal's cost honest all the same (T-02).
		w.Passwords.VerifyDecoy(cmd.Password)
	}
	if !verified {
		w.failure(ctx, FailureWrongCredential)
		return domain.ErrSignInFailed()
	}

	return w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if w.Policy != nil && w.Memberships != nil {
			required, err := w.Policy.RequireAdminTotp(ctx)
			if err != nil {
				return err
			}
			if required {
				admin, err := w.holdsAdminRole(ctx, actor.AccountID)
				if err != nil {
					return err
				}
				if admin {
					return shared.ErrForbidden.WithDetail("auth.mfa_required_by_tenant")
				}
			}
		}

		now := w.Clock.Now()
		removed, err := w.Enrollments.Disable(ctx, actor.AccountID)
		if err != nil {
			return err
		}
		if !removed {
			return shared.ErrConflict.WithDetail("auth.mfa_not_enrolled")
		}
		return w.recordMfaAudit(ctx, MfaDisabledAction, audit.SeverityNotice, domain.Account{
			ID: actor.AccountID, TenantID: actor.TenantID, DisplayName: actor.AccountName,
		}, now)
	})
}

// recordMfaAudit writes the factor's lifecycle evidence: the act and its moment, never a secret,
// a code, or a URI (rule 10).
func (w SessionWriter) recordMfaAudit(
	ctx context.Context, action audit.Action, severity audit.Severity,
	account domain.Account, at time.Time,
) error {
	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   account.TenantID,
		OccurredAt: at,
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   severity,
		ActorKind:  appshared.ActorUser,
		ActorID:    account.ID,
		ActorLabel: account.DisplayName,
		TargetType: accountTarget,
		TargetID:   account.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
	})
}

// Descriptor is the catalogue entry.
func (h EnrollTotp) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: EnrollTotpName,
		Summary: "Begins TOTP enrolment: mints the secret - sealed at rest through the envelope " +
			"encryption - and answers the provisioning URI and the ten recovery codes for the " +
			"only time. Nothing is armed until a valid code confirms. Called signed in, or with " +
			"the pending credential a sign-in under tenant enforcement answered. Enrolling " +
			"while armed is refused: disable first, with the password.",
		SideEffects: "Writes the sealed enrolment, replaces the recovery codes, writes an audit " +
			"entry, and answers three secrets once.",
		Input: []usecase.Field{
			{
				Name: "pending_token", Kind: usecase.KindString,
				Description: "The enforcement sign-in's credential; absent for a signed-in caller.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: MfaEnrollmentStartedAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "An account is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h EnrollTotp) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	minted, err := h.Execute(ctx, actor, EnrollTotpCommand{
		PendingToken: secret.New(in.String("pending_token")),
	})
	if err != nil {
		return nil, err
	}
	codes := make([]any, 0, len(minted.RecoveryCodes))
	for _, code := range minted.RecoveryCodes {
		codes = append(codes, code.Reveal())
	}
	// The one place these three are ever answered (T-18's "shown once" as code).
	return usecase.Output{
		"secret":         minted.Secret.Reveal(),
		"otpauth_uri":    minted.URI.Reveal(),
		"recovery_codes": codes,
	}, nil
}

// Descriptor is the catalogue entry.
func (h ConfirmTotp) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ConfirmTotpName,
		Summary: "Arms a TOTP enrolment with one valid code, proving the authenticator holds " +
			"the secret. From this moment sign-in is two-step. When the pending credential of " +
			"an enforcement sign-in confirms, the answer also carries the session pair - both " +
			"factors are proved by then.",
		SideEffects: "Arms the enrolment, records the confirming step, and - for an enforcement " +
			"sign-in - consumes the pending credential and opens a session.",
		Input: []usecase.Field{
			{
				Name: "code", Kind: usecase.KindString, Required: true,
				Description: "The authenticator's current code.",
			},
			{
				Name: "pending_token", Kind: usecase.KindString,
				Description: "The enforcement sign-in's credential; absent for a signed-in caller.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: MfaEnabledAction, TargetType: accountTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "An account is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ConfirmTotp) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	confirmed, err := h.Execute(ctx, actor, ConfirmTotpCommand{
		PendingToken: secret.New(in.String("pending_token")),
		Code:         in.String("code"),
	})
	if err != nil {
		return nil, err
	}
	out := usecase.Output{"armed": true}
	if confirmed.Pair != nil {
		out["tokens"] = pairOutput(*confirmed.Pair)
	}
	return out, nil
}

// Descriptor is the catalogue entry.
func (h DisableTotp) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DisableTotpName,
		Summary: "Removes the second factor and burns the remaining recovery codes. It demands " +
			"the password afresh - a live session is deliberately not enough, because a stolen " +
			"session removing the factor is the attack the factor exists against. Under tenant " +
			"enforcement an OWNER or ADMIN cannot disable at all.",
		SideEffects: "Removes the enrolment and its codes, and writes an audit entry.",
		Destructive: true,
		Input: []usecase.Field{
			{
				Name: "password", Kind: usecase.KindString, Required: true,
				Description: "Checked afresh against the stored hash.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: MfaDisabledAction, TargetType: accountTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "An account is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h DisableTotp) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	if err := h.Execute(ctx, actor, DisableTotpCommand{
		Password: secret.New(in.String("password")),
	}); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}
