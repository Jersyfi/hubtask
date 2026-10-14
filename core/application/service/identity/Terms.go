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
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// Terms of use at sign-in (UC-ID-19, identity.md §4.2).
//
// **One gate before every session a sign-in opens.** Every way a sign-in ends - the password, the
// second factor, the change step, a provider's return, the link step, a reset - calls
// openOrAgree instead of opening the session itself. Where the account has not agreed to the terms
// in force, the answer is the TERMS step, whose credential carries the session the sign-in would
// have opened; agreeing opens exactly that session. The step comes after every proof, so it never
// opens what the proofs would not, and asks for nothing a proof would need.
//
// **Persons only.** A service account signs in with tokens and has nobody behind it to agree; a
// refresh, an elevation and a token are no sign-in, so a session opened before the terms changed
// runs on and the change is put to the person at their next sign-in, as UC-ID-19 asks.

// AgreeToTermsName is the catalogue name of the TERMS step.
const AgreeToTermsName = "AgreeToTerms"

// TermsAgreedAction records each agreement, with the version and the link, never a text: Hubtask
// holds none (UC-ID-19 check 4).
const TermsAgreedAction audit.Action = "auth.terms_agreed"

// methodTerms is the TERMS step in the challenge's method list.
const methodTerms = "TERMS"

// TermsReader answers the terms of use in force for a workspace: the link and version resolved
// workspace -> installation, and who set them. The zero value is "no terms".
type TermsReader interface {
	TermsInForce(ctx context.Context, tenantID shared.ID) (domain.TermsInForce, error)
}

// TermsInForce answers the terms of use in force for a workspace.
func (r SignInPolicyResolver) TermsInForce(ctx context.Context, tenantID shared.ID) (domain.TermsInForce, error) {
	legal, err := r.Legal(ctx, tenantID)
	if err != nil {
		return domain.TermsInForce{}, err
	}
	return legal.Terms(), nil
}

// termsOwed answers the terms this account still has to agree to before a session opens, or the
// zero value where none are owed - no terms set, the account agreed to exactly these, or it is no
// person. Read outside a transaction: the resolution opens its own read.
func (w SessionWriter) termsOwed(
	ctx context.Context, scope persistence.Scope, account domain.Account,
) (domain.TermsInForce, error) {
	if w.TermsIn == nil || w.Terms == nil || w.Pending == nil || account.Kind != domain.AccountUser {
		return domain.TermsInForce{}, nil
	}
	terms, err := w.TermsIn.TermsInForce(ctx, scope.TenantID)
	if err != nil || !terms.IsSet() {
		return domain.TermsInForce{}, err
	}
	var agreement domain.TermsAgreement
	err = w.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		read, err := w.Terms.TermsAgreementOf(ctx, account.ID)
		agreement = read
		return err
	})
	if err != nil {
		return domain.TermsInForce{}, err
	}
	if terms.AgreedBy(agreement) {
		return domain.TermsInForce{}, nil
	}
	return terms, nil
}

// SessionHint is what a sign-in knows about the session it is about to open: the device it came
// from - a peer address at the first step, the class a pending credential recorded at a later one -
// and how and through which provider the account was proven.
type SessionHint struct {
	UserAgent  string
	RemoteAddr string
	IPClass    string
	Method     string
	Via        shared.ID
	// Subjects are the attempt ledger's entries the proof just cleared (the password sign-in's).
	Subjects []string
}

// actionOf is the trail's name for a session opened by this method.
func actionOf(method string) audit.Action {
	if method == domain.SignedInWithOidc {
		return OidcSignedInAction
	}
	return SignedInAction
}

// openOrAgree opens the session a sign-in proved, or answers the TERMS step that opens it once the
// account agreed to the terms in force.
func (w SessionWriter) openOrAgree(
	ctx context.Context, scope persistence.Scope, account domain.Account,
	hint SessionHint, bounds domain.SessionPolicy,
) (SignInResult, error) {
	terms, err := w.termsOwed(ctx, scope, account)
	if err != nil {
		return SignInResult{}, err
	}
	if terms.IsSet() {
		challenge, err := w.termsStep(ctx, scope, account, hint, terms)
		if err != nil {
			return SignInResult{}, err
		}
		return SignInResult{Challenge: &challenge}, nil
	}

	var pair SessionPair
	if hint.IPClass != "" {
		pair, err = w.openSessionWithHint(ctx, scope, scope.TenantID, account,
			hint.UserAgent, hint.IPClass, actionOf(hint.Method), bounds, hint.Method, hint.Via)
	} else {
		pair, err = w.openSessionVia(ctx, scope, scope.TenantID, account,
			hint.UserAgent, hint.RemoteAddr, actionOf(hint.Method), hint.Subjects, bounds,
			hint.Method, hint.Via)
	}
	if err != nil {
		return SignInResult{}, err
	}
	return SignInResult{Pair: &pair}, nil
}

// openOrAgreeNaming is openOrAgree for a sign-in whose person typed no address - a provider's
// return, a connect link from the mail: the TERMS step names the account on its identity line, as
// the reset's does (UC-ID-02 check 1). The proof is complete, so the address tells the person
// nothing the sign-in had not already established. The link step needs none: its LINK challenge
// named the account already, and the card keeps that name.
func (w SessionWriter) openOrAgreeNaming(
	ctx context.Context, scope persistence.Scope, account domain.Account,
	hint SessionHint, bounds domain.SessionPolicy,
) (SignInResult, error) {
	result, err := w.openOrAgree(ctx, scope, account, hint, bounds)
	if err == nil && result.Challenge != nil {
		result.Challenge.Email = account.Email
	}
	return result, err
}

// termsStep mints the TERMS credential. The proof is complete at this point, so the attempt
// ledger's entries it cleared are cleared here too, as the second factor's step clears them.
func (w SessionWriter) termsStep(
	ctx context.Context, scope persistence.Scope, account domain.Account,
	hint SessionHint, terms domain.TermsInForce,
) (SignInChallenge, error) {
	return w.mintTerms(ctx, scope, account.ID, nil, hint, terms)
}

// newcomerStep mints the TERMS credential for a provider's newcomer: it carries the arrival
// instead of an account, and the account is made only by the agreement. The person typed no
// address, so the step names the one the provider vouched for.
func (w SessionWriter) newcomerStep(
	ctx context.Context, scope persistence.Scope, arrival domain.ArrivalIntent,
	hint SessionHint, terms domain.TermsInForce,
) (SignInChallenge, error) {
	challenge, err := w.mintTerms(ctx, scope, "", &arrival, hint, terms)
	if err != nil {
		return SignInChallenge{}, err
	}
	challenge.Email = arrival.Email
	return challenge, nil
}

// newcomerTerms answers the terms a newcomer would owe - all of them, having agreed to none - or
// the zero value where none are in force or the step is not wired. Read outside a transaction.
func (w SessionWriter) newcomerTerms(ctx context.Context, scope persistence.Scope) (domain.TermsInForce, error) {
	if w.TermsIn == nil || w.Terms == nil || w.Pending == nil {
		return domain.TermsInForce{}, nil
	}
	return w.TermsIn.TermsInForce(ctx, scope.TenantID)
}

// mintTerms writes the TERMS credential for an account, or for a newcomer's arrival.
func (w SessionWriter) mintTerms(
	ctx context.Context, scope persistence.Scope, accountID shared.ID, arrival *domain.ArrivalIntent,
	hint SessionHint, terms domain.TermsInForce,
) (SignInChallenge, error) {
	material, err := w.Entropy.Bytes(domain.TokenSecretBytes)
	if err != nil {
		return SignInChallenge{}, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}
	presented, err := domain.NewPendingToken(scope.TenantID, material)
	if err != nil {
		return SignInChallenge{}, err
	}
	ipClass := hint.IPClass
	if ipClass == "" {
		ipClass = domain.IPClass(hint.RemoteAddr)
	}

	var challenge SignInChallenge
	err = w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		now := w.Clock.Now()
		credential := domain.PendingCredential{
			ID: w.IDs.NewID(), TenantID: scope.TenantID, AccountID: accountID,
			Purpose: domain.PendingTerms, UserAgent: hint.UserAgent, IPClass: ipClass,
			CreatedAt: now.UTC(), ExpiresAt: now.Add(domain.PendingLifetime).UTC(),
			SessionMethod: hint.Method, SessionVia: hint.Via, Arrival: arrival,
		}
		if err := w.Pending.Insert(ctx, credential, presented); err != nil {
			return err
		}
		w.stepOwed(ctx, credential.Purpose)
		for _, subject := range hint.Subjects {
			if err := w.Attempts.Clear(ctx, subject); err != nil {
				return err
			}
		}
		challenge = SignInChallenge{
			Token:     secret.New(presented.Secret()),
			ExpiresAt: credential.ExpiresAt,
			Methods:   []string{methodTerms},
			Terms:     &terms,
		}
		return nil
	})
	if err != nil {
		return SignInChallenge{}, err
	}
	return challenge, nil
}

// recordTermsAgreement stores the agreement on the account and in the workspace's trail, in the
// caller's transaction.
func (w SessionWriter) recordTermsAgreement(
	ctx context.Context, tenantID shared.ID, account domain.Account, terms domain.TermsInForce,
	now time.Time,
) error {
	agreed, err := w.Terms.RecordTermsAgreement(ctx, account.ID, domain.TermsAgreement{
		URL: terms.URL, Version: terms.Version, AgreedAt: now,
	})
	if err != nil {
		return err
	}
	if !agreed {
		return shared.ErrNotFound.WithDetail("accounts.not_found")
	}
	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   tenantID,
		OccurredAt: now,
		Action:     TermsAgreedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  appshared.ActorUser,
		ActorID:    account.ID,
		ActorLabel: account.DisplayName,
		TargetType: accountTarget,
		TargetID:   account.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "terms_url", Classification: audit.Open, To: terms.URL},
			audit.Change{Field: "terms_version", Classification: audit.Open, To: terms.Version},
			audit.Change{Field: "terms_set_by", Classification: audit.Open, To: string(terms.SetBy)},
		),
	})
}

// termsChanged is the refusal of an agreement to terms that are no longer the ones in force: the
// card shows the new ones from the parameters, and the person agrees to those.
func termsChanged(terms domain.TermsInForce) error {
	return shared.ErrConflict.
		WithDetail("auth.terms_changed").
		WithParams(map[string]string{"url": terms.URL, "version": terms.Version})
}

// termsNotAgreed is the refusal of an invitation accepted without the agreement the terms in force
// ask for.
func termsNotAgreed() error {
	return shared.ErrValidation.
		WithDetail("auth.terms_not_agreed").
		WithFields(shared.FieldError{Path: "/terms_url", Code: "auth.terms_not_agreed"})
}

// termsToAgree checks the agreement an invitation card sent against the terms in force: none in
// force needs none (and one sent is ignored), otherwise it must be to exactly these. It answers the
// terms to record.
func (w SessionWriter) termsToAgree(
	ctx context.Context, tenantID shared.ID, url, version string,
) (domain.TermsInForce, error) {
	if w.TermsIn == nil || w.Terms == nil {
		return domain.TermsInForce{}, nil
	}
	terms, err := w.TermsIn.TermsInForce(ctx, tenantID)
	if err != nil || !terms.IsSet() {
		return domain.TermsInForce{}, err
	}
	if url == "" {
		return domain.TermsInForce{}, termsNotAgreed()
	}
	if !terms.Matches(url, version) {
		return domain.TermsInForce{}, termsChanged(terms)
	}
	return terms, nil
}

// agreedOnTheCard records the agreement a provider invitation carried on its flow, once the arrival
// activated the invited account (UC-ID-19 check 1). Terms that changed since the card are not
// recorded: the TERMS step that follows asks for the ones in force.
func (w SessionWriter) agreedOnTheCard(
	ctx context.Context, scope persistence.Scope, account domain.Account, carried domain.TermsAgreement,
) error {
	if w.TermsIn == nil || w.Terms == nil || carried.URL == "" {
		return nil
	}
	terms, err := w.TermsIn.TermsInForce(ctx, scope.TenantID)
	if err != nil || !terms.IsSet() || !terms.Matches(carried.URL, carried.Version) {
		return err
	}
	return w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		return w.recordTermsAgreement(ctx, scope.TenantID, account, terms, w.Clock.Now())
	})
}

// AgreeToTermsCommand carries the TERMS step: the credential, and the terms as they were shown.
type AgreeToTermsCommand struct {
	PendingToken secret.Secret
	TermsURL     string
	TermsVersion string
	// TenantHeader may confirm the token's tenant, never overrule it.
	TenantHeader string
}

// NewcomerArrivals makes the account a provider's newcomer waited on the TERMS step for. The
// provider is opened before the agreement's transaction, as every sign-in opens it; the account is
// made inside it, so the agreement and the account are written together or not at all.
type NewcomerArrivals interface {
	ArrivalProvider(ctx context.Context, scope persistence.Scope, providerID shared.ID) (domain.IdentityProvider, error)
	Arrive(
		ctx context.Context, scope persistence.Scope, configured domain.IdentityProvider,
		arrival domain.ArrivalIntent,
	) (domain.Account, error)
}

// AgreeToTerms completes the TERMS step and opens the session the sign-in proved.
//
// Arrivals makes a provider's newcomer the step carried; without it such a step is refused as an
// unknown credential, never answered with an account made some other way.
type AgreeToTerms struct {
	Writer   SessionWriter
	Arrivals NewcomerArrivals
}

// Execute agrees and signs in. Terms that changed while the step was open are refused without
// spending the credential, so the person can agree to the new ones; terms removed meanwhile need no
// agreement, and the session opens.
func (h AgreeToTerms) Execute(ctx context.Context, cmd AgreeToTermsCommand) (SessionPair, error) {
	w := h.Writer

	token, err := domain.ParsePendingToken(cmd.PendingToken.Reveal())
	if err != nil {
		w.failure(ctx, FailureMfa)
		return SessionPair{}, challengeRefused()
	}
	if cmd.TenantHeader != "" && cmd.TenantHeader != token.TenantID().String() {
		return SessionPair{}, shared.ErrForbidden.WithDetail("access.tenant_mismatch")
	}
	if w.TermsIn == nil || w.Terms == nil {
		return SessionPair{}, challengeRefused()
	}
	scope := persistence.Scope{TenantID: token.TenantID()}
	terms, err := w.TermsIn.TermsInForce(ctx, token.TenantID())
	if err != nil {
		return SessionPair{}, err
	}
	configured, err := h.newcomersProvider(ctx, scope, token)
	if err != nil {
		return SessionPair{}, err
	}

	var (
		account domain.Account
		hint    domain.PendingCredential
	)
	err = w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		lookup, err := w.Pending.FindByToken(ctx, token)
		if errors.Is(err, shared.ErrNotFound) && !configured.ID.IsZero() {
			lookup, err = w.Pending.FindArrivalByToken(ctx, token)
		}
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				w.failure(ctx, FailureMfa)
				return challengeRefused()
			}
			return err
		}
		newcomer := lookup.Credential.Arrival
		if newcomer != nil && newcomer.ProviderID != configured.ID {
			w.failure(ctx, FailureMfa)
			return challengeRefused()
		}
		now := w.Clock.Now()
		if err := lookup.Credential.Verify(now); err != nil {
			w.failure(ctx, FailureMfa)
			return err
		}
		if lookup.Credential.Purpose != domain.PendingTerms {
			w.failure(ctx, FailureMfa)
			return challengeRefused()
		}
		if newcomer == nil {
			if err := lookup.Account.Verify(); err != nil {
				return err
			}
		}
		if err := lookup.TenantStatus.Verify(); err != nil {
			return err
		}
		if terms.IsSet() && !terms.Matches(cmd.TermsURL, cmd.TermsVersion) {
			// Before the credential is spent: the step stays open for the terms now in force.
			return termsChanged(terms)
		}

		consumed, err := w.Pending.Consume(ctx, lookup.Credential.ID, now)
		if err != nil {
			return err
		}
		if !consumed {
			w.failure(ctx, FailureMfa)
			return challengeRefused()
		}
		if newcomer != nil {
			// The account is made here, in the agreement's transaction (UC-ID-19 check 1): a person
			// who declined, or let the step run out, never had one.
			made, err := h.Arrivals.Arrive(ctx, scope, configured, *newcomer)
			if err != nil {
				return err
			}
			lookup.Account = made
		}
		if terms.IsSet() {
			if err := w.recordTermsAgreement(ctx, scope.TenantID, lookup.Account, terms, now); err != nil {
				return err
			}
		}
		account, hint = lookup.Account, lookup.Credential
		return nil
	})
	if err != nil {
		return SessionPair{}, err
	}

	bounds, err := w.sessionBounds(ctx, token.TenantID(), account)
	if err != nil {
		return SessionPair{}, err
	}
	return w.openSessionWithHint(ctx, scope, token.TenantID(), account,
		hint.UserAgent, hint.IPClass, actionOf(hint.SessionMethod), bounds,
		hint.SessionMethod, hint.SessionVia)
}

// newcomersProvider opens the provider a newcomer's step names, before the agreement's transaction
// (a provider is opened in a read of its own). The zero value where the token names no newcomer, or
// nothing can make one.
func (h AgreeToTerms) newcomersProvider(
	ctx context.Context, scope persistence.Scope, token domain.Token,
) (domain.IdentityProvider, error) {
	if h.Arrivals == nil {
		return domain.IdentityProvider{}, nil
	}
	var arrival *domain.ArrivalIntent
	err := h.Writer.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		found, err := h.Writer.Pending.FindArrivalByToken(ctx, token)
		if errors.Is(err, shared.ErrNotFound) {
			return nil
		}
		arrival = found.Credential.Arrival
		return err
	})
	if err != nil || arrival == nil {
		return domain.IdentityProvider{}, err
	}
	return h.Arrivals.ArrivalProvider(ctx, scope, arrival.ProviderID)
}

// Descriptor is the catalogue entry.
func (h AgreeToTerms) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: AgreeToTermsName,
		Summary: "Completes the TERMS step of a sign-in: the account has not agreed to the terms " +
			"of use in force, and the session the sign-in proved opens once it agrees. The " +
			"request repeats the link and version it was shown; terms that changed meanwhile " +
			"are refused without spending the credential.",
		SideEffects: "Consumes the pending credential, stores the agreement on the account, " +
			"writes the agreement and the sign-in to the trail, and opens a session. For a " +
			"newcomer arriving through a provider, it first creates the account, connects the " +
			"provider and gives the newcomer's access.",
		Input: []usecase.Field{
			{Name: "pending_token", Kind: usecase.KindString, Required: true,
				Description: "The TERMS challenge's credential. It dies on use."},
			{Name: "terms_url", Kind: usecase.KindString, Required: true,
				Description: "The link of the terms agreed to, as the challenge showed it."},
			{Name: "terms_version", Kind: usecase.KindString,
				Description: "Their version as the challenge showed it, where they carry one."},
			{Name: "tenant_header", Kind: usecase.KindString,
				Description: "The X-Hubtask-Tenant header, when sent. It may confirm the " +
					"token's tenant, never overrule it."},
		},
		Audit: usecase.AuditDeclaration{
			Action: TermsAgreedAction, TargetType: accountTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A sign-in is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h AgreeToTerms) invoke(
	ctx context.Context, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	pair, err := h.Execute(ctx, AgreeToTermsCommand{
		PendingToken: secret.New(in.String("pending_token")),
		TermsURL:     in.String("terms_url"),
		TermsVersion: in.String("terms_version"),
		TenantHeader: in.String("tenant_header"),
	})
	if err != nil {
		return nil, err
	}
	return pairOutput(pair), nil
}
