// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package oidc is the relying party, and the only package in this repository that knows what
// JOSE is (ADR-0036).
//
// Everything the library is good at happens here - discovery, the key set and its rotation, the
// signature check - and nothing of it escapes: the application layer holds
// `core/port/identityprovider` and its four plain structs, so replacing go-oidc tomorrow is an
// edit to one directory. `gate-architecture` proves that confinement rather than trusting it.
package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	port "github.com/Jersyfi/hubtask/core/port/identityprovider"
)

// TargetClass labels the outbound calls this package makes, for the duration histogram. A class
// rather than a host, because the host is a tenant's choice (§3.2, rule 10).
const TargetClass = "identity_provider"

// maxClockSkew is T-13's tolerance, verbatim: a token issued up to a minute in the future is
// somebody's clock, and anything beyond that is not a clock.
const maxClockSkew = 60 * time.Second

// signingAlgorithms is the allowlist, and it is a list rather than a filter for the reason the
// threat row exists: `none` is not on it, and neither is anything symmetric - a token must be
// signed by a key from the provider's own JWKS, so the family can never be chosen by the token.
var signingAlgorithms = []string{
	gooidc.RS256, gooidc.RS384, gooidc.RS512,
	gooidc.ES256, gooidc.ES384, gooidc.ES512,
	gooidc.PS256, gooidc.PS384, gooidc.PS512,
}

// discoveryTTL is how long a provider's metadata is reused. The key set behind it refetches on
// its own when a token arrives signed by a key it does not know, so this bounds the metadata -
// the endpoints - rather than the keys.
const discoveryTTL = time.Hour

// Provider is the relying party for every workspace on this installation.
//
// One object, many workspaces: the configuration travels per call, and what is cached is keyed by
// issuer. Two workspaces pointed at the same provider share its discovery, which is right - the
// metadata belongs to the provider, not to whoever configured it.
type Provider struct {
	// client is the guarded one, as a standard client, because the library takes one. Every
	// check Do makes is made here too (httpclient.HTTPClient).
	client *http.Client
	clock  clock.Clock

	mu        sync.Mutex
	discovery map[string]cachedProvider
}

type cachedProvider struct {
	provider *gooidc.Provider
	fetched  time.Time
}

// New builds the relying party. The client must be the guarded one: an issuer is a URL a tenant
// administrator typed, so it is an egress channel exactly as a webhook target is (rule 6, T-07).
func New(client *http.Client, clk clock.Clock) *Provider {
	return &Provider{client: client, clock: clk, discovery: map[string]cachedProvider{}}
}

var _ port.Port = (*Provider)(nil)

// AuthorizationURL performs discovery and builds the authorization request.
func (p *Provider) AuthorizationURL(
	ctx context.Context, cfg port.Config, auth port.Authorization,
) (string, error) {
	provider, err := p.discover(ctx, cfg.Issuer)
	if err != nil {
		return "", err
	}

	options := []oauth2.AuthCodeOption{
		gooidc.Nonce(auth.Nonce),
		oauth2.S256ChallengeOption(auth.CodeVerifier),
	}
	if auth.LoginHint != "" {
		options = append(options, oauth2.SetAuthURLParam("login_hint", auth.LoginHint))
	}
	flow := p.oauth(cfg, provider)
	return flow.AuthCodeURL(auth.State, options...), nil
}

// Exchange trades the code and verifies what comes back.
//
// Every failure below the transport answers one code. That is deliberate: which of the checks
// refused a token is a detail that helps whoever forged it and nobody else, and the trail carries
// the distinction for the people who are allowed to see it.
func (p *Provider) Exchange(
	ctx context.Context, cfg port.Config, exchange port.Exchange,
) (port.Identity, error) {
	provider, err := p.discover(ctx, cfg.Issuer)
	if err != nil {
		return port.Identity{}, err
	}

	flow := p.oauth(cfg, provider)
	token, err := flow.Exchange(p.context(ctx), exchange.Code,
		oauth2.VerifierOption(exchange.CodeVerifier))
	if err != nil {
		// The provider refused the code, or could not be reached to be asked. Both are the
		// caller's dead end; which one it was is in the cause, for the log.
		return port.Identity{}, shared.ErrUnavailable.
			WithDetail("auth.provider_exchange_failed").
			WithCause(fmt.Errorf("exchanging the authorization code: %w", err))
	}

	raw, ok := token.Extra("id_token").(string)
	if !ok || raw == "" {
		// An OAuth2 answer without an ID token is a provider that is not an OpenID provider,
		// however well it spoke discovery.
		return port.Identity{}, shared.ErrValidation.WithDetail("auth.identity_token_missing")
	}

	verifier := provider.Verifier(&gooidc.Config{
		ClientID:             cfg.ClientID,
		SupportedSigningAlgs: signingAlgorithms,
		Now:                  p.clock.Now,
	})
	verified, err := verifier.Verify(p.context(ctx), raw)
	if err != nil {
		return port.Identity{}, invalidToken(err)
	}
	if verified.Nonce != exchange.Nonce || exchange.Nonce == "" {
		// The nonce ties the token to the flow this installation started. Without it, a token
		// minted for somebody else's session is a token that verifies perfectly.
		return port.Identity{}, invalidToken(errors.New("the nonce does not belong to this flow"))
	}
	if verified.IssuedAt.After(p.clock.Now().Add(maxClockSkew)) {
		return port.Identity{}, invalidToken(errors.New("issued further ahead than a clock explains"))
	}

	return identityFrom(verified)
}

// discover fetches the provider's metadata, or reuses what was fetched recently.
func (p *Provider) discover(ctx context.Context, issuer string) (*gooidc.Provider, error) {
	now := p.clock.Now()

	p.mu.Lock()
	cached, ok := p.discovery[issuer]
	p.mu.Unlock()
	if ok && now.Sub(cached.fetched) < discoveryTTL {
		return cached.provider, nil
	}

	// go-oidc checks that the metadata's own `issuer` equals the one asked for, which is the
	// check RFC 8414 asks for and the reason this is not a plain document fetch.
	provider, err := gooidc.NewProvider(p.context(ctx), issuer)
	if err != nil {
		// Unreachable, or it disagrees with its own metadata. This is the degradation
		// observability-reliability.md §7 describes: local accounts keep signing in, and the
		// person in front of the provider is told plainly that it is the provider.
		return nil, shared.ErrUnavailable.
			WithDetail("auth.provider_unreachable").
			WithCause(fmt.Errorf("discovering %q: %w", issuer, err))
	}

	p.mu.Lock()
	p.discovery[issuer] = cachedProvider{provider: provider, fetched: now}
	p.mu.Unlock()
	return provider, nil
}

// oauth is the library's configuration for one workspace and one flow.
func (p *Provider) oauth(cfg port.Config, provider *gooidc.Provider) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret.Reveal(),
		Endpoint:     provider.Endpoint(),
		RedirectURL:  cfg.RedirectURL,
		Scopes:       []string{gooidc.ScopeOpenID, "profile", "email"},
	}
}

// context hands the library the guarded client. Both packages read it from the context, which is
// the only way either of them accepts one.
func (p *Provider) context(ctx context.Context) context.Context {
	return gooidc.ClientContext(ctx, p.client)
}

// invalidToken is the single refusal every verification failure answers.
func invalidToken(cause error) error {
	return shared.ErrValidation.
		WithDetail("auth.identity_token_invalid").
		WithCause(fmt.Errorf("verifying the identity token: %w", cause))
}

// identityFrom reads the claims this product has a use for, and no others.
func identityFrom(token *gooidc.IDToken) (port.Identity, error) {
	var claims struct {
		Email         string          `json:"email"`
		EmailVerified json.RawMessage `json:"email_verified"`
		Name          string          `json:"name"`
		Username      string          `json:"preferred_username"`
	}
	if err := token.Claims(&claims); err != nil {
		return port.Identity{}, invalidToken(fmt.Errorf("reading the claims: %w", err))
	}
	if token.Subject == "" {
		return port.Identity{}, invalidToken(errors.New("the token names no subject"))
	}

	name := claims.Name
	if name == "" {
		name = claims.Username
	}
	return port.Identity{
		Subject:       token.Subject,
		Email:         claims.Email,
		EmailVerified: verifiedFlag(claims.EmailVerified),
		DisplayName:   name,
	}, nil
}

// verifiedFlag reads `email_verified` from either shape providers send it in.
//
// The specification says boolean and several large providers have sent the string "true" for
// years. Reading only the boolean would make every address from those providers unverified,
// which does not fail loudly - it quietly stops linking from ever happening.
func verifiedFlag(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var flag bool
	if err := json.Unmarshal(raw, &flag); err == nil {
		return flag
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return false
	}
	flag, err := strconv.ParseBool(text)
	return err == nil && flag
}
