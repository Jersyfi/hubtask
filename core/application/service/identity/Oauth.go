// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const (
	RegisterOauthClientName  = "RegisterOauthClient"
	ListOauthClientsName     = "ListOauthClients"
	DeleteOauthClientName    = "DeleteOauthClient"
	AuthorizeOauthClientName = "AuthorizeOauthClient"
	ExchangeOauthCodeName    = "ExchangeOauthCode"
	ListOauthGrantsName      = "ListOauthGrants"
	RevokeOauthGrantName     = "RevokeOauthGrant"

	// oauthManage is the registry's scope (api-guidelines.md §7): who may register the apps that
	// ask people here for access.
	oauthManage = "oauth:manage"

	oauthClientTarget = "oauth_client"
	oauthGrantTarget  = "oauth_grant"
)

// The audit codes of the provider (H-05). Registering a door, opening it, and closing it are all
// the class of event a review looks for.
const (
	OauthClientRegisteredAction audit.Action = "oauth.client_registered"
	OauthClientDeletedAction    audit.Action = "oauth.client_deleted"
	OauthConsentedAction        audit.Action = "oauth.consented"
	OauthExchangedAction        audit.Action = "oauth.code_exchanged"
	OauthGrantRevokedAction     audit.Action = "oauth.grant_revoked"
	OauthClientReadAction       audit.Action = "oauth.client_read"
	OauthGrantReadAction        audit.Action = "oauth.grant_read"
)

// OauthWriter is what the provider's use cases share, AccessTokenWriter's shape.
type OauthWriter struct {
	Session SessionWriter
	Clients repository.OauthClients
	Grants  repository.OauthGrants
	Codes   repository.OauthCodes
	// Authorizer guards the registry: registering an app is deciding who may ask people here
	// for access, the service accounts' permission.
	Authorizer Authorizer
	// KnownScopes bounds what a person can consent to: the catalogue's own vocabulary, no
	// parallel one (decision 5).
	KnownScopes []string
}

// RegisterOauthClient registers a third-party app (H-05).
type RegisterOauthClient struct{ Writer OauthWriter }

// RegisterOauthClientCommand is the input, typed.
type RegisterOauthClientCommand struct {
	Name         string
	Confidential bool
	RedirectURIs []string
}

// RegisteredClient is the registration and, for a confidential client, the single showing.
type RegisteredClient struct {
	Client domain.OauthClient
	Secret secret.Secret
}

// Execute registers, and answers the secret - where there is one - for the only time.
func (h RegisterOauthClient) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd RegisterOauthClientCommand,
) (RegisteredClient, error) {
	w := h.Writer
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     OauthClientRegisteredAction,
		TokenScope: oauthManage,
		TargetType: oauthClientTarget,
	}); err != nil {
		return RegisteredClient{}, err
	}

	client, err := domain.NewOauthClient(domain.NewOauthClientInput{
		ID: w.Session.IDs.NewID(), TenantID: actor.TenantID,
		Name: cmd.Name, Confidential: cmd.Confidential, RedirectURIs: cmd.RedirectURIs,
		CreatedBy: actor.AccountID, Now: w.Session.Clock.Now(),
	})
	if err != nil {
		return RegisteredClient{}, err
	}

	var presented domain.Token
	if client.Confidential {
		material, err := w.Session.Entropy.Bytes(domain.TokenSecretBytes)
		if err != nil {
			return RegisteredClient{}, shared.ErrInternal.
				WithDetail("auth.session_unmintable").WithCause(err)
		}
		if presented, err = domain.NewOauthClientSecret(actor.TenantID, material); err != nil {
			return RegisteredClient{}, err
		}
	}

	err = w.Session.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := w.Clients.Insert(ctx, client, presented); err != nil {
			return err
		}
		return w.recordClientAudit(ctx, actor, OauthClientRegisteredAction, client)
	})
	if err != nil {
		return RegisteredClient{}, err
	}

	registered := RegisteredClient{Client: client}
	if client.Confidential {
		registered.Secret = secret.New(presented.Secret())
	}
	return registered, nil
}

// ListOauthClients answers the registry.
type ListOauthClients struct{ Writer OauthWriter }

func (h ListOauthClients) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]domain.OauthClient, error) {
	w := h.Writer
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission:  service.PermissionManageMembers,
		Alternative: service.PermissionReadConfiguration,
		Path:        []domain.Scope{domain.TenantScope()},
		Action:      OauthClientReadAction,
		TokenScope:  oauthManage,
		TargetType:  oauthClientTarget,
	}); err != nil {
		return nil, err
	}

	var clients []domain.OauthClient
	err := w.Session.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(),
		func(ctx context.Context) error {
			found, err := w.Clients.List(ctx)
			clients = found
			return err
		})
	if err != nil {
		return nil, err
	}
	return clients, nil
}

// DeleteOauthClient removes an app: the grants go with it, and the sessions those grants leashed.
type DeleteOauthClient struct{ Writer OauthWriter }

func (h DeleteOauthClient) Execute(
	ctx context.Context, actor appshared.ActorContext, clientID shared.ID,
) error {
	w := h.Writer
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     OauthClientDeletedAction,
		TokenScope: oauthManage,
		TargetType: oauthClientTarget,
		TargetID:   clientID,
	}); err != nil {
		return err
	}
	if clientID.IsZero() {
		return shared.ErrNotFound.WithDetail("oauth.client_not_found")
	}

	return w.Session.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		client, err := w.Clients.Find(ctx, clientID)
		if err != nil {
			return err
		}
		removed, err := w.Clients.Delete(ctx, clientID)
		if err != nil {
			return err
		}
		if !removed {
			return shared.ErrNotFound.WithDetail("oauth.client_not_found")
		}
		return w.recordClientAudit(ctx, actor, OauthClientDeletedAction, client)
	})
}

// AuthorizeOauthClientCommand is the consent, typed.
type AuthorizeOauthClientCommand struct {
	ClientID    shared.ID
	RedirectURI string
	Scopes      []string
	Challenge   string
	Method      string
	State       string
}

// AuthorizedCode is the single showing of the code.
type AuthorizedCode struct {
	Code      secret.Secret
	ExpiresAt time.Time
	State     string
}

// AuthorizeOauthClient records a person's consent and mints the code (H-05, headless).
type AuthorizeOauthClient struct{ Writer OauthWriter }

// Execute consents. The consent is a person's act: it demands a session, StepUp's reasoning - a
// personal access token consenting on somebody's behalf would be an app granting apps.
func (h AuthorizeOauthClient) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd AuthorizeOauthClientCommand,
) (AuthorizedCode, error) {
	w := h.Writer
	if !actor.IsAuthenticated() || actor.AccountID.IsZero() {
		return AuthorizedCode{}, shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	if cmd.Method != "S256" {
		return AuthorizedCode{}, shared.ErrValidation.
			WithDetail("oauth.pkce_challenge_invalid").
			WithFields(shared.FieldError{Path: "/code_challenge_method", Code: "oauth.pkce_challenge_invalid"})
	}
	if err := domain.CheckPKCEChallenge(cmd.Challenge); err != nil {
		return AuthorizedCode{}, err
	}
	if err := w.checkScopes(cmd.Scopes); err != nil {
		return AuthorizedCode{}, err
	}

	// The consent lands on the person's own session: a machine credential cannot consent.
	err := w.Session.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(),
		func(ctx context.Context) error {
			credential, err := w.Session.Sessions.FindForAuth(ctx, actor.TokenID)
			if err != nil || credential.Session.AccountID != actor.AccountID {
				return shared.ErrForbidden.WithDetail("auth.step_up_session_required")
			}
			if !credential.Session.GrantID.IsZero() {
				// An app's session consenting to another app would be an app granting apps.
				return shared.ErrForbidden.WithDetail("auth.step_up_session_required")
			}
			return nil
		})
	if err != nil {
		return AuthorizedCode{}, err
	}

	material, err := w.Session.Entropy.Bytes(domain.TokenSecretBytes)
	if err != nil {
		return AuthorizedCode{}, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}
	presented, err := domain.NewOauthCode(actor.TenantID, material)
	if err != nil {
		return AuthorizedCode{}, err
	}

	var minted AuthorizedCode
	err = w.Session.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		client, err := w.Clients.Find(ctx, cmd.ClientID)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return shared.ErrValidation.WithDetail("oauth.authorization_failed")
			}
			return err
		}
		if !client.AllowsRedirect(cmd.RedirectURI) {
			return shared.ErrValidation.
				WithDetail("oauth.redirect_uri_mismatch").
				WithFields(shared.FieldError{Path: "/redirect_uri", Code: "oauth.redirect_uri_mismatch"})
		}

		now := w.Session.Clock.Now()
		grantID, err := w.Grants.Upsert(ctx, domain.OauthGrant{
			ID: w.Session.IDs.NewID(), TenantID: actor.TenantID,
			AccountID: actor.AccountID, ClientID: client.ID,
			Scopes: cmd.Scopes, CreatedAt: now.UTC(),
		})
		if err != nil {
			return err
		}

		code := domain.OauthCode{
			ID: w.Session.IDs.NewID(), TenantID: actor.TenantID,
			ClientID: client.ID, AccountID: actor.AccountID, GrantID: grantID,
			Challenge: cmd.Challenge, RedirectURI: cmd.RedirectURI,
			CreatedAt: now.UTC(), ExpiresAt: now.Add(domain.OauthCodeLifetime).UTC(),
		}
		if err := w.Codes.Insert(ctx, code, presented); err != nil {
			return err
		}

		if err := w.recordGrantAudit(ctx, actor.Kind, actor.AccountID, actor.AccountName,
			actor.TenantID, OauthConsentedAction, grantID, client.ID,
			strings.Join(cmd.Scopes, " "), now); err != nil {
			return err
		}

		minted = AuthorizedCode{
			Code:      secret.New(presented.Secret()),
			ExpiresAt: code.ExpiresAt,
			State:     cmd.State,
		}
		return nil
	})
	if err != nil {
		return AuthorizedCode{}, err
	}
	return minted, nil
}

// ExchangeOauthCodeCommand is the token request, typed.
type ExchangeOauthCodeCommand struct {
	Code         secret.Secret
	RedirectURI  string
	ClientID     shared.ID
	Verifier     string
	ClientSecret secret.Secret
	TenantHeader string
}

// ExchangeOauthCode is the token endpoint (H-05): the code, the PKCE verifier and - for a
// confidential client - the secret, exchanged for the pair sign-in mints, leashed to the grant.
type ExchangeOauthCode struct{ Writer OauthWriter }

// Execute exchanges. Unknown, expired, replayed, misdirected and mis-verified codes are one
// generic refusal: which of them applies is exactly what a thief holding half the dance wants
// to know.
func (h ExchangeOauthCode) Execute(
	ctx context.Context, cmd ExchangeOauthCodeCommand,
) (SessionPair, error) {
	w := h.Writer

	token, err := domain.ParseOauthCode(cmd.Code.Reveal())
	if err != nil {
		w.Session.failure(ctx, FailureOauth)
		return SessionPair{}, exchangeRefused()
	}
	if cmd.TenantHeader != "" && cmd.TenantHeader != token.TenantID().String() {
		return SessionPair{}, shared.ErrForbidden.WithDetail("access.tenant_mismatch")
	}

	scope := persistence.Scope{TenantID: token.TenantID()}
	var (
		account domain.Account
		grant   domain.OauthGrant
	)
	err = w.Session.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		client, err := w.Clients.Find(ctx, cmd.ClientID)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				w.Session.failure(ctx, FailureOauth)
				return exchangeRefused()
			}
			return err
		}

		// The client authenticates first: a confidential one with its secret, a public one
		// with PKCE alone - checked below against the code's own challenge.
		if client.Confidential {
			matches, err := w.Clients.SecretMatches(ctx, client.ID, cmd.ClientSecret)
			if err != nil {
				return err
			}
			if !matches {
				w.Session.failure(ctx, FailureOauth)
				return exchangeRefused()
			}
		} else if cmd.Verifier == "" {
			w.Session.failure(ctx, FailureOauth)
			return shared.ErrValidation.
				WithDetail("oauth.pkce_required").
				WithFields(shared.FieldError{Path: "/code_verifier", Code: "oauth.pkce_required"})
		}

		now := w.Session.Clock.Now()
		code, consumed, err := w.Codes.Consume(ctx, token, now)
		if err != nil {
			return err
		}
		if !consumed || code.ClientID != client.ID || code.RedirectURI != cmd.RedirectURI {
			w.Session.failure(ctx, FailureOauth)
			return exchangeRefused()
		}
		if cmd.Verifier != "" && !domain.VerifyPKCE(cmd.Verifier, code.Challenge) {
			w.Session.failure(ctx, FailureOauth)
			return exchangeRefused()
		}
		if !client.Confidential && cmd.Verifier == "" {
			w.Session.failure(ctx, FailureOauth)
			return exchangeRefused()
		}

		found, err := w.Grants.Find(ctx, code.GrantID)
		if err != nil {
			return err
		}
		if err := found.Verify(); err != nil {
			w.Session.failure(ctx, FailureOauth)
			return exchangeRefused()
		}
		holder, err := w.Session.People.Find(ctx, code.AccountID)
		if err != nil {
			return err
		}
		if err := holder.Verify(); err != nil {
			return err
		}
		holder.TenantID = token.TenantID()

		account, grant = holder, found
		return w.recordGrantAudit(ctx, appshared.ActorUser, holder.ID, holder.DisplayName,
			token.TenantID(), OauthExchangedAction, found.ID, client.ID,
			strings.Join(found.Scopes, " "), now)
	})
	if err != nil {
		return SessionPair{}, err
	}

	return w.Session.openLeashedSession(ctx, scope, token.TenantID(), account, grant)
}

// ListOauthGrants answers the caller's own grants, the session listing's reasoning.
type ListOauthGrants struct{ Writer OauthWriter }

func (h ListOauthGrants) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]repository.GrantListing, error) {
	w := h.Writer
	if err := actor.RequireScope(accountsRead); err != nil {
		return nil, err
	}
	if actor.AccountID.IsZero() {
		return nil, shared.ErrForbidden.WithDetail("access.token_owner_required")
	}

	var listings []repository.GrantListing
	err := w.Session.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(),
		func(ctx context.Context) error {
			found, err := w.Grants.ListForAccount(ctx, actor.AccountID)
			listings = found
			return err
		})
	if err != nil {
		return nil, err
	}
	return listings, nil
}

// RevokeOauthGrant withdraws what an app was allowed, immediately.
type RevokeOauthGrant struct{ Writer OauthWriter }

func (h RevokeOauthGrant) Execute(
	ctx context.Context, actor appshared.ActorContext, grantID shared.ID,
) error {
	w := h.Writer
	if err := actor.RequireScope(accountRead); err != nil {
		return err
	}
	if grantID.IsZero() {
		return shared.ErrNotFound.WithDetail("oauth.grant_not_found")
	}

	return w.Session.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Session.Clock.Now()
		changed, err := w.Grants.Revoke(ctx, grantID, actor.AccountID, now)
		if err != nil {
			return err
		}
		if !changed {
			// Already withdrawn, or somebody else's: idempotent for one's own, the
			// indistinguishable not-found for the rest, RevokeSession's shape.
			grant, err := w.Grants.Find(ctx, grantID)
			if err != nil || grant.AccountID != actor.AccountID {
				return shared.ErrNotFound.WithDetail("oauth.grant_not_found")
			}
			return nil
		}
		ended, err := w.Grants.RevokeSessions(ctx, grantID, now)
		if err != nil {
			return err
		}
		grant, err := w.Grants.Find(ctx, grantID)
		if err != nil {
			return err
		}
		return w.recordGrantAudit(ctx, actor.Kind, actor.AccountID, actor.AccountName,
			actor.TenantID, OauthGrantRevokedAction, grantID, grant.ClientID,
			strconv.Itoa(ended), now)
	})
}

// openLeashedSession opens the pair for a grant (H-05): the session carries the grant's scopes
// and dies with the grant's revocation.
func (w SessionWriter) openLeashedSession(
	ctx context.Context, scope persistence.Scope, tenantID shared.ID,
	account domain.Account, grant domain.OauthGrant,
) (SessionPair, error) {
	material, err := w.Entropy.Bytes(domain.TokenSecretBytes)
	if err != nil {
		return SessionPair{}, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}

	var pair SessionPair
	err = w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		now := w.Clock.Now()
		session, err := domain.NewSession(domain.NewSessionInput{
			ID: w.IDs.NewID(), TenantID: tenantID, AccountID: account.ID, Now: now,
		})
		if err != nil {
			return err
		}
		session.GrantID = grant.ID
		session.Scopes = grant.Scopes
		if err := w.Sessions.Insert(ctx, session); err != nil {
			return err
		}

		first, presented, err := w.mintRefresh(session, material, now)
		if err != nil {
			return err
		}
		if err := w.Refresh.Insert(ctx, first, presented); err != nil {
			return err
		}

		pair = w.pairFor(session, account, presented, first, now)
		return nil
	})
	if err != nil {
		return SessionPair{}, err
	}
	return pair, nil
}

// FailureOauth is the metric reason of a refused exchange.
const FailureOauth = "oauth_refused"

// exchangeRefused is the token endpoint's one probe-facing refusal.
func exchangeRefused() error {
	return shared.ErrUnauthenticated.WithDetail("oauth.exchange_failed")
}

// checkScopes refuses a name the installation does not declare, the token mint's rule: a scope
// nothing checks is a bound nothing applies.
func (w OauthWriter) checkScopes(requested []string) error {
	if len(requested) == 0 {
		return shared.ErrValidation.
			WithDetail("access.token_scopes_required").
			WithFields(shared.FieldError{Path: "/scopes", Code: "access.token_scopes_required"})
	}
	for _, scope := range requested {
		if !slices.Contains(w.KnownScopes, scope) {
			return shared.ErrValidation.
				WithDetail("access.token_scope_unknown").
				WithParams(map[string]string{"scope": scope}).
				WithFields(shared.FieldError{Path: "/scopes", Code: "access.token_scope_unknown"})
		}
	}
	return nil
}

// recordClientAudit writes the registry's evidence: identifiers and the registration's shape,
// never a secret.
func (w OauthWriter) recordClientAudit(
	ctx context.Context, actor appshared.ActorContext, action audit.Action,
	client domain.OauthClient,
) error {
	kind := "public"
	if client.Confidential {
		kind = "confidential"
	}
	return w.Session.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: w.Session.Clock.Now(),
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: oauthClientTarget,
		TargetID:   client.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx), APIClient: client.ID.String()},
		Changes: audit.Changes(
			audit.Change{Field: "kind", Classification: audit.Open, To: kind},
			audit.Change{Field: "redirect_uris", Classification: audit.Open,
				To: strconv.Itoa(len(client.RedirectURIs))},
		),
	})
}

// recordGrantAudit writes a grant event with the client as a first-class actor attribute (H-05):
// "which app did this" is the question this feature exists to answer.
func (w OauthWriter) recordGrantAudit(
	ctx context.Context, kind appshared.ActorKind, actorID shared.ID, actorLabel string,
	tenantID shared.ID, action audit.Action, grantID, clientID shared.ID,
	detail string, at time.Time,
) error {
	return w.Session.Audit.Append(ctx, audit.Entry{
		TenantID:   tenantID,
		OccurredAt: at,
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  kind,
		ActorID:    actorID,
		ActorLabel: actorLabel,
		TargetType: oauthGrantTarget,
		TargetID:   grantID,
		Context: audit.Context{
			RequestID: correlation.RequestIDFrom(ctx),
			APIClient: clientID.String(),
		},
		Changes: audit.Changes(audit.Change{
			Field: "detail", Classification: audit.Open, To: detail,
		}),
	})
}

// clientOutput is the projection every channel gets. The secret is not in it.
func clientOutput(client domain.OauthClient) usecase.Output {
	uris := make([]any, 0, len(client.RedirectURIs))
	for _, uri := range client.RedirectURIs {
		uris = append(uris, uri)
	}
	return usecase.Output{
		"id":            client.ID.String(),
		"name":          client.Name,
		"confidential":  client.Confidential,
		"redirect_uris": uris,
		"created_at":    client.CreatedAt.UTC(),
	}
}

// Descriptor is the catalogue entry.
func (h RegisterOauthClient) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RegisterOauthClientName,
		Summary: "Registers a third-party app: a name, the exact redirect URIs, and whether it " +
			"can keep a secret. A confidential client's secret is answered once and stored as " +
			"a hash; a public client gets none and must bring PKCE to every authorization.",
		SideEffects: "Writes the registration and an audit entry, and answers a secret once.",
		TokenScope:  oauthManage,
		Input: []usecase.Field{
			{Name: "name", Kind: usecase.KindString, Required: true,
				Description: "What people read when deciding to allow the app."},
			{Name: "redirect_uris", Kind: usecase.KindList, Required: true,
				Description: "Absolute, fragment-free, matched exactly at every step."},
			{Name: "confidential", Kind: usecase.KindBool,
				Description: "True for an app that can keep a secret on a server."},
		},
		Audit: usecase.AuditDeclaration{
			Action: OauthClientRegisteredAction, TargetType: oauthClientTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A registration is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RegisterOauthClient) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	uris, err := in.StringList("redirect_uris")
	if err != nil {
		return nil, err
	}
	registered, err := h.Execute(ctx, actor, RegisterOauthClientCommand{
		Name:         in.String("name"),
		Confidential: in.Bool("confidential"),
		RedirectURIs: uris,
	})
	if err != nil {
		return nil, err
	}
	out := clientOutput(registered.Client)
	if registered.Client.Confidential {
		// The one place the secret is ever answered (T-18's "shown once").
		out["client_secret"] = registered.Secret.Reveal()
	}
	return out, nil
}

// Descriptor is the catalogue entry.
func (h ListOauthClients) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListOauthClientsName,
		Summary: "The registry of third-party apps: name, kind, redirect URIs. The secrets are " +
			"not among the fields - each was answered once, at registration.",
		SideEffects: "None. Reads only.",
		TokenScope:  oauthManage,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: OauthClientReadAction, TargetType: oauthClientTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListOauthClients) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	clients, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	rows := make([]usecase.Output, 0, len(clients))
	for _, client := range clients {
		rows = append(rows, clientOutput(client))
	}
	return usecase.Output{"data": rows}, nil
}

// Descriptor is the catalogue entry.
func (h DeleteOauthClient) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DeleteOauthClientName,
		Summary: "Removes a registered app. Every grant that pointed at it goes too, and the " +
			"sessions those grants leashed refuse on their next request: a door is removed " +
			"with every key that opened it.",
		SideEffects: "Removes the client, its grants and their sessions, and writes an audit entry.",
		TokenScope:  oauthManage,
		Destructive: true,
		Input: []usecase.Field{
			{Name: "client_id", Kind: usecase.KindID, Required: true,
				Description: "The app to remove."},
		},
		Audit: usecase.AuditDeclaration{
			Action: OauthClientDeletedAction, TargetType: oauthClientTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A registration is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h DeleteOauthClient) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	clientID, err := in.ID("client_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, clientID); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}

// Descriptor is the catalogue entry.
func (h AuthorizeOauthClient) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: AuthorizeOauthClientName,
		Summary: "Records a person's consent to an app's request for bounded access and mints " +
			"the single-use code the app will exchange. The scopes come from this " +
			"installation's own catalogue; the redirect URI matches the registration exactly; " +
			"S256 is the only PKCE method. The consent is a grant the person sees and revokes " +
			"beside their sessions.",
		SideEffects: "Records or refreshes the grant, mints a two-minute code, and writes an " +
			"audit entry naming the client.",
		Input: []usecase.Field{
			{Name: "client_id", Kind: usecase.KindID, Required: true,
				Description: "The app asking."},
			{Name: "redirect_uri", Kind: usecase.KindString, Required: true,
				Description: "One of the app's registered URIs, exactly."},
			{Name: "scopes", Kind: usecase.KindList, Required: true,
				Description: "What the app may do, from the installation's catalogue."},
			{Name: "code_challenge", Kind: usecase.KindString, Required: true,
				Description: "BASE64URL(SHA256(code_verifier)), RFC 7636."},
			{Name: "code_challenge_method", Kind: usecase.KindString, Required: true,
				Enum: []string{"S256"}, Description: "S256 and nothing else."},
			{Name: "state", Kind: usecase.KindString,
				Description: "Echoed back untouched, for the client's own CSRF binding."},
		},
		Audit: usecase.AuditDeclaration{
			Action: OauthConsentedAction, TargetType: oauthGrantTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A consent is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h AuthorizeOauthClient) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	clientID, err := in.ID("client_id")
	if err != nil {
		return nil, err
	}
	scopes, err := in.StringList("scopes")
	if err != nil {
		return nil, err
	}
	minted, err := h.Execute(ctx, actor, AuthorizeOauthClientCommand{
		ClientID:    clientID,
		RedirectURI: in.String("redirect_uri"),
		Scopes:      scopes,
		Challenge:   in.String("code_challenge"),
		Method:      in.String("code_challenge_method"),
		State:       in.String("state"),
	})
	if err != nil {
		return nil, err
	}
	out := usecase.Output{
		"code":       minted.Code.Reveal(),
		"expires_at": minted.ExpiresAt.UTC(),
	}
	if minted.State != "" {
		out["state"] = minted.State
	}
	return out, nil
}

// Descriptor is the catalogue entry.
func (h ExchangeOauthCode) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ExchangeOauthCodeName,
		Summary: "Exchanges a single-use authorization code for the pair sign-in mints, leashed " +
			"to the grant. The PKCE verifier has to prove the code's challenge; a confidential " +
			"client also presents its secret; the redirect URI repeats the authorization's " +
			"exactly. A code replayed after exchange is refused.",
		SideEffects: "Burns the code, opens a grant-leashed session, answers two credentials " +
			"once, and writes an audit entry naming the client.",
		Input: []usecase.Field{
			{Name: "grant_type", Kind: usecase.KindString, Required: true,
				Enum: []string{"authorization_code"}, Description: "The one grant this provider speaks."},
			{Name: "code", Kind: usecase.KindString, Required: true,
				Description: "The authorization's code. It dies on this call."},
			{Name: "redirect_uri", Kind: usecase.KindString, Required: true,
				Description: "The authorization's redirect URI, exactly."},
			{Name: "client_id", Kind: usecase.KindID, Required: true,
				Description: "The app exchanging."},
			{Name: "code_verifier", Kind: usecase.KindString,
				Description: "RFC 7636's verifier. Mandatory for a public client."},
			{Name: "client_secret", Kind: usecase.KindString,
				Description: "The confidential client's credential."},
			{Name: "tenant_header", Kind: usecase.KindString,
				Description: "The X-Hubtask-Tenant header, when sent. Confirms, never overrules."},
		},
		Audit: usecase.AuditDeclaration{
			Action: OauthExchangedAction, TargetType: oauthGrantTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "An exchange is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ExchangeOauthCode) invoke(
	ctx context.Context, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	clientID, err := in.ID("client_id")
	if err != nil {
		return nil, err
	}
	pair, err := h.Execute(ctx, ExchangeOauthCodeCommand{
		Code:         secret.New(in.String("code")),
		RedirectURI:  in.String("redirect_uri"),
		ClientID:     clientID,
		Verifier:     in.String("code_verifier"),
		ClientSecret: secret.New(in.String("client_secret")),
		TenantHeader: in.String("tenant_header"),
	})
	if err != nil {
		return nil, err
	}
	return pairOutput(pair), nil
}

// Descriptor is the catalogue entry.
func (h ListOauthGrants) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListOauthGrantsName,
		Summary: "The caller's own grants, newest first: which app, which scopes, when it was " +
			"allowed and when a session under it last acted. Never anybody else's.",
		SideEffects: "None. Reads only.",
		TokenScope:  accountsRead,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: OauthGrantReadAction, TargetType: oauthGrantTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListOauthGrants) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	listings, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	rows := make([]usecase.Output, 0, len(listings))
	for _, listing := range listings {
		scopes := make([]any, 0, len(listing.Grant.Scopes))
		for _, scope := range listing.Grant.Scopes {
			scopes = append(scopes, scope)
		}
		row := usecase.Output{
			"id":           listing.Grant.ID.String(),
			"client_id":    listing.Grant.ClientID.String(),
			"client_name":  listing.ClientName,
			"scopes":       scopes,
			"created_at":   listing.Grant.CreatedAt.UTC(),
			"last_used_at": nil,
		}
		if !listing.LastUsedAt.IsZero() {
			row["last_used_at"] = listing.LastUsedAt.UTC()
		}
		rows = append(rows, row)
	}
	return usecase.Output{"data": rows}, nil
}

// Descriptor is the catalogue entry.
func (h RevokeOauthGrant) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RevokeOauthGrantName,
		Summary: "Withdraws what an app was allowed, immediately: every session the grant " +
			"leashed refuses on its next request. Somebody else's grant is not found; revoking " +
			"twice is not an error.",
		SideEffects: "Stamps the grant, ends its sessions, and writes an audit entry naming the client.",
		TokenScope:  accountRead,
		Destructive: true,
		Input: []usecase.Field{
			{Name: "grant_id", Kind: usecase.KindID, Required: true,
				Description: "The grant to withdraw, from the caller's own list."},
		},
		Audit: usecase.AuditDeclaration{
			Action: OauthGrantRevokedAction, TargetType: oauthGrantTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A consent is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RevokeOauthGrant) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	grantID, err := in.ID("grant_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, grantID); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}
