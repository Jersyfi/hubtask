// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"slices"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	provider "github.com/Jersyfi/hubtask/core/port/identityprovider"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	stepupport "github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The step-up at the provider (ADR-0075 §2): the fourth method, for the accounts deployments D4 to
// D6 produce - people who sign in only through their organisation's directory and hold neither a
// password nor a factor here. The proof is a fresh sign-in at the provider the account is already
// connected to, verified twice on the way back: the identity must be that connection's, and
// `auth_time` must be named and inside the step-up's window.

const StartProviderStepUpName = "StartProviderStepUp"

// ProviderStepUpStartedAction is the first half, recorded as a sign-in's start is: a session that
// keeps starting step-ups it never finishes is worth a reader's eye.
const ProviderStepUpStartedAction audit.Action = "auth.step_up_provider_started"

// ProviderStepUps is what the fourth method needs. It sits on the session writer and is set where
// that writer is built, because every copy of it - one verifier per privileged operation - has to
// name PROVIDER and prove it alike (the copies are values, and a field assigned later reaches none
// of them). A zero value switches the method off: nothing is offered, nothing can be started.
type ProviderStepUps struct {
	Providers repository.IdentityProviders
	Flows     repository.OidcFlows
	External  repository.ExternalAccounts
	// Workspaces answers the workspace's own switches, for a provider the installation offers.
	Workspaces repository.Workspaces
	Relying    provider.Port
	// RedirectURL is this installation's one callback, the address every registration permits.
	RedirectURL string
}

func (p ProviderStepUps) available() bool {
	return p.Providers != nil && p.Flows != nil && p.External != nil && p.Relying != nil
}

// stepUpProvider answers the provider a PROVIDER proof goes to: the first, in the workspace's own
// order, that this account is connected to and that is switched on here. False where there is none.
// Inside a transaction bound to the account's workspace.
func (w SessionWriter) stepUpProvider(
	ctx context.Context, accountID shared.ID,
) (domain.IdentityProvider, bool, error) {
	p := w.StepUpProviders
	if !p.available() {
		return domain.IdentityProvider{}, false, nil
	}
	return p.connectedHere(ctx, accountID, w.Clock.Now())
}

// connectedHere answers the first provider, in the workspace's own order, that the account is
// connected to and that is a way in here now. The step-up asks it to know where a proof goes; the
// reset asks it to know whether a provider still lets the account in (ADR-0077 §4). Inside a
// transaction bound to the account's workspace.
func (p ProviderStepUps) connectedHere(
	ctx context.Context, accountID shared.ID, now time.Time,
) (domain.IdentityProvider, bool, error) {
	connected, err := p.External.ProvidersOf(ctx, accountID)
	if err != nil || len(connected) == 0 {
		return domain.IdentityProvider{}, false, err
	}
	listed, err := p.Providers.List(ctx)
	if err != nil {
		return domain.IdentityProvider{}, false, err
	}
	var settings domain.WorkspaceSettings
	if p.Workspaces != nil {
		workspace, err := p.Workspaces.Find(ctx)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return domain.IdentityProvider{}, false, err
		}
		settings = workspace.Settings
	}
	for _, candidate := range listed {
		if slices.Contains(connected, candidate.ID) && offeredHere(candidate, settings, now) {
			return candidate, true, nil
		}
	}
	return domain.IdentityProvider{}, false, nil
}

// stepUpConfig is the provider's configuration as the relying party needs it, the client secret
// opened the way a sign-in opens it.
func (w SessionWriter) stepUpConfig(
	ctx context.Context, scope persistence.Scope, providerID shared.ID,
) (domain.IdentityProvider, provider.Config, error) {
	p := w.StepUpProviders
	configured, opened, err := openProvider(ctx, w, p.Providers, p.Workspaces, scope, providerID, nil)
	if err != nil {
		// No longer a way in here - switched off, not taken, or past an announced withdrawal
		// (ADR-0076 §2): the step-up's own refusal, the one its start gives, rather than the
		// sign-in's sentence.
		if shared.AsError(err).DetailCode == "identity_provider.disabled" {
			return domain.IdentityProvider{}, provider.Config{},
				shared.ErrValidation.WithDetail("auth.step_up_no_provider")
		}
		return domain.IdentityProvider{}, provider.Config{}, err
	}
	return configured, provider.Config{
		Issuer: configured.Issuer, ClientID: configured.ClientID,
		ClientSecret: opened, RedirectURL: p.RedirectURL,
		DirectoryClaim: directoryClaimOf(configured),
	}, nil
}

// ProviderName answers the name the refusal carries beside PROVIDER (stepup.ProviderNamer).
func (v StepUpVerifier) ProviderName(ctx context.Context, tenantID, accountID shared.ID) (string, error) {
	w := v.Writer
	name := ""
	err := w.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID, ActorID: accountID},
		func(ctx context.Context) error {
			chosen, found, err := w.stepUpProvider(ctx, accountID)
			if found {
				name = chosen.DisplayName
			}
			return err
		})
	return name, err
}

var _ stepupport.ProviderNamer = StepUpVerifier{}

// StartProviderStepUp is the first half: where to send the browser.
type StartProviderStepUp struct{ Writer SessionWriter }

// ProviderStepUpStart is its answer.
type ProviderStepUpStart struct {
	URL          string
	ExpiresAt    time.Time
	ProviderID   shared.ID
	ProviderName string
}

// Execute opens a flow bound to the caller's session and builds a fresh authorization request.
func (h StartProviderStepUp) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (ProviderStepUpStart, error) {
	w := h.Writer
	session, err := w.stepUpSession(ctx, actor)
	if err != nil {
		return ProviderStepUpStart{}, err
	}

	var chosen domain.IdentityProvider
	if err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, held, err := w.stepUpProvider(ctx, actor.AccountID)
		if err != nil {
			return err
		}
		if !held {
			return shared.ErrValidation.WithDetail("auth.step_up_no_provider")
		}
		chosen = found
		return nil
	}); err != nil {
		return ProviderStepUpStart{}, err
	}

	p := w.StepUpProviders
	scope := actor.PersistenceScope()
	configured, config, err := w.stepUpConfig(ctx, scope, chosen.ID)
	if err != nil {
		return ProviderStepUpStart{}, err
	}
	state, verifier, nonce, err := drawFlow(w.Entropy, actor.TenantID)
	if err != nil {
		return ProviderStepUpStart{}, err
	}
	flow, err := domain.NewOidcFlow(domain.NewOidcFlowInput{
		ID: w.IDs.NewID(), TenantID: actor.TenantID, ProviderID: configured.ID,
		SessionID: session.ID, Nonce: nonce, Verifier: verifier, Now: w.Clock.Now(),
	})
	if err != nil {
		return ProviderStepUpStart{}, err
	}

	// Asked before the flow is written, the sign-in's order: an unreachable provider leaves no row.
	url, err := p.Relying.AuthorizationURL(ctx, config, provider.Authorization{
		State: state.Secret(), Nonce: nonce, CodeVerifier: verifier, Fresh: true,
	})
	if err != nil {
		return ProviderStepUpStart{}, err
	}
	if err := w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		if err := p.Flows.Insert(ctx, flow, state); err != nil {
			return err
		}
		return w.recordProviderStepUpStart(ctx, actor, session, configured)
	}); err != nil {
		return ProviderStepUpStart{}, err
	}

	return ProviderStepUpStart{
		URL: url, ExpiresAt: flow.ExpiresAt,
		ProviderID: configured.ID, ProviderName: configured.DisplayName,
	}, nil
}

// proveAtProvider is the second half, inside StepUp: the flow this session opened is burned, the
// code exchanged, and the identity judged - the connected one, freshly signed in.
//
// The burn comes first and in its own transaction, the sign-in's discipline: whatever happens
// afterwards, the state is spent. The exchange is a network call and runs outside any transaction.
func (w SessionWriter) proveAtProvider(
	ctx context.Context, actor appshared.ActorContext, session domain.Session, cmd StepUpCommand,
) error {
	if !w.StepUpProviders.available() {
		return shared.ErrValidation.WithDetail("auth.step_up_no_provider")
	}
	state, err := domain.ParseOidcFlowState(cmd.State.Reveal())
	if err != nil || state.TenantID() != actor.TenantID {
		w.failure(ctx, FailureOidc)
		return oidcRefused()
	}
	p := w.StepUpProviders
	scope := actor.PersistenceScope()

	var flow domain.OidcFlow
	if err := w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		found, ok, err := p.Flows.ConsumeForStepUp(ctx, state, session.ID, w.Clock.Now())
		if err != nil {
			return err
		}
		if !ok {
			w.failure(ctx, FailureOidc)
			return oidcRefused()
		}
		flow = found
		return nil
	}); err != nil {
		return err
	}

	configured, config, err := w.stepUpConfig(ctx, scope, flow.ProviderID)
	if err != nil {
		return err
	}
	arrived, err := p.Relying.Exchange(ctx, config,
		provider.Exchange{Code: cmd.AuthorizationCode, CodeVerifier: flow.Verifier, Nonce: flow.Nonce})
	if err != nil {
		w.failure(ctx, FailureOidc)
		return err
	}

	subject := stepUpSubject(actor.AccountID)
	return w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		now := w.Clock.Now()
		if err := w.checkLocked(ctx, []string{subject}, now); err != nil {
			return err
		}
		// Still a way in here, as the start found it: a provider switched off in the workspace - its
		// own row, or the workspace's switch for one the installation offers - while the person was
		// away proves nothing on the way back.
		current, held, err := w.stepUpProvider(ctx, actor.AccountID)
		if err != nil {
			return err
		}
		if !held || current.ID != configured.ID {
			w.failure(ctx, FailureOidc)
			return shared.ErrValidation.WithDetail("auth.step_up_no_provider")
		}
		// The identity already connected to this account, and no other: a colleague signed in at
		// the same provider on this machine proves nothing about this account, and is not
		// connected to it either.
		owner, err := p.External.FindBySubject(ctx, configured.ID, arrived.Subject)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return err
		}
		if err != nil || owner.ID != actor.AccountID {
			w.failure(ctx, FailureOidc)
			return errors.Join(w.recordMfaFailure(ctx, subject, now),
				shared.ErrForbidden.WithDetail("auth.step_up_provider_mismatch"))
		}
		if !domain.ProviderProofFresh(arrived.AuthTime, now, w.stepUpWindow()) {
			w.failure(ctx, FailureOidc)
			return errors.Join(w.recordMfaFailure(ctx, subject, now),
				shared.ErrForbidden.WithDetail("auth.step_up_provider_not_fresh"))
		}
		return nil
	})
}

// recordProviderStepUpStart writes the start into the trail: the provider asked, never a state.
func (w SessionWriter) recordProviderStepUpStart(
	ctx context.Context, actor appshared.ActorContext, session domain.Session,
	configured domain.IdentityProvider,
) error {
	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: w.Clock.Now(),
		Action:     ProviderStepUpStartedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: sessionTarget,
		TargetID:   session.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(audit.Change{
			Field: "provider_id", Classification: audit.Open, To: configured.ID.String(),
		}),
	})
}

// stepUpSession is the live session a step-up lands on: the actor's own, or the refusal that only a
// session may step up - a personal access token has no session and no person at the keyboard.
func (w SessionWriter) stepUpSession(
	ctx context.Context, actor appshared.ActorContext,
) (domain.Session, error) {
	if !actor.IsAuthenticated() || actor.AccountID.IsZero() {
		return domain.Session{}, shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
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
		return domain.Session{}, err
	}
	if err := session.Verify(w.Clock.Now()); err != nil {
		return domain.Session{}, err
	}
	return session, nil
}

// Descriptor is the catalogue entry.
func (h StartProviderStepUp) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: StartProviderStepUpName,
		Summary: "Begins a step-up at the provider this account is connected to and that is " +
			"switched on for its workspace (ADR-0075 §2): a fresh sign-in there, with " +
			"prompt=login and max_age=0, bound to the session that asked. The browser comes back " +
			"to this installation's callback, and the state and code it carries finish the " +
			"step-up at StepUp.",
		SideEffects: "Writes a short-lived flow bound to the session and asks the provider for its metadata.",
		Audit: usecase.AuditDeclaration{
			Action: ProviderStepUpStartedAction, TargetType: sessionTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A proof is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h StartProviderStepUp) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	started, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"authorization_url": started.URL,
		"expires_at":        started.ExpiresAt.UTC(),
		"provider_id":       started.ProviderID.String(),
		"provider_name":     started.ProviderName,
	}, nil
}

// presentedProviderProof reports whether a step-up command carries the provider's return.
func (cmd StepUpCommand) presentedProviderProof() bool {
	return !cmd.State.IsEmpty() || cmd.AuthorizationCode != ""
}
