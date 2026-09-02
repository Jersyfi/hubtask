// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	provider "github.com/Jersyfi/hubtask/core/port/identityprovider"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const (
	StartOidcSignInName    = "StartOidcSignIn"
	CompleteOidcSignInName = "CompleteOidcSignIn"

	// verifierBytes is what PKCE's verifier is drawn from. 32 bytes become 43 base64url
	// characters, which is RFC 7636's minimum and enough: the verifier defends one round trip.
	verifierBytes = 32
	// nonceBytes is the same for the value that ties an identity token to this flow.
	nonceBytes = 32
)

// The audit codes of a sign-in through somebody else's provider (H-04).
const (
	// OidcSignInStartedAction is the first half, EnrollTotp's precedent: a two-step flow whose
	// beginning is worth recording, because a workspace full of starts that never complete is
	// the signature of a broken provider or of somebody probing.
	OidcSignInStartedAction audit.Action = "identity.provider_sign_in_started"
	// OidcSignedInAction is the arrival itself, beside the password sign-in's own entry.
	OidcSignedInAction audit.Action = "identity.provider_signed_in"
	// OidcProvisionedAction is the first arrival of a subject: an account that exists because
	// a provider vouched for somebody, which is a creation nobody here performed.
	OidcProvisionedAction audit.Action = "identity.provider_provisioned"
	// OidcLinkedAction is the one that matters most in a review: an arriving subject took over
	// an account that already existed, on the strength of a verified address.
	OidcLinkedAction audit.Action = "identity.provider_linked"
)

// OidcWriter is what the two halves of the flow share.
type OidcWriter struct {
	Session   SessionWriter
	Providers repository.IdentityProviders
	Flows     repository.OidcFlows
	External  repository.ExternalAccounts
	Accounts  repository.Accounts
	Relying   provider.Port
	// RedirectURL is where the provider sends the browser back, and it is this installation's
	// own - computed by the composition root from the configured base URL. Never from a request:
	// a redirect target a caller chooses is how authorization codes end up somewhere else.
	RedirectURL string
}

// StartOidcSignIn is the first half: where to send the browser.
type StartOidcSignIn struct{ Writer OidcWriter }

// StartOidcSignInCommand carries the tenant hints of decision 3, and a login hint if the caller
// has one.
type StartOidcSignInCommand struct {
	LoginHint    string
	TenantSlug   string
	TenantHeader string
}

// OidcAuthorization is the answer: where to go, and the handle to come back with.
type OidcAuthorization struct {
	URL       string
	State     secret.Secret
	ExpiresAt time.Time
}

// Execute opens a flow and builds the authorization request.
func (h StartOidcSignIn) Execute(
	ctx context.Context, cmd StartOidcSignInCommand,
) (OidcAuthorization, error) {
	w := h.Writer

	tenantID, err := w.Session.resolveTenant(ctx, cmd.TenantSlug, cmd.TenantHeader)
	if err != nil {
		return OidcAuthorization{}, err
	}
	scope := persistence.Scope{TenantID: tenantID}

	configured, sealed, err := w.provider(ctx, scope)
	if err != nil {
		return OidcAuthorization{}, err
	}

	state, verifier, nonce, err := w.draw(tenantID)
	if err != nil {
		return OidcAuthorization{}, err
	}

	now := w.Session.Clock.Now()
	flow, err := domain.NewOidcFlow(domain.NewOidcFlowInput{
		ID: w.Session.IDs.NewID(), TenantID: tenantID,
		Nonce: nonce, Verifier: verifier, Now: now,
	})
	if err != nil {
		return OidcAuthorization{}, err
	}

	// The provider is asked before the flow is written: an unreachable one leaves no row behind,
	// and the person is told it is the provider rather than being sent to a page that is not there.
	url, err := w.Relying.AuthorizationURL(ctx, provider.Config{
		Issuer: configured.Issuer, ClientID: configured.ClientID,
		ClientSecret: sealed, RedirectURL: w.RedirectURL,
	}, provider.Authorization{
		State: state.Secret(), Nonce: nonce, CodeVerifier: verifier, LoginHint: cmd.LoginHint,
	})
	if err != nil {
		return OidcAuthorization{}, err
	}

	if err := w.Session.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		if err := w.Flows.Insert(ctx, flow, state); err != nil {
			return err
		}
		return w.recordStart(ctx, tenantID, configured)
	}); err != nil {
		return OidcAuthorization{}, err
	}

	return OidcAuthorization{
		URL: url, State: secret.New(state.Secret()), ExpiresAt: flow.ExpiresAt,
	}, nil
}

// CompleteOidcSignIn is the second half: the code becomes a session.
type CompleteOidcSignIn struct{ Writer OidcWriter }

// CompleteOidcSignInCommand is what the provider's redirect carried back.
type CompleteOidcSignInCommand struct {
	Code  string
	State secret.Secret
	// UserAgent and RemoteAddr are recorded on the session, as on every other way in.
	UserAgent    string
	RemoteAddr   string
	TenantHeader string
}

// Execute burns the flow, verifies the token, finds or makes the account, and opens the session.
//
// The workspace comes from the state rather than from the request. The browser arrives from
// somebody else's site on this leg, and the handle this installation minted is the only thing
// about that arrival it can vouch for.
func (h CompleteOidcSignIn) Execute(
	ctx context.Context, cmd CompleteOidcSignInCommand,
) (SessionPair, error) {
	w := h.Writer

	state, err := domain.ParseOidcFlowState(cmd.State.Reveal())
	if err != nil {
		w.Session.failure(ctx, FailureOidc)
		return SessionPair{}, oidcRefused()
	}
	if cmd.TenantHeader != "" && cmd.TenantHeader != state.TenantID().String() {
		return SessionPair{}, shared.ErrForbidden.WithDetail("access.tenant_mismatch")
	}
	scope := persistence.Scope{TenantID: state.TenantID()}

	configured, sealed, err := w.provider(ctx, scope)
	if err != nil {
		return SessionPair{}, err
	}

	// Burned first, in its own transaction: whatever happens afterwards, this state is spent.
	// A failure that rolled the burn back would leave a handle somebody could present again.
	var flow domain.OidcFlow
	if err := w.Session.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		found, ok, err := w.Flows.Consume(ctx, state, w.Session.Clock.Now())
		if err != nil {
			return err
		}
		if !ok {
			w.Session.failure(ctx, FailureOidc)
			return oidcRefused()
		}
		flow = found
		return nil
	}); err != nil {
		return SessionPair{}, err
	}

	identity, err := w.Relying.Exchange(ctx, provider.Config{
		Issuer: configured.Issuer, ClientID: configured.ClientID,
		ClientSecret: sealed, RedirectURL: w.RedirectURL,
	}, provider.Exchange{
		Code: cmd.Code, CodeVerifier: flow.Verifier, Nonce: flow.Nonce,
	})
	if err != nil {
		w.Session.failure(ctx, FailureOidc)
		return SessionPair{}, err
	}

	account, err := w.settleAccount(ctx, scope, configured, identity)
	if err != nil {
		return SessionPair{}, err
	}

	// The same check every other way in makes, and the acceptance criterion names it: a
	// suspended account is refused here and not only on its first sign-in.
	if err := account.Verify(); err != nil {
		w.Session.failure(ctx, FailureOidc)
		return SessionPair{}, err
	}

	return w.Session.openSession(ctx, scope, state.TenantID(), account,
		cmd.UserAgent, cmd.RemoteAddr, OidcSignedInAction, nil)
}

// settleAccount finds the person the subject names, links them, or makes them.
//
// Three cases in one place, because which of them happened is the interesting part of a review
// and splitting them would leave nobody able to see the order they are tried in.
func (w OidcWriter) settleAccount(
	ctx context.Context, scope persistence.Scope,
	configured domain.IdentityProvider, arriving provider.Identity,
) (domain.Account, error) {
	var account domain.Account
	err := w.Session.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		// The subject first: on every arrival after the first, this is the whole of it.
		found, err := w.External.FindBySubject(ctx, arriving.Subject)
		switch {
		case err == nil:
			account = found
			return nil
		case !errors.Is(err, shared.ErrNotFound):
			return err
		}

		// A first arrival. If the provider vouched for an address inside the configured
		// domains, and an account here already holds it, this is the same person.
		if configured.LinksAddress(arriving.Email, arriving.EmailVerified) {
			existing, err := w.Accounts.FindByEmail(ctx, arriving.Email)
			switch {
			case err == nil:
				linked, err := w.External.LinkSubject(
					ctx, existing.ID, arriving.Subject, w.Session.Clock.Now())
				if err != nil {
					return err
				}
				if !linked {
					// The account is already bound to another subject, or is gone. Refusing is
					// the only safe answer: quietly re-pointing an account at a new subject is
					// how one person's address becomes another person's session.
					return shared.ErrConflict.WithDetail("identity_provider.account_taken")
				}
				account = existing
				return w.record(ctx, OidcLinkedAction, existing, configured, arriving)
			case !errors.Is(err, shared.ErrNotFound):
				return err
			}
		}

		provisioned, err := domain.ProvisionExternal(
			w.Session.IDs.NewID(), scope.TenantID, arriving.Email, arriving.DisplayName)
		if err != nil {
			return err
		}
		if err := w.Accounts.Insert(ctx, provisioned); err != nil {
			return err
		}
		if _, err := w.External.LinkSubject(
			ctx, provisioned.ID, arriving.Subject, w.Session.Clock.Now()); err != nil {
			return err
		}
		account = provisioned
		return w.record(ctx, OidcProvisionedAction, provisioned, configured, arriving)
	})
	if err != nil {
		return domain.Account{}, err
	}
	return account, nil
}

// provider reads the workspace's configuration and refuses the flow when there is none or it is
// switched off - which is a refusal a person can act on, rather than a page that fails later.
func (w OidcWriter) provider(
	ctx context.Context, scope persistence.Scope,
) (domain.IdentityProvider, secret.Secret, error) {
	var (
		configured domain.IdentityProvider
		opened     secret.Secret
	)
	err := w.Session.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		found, sealed, err := w.Providers.FindWithSecret(ctx)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return shared.ErrValidation.WithDetail("identity_provider.not_configured")
			}
			return err
		}
		if !found.Enabled {
			return shared.ErrValidation.WithDetail("identity_provider.disabled")
		}
		plaintext, err := w.Session.Encryptor.Open(ctx, sealed, clientSecretPurpose(scope.TenantID))
		if err != nil {
			return err
		}
		configured, opened = found, plaintext
		return nil
	})
	if err != nil {
		return domain.IdentityProvider{}, secret.Secret{}, err
	}
	return configured, opened, nil
}

// draw mints the three unguessable values one flow needs, through the entropy port (rule 4).
func (w OidcWriter) draw(tenantID shared.ID) (domain.Token, string, string, error) {
	material, err := w.Session.Entropy.Bytes(domain.TokenSecretBytes)
	if err != nil {
		return domain.Token{}, "", "", shared.ErrInternal.
			WithDetail("auth.session_unmintable").WithCause(err)
	}
	state, err := domain.NewOidcFlowState(tenantID, material)
	if err != nil {
		return domain.Token{}, "", "", err
	}

	verifierBytesDrawn, err := w.Session.Entropy.Bytes(verifierBytes)
	if err != nil {
		return domain.Token{}, "", "", shared.ErrInternal.
			WithDetail("auth.session_unmintable").WithCause(err)
	}
	nonceBytesDrawn, err := w.Session.Entropy.Bytes(nonceBytes)
	if err != nil {
		return domain.Token{}, "", "", shared.ErrInternal.
			WithDetail("auth.session_unmintable").WithCause(err)
	}

	return state,
		base64.RawURLEncoding.EncodeToString(verifierBytesDrawn),
		base64.RawURLEncoding.EncodeToString(nonceBytesDrawn),
		nil
}

// recordStart notes that a sign-in was begun. The actor is the installation: nobody has proved
// who they are yet, and an unauthenticated request performs no auditable action of its own
// (shared.ActorAnonymous). What the entry answers is "somebody began a sign-in for this
// workspace, at this moment, against this issuer".
func (w OidcWriter) recordStart(
	ctx context.Context, tenantID shared.ID, configured domain.IdentityProvider,
) error {
	return w.Session.Audit.Append(ctx, audit.Entry{
		TenantID:   tenantID,
		OccurredAt: w.Session.Clock.Now(),
		Action:     OidcSignInStartedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  shared.ActorSystem,
		TargetType: identityProviderTarget,
		TargetID:   tenantID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "issuer", Classification: audit.Open, To: configured.Issuer}),
	})
}

// record writes the trail entry for an account that arrived through the provider. The subject is
// not in it: it identifies a person at their provider, and the account it became is the thing a
// reader needs.
func (w OidcWriter) record(
	ctx context.Context, action audit.Action, account domain.Account,
	configured domain.IdentityProvider, arriving provider.Identity,
) error {
	return w.Session.Audit.Append(ctx, audit.Entry{
		TenantID:    account.TenantID,
		OccurredAt:  w.Session.Clock.Now(),
		Action:      action,
		Outcome:     audit.OutcomeSuccess,
		Severity:    audit.SeverityNotice,
		ActorKind:   shared.ActorSystem,
		ActorID:     account.ID,
		ActorLabel:  account.DisplayName,
		TargetType:  accountTarget,
		TargetID:    account.ID,
		TargetLabel: account.DisplayName,
		Context:     audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "issuer", Classification: audit.Open, To: configured.Issuer},
			audit.Change{Field: "email_verified", Classification: audit.Open,
				To: strconv.FormatBool(arriving.EmailVerified)},
		),
	})
}

// oidcRefused is the one answer every dead end of the flow gives. Which of them it was - an
// unknown state, an expired one, one already spent - is in the trail and the metric, never in
// the answer (T-02's discipline, applied to the second way in).
func oidcRefused() error {
	return shared.ErrUnauthenticated.WithDetail("auth.oidc_failed")
}

func (h StartOidcSignIn) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: StartOidcSignInName,
		Summary: "Begins a sign-in through the workspace's identity provider and answers where " +
			"to send the browser. The redirect the provider will use is this installation's " +
			"own - nothing about it comes from the request - and the handle answered here is " +
			"single use: it is spent at the callback whether or not the callback succeeds.",
		SideEffects: "Writes a short-lived sign-in flow and asks the provider for its metadata.",
		Input: []usecase.Field{
			{Name: "login_hint", Kind: usecase.KindString,
				Description: "An address to save somebody typing it twice. It never decides which account is signed in."},
			{Name: "tenant_slug", Kind: usecase.KindString,
				Description: "The subdomain the request arrived under, in multi mode."},
			{Name: "tenant_header", Kind: usecase.KindString,
				Description: "The X-Hubtask-Tenant header, when sent."},
		},
		Audit: usecase.AuditDeclaration{
			Action: OidcSignInStartedAction, TargetType: identityProviderTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A sign-in is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h StartOidcSignIn) invoke(
	ctx context.Context, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	authorization, err := h.Execute(ctx, StartOidcSignInCommand{
		LoginHint:    in.String("login_hint"),
		TenantSlug:   in.String("tenant_slug"),
		TenantHeader: in.String("tenant_header"),
	})
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"authorization_url": authorization.URL,
		// The one place the state is ever answered: it is a credential for one round trip.
		"state":      authorization.State.Reveal(),
		"expires_at": authorization.ExpiresAt,
	}, nil
}

func (h CompleteOidcSignIn) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CompleteOidcSignInName,
		Summary: "Finishes a sign-in through the workspace's provider: the code is exchanged " +
			"with the verifier this installation kept, the identity token is verified in full, " +
			"and the answer is the same pair a password sign-in answers - because it is the " +
			"same session. A subject arriving for the first time is provisioned, or linked to " +
			"an account whose verified address falls inside the configured domains.",
		SideEffects: "Spends the flow, may create or link an account, opens a session, and " +
			"writes an audit entry.",
		Input: []usecase.Field{
			{Name: "code", Kind: usecase.KindString, Required: true,
				Description: "The authorization code, exchanged once."},
			{Name: "state", Kind: usecase.KindString, Required: true,
				Description: "The handle the start answered. Single use, and it names the workspace."},
			{Name: "user_agent", Kind: usecase.KindString,
				Description: "The client as it introduced itself; recorded on the session."},
			{Name: "remote_addr", Kind: usecase.KindString,
				Description: "The peer's address; only its network class is ever recorded."},
			{Name: "tenant_header", Kind: usecase.KindString,
				Description: "The X-Hubtask-Tenant header, when sent. It may confirm the state's workspace, never overrule it."},
		},
		Audit: usecase.AuditDeclaration{
			Action: OidcSignedInAction, TargetType: sessionTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A sign-in is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CompleteOidcSignIn) invoke(
	ctx context.Context, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	pair, err := h.Execute(ctx, CompleteOidcSignInCommand{
		Code:         in.String("code"),
		State:        secret.New(in.String("state")),
		UserAgent:    in.String("user_agent"),
		RemoteAddr:   in.String("remote_addr"),
		TenantHeader: in.String("tenant_header"),
	})
	if err != nil {
		return nil, err
	}
	return pairOutput(pair), nil
}
