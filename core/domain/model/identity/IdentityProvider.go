// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// MaxAllowedEmailDomains bounds the linking list. Ten is more organisations than any workspace
// that signs in through one provider has, and an unbounded list is a row somebody eventually
// pastes a directory into.
const MaxAllowedEmailDomains = 10

// Provisioning is who gets an account on a first arrival through a provider (SI-10).
//
// One axis, three positions, and the axis is **how freely an arriving subject may claim an account
// that already exists here**. That is the only question with a security answer: creating an account
// gives somebody an empty desk, and claiming one gives them somebody else's.
//
//   - INVITED_ONLY - it must claim one. A verified address that meets no account here is refused,
//     and nothing is created. The mode a public provider is held to.
//   - DOMAINS - it may claim one inside `AllowedEmailDomains`, and is provisioned otherwise. What
//     this installation did before there was a column for it.
//   - ANY - it may claim one on any address the provider says it verified. For a provider that *is*
//     the workspace's directory, where every address in it belongs to the workspace anyway.
//
// An unverified address never claims anything, in any of the three: an address the provider did not
// vouch for is somebody typing, and acting on it hands over the account it belongs to.
type Provisioning string

const (
	ProvisionInvitedOnly Provisioning = "INVITED_ONLY"
	ProvisionDomains     Provisioning = "DOMAINS"
	ProvisionAny         Provisioning = "ANY"
)

// ParseProvisioning reads the mode and refuses what is not one.
func ParseProvisioning(raw string) (Provisioning, error) {
	switch mode := Provisioning(strings.ToUpper(strings.TrimSpace(raw))); mode {
	case "":
		return ProvisionDomains, nil
	case ProvisionInvitedOnly, ProvisionDomains, ProvisionAny:
		return mode, nil
	default:
		return "", shared.ErrValidation.
			WithDetail("identity_provider.provisioning_invalid").
			WithParams(map[string]string{"provisioning": strings.TrimSpace(raw)})
	}
}

// MaxProviderPosition bounds the order. A workspace that needs a hundredth sign-in button has a
// problem this field will not solve.
const MaxProviderPosition = 99

// IdentityProvider is a provider people sign in through (H-04, ADR-0005, SI-10).
//
// Plural since SI-10, and with a level above the workspace: a zero `TenantID` is the
// installation's own row - the one every workspace reads and none writes. What lives here is the
// part that has rules: an issuer that must look like an issuer, a preset the issuer has to belong
// to, and a provisioning mode the preset may forbid.
//
// The client secret is deliberately not a field. It travels sealed, from the use case that
// receives it to the adapter that stores it and back out only at a token exchange - a struct
// that carried it would eventually be logged by somebody who had no idea it was in there.
type IdentityProvider struct {
	ID shared.ID
	// TenantID is the workspace this belongs to, and zero for the installation's own.
	TenantID            shared.ID
	Issuer              string
	ClientID            string
	DisplayName         string
	Kind                ProviderKind
	Provisioning        Provisioning
	Position            int
	AllowedEmailDomains []string
	Enabled             bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
	Version             int
}

// Installation reports whether this row belongs to no workspace.
func (p IdentityProvider) Installation() bool { return p.TenantID.IsZero() }

// NewIdentityProviderInput is what configuring one needs.
type NewIdentityProviderInput struct {
	ID shared.ID
	// TenantID is zero for the installation's own provider, which is a decision the use case has
	// already made by the time it gets here: the scope it writes under is what decides it, not a
	// field a caller sends.
	TenantID            shared.ID
	Issuer              string
	ClientID            string
	DisplayName         string
	Kind                string
	Provisioning        string
	Position            int
	AllowedEmailDomains []string
	Enabled             bool
	Now                 time.Time
}

// NewIdentityProvider validates a configuration and normalises what has a normal form.
func NewIdentityProvider(in NewIdentityProviderInput) (IdentityProvider, error) {
	if in.ID.IsZero() || in.Now.IsZero() {
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

	kind, preset, err := resolvedKind(in.Kind, issuer)
	if err != nil {
		return IdentityProvider{}, err
	}

	provisioning, err := resolvedProvisioning(in.Provisioning, preset)
	if err != nil {
		return IdentityProvider{}, err
	}

	domains, err := normalisedDomains(in.AllowedEmailDomains)
	if err != nil {
		return IdentityProvider{}, err
	}

	if in.Position < 0 || in.Position > MaxProviderPosition {
		return IdentityProvider{}, shared.ErrValidation.
			WithDetail("identity_provider.position_invalid").
			WithParams(map[string]string{"limit": strconv.Itoa(MaxProviderPosition)})
	}

	displayName, err := providerDisplayName(in.DisplayName, issuer)
	if err != nil {
		return IdentityProvider{}, err
	}

	return IdentityProvider{
		ID: in.ID, TenantID: in.TenantID, Issuer: issuer, ClientID: clientID,
		DisplayName: displayName, Kind: kind, Provisioning: provisioning, Position: in.Position,
		AllowedEmailDomains: domains, Enabled: in.Enabled,
		CreatedAt: in.Now.UTC(), Version: 1,
	}, nil
}

// MaxProviderDisplayName bounds the name on the button.
const MaxProviderDisplayName = 200

// providerDisplayName trims the stated name, and falls back to the issuer's host.
//
// The host is what a button said before there was a column to put a name in, and it discloses
// nothing new: pressing the button sends the person to exactly that host.
func providerDisplayName(stated, issuer string) (string, error) {
	name := strings.TrimSpace(stated)
	if name == "" {
		return IssuerHost(issuer), nil
	}
	if len([]rune(name)) > MaxProviderDisplayName {
		return "", shared.ErrValidation.
			WithDetail("identity_provider.display_name_too_long").
			WithParams(map[string]string{"limit": strconv.Itoa(MaxProviderDisplayName)})
	}
	return name, nil
}

// resolvedKind reads the stated preset, or derives it from the issuer, and refuses a mark put on
// an issuer that does not belong to it.
//
// The refusal is the point rather than tidiness: `kind` decides which logo is drawn (ADR-0069), and
// a `GOOGLE` mark above somebody else's issuer is a borrowed piece of trust on a sign-in screen.
func resolvedKind(stated, issuer string) (ProviderKind, ProviderPreset, error) {
	host := IssuerHost(issuer)
	if strings.TrimSpace(stated) == "" {
		kind := KindOfIssuer(host)
		preset, _ := PresetOf(kind)
		return kind, preset, nil
	}

	kind, err := ParseProviderKind(stated)
	if err != nil {
		return "", ProviderPreset{}, err
	}
	preset, _ := PresetOf(kind)
	if !preset.SpeaksFor(host) {
		return "", ProviderPreset{}, shared.ErrValidation.
			WithDetail("identity_provider.kind_mismatch").
			WithParams(map[string]string{"kind": string(kind), "issuer": issuer})
	}
	// The one provider whose own multi-directory endpoint cannot work here: a token minted behind
	// it names the directory in `iss`, and ADR-0036 compares that exactly.
	if kind == KindMicrosoft && multiDirectoryIssuer(issuer) {
		return "", ProviderPreset{}, shared.ErrValidation.
			WithDetail("identity_provider.issuer_multi_directory").
			WithParams(map[string]string{"issuer": issuer})
	}
	return kind, preset, nil
}

// multiDirectoryIssuer reports whether an issuer's path is the shared endpoint rather than one
// directory's.
func multiDirectoryIssuer(issuer string) bool {
	parsed, err := url.Parse(issuer)
	if err != nil {
		return false
	}
	return parsed.Path == microsoftCommonSegment ||
		strings.HasPrefix(parsed.Path, microsoftCommonSegment+"/")
}

// resolvedProvisioning reads the mode and holds it to what the preset permits.
//
// Two refusals, and each is a hole somebody would otherwise configure by accident:
//
//   - A **public** issuer may only be INVITED_ONLY. Anything else means every person who holds an
//     account at that provider - which is everybody - is provisioned one here.
//   - A preset whose addresses this installation cannot vouch for may **not** be INVITED_ONLY,
//     because that mode gives an existing account away on the strength of an address.
//
// Nothing stated is the safe value rather than the permissive one: a public provider defaults to
// INVITED_ONLY, and everything else to what this installation did before the column existed.
func resolvedProvisioning(stated string, preset ProviderPreset) (Provisioning, error) {
	if strings.TrimSpace(stated) == "" {
		if preset.Public {
			return ProvisionInvitedOnly, nil
		}
		return ProvisionDomains, nil
	}

	mode, err := ParseProvisioning(stated)
	if err != nil {
		return "", err
	}
	if preset.Public && mode != ProvisionInvitedOnly {
		return "", shared.ErrValidation.
			WithDetail("identity_provider.provisioning_public").
			WithParams(map[string]string{"kind": string(preset.Kind)})
	}
	if mode == ProvisionInvitedOnly && !preset.AddressesVerified {
		return "", shared.ErrValidation.
			WithDetail("identity_provider.provisioning_unverified").
			WithParams(map[string]string{"kind": string(preset.Kind)})
	}
	return mode, nil
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

// MayLink answers whether an arriving address may claim an account that already exists here.
//
// Two conditions in every mode, and both are the point. The address must be one the provider says
// it verified - an unverified claim is somebody typing an address, and acting on it hands them the
// account it belongs to. And the mode has to permit it: DOMAINS permits it inside the configured
// list, which empty means nowhere; INVITED_ONLY and ANY permit it anywhere, which is what they are
// for - the first because claiming is the only way in it has, the second because the provider is
// the workspace's own directory.
func (p IdentityProvider) MayLink(email string, verified bool) bool {
	if !verified {
		return false
	}
	switch p.Provisioning {
	case ProvisionInvitedOnly, ProvisionAny:
		return emailDomain(email) != ""
	case ProvisionDomains:
		return p.linksDomain(email)
	default:
		// An unknown mode links nothing. A row this build does not understand is a row it does
		// not act on, which is the only safe reading of a value written by a newer one.
		return false
	}
}

// MayProvision answers whether a subject with no account here gets one.
//
// False for INVITED_ONLY alone, and that is the whole of the mode: somebody has to have invited
// them first.
func (p IdentityProvider) MayProvision() bool {
	return p.Provisioning == ProvisionDomains || p.Provisioning == ProvisionAny
}

// linksDomain is DOMAINS' own half: the address's domain has to be on the configured list, and an
// empty list links nothing - the safe reading of a workspace that never said which domains its
// provider speaks for.
func (p IdentityProvider) linksDomain(email string) bool {
	if len(p.AllowedEmailDomains) == 0 {
		return false
	}
	domain := emailDomain(email)
	if domain == "" {
		return false
	}
	for _, allowed := range p.AllowedEmailDomains {
		if domain == allowed {
			return true
		}
	}
	return false
}

// emailDomain is the part after the last `@`, lowercased, and empty where there is none.
func emailDomain(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return ""
	}
	return strings.ToLower(email[at+1:])
}

// IssuerHost is an issuer's host: what a button says when a provider has no display name of its
// own, and what a preset is matched against.
//
// Parsed rather than trimmed, and not only because it is shorter: a trimmed prefix would put the
// scheme's own spelling into this file, which gate PG-6 reads as an address written into the source.
// The parser knows what a scheme is, and nothing here has to.
func IssuerHost(issuer string) string {
	parsed, err := url.Parse(issuer)
	if err != nil || parsed.Host == "" {
		// Not an address this build can read. Answered whole rather than emptied, because the row
		// was validated when it was configured and a label is not the place to refuse it.
		return issuer
	}
	return parsed.Host
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
	ID       shared.ID
	TenantID shared.ID
	// ProviderID is the way in this sign-in left through. Zero only for a flow opened before there
	// was more than one, which the callback reads as "the one provider this workspace had".
	ProviderID shared.ID
	Nonce      string
	Verifier   string
	CreatedAt  time.Time
	ExpiresAt  time.Time
}

// NewOidcFlowInput is what starting a sign-in needs.
type NewOidcFlowInput struct {
	ID         shared.ID
	TenantID   shared.ID
	ProviderID shared.ID
	Nonce      string
	Verifier   string
	Now        time.Time
}

// NewOidcFlow opens one.
//
// The verifier's length is RFC 7636's, and it is checked here rather than trusted: a verifier
// short enough to guess makes PKCE decorative, and the one place that would notice is a test
// nobody wrote.
func NewOidcFlow(in NewOidcFlowInput) (OidcFlow, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() || in.ProviderID.IsZero() || in.Now.IsZero() ||
		in.Nonce == "" || len(in.Verifier) < 43 || len(in.Verifier) > 128 {
		return OidcFlow{}, shared.ErrInternal.WithDetail("identity_provider.flow_incomplete")
	}
	return OidcFlow{
		ID: in.ID, TenantID: in.TenantID, ProviderID: in.ProviderID,
		Nonce: in.Nonce, Verifier: in.Verifier,
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
	domains text.DomainEncoder, form text.Normalizer,
) (Account, error) {
	if id.IsZero() || tenantID.IsZero() {
		return Account{}, shared.ErrInternal.WithDetail("accounts.identity_incomplete")
	}

	address := ""
	if strings.TrimSpace(email) != "" {
		normalised, err := emailAddress(email, domains)
		if err != nil {
			return Account{}, err
		}
		address = normalised
	}

	name, err := accountDisplayName(displayName, address, form)
	if err != nil {
		return Account{}, err
	}

	return Account{
		ID: id, TenantID: tenantID, Kind: AccountUser,
		Email: address, DisplayName: name, Status: AccountActive,
	}, nil
}
