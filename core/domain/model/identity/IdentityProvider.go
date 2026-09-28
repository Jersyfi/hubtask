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

// Provisioning is who a provider may bring in at all (SI-10, the concept's §8).
//
// One axis, three positions, and the axis is **who comes in** - not how freely somebody claims an
// account that already exists. That is the reading the concept fixes, and the difference is one
// case: a subject whose address is outside `AllowedEmailDomains`.
//
//   - INVITED_ONLY - only somebody who was invited here first. A verified address must meet an
//     account that already exists; anything else is refused and nothing is created. The mode a
//     public provider is held to.
//   - DOMAINS - only inside `AllowedEmailDomains`. A verified address in the list comes in - linked
//     to the account that already holds it, or provisioned where there is none. An address outside
//     the list is **refused**, which is what makes this different from ANY.
//   - ANY - everybody the provider vouches for. For a provider that is the workspace's own
//     directory, where its population *is* the workspace's.
//
// An unverified address comes in nowhere. An address the provider did not vouch for is somebody
// typing, and acting on it hands over the account it belongs to - or invents one in its name.
//
// **An empty list under DOMAINS admits nobody.** Before this column existed, an empty list meant
// "link nobody, provision everybody"; it means "nobody" now. That is the one place where the safe
// reading changed rather than stayed, and it is the reading the mode's own name promises.

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
	// OfferedHere is whether this provider is a way into the workspace that is reading it. Not a
	// column: for a workspace's own row it *is* `Enabled`, and for the installation's it is the
	// reading workspace's own switch, which lives in its settings (SI-10).
	OfferedHere bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Version     int
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

	// The second half of the concept's §8 security sentence: a provider whose addresses this
	// installation cannot vouch for "kann nur ANY oder DOMAINS sein **und ist deshalb nie
	// Installations-Anbieter**". The reason is the blast radius: an installation's provider is
	// offered to every workspace, and one that can only be DOMAINS or ANY provisions accounts in
	// each of them.
	if in.TenantID.IsZero() && !preset.AddressesVerified {
		return IdentityProvider{}, shared.ErrValidation.
			WithDetail("identity_provider.installation_unverified").
			WithParams(map[string]string{"kind": string(kind)})
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

// MayAdmit answers whether this provider may bring an arriving subject into the workspace at all.
//
// The first gate, and the one that decides between a refusal and everything else. `MayClaim` and
// `MayProvision` below say what happens to somebody who got through it.
func (p IdentityProvider) MayAdmit(email string, verified bool) bool {
	if !verified || emailDomain(email) == "" {
		return false
	}
	switch p.Provisioning {
	case ProvisionInvitedOnly, ProvisionAny:
		return true
	case ProvisionDomains:
		return p.linksDomain(email)
	default:
		// A mode this build does not understand admits nobody. A row written by a newer one is a
		// row this one does not act on, which is the only safe reading of it.
		return false
	}
}

// MayClaim answers whether an admitted subject may take over an account that already exists.
//
// True in every mode, because admission already required an address the provider vouched for and,
// under DOMAINS, one inside the configured list. What differs between the modes is who is admitted,
// not what an admitted person may do - which is the whole of the concept's reading.
func (p IdentityProvider) MayClaim(email string, verified bool) bool {
	return p.MayAdmit(email, verified)
}

// MayProvision answers whether an admitted subject with no account here gets one.
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
