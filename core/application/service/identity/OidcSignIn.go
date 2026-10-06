// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"encoding/base64"
	"errors"
	"slices"
	"strconv"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	provider "github.com/Jersyfi/hubtask/core/port/identityprovider"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
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
	// OidcRefusedAction is a subject the provider vouched for and this workspace would not have
	// (SI-10): an INVITED_ONLY provider met somebody nobody invited. A trail of these is either a
	// provisioning rule set too tight or somebody trying the door, and both are worth reading.
	OidcRefusedAction audit.Action = "identity.provider_refused"
)

// OidcWriter is what the two halves of the flow share.
type OidcWriter struct {
	Session   SessionWriter
	Providers repository.IdentityProviders
	// Workspaces answers which of the installation's providers this workspace took (SI-10): a flow
	// through one it did not take, or one whose withdrawal has come, is refused here whatever
	// identifier the caller sends - the card not drawing a button is not what refuses it. Nil reads
	// "nothing taken", so an installation's provider opens no flow on a build wired without it.
	Workspaces repository.Workspaces
	Flows      repository.OidcFlows
	External   repository.ExternalAccounts
	Accounts   repository.Accounts
	Relying    provider.Port
	// Domains brings a provisioned address's domain to its ASCII form (M-10).
	Domains text.DomainEncoder
	// Text brings a provisioned display name to normal form C (i18n-l10n.md §5, M-07).
	Text text.Normalizer
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
	// ProviderID names the way in. Zero is allowed while a workspace has exactly one, which keeps
	// a caller that predates the plural working; with a choice to make, not making it is refused
	// rather than guessed.
	ProviderID   shared.ID
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

	configured, sealed, err := w.provider(ctx, scope, cmd.ProviderID)
	if err != nil {
		return OidcAuthorization{}, err
	}

	state, verifier, nonce, err := w.draw(tenantID)
	if err != nil {
		return OidcAuthorization{}, err
	}

	now := w.Session.Clock.Now()
	flow, err := domain.NewOidcFlow(domain.NewOidcFlowInput{
		ID: w.Session.IDs.NewID(), TenantID: tenantID, ProviderID: configured.ID,
		Nonce: nonce, Verifier: verifier, Now: now,
	})
	if err != nil {
		return OidcAuthorization{}, err
	}

	// The provider is asked before the flow is written: an unreachable one leaves no row behind,
	// and the person is told it is the provider rather than being sent to a page that is not there.
	url, err := w.Relying.AuthorizationURL(ctx, relyingConfig(configured, sealed, w.RedirectURL), provider.Authorization{
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
) (SignInResult, error) {
	w := h.Writer

	state, err := domain.ParseOidcFlowState(cmd.State.Reveal())
	if err != nil {
		w.Session.failure(ctx, FailureOidc)
		return SignInResult{}, oidcRefused()
	}
	if cmd.TenantHeader != "" && cmd.TenantHeader != state.TenantID().String() {
		return SignInResult{}, shared.ErrForbidden.WithDetail("access.tenant_mismatch")
	}
	scope := persistence.Scope{TenantID: state.TenantID()}

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
		return SignInResult{}, err
	}

	// The provider is read *after* the burn and from the flow, not from the request: which way in
	// this sign-in left through is the one thing about the return leg that this installation itself
	// wrote down, and signing the exchange with another provider's secret is how a code meant for
	// one becomes an identity from another.
	configured, sealed, err := w.provider(ctx, scope, flow.ProviderID)
	if err != nil {
		return SignInResult{}, err
	}

	identity, err := w.Relying.Exchange(ctx, relyingConfig(configured, sealed, w.RedirectURL), provider.Exchange{
		Code: cmd.Code, CodeVerifier: flow.Verifier, Nonce: flow.Nonce,
	})
	if err != nil {
		w.Session.failure(ctx, FailureOidc)
		return SignInResult{}, err
	}

	account, owed, err := w.settleAccount(ctx, scope, configured, identity)
	if err != nil {
		return SignInResult{}, err
	}

	// The same check every other way in makes, and the acceptance criterion names it: a
	// suspended account is refused here and not only on its first sign-in.
	if err := account.Verify(); err != nil {
		w.Session.failure(ctx, FailureOidc)
		return SignInResult{}, err
	}

	// The account already holds a credential of its own, so the provider's word does not open it:
	// the card asks for that credential first (ADR-0071's addendum, E2).
	if owed {
		challenge, err := w.challengeLink(ctx, scope, account, configured, identity, cmd)
		if err != nil {
			return SignInResult{}, err
		}
		return SignInResult{Challenge: &challenge}, nil
	}

	// A provider sign-in holds no password for the rule to judge, but its session answers to the
	// workspace's bounds like every other (UC-ID-08 check 4), and it records how it was opened.
	bounds, err := w.Session.sessionBounds(ctx, state.TenantID(), account)
	if err != nil {
		return SignInResult{}, err
	}
	pair, err := w.Session.openSessionVia(ctx, scope, state.TenantID(), account,
		cmd.UserAgent, cmd.RemoteAddr, OidcSignedInAction, nil,
		bounds, domain.SignedInWithOidc, configured.ID)
	if err != nil {
		return SignInResult{}, err
	}
	return SignInResult{Pair: &pair}, nil
}

// settleAccount finds the person the subject names, links them, or makes them.
//
// Three cases in one place, because which of them happened is the interesting part of a review
// and splitting them would leave nobody able to see the order they are tried in.
//
// The second answer is whether the account the arrival matched still owes its own proof before the
// provider may be connected to it (ADR-0071's addendum, E2). Then nothing is linked yet.
func (w OidcWriter) settleAccount(
	ctx context.Context, scope persistence.Scope,
	configured domain.IdentityProvider, arriving provider.Identity,
) (domain.Account, bool, error) {
	var (
		account domain.Account
		owed    bool
	)
	err := w.Session.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		// The subject first: on every arrival after the first, this is the whole of it.
		found, err := w.External.FindBySubject(ctx, configured.ID, arriving.Subject)
		switch {
		case err == nil:
			// Connected before, and still invited: an arrival before SC-24 connected the provider
			// and was then refused as an account that may not act. The invitation is accepted now.
			if found.Status == domain.AccountInvited {
				accepted, err := w.acceptInvitation(ctx, found, configured)
				if err != nil {
					return err
				}
				found = accepted
			}
			account = found
			return nil
		case !errors.Is(err, shared.ErrNotFound):
			return err
		}

		// A first arrival, and the first gate is admission: may this provider bring this person
		// into this workspace at all (SI-10, the concept's §8). Under DOMAINS an address outside
		// the configured list is refused here - not provisioned a desk of its own, which is what
		// made the mode indistinguishable from ANY.
		if !configured.MayAdmit(admissionOf(arriving)) {
			if err := w.recordRefusal(ctx, scope.TenantID, configured); err != nil {
				return err
			}
			return shared.ErrForbidden.WithDetail("identity_provider.not_admitted")
		}

		// Admitted. If an account here already holds the address the provider vouched for, this is
		// the same person.
		if configured.MayClaim(admissionOf(arriving)) {
			existing, err := w.Accounts.FindByEmail(ctx, domain.LookupAddress(arriving.Email, w.Domains))
			switch {
			case err == nil:
				// The address matched. Whether the provider may simply be connected depends on what
				// the account already holds, not on who configured the provider: an account with a
				// password is connected only after that password (and its second factor) is proven,
				// and one that signs in some other way cannot give that proof here.
				proof, err := w.proofOwed(ctx, existing)
				if err != nil {
					return err
				}
				switch proof {
				case proofPassword:
					account, owed = existing, true
					return nil
				case proofElsewhere:
					if err := w.recordRefusal(ctx, scope.TenantID, configured); err != nil {
						return err
					}
					return shared.ErrForbidden.WithDetail("identity_provider.link_needs_own_way_in")
				}

				// An invitation nobody redeemed is accepted here, through the provider, in the
				// same transaction as the connection (SC-24): the account becomes ACTIVE as
				// redeeming would make it, and an invited person in a workspace without the password
				// has a way in. One that ran out stays refused - the provider's word does not renew
				// the operator's offer.
				if existing.Status == domain.AccountInvited {
					accepted, err := w.acceptInvitation(ctx, existing, configured)
					if err != nil {
						return err
					}
					existing = accepted
				}

				linked, err := w.External.LinkSubject(
					ctx, configured.ID, existing.ID, arriving.Subject, w.Session.Clock.Now())
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

		// Admitted, and no account here holds the address: INVITED_ONLY has no way in but an
		// account that already exists, so this is somebody nobody invited. Refused and recorded -
		// a provider whose people are all being turned away is something an operator has to be
		// able to read, and the person is told plainly rather than being provisioned a desk they
		// were never meant to have.
		if !configured.MayProvision() {
			if err := w.recordRefusal(ctx, scope.TenantID, configured); err != nil {
				return err
			}
			return shared.ErrForbidden.WithDetail("identity_provider.not_invited")
		}

		provisioned, err := domain.ProvisionExternal(
			w.Session.IDs.NewID(), scope.TenantID, arriving.Email, arriving.DisplayName, w.Domains, w.Text)
		if err != nil {
			return err
		}
		if err := w.Accounts.Insert(ctx, provisioned); err != nil {
			return err
		}
		if _, err := w.External.LinkSubject(
			ctx, configured.ID, provisioned.ID, arriving.Subject, w.Session.Clock.Now()); err != nil {
			return err
		}
		account = provisioned
		return w.record(ctx, OidcProvisionedAction, provisioned, configured, arriving)
	})
	if err != nil {
		return domain.Account{}, false, err
	}
	return account, owed, nil
}

// acceptInvitation activates an invited account through the provider and records it as the
// redemption is recorded: the same action, the status it moved, and the provider it came through.
func (w OidcWriter) acceptInvitation(
	ctx context.Context, invited domain.Account, configured domain.IdentityProvider,
) (domain.Account, error) {
	now := w.Session.Clock.Now()
	accepted, err := w.Accounts.AcceptInvitation(ctx, invited.ID, now)
	if err != nil {
		return domain.Account{}, err
	}
	if !accepted {
		return domain.Account{}, shared.ErrForbidden.WithDetail("identity_provider.invitation_lapsed")
	}
	invited.Status = domain.AccountActive
	return invited, w.Session.Audit.Append(ctx, audit.Entry{
		TenantID:   invited.TenantID,
		OccurredAt: now,
		Action:     InvitationRedeemedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  appshared.ActorUser,
		ActorID:    invited.ID,
		ActorLabel: invited.DisplayName,
		TargetType: accountTarget,
		TargetID:   invited.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{
				Field: "status", Classification: audit.Open,
				From: string(domain.AccountInvited), To: string(domain.AccountActive),
			},
			audit.Change{Field: "provider_id", Classification: audit.Open, To: configured.ID.String()}),
	})
}

// linkProof is what an existing account demands before a provider may be connected to it.
type linkProof int

const (
	// proofNone: the account holds no credential yet - an invitation nobody redeemed - so there is
	// nothing to prove and nothing to take over. That is what "invited" means.
	proofNone linkProof = iota
	// proofPassword: the account has a password, and the card asks for it (and for the second
	// factor, if one is armed) before connecting.
	proofPassword
	// proofElsewhere: no password, but a second factor or another provider's identity. Neither can
	// be proven on this card, so the arrival is refused and pointed at the way in the account has.
	proofElsewhere
)

// proofOwed reads what the account holds, in the transaction the arrival already opened.
func (w OidcWriter) proofOwed(ctx context.Context, existing domain.Account) (linkProof, error) {
	hash, err := w.Session.Accounts.PasswordHashOf(ctx, existing.ID)
	if err != nil && !errors.Is(err, shared.ErrNotFound) {
		return proofNone, err
	}
	if err == nil && !hash.IsEmpty() {
		return proofPassword, nil
	}

	if w.Session.Enrollments != nil {
		enrollment, err := w.Session.Enrollments.Find(ctx, existing.ID)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return proofNone, err
		}
		if err == nil && !enrollment.ConfirmedAt.IsZero() {
			return proofElsewhere, nil
		}
	}
	held, err := w.External.HasIdentity(ctx, existing.ID)
	if err != nil {
		return proofNone, err
	}
	if held {
		return proofElsewhere, nil
	}
	return proofNone, nil
}

// challengeLink hands the arrival a LINK credential carrying the identity it will connect, and the
// card what it needs to ask for the account's password: whose account, and which provider.
func (w OidcWriter) challengeLink(
	ctx context.Context, scope persistence.Scope, account domain.Account,
	configured domain.IdentityProvider, arriving provider.Identity, cmd CompleteOidcSignInCommand,
) (SignInChallenge, error) {
	var challenge SignInChallenge
	err := w.Session.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		material, err := w.Session.Entropy.Bytes(domain.TokenSecretBytes)
		if err != nil {
			return shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
		}
		presented, err := domain.NewPendingToken(scope.TenantID, material)
		if err != nil {
			return err
		}
		now := w.Session.Clock.Now()
		credential := domain.PendingCredential{
			ID:        w.Session.IDs.NewID(),
			TenantID:  scope.TenantID,
			AccountID: account.ID,
			Purpose:   domain.PendingLink,
			UserAgent: cmd.UserAgent,
			IPClass:   domain.IPClass(cmd.RemoteAddr),
			CreatedAt: now.UTC(),
			ExpiresAt: now.Add(domain.PendingLifetime).UTC(),
			Link:      &domain.LinkIntent{ProviderID: configured.ID, Subject: arriving.Subject},
		}
		if err := w.Session.Pending.Insert(ctx, credential, presented); err != nil {
			return err
		}
		challenge = SignInChallenge{
			Token:        secret.New(presented.Secret()),
			ExpiresAt:    credential.ExpiresAt,
			Methods:      []string{methodLink},
			Email:        account.Email,
			ProviderName: configured.DisplayName,
		}
		return nil
	})
	return challenge, err
}

// provider reads the configuration a flow runs under and refuses the flow when there is none or it
// is switched off - which is a refusal a person can act on, rather than a page that fails later.
//
// A zero identifier is answered from the collection: with exactly one way in there is nothing to
// choose, and with several, not choosing is refused rather than guessed. That is also what a flow
// opened before the column existed reads as.
func (w OidcWriter) provider(
	ctx context.Context, scope persistence.Scope, providerID shared.ID,
) (domain.IdentityProvider, secret.Secret, error) {
	return openProvider(ctx, w.Session, w.Providers, w.Workspaces, scope, providerID, w.onlyProvider)
}

// openProvider is provider's body, shared with the step-up at the provider (ADR-0075 §2), which
// opens a provider exactly as a sign-in does and needs nothing else of the sign-in's writer. `only`
// answers the one way in where no identifier came; nil refuses that case as not configured.
func openProvider(
	ctx context.Context, session SessionWriter, providers repository.IdentityProviders,
	workspaces repository.Workspaces, scope persistence.Scope, providerID shared.ID,
	only func(context.Context, domain.WorkspaceSettings, time.Time) (shared.ID, error),
) (domain.IdentityProvider, secret.Secret, error) {
	var (
		configured domain.IdentityProvider
		opened     secret.Secret
	)
	err := session.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		settings, err := offersIn(ctx, workspaces)
		if err != nil {
			return err
		}
		now := session.Clock.Now()
		chosen := providerID
		if chosen.IsZero() {
			if only == nil {
				return shared.ErrValidation.WithDetail("identity_provider.not_configured")
			}
			sole, err := only(ctx, settings, now)
			if err != nil {
				return err
			}
			chosen = sole
		}

		found, sealed, err := providers.FindWithSecret(ctx, chosen)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return shared.ErrValidation.WithDetail("identity_provider.not_configured")
			}
			return err
		}
		// The one reading of "a way in here" (offeredHere): switched on, taken by this workspace
		// where the row is the installation's, and not past an announced withdrawal (ADR-0076 §2).
		if !offeredHere(found, settings, now) {
			return shared.ErrValidation.WithDetail("identity_provider.disabled")
		}
		// The purpose is the level's, so a row of the installation's opens under the installation's
		// purpose and a workspace's under its own - which is what keeps a ciphertext from opening
		// in a workspace it was not sealed for (E-02).
		plaintext, err := session.Encryptor.Open(ctx, sealed, ClientSecretPurpose(found.TenantID))
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

// offersIn reads this workspace's own switches for the installation's providers. Nil reads "nothing
// taken".
func offersIn(ctx context.Context, workspaces repository.Workspaces) (domain.WorkspaceSettings, error) {
	if workspaces == nil {
		return domain.WorkspaceSettings{}, nil
	}
	workspace, err := workspaces.Find(ctx)
	if err != nil && !errors.Is(err, shared.ErrNotFound) {
		return domain.WorkspaceSettings{}, err
	}
	return workspace.Settings, nil
}

// onlyProvider answers the single way in, or says which of the refusals applies: none configured,
// or a choice nobody made.
func (w OidcWriter) onlyProvider(
	ctx context.Context, settings domain.WorkspaceSettings, now time.Time,
) (shared.ID, error) {
	inForce, err := w.Providers.List(ctx)
	if err != nil {
		return "", err
	}
	enabled := make([]domain.IdentityProvider, 0, len(inForce))
	for _, candidate := range inForce {
		if offeredHere(candidate, settings, now) {
			enabled = append(enabled, candidate)
		}
	}
	switch len(enabled) {
	case 0:
		return "", shared.ErrValidation.WithDetail("identity_provider.not_configured")
	case 1:
		return enabled[0].ID, nil
	default:
		return "", shared.ErrValidation.WithDetail("identity_provider.choice_required")
	}
}

// draw mints the three unguessable values one flow needs, through the entropy port (rule 4).
func (w OidcWriter) draw(tenantID shared.ID) (domain.Token, string, string, error) {
	return drawFlow(w.Session.Entropy, tenantID)
}

// drawFlow is draw's body, shared with the step-up at the provider.
func drawFlow(entropy clock.Entropy, tenantID shared.ID) (domain.Token, string, string, error) {
	material, err := entropy.Bytes(domain.TokenSecretBytes)
	if err != nil {
		return domain.Token{}, "", "", shared.ErrInternal.
			WithDetail("auth.session_unmintable").WithCause(err)
	}
	state, err := domain.NewOidcFlowState(tenantID, material)
	if err != nil {
		return domain.Token{}, "", "", err
	}

	verifierBytesDrawn, err := entropy.Bytes(verifierBytes)
	if err != nil {
		return domain.Token{}, "", "", shared.ErrInternal.
			WithDetail("auth.session_unmintable").WithCause(err)
	}
	nonceBytesDrawn, err := entropy.Bytes(nonceBytes)
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

// recordRefusal notes a subject that was turned away. No address and no subject: what a reader
// needs is that this provider refused somebody, and which issuer it was.
func (w OidcWriter) recordRefusal(
	ctx context.Context, tenantID shared.ID, configured domain.IdentityProvider,
) error {
	return w.Session.Audit.Append(ctx, audit.Entry{
		TenantID:   tenantID,
		OccurredAt: w.Session.Clock.Now(),
		Action:     OidcRefusedAction,
		Outcome:    audit.OutcomeDenied,
		Severity:   audit.SeverityWarning,
		ActorKind:  shared.ActorSystem,
		TargetType: identityProviderTarget,
		TargetID:   tenantID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "issuer", Classification: audit.Open, To: configured.Issuer},
			audit.Change{Field: "provisioning", Classification: audit.Open,
				To: string(configured.Provisioning)}),
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
			{Name: "provider_id", Kind: usecase.KindString,
				Description: "Which way in to use. Absent is allowed while the workspace has exactly one."},
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
	providerID, err := in.ID("provider_id")
	if err != nil {
		return nil, err
	}
	authorization, err := h.Execute(ctx, StartOidcSignInCommand{
		ProviderID:   providerID,
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
			"an account whose verified address falls inside the configured domains - at once " +
			"where that account holds no credential yet, and otherwise only after its own " +
			"password (and second factor): the answer is then a LINK challenge instead of a pair.",
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
	result, err := h.Execute(ctx, CompleteOidcSignInCommand{
		Code:         in.String("code"),
		State:        secret.New(in.String("state")),
		UserAgent:    in.String("user_agent"),
		RemoteAddr:   in.String("remote_addr"),
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

// relyingConfig is one provider's configuration as the relying party needs it, for a sign-in and
// for a step-up alike.
//
// The directory claim and the way the provider says it hosts a mailbox are read from the preset
// rather than stored on the row: they are properties of the provider, not of a workspace's
// configuration of it, and a column would be a second place for them to be wrong. A kind this build
// does not know is the zero preset - no directory, never authoritative (ADR-0078 §5).
func relyingConfig(
	configured domain.IdentityProvider, opened secret.Secret, redirectURL string,
) provider.Config {
	preset, _ := domain.PresetOf(configured.Kind)
	return provider.Config{
		Issuer: configured.Issuer, ClientID: configured.ClientID,
		ClientSecret: opened, RedirectURL: redirectURL,
		DirectoryClaim: preset.DirectoryClaim,
		Authority: provider.Authority{
			Claim:             preset.AuthorityClaim,
			OwnDomains:        slices.Clone(preset.OwnMailDomains),
			DirectoryIsDomain: preset.DirectoryIsMailDomain,
		},
	}
}

// admissionOf is the port's identity as the domain's admission question (ADR-0071 §1).
//
// A translation and nothing else: the adapter reads whichever claim its preset names and the domain
// asks about `Directory` and `AddressAuthoritative` without knowing that one provider calls it
// `tid` and another `hd`. Rule 1 — the domain learns no provider's vocabulary.
func admissionOf(arriving provider.Identity) domain.Arriving {
	return domain.Arriving{
		Email:                arriving.Email,
		EmailVerified:        arriving.EmailVerified,
		Directory:            arriving.Directory,
		AddressAuthoritative: arriving.AddressAuthoritative,
	}
}
