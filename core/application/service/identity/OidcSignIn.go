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
	// (SI-10): somebody admission turned away, nobody invited, an account that signs in another
	// way, or an invited account arriving without its second proof (ADR-0078 §1). A trail of these
	// is either a provisioning rule set too tight or somebody trying the door, and both are worth
	// reading. Written in a transaction of its own, since the refused arrival's is rolled back.
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
	// InvitationToken is the redemption token of the invitation this sign-in accepts, when it
	// began on the invitation card (ADR-0078 §1). Empty for every other sign-in.
	InvitationToken secret.Secret
	// ConnectToken is the CONNECT link a workspace without the password mailed, when the sign-in
	// began on the card that link opens (ADR-0078 §1, SC-33). Empty for every other sign-in.
	ConnectToken secret.Secret
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

	// Checked before the provider is asked, so an invitation that cannot be redeemed is told on the
	// card rather than after a round trip to somebody else's site.
	invited, err := w.invitationOf(ctx, scope, cmd.InvitationToken)
	if err != nil {
		return OidcAuthorization{}, err
	}
	// One link per sign-in: an invited account holds nothing to connect, and a connection is not an
	// invitation. A client that sends both is told which field it should not have.
	if !invited.IsZero() && !cmd.ConnectToken.IsEmpty() {
		return OidcAuthorization{}, shared.ErrValidation.WithDetail("usecase.input_invalid").
			WithFields(shared.FieldError{Path: "/connect_token", Code: "usecase.input_invalid"})
	}
	connect, err := w.connectLinkOf(ctx, scope, cmd.ConnectToken)
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
		Nonce: nonce, Verifier: verifier, Now: now, InvitedAccountID: invited, PendingID: connect,
	})
	if err != nil {
		return OidcAuthorization{}, err
	}

	// The provider is asked before the flow is written: an unreachable one leaves no row behind,
	// and the person is told it is the provider rather than being sent to a page that is not there.
	url, err := w.Relying.AuthorizationURL(ctx, relyingConfig(configured, sealed, w.RedirectURL), provider.Authorization{
		State: state.Secret(), Nonce: nonce, CodeVerifier: verifier, LoginHint: cmd.LoginHint,
		// The mailbox is half of a connection's proof and a sign-in at the provider the other half -
		// one made now, not a session the browser happened to keep there (ADR-0078 §1).
		Fresh: !connect.IsZero(),
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

// invitationOf reads the invitation a sign-in starts from and answers the account it invites, or
// zero where the sign-in carries none (ADR-0078 §1).
//
// Checked and **not spent**: what the flow keeps is which account the invitation names, and only an
// arrival that succeeds accepts it. Unknown, expired, already accepted and another workspace's are
// one refusal, the redemption's own - which addresses hold an open invitation is not for a probe to
// learn, here any more than at the password's door.
func (w OidcWriter) invitationOf(
	ctx context.Context, scope persistence.Scope, presented secret.Secret,
) (shared.ID, error) {
	if presented.IsEmpty() {
		return "", nil
	}
	token, err := domain.ParseRedemptionToken(presented.Reveal())
	if err != nil || token.TenantID() != scope.TenantID {
		w.Session.failure(ctx, FailureRedemption)
		return "", redemptionRefused()
	}
	var invited shared.ID
	err = w.Session.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		found, err := w.Session.Accounts.FindByRedemptionToken(ctx, token)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				w.Session.failure(ctx, FailureRedemption)
				return redemptionRefused()
			}
			return err
		}
		if !found.ExpiresAt.After(w.Session.Clock.Now()) || found.Account.Status != domain.AccountInvited {
			w.Session.failure(ctx, FailureRedemption)
			return redemptionRefused()
		}
		// The workspace's standing (H-06), as the redemption asks it: an invitation into a
		// suspended workspace waits the suspension out.
		if err := found.TenantStatus.Verify(); err != nil {
			return err
		}
		invited = found.Account.ID
		return nil
	})
	return invited, err
}

// connectLinkOf reads the CONNECT link a sign-in starts from and answers its credential, or zero
// where the sign-in carries none (ADR-0078 §1, SC-33).
//
// Checked and **not spent**: the flow keeps which credential it carries, and only an arrival that
// connects the provider spends it. Unknown, expired, spent and another workspace's are one refusal,
// the reset link's own - and so is a link whose reason has gone: the password is open again, or a
// provider switched on here lets the account in by now.
func (w OidcWriter) connectLinkOf(
	ctx context.Context, scope persistence.Scope, presented secret.Secret,
) (shared.ID, error) {
	if presented.IsEmpty() {
		return "", nil
	}
	token, err := domain.ParsePendingToken(presented.Reveal())
	if err != nil || token.TenantID() != scope.TenantID {
		w.Session.failure(ctx, FailureOidc)
		return "", resetRefused()
	}
	if err := w.passwordShutFor(ctx, scope); err != nil {
		return "", err
	}
	var credential shared.ID
	err = w.Session.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		found, err := w.Session.Pending.FindByToken(ctx, token)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				w.Session.failure(ctx, FailureOidc)
				return resetRefused()
			}
			return err
		}
		stands, err := w.connectStands(ctx, found)
		if err != nil {
			return err
		}
		if !stands {
			w.Session.failure(ctx, FailureOidc)
			return resetRefused()
		}
		credential = found.Credential.ID
		return nil
	})
	return credential, err
}

// passwordShutFor refuses a CONNECT link where the password is open again - switched back on, or open
// as the fallback: there the mailbox does not stand in for the password, and *Forgot your
// password?* mails the reset link instead. Asked outside any transaction, because the rule reads the
// installation's level under a scope of its own.
func (w OidcWriter) passwordShutFor(ctx context.Context, scope persistence.Scope) error {
	door, ok := w.Session.Rule.(PasswordDoor)
	if !ok {
		return nil
	}
	open, _, err := door.PasswordOpen(ctx, scope.TenantID)
	if err != nil {
		return err
	}
	if open {
		w.Session.failure(ctx, FailureOidc)
		return resetRefused()
	}
	return nil
}

// connectStands answers whether a CONNECT link may still stand in for the account's password, inside
// a transaction bound to the link's workspace: a live CONNECT credential, an active account of a
// workspace in good standing, and no provider switched on there that already lets the account in.
// The start asks it before the browser leaves, the callback again when it comes back.
func (w OidcWriter) connectStands(ctx context.Context, found repository.PendingLookup) (bool, error) {
	now := w.Session.Clock.Now()
	if found.Credential.Purpose != domain.PendingConnect || found.Credential.Verify(now) != nil ||
		found.Account.Status != domain.AccountActive {
		return false, nil
	}
	if err := found.TenantStatus.Verify(); err != nil {
		return false, err
	}
	return w.connectsHere().connectStands(ctx, found.Account.ID, now)
}

// connectsHere is the provider stores as the reset's question reads them: which providers the
// account is connected to, and which are switched on here.
func (w OidcWriter) connectsHere() ProviderStepUps {
	return ProviderStepUps{Providers: w.Providers, External: w.External, Workspaces: w.Workspaces}
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

	// A sign-in begun from a CONNECT link connects that link's account and no other, with the
	// mailbox and this fresh sign-in as its proof (ADR-0078 §1).
	if !flow.PendingID.IsZero() {
		return w.connectByMail(ctx, scope, configured, identity, flow.PendingID, cmd)
	}

	account, owed, err := w.settleAccount(ctx, scope, configured, identity, flow.InvitedAccountID)
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
//
// `invitedID` is the invited account the flow started from, zero for every other sign-in. It is
// one of the two second proofs that activate an invited account; the provider being authoritative
// for the address is the other (ADR-0078 §1). Without either, a provider's word activates nothing
// and connects nothing.
func (w OidcWriter) settleAccount(
	ctx context.Context, scope persistence.Scope,
	configured domain.IdentityProvider, arriving provider.Identity, invitedID shared.ID,
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
			// An invitation's flow finishes that invitation and no other account's sign-in: an
			// identity connected to somebody else here is not the invited person arriving.
			if !invitedID.IsZero() && found.ID != invitedID {
				return turnedAway(invitationAddressDiffers())
			}
			// Connected before, and still invited: an arrival before SC-24 connected the provider
			// on its word alone, and SC-24 then accepted the invitation on the next one. A link
			// made without a second proof activates nothing (ADR-0078 §1): the arrival has to
			// bring the proof now, as a first arrival would.
			if found.Status == domain.AccountInvited {
				if err := w.secondProof(ctx, scope, configured, arriving, found, invitedID); err != nil {
					return err
				}
				accepted, err := w.activateInvited(ctx, found, configured, arriving, invitedID)
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

		// A first arrival through the invitation's own link: the invited account, and only it.
		if !invitedID.IsZero() {
			accepted, err := w.arriveInvited(ctx, scope, configured, arriving, invitedID)
			if err != nil {
				return err
			}
			account = accepted
			return nil
		}

		// A first arrival, and the first gate is admission: may this provider bring this person
		// into this workspace at all (SI-10, the concept's §8). Under DOMAINS an address outside
		// the configured list is refused here - not provisioned a desk of its own, which is what
		// made the mode indistinguishable from ANY.
		if !configured.MayAdmit(admissionOf(arriving)) {
			existing, err := w.notAdmitted(ctx, configured, arriving)
			if err != nil {
				if !errors.Is(err, shared.ErrForbidden) {
					return err
				}
				return turnedAway(err)
			}
			// An existing member whose provider is not authoritative for the address: the LINK
			// step, where the account's own proof is asked and nothing is connected without it.
			account, owed = existing, true
			return nil
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
					return turnedAway(shared.ErrForbidden.WithDetail("identity_provider.link_needs_own_way_in"))
				case proofMailbox:
					return turnedAway(linkNeedsMailbox())
				}

				// An invitation nobody redeemed is accepted here only with a second proof: this
				// arrival did not come through the invitation's link, so the provider has to be
				// authoritative for the address (ADR-0078 §1). The address is the account's own -
				// it is what found it.
				if existing.Status == domain.AccountInvited {
					if err := w.secondProof(ctx, scope, configured, arriving, existing, ""); err != nil {
						return err
					}
					accepted, err := w.activateInvited(ctx, existing, configured, arriving, "")
					if err != nil {
						return err
					}
					account = accepted
					return nil
				}

				// An account that holds no credential at all - its provider removed, its identity
				// gone with it - has nothing to prove on the card. The provider's word connects it
				// only where the provider hosts the mailbox (ADR-0078 §1, §5): that is a mailbox
				// proof. Anything else is an issuer vouching for an address it does not own, which
				// is how an administrator's own provider would sign in as a member (P-02). The way
				// back is the mailbox: *Forgot your password?*.
				if !arriving.AddressAuthoritative {
					return turnedAway(linkNeedsMailbox())
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
			return turnedAway(shared.ErrForbidden.WithDetail("identity_provider.not_invited"))
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
	// A refusal is recorded after the arrival's transaction is over, in one of its own: everything
	// that transaction wrote is rolled back with the refusal, and the entry is what an operator
	// reads to see a provider turning people away (SC-32).
	var refusal turnedAwayError
	if errors.As(err, &refusal) {
		if recordErr := w.recordRefusal(ctx, scope, configured); recordErr != nil {
			return domain.Account{}, false, recordErr
		}
		return domain.Account{}, false, refusal.cause
	}
	if err != nil {
		return domain.Account{}, false, err
	}
	return account, owed, nil
}

// connectByMail is the arrival of a sign-in begun from a CONNECT link (ADR-0078 §1, SC-33): the
// mailbox proved by the link, and a fresh sign-in at the provider, are together the account's proof -
// in place of the password, and of an identity at an offer that ended. They are never the second
// factor's: an armed one is still asked, and the connection is written at the end of that step.
// No password is stored.
//
// The link is spent in the same transaction that connects the identity, or that hands on to the
// factor, and only if it is still unspent - so a second callback for the same link is refused. Every
// refusal before that leaves it unspent, and an arrival the workspace turned away is recorded in a
// transaction of its own, the sign-in's discipline.
func (w OidcWriter) connectByMail(
	ctx context.Context, scope persistence.Scope, configured domain.IdentityProvider,
	arriving provider.Identity, pendingID shared.ID, cmd CompleteOidcSignInCommand,
) (SignInResult, error) {
	s := w.Session
	// Switched back on meanwhile, or open as the fallback: there the password is the proof again,
	// and the link is one whose reason has gone.
	if err := w.passwordShutFor(ctx, scope); err != nil {
		return SignInResult{}, err
	}

	var (
		account   domain.Account
		challenge *SignInChallenge
	)
	err := s.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		now := s.Clock.Now()
		found, err := s.Pending.FindByID(ctx, pendingID)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return err
		}
		stands := false
		if err == nil {
			if stands, err = w.connectStands(ctx, found); err != nil {
				return err
			}
		}
		if !stands {
			s.failure(ctx, FailureOidc)
			return resetRefused()
		}
		held, err := w.Accounts.Find(ctx, found.Account.ID)
		if err != nil {
			return err
		}

		// A sign-in the browser kept at the provider proves nothing about who is at the keyboard
		// now; the start asked for a fresh one, and a provider that ignored the question is not
		// taken at its word (ADR-0075 §2's reading of auth_time).
		if !domain.ProviderProofFresh(arriving.AuthTime, now, s.stepUpWindow()) {
			s.failure(ctx, FailureOidc)
			return turnedAway(shared.ErrForbidden.WithDetail("identity_provider.connect_not_fresh"))
		}
		// The address the link was mailed to, and no other: a differently addressed identity is
		// connected from a signed-in session, never at the front door (ADR-0078 §1, SC-37).
		if !arriving.EmailVerified || !w.sameAddress(arriving.Email, held.Email) {
			return turnedAway(shared.ErrForbidden.WithDetail("identity_provider.connect_address_differs"))
		}
		// Admission as for every arrival that brings its own proof (SC-32): the mailbox is that proof,
		// under INVITED_ONLY and under DOMAINS alike - the list decides who comes in new.
		if !configured.MayAdmitWithProof(admissionOf(arriving)) {
			return turnedAway(shared.ErrForbidden.WithDetail("identity_provider.not_admitted"))
		}
		// An identity already connected to somebody else here is that person's, and is never
		// re-pointed at the account the link names.
		owner, err := w.External.FindBySubject(ctx, configured.ID, arriving.Subject)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return err
		}
		if err == nil && owner.ID != held.ID {
			return turnedAway(shared.ErrForbidden.WithDetail("identity_provider.connect_identity_taken"))
		}

		spent, err := s.Pending.Consume(ctx, found.Credential.ID, now)
		if err != nil {
			return err
		}
		if !spent {
			s.failure(ctx, FailureOidc)
			return resetRefused()
		}
		link := domain.LinkIntent{
			ProviderID: configured.ID, Subject: arriving.Subject, Proof: domain.LinkProofMailbox,
		}
		armed := false
		if s.Enrollments != nil {
			enrollment, err := s.Enrollments.Find(ctx, held.ID)
			if err != nil && !errors.Is(err, shared.ErrNotFound) {
				return err
			}
			armed = err == nil && !enrollment.ConfirmedAt.IsZero()
		}
		account = held
		if armed {
			next, err := w.handOnToTheFactor(ctx, scope, held, domain.PendingCredential{
				UserAgent: cmd.UserAgent, IPClass: domain.IPClass(cmd.RemoteAddr), Link: &link,
			}, now)
			if err != nil {
				return err
			}
			// The person typed no address on this card: the step says whose account it is, as the
			// reset's code step does (UC-ID-04 check 5).
			next.Email = held.Email
			challenge = &next
			return nil
		}
		return w.Connect(ctx, held, link)
	})
	var refusal turnedAwayError
	if errors.As(err, &refusal) {
		if recordErr := w.recordRefusal(ctx, scope, configured); recordErr != nil {
			return SignInResult{}, recordErr
		}
		return SignInResult{}, refusal.cause
	}
	if err != nil {
		return SignInResult{}, err
	}
	if challenge != nil {
		return SignInResult{Challenge: challenge}, nil
	}

	bounds, err := s.sessionBounds(ctx, scope.TenantID, account)
	if err != nil {
		return SignInResult{}, err
	}
	pair, err := s.openSessionVia(ctx, scope, scope.TenantID, account,
		cmd.UserAgent, cmd.RemoteAddr, OidcSignedInAction, nil,
		bounds, domain.SignedInWithOidc, configured.ID)
	if err != nil {
		return SignInResult{}, err
	}
	return SignInResult{Pair: &pair}, nil
}

// turnedAwayError marks an arrival the provider vouched for and this workspace turned away, so that
// settleAccount records it once the arrival's transaction has rolled back. It never leaves
// settleAccount: the caller receives the refusal it wraps.
type turnedAwayError struct{ cause error }

func (e turnedAwayError) Error() string { return e.cause.Error() }
func (e turnedAwayError) Unwrap() error { return e.cause }

// turnedAway marks a refusal to be recorded.
func turnedAway(cause error) error { return turnedAwayError{cause: cause} }

// notAdmitted answers an arrival admission turned away (ADR-0078 §1, §5).
//
// Under INVITED_ONLY a provider that is not authoritative for the address - a self-hosted one, a
// personal Google account at somebody else's domain - and under DOMAINS an address outside the
// directory or domain list still bring two people to a door they open with their own proof, because
// authority and the list decide only who comes in new, on the provider's word (MayAdmitWithProof):
//
//   - an existing ACTIVE account goes to the LINK step: its password (and its second factor) is
//     asked, and nothing is connected without it. The account is answered, and owes that proof.
//     One that cannot give it on this card is refused with the sentence naming the way in it has.
//     Without this a member of a workspace that chose its provider only would have no way to
//     connect it (P-16).
//   - an INVITED account is pointed at its invitation's link, which is the proof it holds.
//
// Everybody else is refused, and nothing is created here.
func (w OidcWriter) notAdmitted(
	ctx context.Context, configured domain.IdentityProvider, arriving provider.Identity,
) (domain.Account, error) {
	refused := shared.ErrForbidden.WithDetail("identity_provider.not_admitted")
	if !configured.MayAdmitWithProof(admissionOf(arriving)) {
		return domain.Account{}, refused
	}
	existing, err := w.Accounts.FindByEmail(ctx, domain.LookupAddress(arriving.Email, w.Domains))
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return domain.Account{}, refused
		}
		return domain.Account{}, err
	}
	switch existing.Status {
	case domain.AccountInvited:
		return domain.Account{}, shared.ErrForbidden.WithDetail("identity_provider.invitation_needs_link")
	case domain.AccountActive:
		proof, err := w.proofOwed(ctx, existing)
		if err != nil {
			return domain.Account{}, err
		}
		switch proof {
		case proofPassword:
			return existing, nil
		case proofElsewhere:
			return domain.Account{}, shared.ErrForbidden.WithDetail("identity_provider.link_needs_own_way_in")
		}
		// No credential at all, and a provider that is not authoritative: its word does not stand
		// in for a proof, and the way back is the mailbox.
		return domain.Account{}, linkNeedsMailbox()
	default:
		return domain.Account{}, refused
	}
}

// arriveInvited is a first arrival through the invitation's own link (ADR-0078 §1): the invited
// account is activated and connected, or nothing happens and the invitation stays as it was.
//
// The link is the second proof, so under INVITED_ONLY the provider need not be authoritative for
// the address, and under DOMAINS the invited address need not be on the list - for this account
// only (MayAdmitInvitation). What it does not relax is the address: the provider's
// verified address has to be the invited one. A person who signed in there as somebody else is told
// so, and their invitation is still waiting for them.
func (w OidcWriter) arriveInvited(
	ctx context.Context, scope persistence.Scope,
	configured domain.IdentityProvider, arriving provider.Identity, invitedID shared.ID,
) (domain.Account, error) {
	invited, err := w.Accounts.Find(ctx, invitedID)
	if err != nil && !errors.Is(err, shared.ErrNotFound) {
		return domain.Account{}, err
	}
	// Accepted some other way since the flow began, or gone: the redemption's one sentence, the
	// one the start would have answered.
	if err != nil || invited.Status != domain.AccountInvited {
		w.Session.failure(ctx, FailureRedemption)
		return domain.Account{}, redemptionRefused()
	}
	if !configured.MayAdmitInvitation(admissionOf(arriving)) {
		return domain.Account{}, turnedAway(shared.ErrForbidden.WithDetail("identity_provider.not_admitted"))
	}
	if !w.sameAddress(arriving.Email, invited.Email) {
		return domain.Account{}, turnedAway(invitationAddressDiffers())
	}
	// An invited account holds no credential of its own - a connection made before this proof is
	// none, and is dropped below - unless somebody gave it a password, which is then asked for as
	// at every other arrival. The card cannot ask that on the invitation's path, so it is refused
	// with the sentence that names the way in it has.
	proof, err := w.proofOwed(ctx, invited)
	if err != nil {
		return domain.Account{}, err
	}
	if proof != proofNone {
		return domain.Account{}, turnedAway(shared.ErrForbidden.WithDetail("identity_provider.link_needs_own_way_in"))
	}
	return w.activateInvited(ctx, invited, configured, arriving, invitedID)
}

// secondProof refuses to activate an invited account the arrival brings no second proof for
// (ADR-0078 §1): neither the invitation's own link nor a provider authoritative for the address.
// Either way the provider's verified address has to be the account's.
//
// The refusal points at the link, which is the proof the person holds: the provider cannot give
// the other one.
func (w OidcWriter) secondProof(
	ctx context.Context, scope persistence.Scope, configured domain.IdentityProvider,
	arriving provider.Identity, invited domain.Account, invitedID shared.ID,
) error {
	if !arriving.EmailVerified || !w.sameAddress(arriving.Email, invited.Email) {
		return turnedAway(invitationAddressDiffers())
	}
	if invitedID == invited.ID || arriving.AddressAuthoritative {
		return nil
	}
	return turnedAway(shared.ErrForbidden.WithDetail("identity_provider.invitation_needs_link"))
}

// activateInvited accepts the invitation through the provider and connects the arriving identity,
// in the arrival's own transaction: the account becomes ACTIVE as redeeming would make it, the
// invitation is spent, and the arriving subject is connected - or none of it (SC-24, ADR-0078 §1).
//
// Whatever was connected to the account before this second proof is dropped first. Such a link was
// made on a provider's word alone, which is no credential; the person makes it again by signing in.
// The arriving identity is connected afresh, so an account connected earlier through the same
// subject keeps exactly that one, now proven.
//
// One that ran out stays refused - the provider's word does not renew the operator's offer. Through
// the invitation's link that is the redemption's one sentence; without it, the lapse is named.
func (w OidcWriter) activateInvited(
	ctx context.Context, invited domain.Account, configured domain.IdentityProvider,
	arriving provider.Identity, invitedID shared.ID,
) (domain.Account, error) {
	var lapsed error = shared.ErrForbidden.WithDetail("identity_provider.invitation_lapsed")
	if invitedID == invited.ID {
		lapsed = redemptionRefused()
	}
	accepted, err := w.acceptInvitation(ctx, invited, configured, lapsed)
	if err != nil {
		return domain.Account{}, err
	}
	dropped, err := w.External.UnlinkAll(ctx, invited.ID)
	if err != nil {
		return domain.Account{}, err
	}
	linked, err := w.External.LinkSubject(
		ctx, configured.ID, invited.ID, arriving.Subject, w.Session.Clock.Now())
	if err != nil {
		return domain.Account{}, err
	}
	if !linked {
		return domain.Account{}, shared.ErrConflict.WithDetail("identity_provider.account_taken")
	}
	return accepted, w.record(ctx, OidcLinkedAction, accepted, configured, arriving,
		audit.Change{Field: "identities_dropped", Classification: audit.Open, To: strconv.Itoa(dropped)})
}

// sameAddress compares two addresses the way an account's address is looked up: case and an
// internationalised domain's spelling do not make two addresses (M-10).
func (w OidcWriter) sameAddress(arriving, held string) bool {
	if arriving == "" || held == "" {
		return false
	}
	return domain.LookupAddress(arriving, w.Domains) == domain.LookupAddress(held, w.Domains)
}

// linkNeedsMailbox is the refusal of an existing account that holds no credential, arriving from a
// provider that is not authoritative for its address. Its sentence names the way back that works:
// *Forgot your password?* mails the account a link - to set a password where the password is open
// (SC-25), to connect the provider where it is off (SC-33).
func linkNeedsMailbox() error {
	return shared.ErrForbidden.WithDetail("identity_provider.link_needs_mailbox")
}

// invitationAddressDiffers is the refusal of an arrival whose provider identity is not the invited
// address. The invitation is not spent by it.
func invitationAddressDiffers() error {
	return shared.ErrForbidden.WithDetail("identity_provider.invitation_address_differs")
}

// acceptInvitation activates an invited account through the provider and records it as the
// redemption is recorded: the same action, the status it moved, and the provider it came through.
// `lapsed` is the refusal of an invitation that ran out or was accepted meanwhile.
func (w OidcWriter) acceptInvitation(
	ctx context.Context, invited domain.Account, configured domain.IdentityProvider, lapsed error,
) (domain.Account, error) {
	now := w.Session.Clock.Now()
	accepted, err := w.Accounts.AcceptInvitation(ctx, invited.ID, now)
	if err != nil {
		return domain.Account{}, err
	}
	if !accepted {
		return domain.Account{}, lapsed
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
	// no password to ask for. An invited account still needs its second proof before a provider
	// activates it (ADR-0078 §1): the invitation's link, or a provider authoritative for the address.
	proofNone linkProof = iota
	// proofPassword: the account has a password, and the card asks for it (and for the second
	// factor, if one is armed) before connecting.
	proofPassword
	// proofElsewhere: no password, but an identity at another provider that is a way in here. It
	// cannot be proven on this card, so the arrival is refused and pointed at that provider.
	proofElsewhere
	// proofMailbox: no password and no provider that lets the account in here, but a credential all
	// the same - a second factor, or an identity at an offer that ended or a provider switched off.
	// Nothing on this card proves it and no provider's word stands in for it: the arrival is refused
	// and pointed at the mailbox, whose link sets a password or connects a provider (ADR-0078 §1, §4).
	proofMailbox
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

	armed := false
	if w.Session.Enrollments != nil {
		enrollment, err := w.Session.Enrollments.Find(ctx, existing.ID)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return proofNone, err
		}
		armed = err == nil && !enrollment.ConfirmedAt.IsZero()
	}
	// An invited account's provider identities were connected on a provider's word alone, before
	// any second proof: they are not a credential, and activating the account drops them
	// (ADR-0078 §1).
	if existing.Status == domain.AccountInvited {
		if armed {
			return proofElsewhere, nil
		}
		return proofNone, nil
	}
	// The way in the account has is the provider that lets it in here - if one does.
	if w.Providers != nil {
		_, reaches, err := w.connectsHere().connectedHere(ctx, existing.ID, w.Session.Clock.Now())
		if err != nil {
			return proofNone, err
		}
		if reaches {
			return proofElsewhere, nil
		}
	}
	held, err := w.External.HasIdentity(ctx, existing.ID)
	if err != nil {
		return proofNone, err
	}
	if armed || held {
		return proofMailbox, nil
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
			Link: &domain.LinkIntent{
				ProviderID: configured.ID, Subject: arriving.Subject, Proof: domain.LinkProofPassword,
			},
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
//
// In a transaction of its own (SC-32), because the arrival's was rolled back with the refusal - an
// entry written inside it was never stored, which no test over a unit of work that keeps every write
// could show. Not cancelled with the request and bounded by its own deadline, `recordFailure`'s
// reasoning: a client that disconnects the moment it reads the refusal does not take the entry with
// it (rule 7).
func (w OidcWriter) recordRefusal(
	ctx context.Context, scope persistence.Scope, configured domain.IdentityProvider,
) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordFailureTimeout)
	defer cancel()
	return w.Session.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		return w.Session.Audit.Append(ctx, audit.Entry{
			TenantID:   scope.TenantID,
			OccurredAt: w.Session.Clock.Now(),
			Action:     OidcRefusedAction,
			Outcome:    audit.OutcomeDenied,
			Severity:   audit.SeverityWarning,
			ActorKind:  shared.ActorSystem,
			TargetType: identityProviderTarget,
			TargetID:   scope.TenantID,
			Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(
				audit.Change{Field: "issuer", Classification: audit.Open, To: configured.Issuer},
				audit.Change{Field: "provisioning", Classification: audit.Open,
					To: string(configured.Provisioning)}),
		})
	})
}

// record writes the trail entry for an account that arrived through the provider. The subject is
// not in it: it identifies a person at their provider, and the account it became is the thing a
// reader needs.
func (w OidcWriter) record(
	ctx context.Context, action audit.Action, account domain.Account,
	configured domain.IdentityProvider, arriving provider.Identity, extra ...audit.Change,
) error {
	changes := append([]audit.Change{
		{Field: "issuer", Classification: audit.Open, To: configured.Issuer},
		{Field: "email_verified", Classification: audit.Open, To: strconv.FormatBool(arriving.EmailVerified)},
	}, extra...)
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
		Changes:     audit.Changes(changes...),
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
			{Name: "invitation_token", Kind: usecase.KindString,
				Description: "The redemption token of the invitation this sign-in accepts. Checked, not " +
					"spent: the flow remembers the invited account, and only an arrival that succeeds " +
					"accepts the invitation."},
			{Name: "connect_token", Kind: usecase.KindString,
				Description: "The token of the link a workspace without the password mails to connect " +
					"its provider. Checked, not spent: the flow remembers the link, the provider is " +
					"asked for a fresh sign-in, and only an arrival that connects spends it."},
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
		ProviderID:      providerID,
		LoginHint:       in.String("login_hint"),
		TenantSlug:      in.String("tenant_slug"),
		TenantHeader:    in.String("tenant_header"),
		InvitationToken: secret.New(in.String("invitation_token")),
		ConnectToken:    secret.New(in.String("connect_token")),
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
			"an account whose verified address falls inside the configured domains - after its " +
			"own password (and second factor) where it holds one, the answer then a LINK " +
			"challenge instead of a pair. An invited account is activated only with a second " +
			"proof: the flow started from its invitation's link, or a provider authoritative for " +
			"the address, and the provider's verified address equal to the invited one. A sign-in " +
			"begun from the link a workspace without the password mails connects that link's " +
			"account with the mailbox and a fresh sign-in at the provider as its proof - the " +
			"provider's verified address equal to the account's, an armed second factor still asked.",
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
