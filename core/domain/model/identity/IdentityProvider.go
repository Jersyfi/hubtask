// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"net/url"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// MaxAllowedEmailDomains bounds the linking list. Ten is more organisations than any workspace
// that signs in through one provider has, and an unbounded list is a row somebody eventually
// pastes a directory into.
const MaxAllowedEmailDomains = 10

// IdentityProvider is the provider a workspace signs its people in through (H-04, ADR-0005).
//
// One per workspace, and the row's primary key enforces that rather than this type. What lives
// here is the part that has rules: an issuer that must look like an issuer, and the domains
// inside which an arriving address may claim an account that already exists.
//
// The client secret is deliberately not a field. It travels sealed, from the use case that
// receives it to the adapter that stores it and back out only at a token exchange - a struct
// that carried it would eventually be logged by somebody who had no idea it was in there.
type IdentityProvider struct {
	TenantID            shared.ID
	Issuer              string
	ClientID            string
	AllowedEmailDomains []string
	Enabled             bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
	Version             int
}

// NewIdentityProviderInput is what configuring one needs.
type NewIdentityProviderInput struct {
	TenantID            shared.ID
	Issuer              string
	ClientID            string
	AllowedEmailDomains []string
	Enabled             bool
	Now                 time.Time
}

// NewIdentityProvider validates a configuration and normalises what has a normal form.
func NewIdentityProvider(in NewIdentityProviderInput) (IdentityProvider, error) {
	if in.TenantID.IsZero() || in.Now.IsZero() {
		return IdentityProvider{}, shared.ErrInternal.WithDetail("identity_provider.incomplete")
	}

	issuer, err := normalisedIssuer(in.Issuer)
	if err != nil {
		return IdentityProvider{}, err
	}

	clientID := strings.TrimSpace(in.ClientID)
	if clientID == "" {
		return IdentityProvider{}, shared.ErrValidation.
			WithDetail("identity_provider.client_id_required")
	}

	domains, err := normalisedDomains(in.AllowedEmailDomains)
	if err != nil {
		return IdentityProvider{}, err
	}

	return IdentityProvider{
		TenantID: in.TenantID, Issuer: issuer, ClientID: clientID,
		AllowedEmailDomains: domains, Enabled: in.Enabled,
		CreatedAt: in.Now.UTC(), Version: 1,
	}, nil
}

// normalisedIssuer holds the issuer to what OpenID Connect Discovery says one is: an https URL
// with a host and nothing else on the end of it.
//
// The scheme is not negotiable. An issuer reached over plain HTTP is one anybody on the path can
// answer for, and every check that follows - the metadata, the keys, the signature - is then a
// check against whatever they said.
func normalisedIssuer(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimSuffix(trimmed, "/")
	if trimmed == "" {
		return "", shared.ErrValidation.WithDetail("identity_provider.issuer_required")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return "", shared.ErrValidation.
			WithDetail("identity_provider.issuer_invalid").
			WithParams(map[string]string{"issuer": trimmed})
	}
	return trimmed, nil
}

// normalisedDomains lowercases, de-duplicates and refuses what is not a domain.
//
// A wildcard is refused rather than interpreted. "Anything ending in example.com" is how
// `evil-example.com` becomes a way in, and a list somebody has to write out is a list somebody
// has to think about.
func normalisedDomains(raw []string) ([]string, error) {
	if len(raw) > MaxAllowedEmailDomains {
		return nil, shared.ErrValidation.
			WithDetail("identity_provider.domains_too_many").
			WithParams(map[string]string{"limit": "10"})
	}

	seen := map[string]bool{}
	domains := make([]string, 0, len(raw))
	for _, entry := range raw {
		domain := strings.ToLower(strings.TrimSpace(entry))
		domain = strings.TrimPrefix(domain, "@")
		if domain == "" || !strings.Contains(domain, ".") ||
			strings.ContainsAny(domain, "@/ *") || strings.HasPrefix(domain, ".") ||
			strings.HasSuffix(domain, ".") {
			return nil, shared.ErrValidation.
				WithDetail("identity_provider.domain_invalid").
				WithParams(map[string]string{"domain": strings.TrimSpace(entry)})
		}
		if seen[domain] {
			continue
		}
		seen[domain] = true
		domains = append(domains, domain)
	}
	return domains, nil
}

// LinksAddress answers whether a verified address from the provider may claim an existing local
// account.
//
// Two conditions, and both are the point. The address must be one the provider says it verified -
// an unverified claim is somebody typing an address, and acting on it hands them the account it
// belongs to. And its domain must be on the configured list: an empty list links nothing, which
// is the safe reading of a workspace that never said which domains its provider speaks for.
func (p IdentityProvider) LinksAddress(email string, verified bool) bool {
	if !verified || len(p.AllowedEmailDomains) == 0 {
		return false
	}
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return false
	}
	domain := strings.ToLower(email[at+1:])
	for _, allowed := range p.AllowedEmailDomains {
		if domain == allowed {
			return true
		}
	}
	return false
}

// OidcFlowPrefix labels the state a sign-in flow hands the browser, so a value found in a log or
// a bug report says what it was without anybody having to guess (D-08's prefix catalogue).
const OidcFlowPrefix = "hbt_osf_"

// OidcFlowLifetime is how long a sign-in may sit between leaving for the provider and coming
// back. Ten minutes: a person types a password and possibly a second factor in that window, and
// a handle that could wait an hour is a credential lying around in a browser history.
const OidcFlowLifetime = 10 * time.Minute

// NewOidcFlowState mints the handle the callback presents back.
func NewOidcFlowState(tenantID shared.ID, material []byte) (Token, error) {
	return newPrefixed(OidcFlowPrefix, tenantID, material)
}

// OidcFlow is one browser round trip: what the callback has to check the identity token against,
// and what the exchange has to present.
//
// The state is not a field. It is minted, hashed and stored by the adapter the way every
// presented token here is - what this carries is the two values that never leave the server.
type OidcFlow struct {
	ID        shared.ID
	TenantID  shared.ID
	Nonce     string
	Verifier  string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// NewOidcFlowInput is what starting a sign-in needs.
type NewOidcFlowInput struct {
	ID       shared.ID
	TenantID shared.ID
	Nonce    string
	Verifier string
	Now      time.Time
}

// NewOidcFlow opens one.
//
// The verifier's length is RFC 7636's, and it is checked here rather than trusted: a verifier
// short enough to guess makes PKCE decorative, and the one place that would notice is a test
// nobody wrote.
func NewOidcFlow(in NewOidcFlowInput) (OidcFlow, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() || in.Now.IsZero() ||
		in.Nonce == "" || len(in.Verifier) < 43 || len(in.Verifier) > 128 {
		return OidcFlow{}, shared.ErrInternal.WithDetail("identity_provider.flow_incomplete")
	}
	return OidcFlow{
		ID: in.ID, TenantID: in.TenantID, Nonce: in.Nonce, Verifier: in.Verifier,
		CreatedAt: in.Now.UTC(), ExpiresAt: in.Now.Add(OidcFlowLifetime).UTC(),
	}, nil
}

// ParseOidcFlowState reads a presented state back into its token, and with it the workspace the
// flow belongs to. The tenant travels inside the handle rather than being taken from the request
// on the return leg: the browser comes back from somebody else's site, and what it carries is
// the only thing about that leg this installation minted itself.
func ParseOidcFlowState(raw string) (Token, error) { return parsePrefixed(raw, OidcFlowPrefix) }

// ProvisionExternal builds the account a subject gets on its first arrival (H-04).
//
// Active immediately and with no password, which is the whole difference from an invitation: the
// provider has just vouched for this person, so there is nothing left for them to prove here, and
// there is no local credential to prove it with. An address is optional - a provider that sends
// none leaves the account without one, and everything that needs an address says so itself rather
// than inventing one.
func ProvisionExternal(
	id, tenantID shared.ID, email, displayName string,
) (Account, error) {
	if id.IsZero() || tenantID.IsZero() {
		return Account{}, shared.ErrInternal.WithDetail("accounts.identity_incomplete")
	}

	address := ""
	if strings.TrimSpace(email) != "" {
		normalised, err := emailAddress(email)
		if err != nil {
			return Account{}, err
		}
		address = normalised
	}

	name, err := accountDisplayName(displayName, address)
	if err != nil {
		return Account{}, err
	}

	return Account{
		ID: id, TenantID: tenantID, Kind: AccountUser,
		Email: address, DisplayName: name, Status: AccountActive,
	}, nil
}
