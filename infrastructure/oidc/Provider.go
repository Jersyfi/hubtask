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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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

// tenantIDPlaceholder is what Microsoft publishes in the issuer of its multi-directory discovery
// documents: `https://login.microsoftonline.com/{tenantid}/v2.0` (ADR-0071 §3).
//
// A templated issuer is **not** a weaker comparison. The token's `tid` is substituted and `iss` is
// then compared exactly, which is ADR-0036 §2 unchanged; what the substitution buys is that the
// comparand is the directory the token itself names, so `iss` and `tid` cannot disagree. Reading
// `tid` before verification decides nothing - the signature still has to come from the key set the
// template's own discovery published - and *which* directories may come in is the domain's
// question, answered against `allowed_directories`.
const tenantIDPlaceholder = "{tenantid}"

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
	// keySet is kept beside the provider so that a templated issuer can be verified against a
	// per-token issuer without building a second key set - which would throw away the JWKS cache
	// and the rotation behaviour that is the whole reason ADR-0036 chose this library.
	keySet *gooidc.RemoteKeySet
	// published is the issuer the discovery document named, which is the configured one for every
	// ordinary provider and the endpoint's own answer for a multi-directory address.
	published string
	fetched   time.Time
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
	entry, err := p.cached(ctx, cfg.Issuer)
	if err != nil {
		return port.Identity{}, err
	}
	provider := entry.provider

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

	expected, err := expectedIssuer(entry, cfg.Issuer, raw)
	if err != nil {
		return port.Identity{}, err
	}
	// `NewVerifier` rather than `provider.Verifier` because the issuer has to be this token's,
	// and the key set is the entry's so that rotation and the JWKS cache are unaffected. For an
	// ordinary provider `expected` is the configured issuer and this is the same verifier the
	// library would have built.
	verifier := gooidc.NewVerifier(expected, entry.keySet, &gooidc.Config{
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

	return identityFrom(verified, cfg.DirectoryClaim)
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
	published := issuer
	provider, err := gooidc.NewProvider(p.context(ctx), issuer)
	if err != nil {
		// **One disagreement is not a disagreement.** A multi-directory endpoint is an *address*
		// rather than an identity: `/common/v2.0` publishes the issuer as a template, and
		// `/consumers/v2.0` publishes the fixed directory of personal Microsoft accounts. Both say
		// so in the document the check just read, and what a token carries is the published value
		// rather than the address it was fetched from (ADR-0071 §3).
		//
		// The bound is the **host**. A document that names an issuer at the same host is naming
		// its own directories; one that points at another host is redirecting trust, and that is
		// the mismatch this check exists for. Which of the host's directories may actually come in
		// is not settled here at all - `allowed_directories` settles it, inwards of this package.
		var mismatch *gooidc.IssuerMismatchError
		if !errors.As(err, &mismatch) || !sameOrigin(issuer, mismatch.Discovered) {
			// Unreachable, or it disagrees with its own metadata. This is the degradation
			// observability-reliability.md §7 describes: local accounts keep signing in, and the
			// person in front of the provider is told plainly that it is the provider.
			return nil, shared.ErrUnavailable.
				WithDetail("auth.provider_unreachable").
				WithCause(fmt.Errorf("discovering %q: %w", issuer, err))
		}
		published = mismatch.Discovered
		// Fetched again with the published value as the expected issuer, so the provider this
		// builds carries what tokens will name. The document is the same one the first call
		// already read and agreed with in every other respect.
		provider, err = gooidc.NewProvider(
			gooidc.InsecureIssuerURLContext(p.context(ctx), published), issuer)
		if err != nil {
			return nil, shared.ErrUnavailable.
				WithDetail("auth.provider_unreachable").
				WithCause(fmt.Errorf("discovering %q against its published issuer: %w", issuer, err))
		}
	}

	keys, err := keySetOf(p.context(ctx), provider)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	p.discovery[issuer] = cachedProvider{
		provider: provider, keySet: keys, published: published, fetched: now,
	}
	p.mu.Unlock()
	return provider, nil
}

// cached answers the whole entry, for the caller that needs the key set or the template.
func (p *Provider) cached(ctx context.Context, issuer string) (cachedProvider, error) {
	if _, err := p.discover(ctx, issuer); err != nil {
		return cachedProvider{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.discovery[issuer], nil
}

// keySetOf builds the rotating key set from the document's own `jwks_uri`.
//
// One per issuer, kept for the life of the entry: `RemoteKeySet` is what refetches when a token
// arrives signed by a key it does not know, and a set built per sign-in would ask the provider for
// its keys on every single one.
func keySetOf(ctx context.Context, provider *gooidc.Provider) (*gooidc.RemoteKeySet, error) {
	var document struct {
		JWKSURL string `json:"jwks_uri"`
	}
	if err := provider.Claims(&document); err != nil || document.JWKSURL == "" {
		return nil, shared.ErrUnavailable.
			WithDetail("auth.provider_unreachable").
			WithCause(fmt.Errorf("the discovery document names no key set: %w", err))
	}
	return gooidc.NewRemoteKeySet(ctx, document.JWKSURL), nil
}

// expectedIssuer answers what this token's `iss` has to equal.
//
// For an ordinary provider that is the configured issuer, untouched. For a templated one it is the
// template with the token's own `tid` substituted - Microsoft's documented rule, and the reason the
// comparison that follows stays exact.
func expectedIssuer(entry cachedProvider, configured, raw string) (string, error) {
	published := entry.published
	if published == "" {
		published = configured
	}
	if !strings.Contains(published, tenantIDPlaceholder) {
		return published, nil
	}
	directory, err := unverifiedClaim(raw, "tid")
	if err != nil || directory == "" {
		return "", invalidToken(errors.New("a templated issuer needs the token to name its directory"))
	}
	// A directory that is not a plain identifier could rewrite the issuer into another host. It
	// cannot pass the exact comparison that follows, but building the string at all is the kind of
	// thing that becomes an injection the day somebody logs it.
	if strings.ContainsAny(directory, "/?#%\\") {
		return "", invalidToken(errors.New("the directory is not an identifier"))
	}
	return strings.ReplaceAll(published, tenantIDPlaceholder, directory), nil
}

// sameOrigin answers whether two issuer strings name the same scheme and host.
//
// The whole of what a published issuer is allowed to differ in: its path. A document at
// `login.microsoftonline.com/common/v2.0` may name `login.microsoftonline.com/{tenantid}/v2.0`
// because those are the same provider's own directories; one that names another host is a document
// handing this installation's trust somewhere else.
//
// The scheme is compared rather than required to be https, because the domain already refuses a
// configured issuer that is not (`identity_provider.issuer_invalid`) - and an equality check there
// is what makes an http document unable to satisfy an https configuration. Two checks that say the
// same thing in two places is how one of them ends up saying something slightly different.
func sameOrigin(configured, discovered string) bool {
	left, err := url.Parse(configured)
	if err != nil {
		return false
	}
	right, err := url.Parse(discovered)
	if err != nil {
		return false
	}
	return left.Host != "" && left.Scheme == right.Scheme && strings.EqualFold(left.Host, right.Host)
}

// unverifiedClaim reads one string claim out of a token nobody has verified yet.
//
// Used for exactly one thing and worth the sentence: picking which issuer to *check against*. What
// it returns is never trusted - the exact comparison and the signature are what decide - and it
// reads a single field rather than handing the payload to anything.
func unverifiedClaim(raw, name string) (string, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return "", errors.New("not a token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	var claims map[string]json.RawMessage
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", err
	}
	var value string
	if err := json.Unmarshal(claims[name], &value); err != nil {
		return "", err
	}
	return value, nil
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
//
// `directoryClaim` is the preset's name for the claim that says which organisation this person
// belongs to (ADR-0071 §1). It is read by name rather than by a switch on the provider, so nothing
// here learns that Microsoft calls it `tid` and Google calls it `hd` - the preset does.
func identityFrom(token *gooidc.IDToken, directoryClaim string) (port.Identity, error) {
	var claims struct {
		Email         string          `json:"email"`
		EmailVerified json.RawMessage `json:"email_verified"`
		Name          string          `json:"name"`
		Username      string          `json:"preferred_username"`
		// DomainOwnerVerified is Microsoft's `xms_edov`, and it is `email_verified`'s meaning
		// under another name: "whether the user's email domain owner has been verified". Entra ID
		// issues no `email_verified` at all, so without this claim every Microsoft account is
		// unverified and therefore refused - which is what it did until ADR-0071.
		DomainOwnerVerified json.RawMessage `json:"xms_edov"`
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

	directory := ""
	if directoryClaim != "" {
		// A failure here is a token that named no directory, which is a personal account at a
		// provider that has organisations - not an error, and the domain decides what it means.
		directory, _ = tokenClaim(token, directoryClaim)
	}

	verified := verifiedFlag(claims.EmailVerified)
	if len(claims.DomainOwnerVerified) > 0 {
		// Microsoft's claim answers the question `email_verified` answers elsewhere, so a token
		// that carries it is verified by it. It is an *optional* claim an operator has to switch
		// on in the app registration, and the provider screen says so.
		verified = verifiedFlag(claims.DomainOwnerVerified)
	}

	return port.Identity{
		Subject:              token.Subject,
		Email:                claims.Email,
		EmailVerified:        verified,
		DisplayName:          name,
		Directory:            directory,
		AddressAuthoritative: authoritative(claims.Email, verified, directory, claims.DomainOwnerVerified),
	}, nil
}

// tokenClaim reads one string claim out of a verified token.
func tokenClaim(token *gooidc.IDToken, name string) (string, error) {
	var claims map[string]json.RawMessage
	if err := token.Claims(&claims); err != nil {
		return "", err
	}
	raw, held := claims[name]
	if !held {
		return "", errors.New("the token carries no such claim")
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	return value, nil
}

// authoritative answers whether the provider vouches for the address **and** its domain.
//
// Three readings, and each is the provider's own (ADR-0071 §1):
//
//   - Microsoft answers it directly with `xms_edov`, which is documented as exactly this question.
//   - Google does not, and says why the obvious substitute is wrong: "The domain of the email claim
//     is insufficient to ensure that the account is managed by a domain or organization - you must
//     verify the `hd` claim explicitly." So it is verified **and** the directory the token named is
//     the address's own domain. A personal account is verified and not authoritative.
//   - An issuer with no directory claim has nothing more to say than `email_verified`, and that is
//     what it gets. A self-hosted provider is the workspace's own directory; treating its word as
//     authoritative is the reading that matches what configuring it means.
func authoritative(email string, verified bool, directory string, microsoft json.RawMessage) bool {
	if !verified {
		return false
	}
	if len(microsoft) > 0 {
		return true
	}
	if directory == "" {
		return true
	}
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return false
	}
	return strings.EqualFold(email[at+1:], directory)
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

// Check confirms the issuer is one: reachable, and naming itself the same in its own metadata.
//
// It shares the discovery cache with the flow, which is what makes configuring a provider warm
// the path the first sign-in takes.
func (p *Provider) Check(ctx context.Context, issuer string) error {
	provider, err := p.discover(ctx, issuer)
	if err != nil {
		return err
	}

	// **What a provider promises, checked while somebody is still looking at the form**
	// (ADR-0071 §5). An issuer that signs with nothing this installation accepts is an issuer
	// whose every token is refused, and the difference between finding that out here and finding
	// it out at a sign-in screen is the difference between a message and an outage.
	//
	// It is also the check that keeps working as providers change: discovery is refetched every
	// hour, so a provider that drops an algorithm is met with this refusal on the next
	// configuration rather than with a signature failure nobody can read.
	var document struct {
		Algorithms []string `json:"id_token_signing_alg_values_supported"`
	}
	if err := provider.Claims(&document); err != nil {
		return shared.ErrUnavailable.
			WithDetail("auth.provider_unreachable").
			WithCause(fmt.Errorf("reading the discovery document of %q: %w", issuer, err))
	}
	// An absent list is the specification's "RS256 is required", not "anything goes": a document
	// that names none is one that signs with RS256, which is on the allowlist.
	if len(document.Algorithms) == 0 {
		return nil
	}
	for _, offered := range document.Algorithms {
		for _, accepted := range signingAlgorithms {
			if offered == accepted {
				return nil
			}
		}
	}
	return shared.ErrValidation.
		WithDetail("identity_provider.signing_unsupported").
		WithParams(map[string]string{"algorithms": strings.Join(document.Algorithms, ", ")})
}
