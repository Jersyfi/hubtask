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
	provider "github.com/Jersyfi/hubtask/core/port/identityprovider"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// CompleteLinkName is the catalogue name of the LINK step.
const CompleteLinkName = "CompleteLink"

// linkSubject is the per-account ledger a LINK step's wrong passwords count against. Its own
// subject rather than the password sign-in's, because the address and network the sign-in ledger
// keys on are not what this step was presented with - but per account all the same, so the step
// cannot be used to guess a password faster than the front door allows.
func linkSubject(accountID shared.ID) string { return "link:" + accountID.String() }

// CompleteLinkCommand carries the LINK step: the credential the provider arrival received, and the
// password of the account it matched.
type CompleteLinkCommand struct {
	PendingToken secret.Secret
	Password     secret.Secret
	// TenantHeader may confirm the token's tenant, never overrule it.
	TenantHeader string
}

// CompleteLink is the step ADR-0071's addendum (E2) adds: a provider vouched for an address whose
// account already holds a password, and the provider is connected to it only once that password -
// and the account's second factor, if it has one - has been proven.
//
// It does not ask whether the password is a way in here. A workspace that switched the password off
// closed the sign-in by password, not the account's own proof that it is the person, and a member
// who still knows it connects the workspace's provider with it (ADR-0078 §1). The mailbox is the
// proof for one who does not (connectByMail).
//
// The answer is a pair when the account has no second factor, and otherwise the ordinary TOTP
// challenge with the link still carried, so the connection happens at the end of the second
// factor's step and not a moment before.
type CompleteLink struct{ Writer OidcWriter }

// Execute verifies the password outside a transaction, SignIn's reasoning: Argon2id is slow on
// purpose, and a connection held through it would let a burst of attempts drain the pool.
func (h CompleteLink) Execute(ctx context.Context, cmd CompleteLinkCommand) (SignInResult, error) {
	w := h.Writer
	s := w.Session

	token, err := domain.ParsePendingToken(cmd.PendingToken.Reveal())
	if err != nil {
		s.failure(ctx, FailureMfa)
		return SignInResult{}, challengeRefused()
	}
	if cmd.TenantHeader != "" && cmd.TenantHeader != token.TenantID().String() {
		return SignInResult{}, shared.ErrForbidden.WithDetail("access.tenant_mismatch")
	}
	if cmd.Password.IsEmpty() {
		return SignInResult{}, shared.ErrValidation.
			WithDetail("auth.link_password_required").
			WithFields(shared.FieldError{Path: "/password", Code: "auth.link_password_required"})
	}
	scope := persistence.Scope{TenantID: token.TenantID()}

	// First: the credential, its standing, and the stored hash.
	var (
		credential domain.PendingCredential
		account    domain.Account
		stored     secret.Secret
	)
	err = s.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		lookup, err := s.Pending.FindByToken(ctx, token)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				s.failure(ctx, FailureMfa)
				return challengeRefused()
			}
			return err
		}
		now := s.Clock.Now()
		if err := lookup.Credential.Verify(now); err != nil {
			s.failure(ctx, FailureMfa)
			return err
		}
		if lookup.Credential.Purpose != domain.PendingLink || lookup.Credential.Link == nil {
			// Any other credential connects nothing here: a TOTP one completes a sign-in, an
			// ENROLL one opens enrolment, and neither is a provider waiting for a password.
			s.failure(ctx, FailureMfa)
			return challengeRefused()
		}
		if err := lookup.Account.Verify(); err != nil {
			return err
		}
		if err := lookup.TenantStatus.Verify(); err != nil {
			return err
		}
		if err := s.checkLocked(ctx, []string{linkSubject(lookup.Account.ID)}, now); err != nil {
			return err
		}
		hash, err := s.Accounts.PasswordHashOf(ctx, lookup.Account.ID)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return err
		}
		credential, account, stored = lookup.Credential, lookup.Account, hash
		return nil
	})
	if err != nil {
		return SignInResult{}, err
	}

	verified := false
	if !stored.IsEmpty() {
		ok, verifyErr := s.Passwords.Verify(stored.Reveal(), cmd.Password)
		if verifyErr != nil {
			return SignInResult{}, verifyErr
		}
		verified = ok
	} else {
		s.Passwords.VerifyDecoy(cmd.Password)
	}

	subject := linkSubject(account.ID)
	var challenge *SignInChallenge
	err = s.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		now := s.Clock.Now()
		if !verified {
			s.failure(ctx, FailureWrongCredential)
			return countedRefusal(subject, domain.ErrSignInFailed())
		}
		consumed, err := s.Pending.Consume(ctx, credential.ID, now)
		if err != nil {
			return err
		}
		if !consumed {
			s.failure(ctx, FailureMfa)
			return challengeRefused()
		}
		if err := s.Attempts.Clear(ctx, subject); err != nil {
			return err
		}

		// An armed second factor is the rest of the account's proof. The link travels on into
		// the TOTP credential, and the second factor's step connects it.
		armed := false
		if s.Enrollments != nil {
			enrollment, err := s.Enrollments.Find(ctx, account.ID)
			if err != nil && !errors.Is(err, shared.ErrNotFound) {
				return err
			}
			armed = err == nil && !enrollment.ConfirmedAt.IsZero()
		}
		if armed {
			next, err := w.handOnToTheFactor(ctx, scope, account, credential, now)
			if err != nil {
				return err
			}
			challenge = &next
			return nil
		}
		return w.Connect(ctx, account, *credential.Link)
	})
	err = s.settleRefusal(ctx, scope, err)
	if err != nil {
		return SignInResult{}, err
	}
	if challenge != nil {
		return SignInResult{Challenge: challenge}, nil
	}

	bounds, err := s.sessionBounds(ctx, token.TenantID(), account)
	if err != nil {
		return SignInResult{}, err
	}
	pair, err := s.openSessionWithHint(ctx, scope, token.TenantID(), account,
		credential.UserAgent, credential.IPClass, OidcSignedInAction,
		bounds, domain.SignedInWithOidc, credential.Link.ProviderID)
	if err != nil {
		return SignInResult{}, err
	}
	return SignInResult{Pair: &pair}, nil
}

// handOnToTheFactor mints the TOTP credential that carries the link on to the second factor's step.
func (w OidcWriter) handOnToTheFactor(
	ctx context.Context, scope persistence.Scope, account domain.Account,
	from domain.PendingCredential, now time.Time,
) (SignInChallenge, error) {
	s := w.Session
	material, err := s.Entropy.Bytes(domain.TokenSecretBytes)
	if err != nil {
		return SignInChallenge{}, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}
	presented, err := domain.NewPendingToken(scope.TenantID, material)
	if err != nil {
		return SignInChallenge{}, err
	}
	link := *from.Link
	next := domain.PendingCredential{
		ID:        s.IDs.NewID(),
		TenantID:  scope.TenantID,
		AccountID: account.ID,
		Purpose:   domain.PendingTotp,
		UserAgent: from.UserAgent,
		IPClass:   from.IPClass,
		CreatedAt: now.UTC(),
		ExpiresAt: now.Add(domain.PendingLifetime).UTC(),
		Link:      &link,
	}
	if err := s.Pending.Insert(ctx, next, presented); err != nil {
		return SignInChallenge{}, err
	}
	return SignInChallenge{
		Token:     secret.New(presented.Secret()),
		ExpiresAt: next.ExpiresAt,
		Methods:   []string{methodTotp, methodRecovery},
	}, nil
}

// Connect writes the link once the account's own proof is complete, in the caller's transaction,
// and records it. It is the IdentityConnector the second factor's step calls.
//
// The provider is read again rather than trusted from the credential: one removed while somebody
// was proving themselves leaves nothing to connect, and the step refuses as an unknown credential
// would.
func (w OidcWriter) Connect(ctx context.Context, account domain.Account, link domain.LinkIntent) error {
	configured, _, err := w.Providers.FindWithSecret(ctx, link.ProviderID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return challengeRefused()
		}
		return err
	}
	linked, err := w.External.LinkSubject(ctx, link.ProviderID, account.ID, link.Subject, w.Session.Clock.Now())
	if err != nil {
		return err
	}
	if !linked {
		// Bound to another subject at this provider in the meantime, or gone: never re-pointed.
		return shared.ErrConflict.WithDetail("identity_provider.account_taken")
	}
	// The provider vouched for the address when the arrival was admitted; that is what the entry's
	// email_verified field records. The proof is the one the account gave: its password, or its
	// mailbox with a fresh sign-in at the provider (ADR-0078 §1). A credential written before there
	// were two carries none, and it was the password.
	proof := link.Proof
	if proof == "" {
		proof = domain.LinkProofPassword
	}
	return w.record(ctx, OidcLinkedAction, account, configured, provider.Identity{EmailVerified: true},
		audit.Change{Field: "proof", Classification: audit.Open, To: string(proof)})
}

var _ IdentityConnector = (*OidcWriter)(nil)

// Descriptor is the catalogue entry.
func (h CompleteLink) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CompleteLinkName,
		Summary: "Completes the LINK step of a provider sign-in: the provider vouched for an " +
			"address whose account already holds a password, and it is connected to that " +
			"account only once the password is proven here - and, where the account has a " +
			"second factor, the answer is the ordinary TOTP challenge and the connection " +
			"happens when that step completes. Wrong passwords count against the account. The " +
			"password is a proof here even where the workspace switched it off as a way in.",
		SideEffects: "Consumes the pending credential; then either connects the provider " +
			"identity, opens a session and writes audit entries, or hands on to the second " +
			"factor's step with the connection still pending.",
		Input: []usecase.Field{
			{Name: "pending_token", Kind: usecase.KindString, Required: true,
				Description: "The LINK challenge's credential. It dies on use."},
			{Name: "password", Kind: usecase.KindString, Required: true,
				Description: "The password of the account the provider's address matched."},
			{Name: "tenant_header", Kind: usecase.KindString,
				Description: "The X-Hubtask-Tenant header, when sent. It may confirm the " +
					"token's tenant, never overrule it."},
		},
		// Required: the link is recorded whichever way the step ends - here, or where the second
		// factor's step completes it.
		Audit: usecase.AuditDeclaration{
			Action: OidcLinkedAction, TargetType: accountTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A sign-in is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CompleteLink) invoke(
	ctx context.Context, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	result, err := h.Execute(ctx, CompleteLinkCommand{
		PendingToken: secret.New(in.String("pending_token")),
		Password:     secret.New(in.String("password")),
		TenantHeader: in.String("tenant_header"),
	})
	if err != nil {
		return nil, err
	}
	if result.Challenge != nil {
		return challengeOutput(*result.Challenge), nil
	}
	return pairOutput(*result.Pair), nil
}
