// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"slices"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	provider "github.com/Jersyfi/hubtask/core/port/identityprovider"
	"github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// Connecting a provider from a signed-in session (ADR-0078 §1, identity.md §11): the way an identity
// whose address differs from the account's - a guest's, a changed address - is connected. Never at
// the front door, where the provider's verified address must equal the account's; here, from the
// account's own settings, after two proofs of the person: a step-up of the account, then a fresh
// sign-in at the provider, bound to the session that asked.

const (
	StartProviderConnectName    = "StartProviderConnect"
	CompleteProviderConnectName = "CompleteProviderConnect"
	ListOwnIdentitiesName       = "ListOwnIdentities"
)

// ProviderConnectStartedAction is the first half, recorded as a sign-in's start is.
const ProviderConnectStartedAction audit.Action = "identity.provider_connect_started"

// StartProviderConnect is the first half: the proof of the account, and where to send the browser.
type StartProviderConnect struct {
	Writer OidcWriter
	// StepUp is the fresh proof the start demands. Nil refuses, as every privileged act does.
	StepUp stepup.Verifier
}

// ProviderConnectStart is its answer.
type ProviderConnectStart struct {
	URL          string
	ExpiresAt    time.Time
	ProviderID   shared.ID
	ProviderName string
}

// Execute checks the account and the provider, demands the step-up, and opens a flow bound to the
// caller's session and marked a connection.
func (h StartProviderConnect) Execute(
	ctx context.Context, actor appshared.ActorContext, providerID shared.ID, stepUpToken string,
) (ProviderConnectStart, error) {
	w := h.Writer
	s := w.Session
	session, err := s.stepUpSession(ctx, actor)
	if err != nil {
		return ProviderConnectStart{}, err
	}
	scope := actor.PersistenceScope()

	// What the account may connect is judged before the proof is demanded, so that a refusal does
	// not burn it.
	if err := s.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		account, err := w.Accounts.Find(ctx, actor.AccountID)
		if err != nil {
			return err
		}
		return w.mayConnect(ctx, account, providerID)
	}); err != nil {
		return ProviderConnectStart{}, err
	}
	configured, opened, err := w.provider(ctx, scope, providerID)
	if err != nil {
		return ProviderConnectStart{}, err
	}
	if err := stepup.Demand(ctx, h.StepUp, actor.TenantID, actor.AccountID, stepUpToken); err != nil {
		return ProviderConnectStart{}, err
	}

	state, verifier, nonce, err := drawFlow(s.Entropy, actor.TenantID)
	if err != nil {
		return ProviderConnectStart{}, err
	}
	flow, err := domain.NewOidcFlow(domain.NewOidcFlowInput{
		ID: s.IDs.NewID(), TenantID: actor.TenantID, ProviderID: configured.ID,
		SessionID: session.ID, Connect: true, Nonce: nonce, Verifier: verifier, Now: s.Clock.Now(),
	})
	if err != nil {
		return ProviderConnectStart{}, err
	}
	// Fresh: the provider asks the person again rather than answering from its own session, and no
	// address is hinted - the identity to connect is the person's to choose there.
	url, err := w.Relying.AuthorizationURL(ctx, relyingConfig(configured, opened, w.RedirectURL),
		provider.Authorization{State: state.Secret(), Nonce: nonce, CodeVerifier: verifier, Fresh: true})
	if err != nil {
		return ProviderConnectStart{}, err
	}
	if err := s.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		if err := w.Flows.Insert(ctx, flow, state); err != nil {
			return err
		}
		return s.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: s.Clock.Now(),
			Action: ProviderConnectStartedAction, Outcome: audit.OutcomeSuccess,
			Severity: audit.SeverityInfo, ActorKind: actor.Kind, ActorID: actor.AccountID,
			ActorLabel: actor.AccountName, TargetType: identityProviderTarget, TargetID: configured.ID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		})
	}); err != nil {
		return ProviderConnectStart{}, err
	}
	return ProviderConnectStart{
		URL: url, ExpiresAt: flow.ExpiresAt,
		ProviderID: configured.ID, ProviderName: configured.DisplayName,
	}, nil
}

// mayConnect is what the account itself decides about a connection: it holds an address, and it is
// not already connected at this provider. Inside a transaction bound to the account's workspace.
func (w OidcWriter) mayConnect(ctx context.Context, account domain.Account, providerID shared.ID) error {
	// An account without an address is one no provider reaches, and the password door its
	// workspace keeps open for it rests on that (identity.md §4.1): it adds an address first.
	if account.Email == "" {
		return shared.ErrValidation.WithDetail("identity_provider.connect_needs_address")
	}
	connected, err := w.External.ProvidersOf(ctx, account.ID)
	if err != nil {
		return err
	}
	// Never re-pointed (identity.md §11): one identity per account per provider.
	if slices.Contains(connected, providerID) {
		return shared.ErrConflict.WithDetail("identity_provider.already_connected")
	}
	return nil
}

// CompleteProviderConnect is the second half: the provider's return, judged, and the connection.
type CompleteProviderConnect struct{ Writer OidcWriter }

// CompleteProviderConnectCommand is the provider's return as the browser carried it.
type CompleteProviderConnectCommand struct {
	State             secret.Secret
	AuthorizationCode string
}

// ConnectedIdentity is one connection, as the person sees it: never the subject or an address.
type ConnectedIdentity struct {
	ProviderID   shared.ID
	ProviderName string
	LinkedAt     time.Time
}

// Execute burns the session's connection flow, exchanges the code, and connects the identity.
//
// The burn comes first and in its own transaction, the sign-in's discipline; the exchange is a
// network call outside any transaction.
func (h CompleteProviderConnect) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CompleteProviderConnectCommand,
) (ConnectedIdentity, error) {
	w := h.Writer
	s := w.Session
	session, err := s.stepUpSession(ctx, actor)
	if err != nil {
		return ConnectedIdentity{}, err
	}
	state, err := domain.ParseOidcFlowState(cmd.State.Reveal())
	if err != nil || state.TenantID() != actor.TenantID {
		s.failure(ctx, FailureOidc)
		return ConnectedIdentity{}, oidcRefused()
	}
	scope := actor.PersistenceScope()

	var flow domain.OidcFlow
	if err := s.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		found, ok, err := w.Flows.ConsumeForConnect(ctx, state, session.ID, s.Clock.Now())
		if err != nil {
			return err
		}
		if !ok {
			s.failure(ctx, FailureOidc)
			return oidcRefused()
		}
		flow = found
		return nil
	}); err != nil {
		return ConnectedIdentity{}, err
	}

	configured, sealed, err := w.provider(ctx, scope, flow.ProviderID)
	if err != nil {
		return ConnectedIdentity{}, err
	}
	arrived, err := w.Relying.Exchange(ctx, relyingConfig(configured, sealed, w.RedirectURL),
		provider.Exchange{Code: cmd.AuthorizationCode, CodeVerifier: flow.Verifier, Nonce: flow.Nonce})
	if err != nil {
		s.failure(ctx, FailureOidc)
		return ConnectedIdentity{}, err
	}

	var linkedAt time.Time
	err = s.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		now := s.Clock.Now()
		account, err := w.Accounts.Find(ctx, actor.AccountID)
		if err != nil {
			return err
		}
		// A sign-in made after the connection began, at the provider's own prompt: one answered from
		// a session the provider already held proves nobody is at the keyboard now.
		if !domain.ProviderProofFreshSince(arrived.AuthTime, flow.CreatedAt, now, s.stepUpWindow()) {
			return turnedAway(shared.ErrForbidden.WithDetail("identity_provider.connection_not_fresh"))
		}
		// The person's own proof, not the lists, decides a connection (identity.md §10.4): the
		// lists bound who comes in new. The address need not be the account's - that is the
		// point - but the provider must have verified it.
		if !configured.MayAdmitWithProof(admissionOf(arrived)) {
			return turnedAway(shared.ErrForbidden.WithDetail("identity_provider.not_admitted"))
		}
		owner, err := w.External.FindBySubject(ctx, configured.ID, arrived.Subject)
		switch {
		case err == nil && owner.ID != account.ID:
			// Somebody else's identity here, and never re-pointed.
			return turnedAway(shared.ErrForbidden.WithDetail("identity_provider.connection_taken"))
		case err == nil:
			// Already this account's: what was asked for stands, and nothing is written.
			linkedAt = now
			return nil
		case !errors.Is(err, shared.ErrNotFound):
			return err
		}
		if err := w.mayConnect(ctx, account, configured.ID); err != nil {
			return err
		}
		linkedAt = now
		return w.Connect(ctx, account, domain.LinkIntent{
			ProviderID: configured.ID, Subject: arrived.Subject, Proof: domain.LinkProofSession,
		})
	})
	var refusal turnedAwayError
	if errors.As(err, &refusal) {
		s.failure(ctx, FailureOidc)
		if recordErr := w.recordRefusal(ctx, scope, configured); recordErr != nil {
			return ConnectedIdentity{}, recordErr
		}
		return ConnectedIdentity{}, refusal.cause
	}
	if err != nil {
		return ConnectedIdentity{}, err
	}
	return ConnectedIdentity{
		ProviderID: configured.ID, ProviderName: configured.DisplayName, LinkedAt: linkedAt,
	}, nil
}

// ListOwnIdentities answers the provider identities connected to the caller's own account.
type ListOwnIdentities struct{ Writer OidcWriter }

// Execute reads them. A provider that is no longer a way in here is still named while the connection
// stands; one whose row is gone is not listed, since its connections went with it.
func (h ListOwnIdentities) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]ConnectedIdentity, error) {
	if !actor.IsAuthenticated() || actor.AccountID.IsZero() {
		return nil, shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	w := h.Writer
	var listed []ConnectedIdentity
	err := w.Session.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		connections, err := w.External.IdentitiesOf(ctx, actor.AccountID)
		if err != nil {
			return err
		}
		for _, connection := range connections {
			configured, err := w.Providers.Find(ctx, connection.ProviderID)
			if errors.Is(err, shared.ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			listed = append(listed, ConnectedIdentity{
				ProviderID: configured.ID, ProviderName: configured.DisplayName, LinkedAt: connection.LinkedAt,
			})
		}
		return nil
	})
	return listed, err
}

// OwnIdentitiesReadAction names the read for the trail's refusals.
const OwnIdentitiesReadAction audit.Action = "identity.own_identities_read"

// Descriptor is the catalogue entry.
func (h ListOwnIdentities) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListOwnIdentitiesName,
		Summary: "Lists the sign-in providers the caller's own account is connected to: which " +
			"provider, its name and since when - never the provider's subject or address.",
		SideEffects: "None. Reads only.",
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			// The caller's own records: nothing to refuse beyond the credential, nothing to record.
			Action: OwnIdentitiesReadAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(func(
			ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
		) (usecase.Output, error) {
			listed, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(listed))
			for _, each := range listed {
				data = append(data, connectedOutput(each))
			}
			return usecase.Output{"data": data}, nil
		}),
	}
}

// Descriptor is the catalogue entry.
func (h StartProviderConnect) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: StartProviderConnectName,
		Summary: "Begins connecting a sign-in provider to the caller's own account - also an " +
			"identity whose address differs from the account's: a fresh proof of the account, then " +
			"a fresh sign-in at the provider with prompt=login and max_age=0, bound to the session " +
			"that asked. The browser comes back to this installation's callback, and the state and " +
			"code it carries finish the connection at CompleteProviderConnect.",
		SideEffects: "Consumes the step-up; writes a short-lived flow bound to the session; asks the " +
			"provider for its metadata; writes an audit entry.",
		Input: []usecase.Field{
			{Name: "provider_id", Kind: usecase.KindID, Required: true,
				Description: "Which provider, from the sign-in rules: one that is a way in here."},
			{
				// Declared but not required: a caller without a proof needs the 403 that names
				// the methods rather than a 422 about a missing field.
				Name: "step_up_token", Kind: usecase.KindString,
				Description: "The fresh proof this demands. It is consumed by this call.",
			},
		},
		StepUp: "every connection of a provider from a session",
		Audit: usecase.AuditDeclaration{
			Action: ProviderConnectStartedAction, TargetType: identityProviderTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A way in is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h StartProviderConnect) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	started, err := h.Execute(ctx, actor, shared.ID(in.String("provider_id")), in.String("step_up_token"))
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

// Descriptor is the catalogue entry.
func (h CompleteProviderConnect) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CompleteProviderConnectName,
		Summary: "Finishes connecting a sign-in provider to the caller's own account with the " +
			"state and code the provider sent the browser back with. The provider's sign-in must " +
			"be fresh and its address verified; it need not be the account's. An identity " +
			"connected to another account here is refused, and the account's connection to the " +
			"provider is never re-pointed.",
		SideEffects: "Consumes the flow; connects the identity, writes an audit entry and queues " +
			"the person's notice; a refusal is recorded in its own transaction.",
		Input: []usecase.Field{
			{Name: "state", Kind: usecase.KindString, Required: true,
				Description: "The handle StartProviderConnect minted, as the provider echoed it."},
			{Name: "authorization_code", Kind: usecase.KindString, Required: true,
				Description: "The code the provider issued."},
		},
		Audit: usecase.AuditDeclaration{
			Action: OidcLinkedAction, TargetType: accountTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A way in is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CompleteProviderConnect) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	connected, err := h.Execute(ctx, actor, CompleteProviderConnectCommand{
		State: secret.New(in.String("state")), AuthorizationCode: in.String("authorization_code"),
	})
	if err != nil {
		return nil, err
	}
	return connectedOutput(connected), nil
}

func connectedOutput(connected ConnectedIdentity) usecase.Output {
	return usecase.Output{
		"provider_id":   connected.ProviderID.String(),
		"provider_name": connected.ProviderName,
		"linked_at":     connected.LinkedAt.UTC(),
	}
}
