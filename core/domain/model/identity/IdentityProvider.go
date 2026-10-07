// SPDX-License-Identifier: Apache-2.0
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

// Provisioning is who a provider may bring in at all (identity.md §10.4).
//
// One axis, three positions, and the axis is **who comes in** - not how freely somebody claims an
// account that already exists. That is the reading identity.md §10.4 fixes, and the difference is
// one case: a subject whose address is outside `AllowedEmailDomains`.
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

// IdentityProvider is a provider people sign in through (identity.md §10, ADR-0005).
//
// Plural, and with a level above the workspace (identity.md §10.2): a zero `TenantID` is the
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
	// AllowedDirectories are the organisations this row admits, in the provider's own identifiers
	// (ADR-0071 §2). Read instead of the domains where the preset has a directory claim.
	AllowedDirectories []string
	Enabled            bool
	// OfferedHere is whether this provider is a way into the workspace that is reading it. Not a
	// column: for a workspace's own row it *is* `Enabled`, and for the installation's it is the
	// reading workspace's own switch, which lives in its settings (identity.md §10.2).
	OfferedHere bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Version     int
	// WithdrawAt is when the installation's offer ends (ADR-0076 §2); zero while it is offered
	// without an end. Until then the provider works; from then it is a way in nowhere.
	WithdrawAt time.Time
	// OfferedWorkspaces is how many workspaces have the installation's row switched on - a number,
	// never names (ADR-0076 §1). Counted from the workspaces' own switches where the installation
	// reads it, so every write to them leaves it true, and zero wherever a workspace reads it
	// (ADR-0077 §1). Answered to the operator only.
	OfferedWorkspaces int
}

// WithdrawalNotice is how far ahead a withdrawal is announced where the operator names no date
// (ADR-0076 §2): two weeks for every workspace that uses the provider to switch on another way.
const WithdrawalNotice = 14 * 24 * time.Hour

// MinimumWithdrawalNotice is the least that counts as notice at all. A withdrawal sooner than this
// is Withdraw now in all but name, and asks for the count as Withdraw now does - otherwise the
// confirmation could be skipped by naming a moment a second away.
const MinimumWithdrawalNotice = 24 * time.Hour

// OfferedAt reports whether the row is a way in at all at this moment: switched on, and not past an
// announced withdrawal. The date is honoured where the offer is read, so no job has to end it.
func (p IdentityProvider) OfferedAt(now time.Time) bool {
	return p.Enabled && (p.WithdrawAt.IsZero() || now.Before(p.WithdrawAt))
}

// Withdrawing reports whether an announced withdrawal is still ahead: the time in which the
// workspaces that use it are told when it ends.
func (p IdentityProvider) Withdrawing(now time.Time) bool {
	return !p.WithdrawAt.IsZero() && now.Before(p.WithdrawAt)
}

// RemovableAt answers nil where the row may be removed at this moment, and the refusal where it may
// not (ADR-0077 §2). Removing a provider deletes the connections between people and it, which
// offering it again does not restore - so the installation's row goes only once its offer has ended
// or where no workspace uses it, after a withdrawal that announced itself. A workspace's own row is
// the workspace's to remove, and its own rule (the last way in) is asked elsewhere.
//
// While a withdrawal is already announced the operator has done what the refusal would ask, so it
// says when instead: the day the offer ends, from which the row may go.
//
// The statement that deletes the row asks the same three facts again, so that nothing that changed
// between a caller's look and the delete slips past it; the two are kept in step by hand.
func (p IdentityProvider) RemovableAt(now time.Time) error {
	if !p.Installation() || !p.OfferedAt(now) || p.OfferedWorkspaces == 0 {
		return nil
	}
	if p.Withdrawing(now) {
		return shared.ErrValidation.
			WithDetail("identity_provider.remove_after_withdrawal").
			WithParams(map[string]string{"date": p.WithdrawAt.UTC().Format(time.RFC3339)})
	}
	return shared.ErrValidation.
		WithDetail("identity_provider.withdraw_first").
		WithParams(map[string]string{"count": strconv.Itoa(p.OfferedWorkspaces)})
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
	// AllowedDirectories are the organisations this row admits, in the provider's own identifiers
	// (ADR-0071 §2). Read instead of the domains where the preset has a directory claim.
	AllowedDirectories []string
	Enabled            bool
	Now                time.Time
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

	provisioning, err := resolvedProvisioning(in.Provisioning, preset, in.TenantID.IsZero())
	if err != nil {
		return IdentityProvider{}, err
	}

	// An installation's provider is offered to every workspace, so what it admits is multiplied by
	// the number of workspaces that take the offer. One with a directory claim is bounded by the
	// directories it names; one without - a self-hosted issuer - is bounded by nothing but the
	// address domain, which is the wrong thing to authorise on (ADR-0071 §1). Offered as
	// INVITED_ONLY it admits the people each workspace invited and nobody else (ADR-0071's
	// addendum).
	if in.TenantID.IsZero() && preset.DirectoryClaim == "" && provisioning != ProvisionInvitedOnly {
		return IdentityProvider{}, shared.ErrValidation.
			WithDetail("identity_provider.installation_invited_only").
			WithParams(map[string]string{"kind": string(kind)})
	}

	domains, err := normalisedDomains(in.AllowedEmailDomains)
	if err != nil {
		return IdentityProvider{}, err
	}

	directories, err := normalisedDirectories(in.AllowedDirectories, preset)
	if err != nil {
		return IdentityProvider{}, err
	}

	// A multi-directory endpoint with nothing naming the directories is every organisation in the
	// world, and that is a decision rather than a default (ADR-0071 §3). Refused at configuration
	// so that nobody discovers it by meeting a stranger in their workspace.
	if multiDirectoryIssuer(issuer) && preset.SupportsTemplatedIssuer && len(directories) == 0 {
		return IdentityProvider{}, shared.ErrValidation.
			WithDetail("identity_provider.directories_required").
			WithParams(map[string]string{"issuer": issuer})
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
		AllowedEmailDomains: domains, AllowedDirectories: directories, Enabled: in.Enabled,
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
	// A multi-directory endpoint is not refused for itself, although ADR-0036 compares `iss`
	// exactly and a token minted behind `common` names the *directory* rather than `common`.
	// ADR-0071 §3 keeps the exact comparison and changes what it compares against: the issuer
	// template the provider itself publishes, with the token's `tid` substituted. What the endpoint
	// needs instead is a bound - `AllowedDirectories` - and that is checked below, where the list
	// is.
	if kind == KindMicrosoft && multiDirectoryIssuer(issuer) && !preset.SupportsTemplatedIssuer {
		return "", ProviderPreset{}, shared.ErrValidation.
			WithDetail("identity_provider.issuer_multi_directory").
			WithParams(map[string]string{"issuer": issuer})
	}
	return kind, preset, nil
}

// MaxAllowedDirectories bounds the directory list, for the reason the domains list is bounded.
const MaxAllowedDirectories = 20

// normalisedDirectories checks the list against what the preset can read it as (ADR-0071 §2).
//
// A preset with no directory claim has no use for one, and a list on such a row would be a list
// nothing consults - which is worse than no list, because somebody wrote it believing it bounded
// something. Refused rather than dropped.
func normalisedDirectories(raw []string, preset ProviderPreset) ([]string, error) {
	if len(raw) > MaxAllowedDirectories {
		return nil, shared.ErrValidation.
			WithDetail("identity_provider.directories_too_many").
			WithParams(map[string]string{"limit": strconv.Itoa(MaxAllowedDirectories)})
	}
	if len(raw) > 0 && preset.DirectoryClaim == "" {
		return nil, shared.ErrValidation.
			WithDetail("identity_provider.directories_unsupported").
			WithParams(map[string]string{"kind": string(preset.Kind)})
	}

	seen := map[string]bool{}
	directories := make([]string, 0, len(raw))
	for _, entry := range raw {
		// Lower case and nothing else: a directory identifier is a GUID at one provider and a host
		// at another, and this package is not the place that knows which. What it refuses is the
		// shapes that could not be either.
		directory := strings.ToLower(strings.TrimSpace(entry))
		if directory == "" || strings.ContainsAny(directory, " /?#%@\\") {
			return nil, shared.ErrValidation.
				WithDetail("identity_provider.directory_invalid").
				WithParams(map[string]string{"directory": strings.TrimSpace(entry)})
		}
		if seen[directory] {
			continue
		}
		seen[directory] = true
		directories = append(directories, directory)
	}
	return directories, nil
}

// multiDirectoryIssuer reports whether an issuer's path is the shared endpoint rather than one
// directory's.
func multiDirectoryIssuer(issuer string) bool {
	parsed, err := url.Parse(issuer)
	if err != nil {
		return false
	}
	for _, segment := range microsoftMultiDirectorySegments {
		if parsed.Path == segment || strings.HasPrefix(parsed.Path, segment+"/") {
			return true
		}
	}
	return false
}

// resolvedProvisioning reads the mode and holds it to what the preset permits.
//
// One refusal: a **public** issuer may only be INVITED_ONLY. Anything else means every person who
// holds an account at that provider - which is everybody - is provisioned one here.
//
// There is deliberately no second - "a preset whose addresses this installation cannot vouch for
// may not be INVITED_ONLY, because that mode gives an existing account away on the strength of an
// address". It would protect nothing: the other modes claim existing accounts on exactly the same
// signal and create new ones besides. What stops an address from handing over an account is the
// account's own proof, asked when an arrival would connect to an account that already holds a
// credential (ADR-0071's addendum, identity.md §11). INVITED_ONLY is therefore the strictest mode
// for every preset, and every preset may have it.
//
// Nothing stated is the safe value rather than the permissive one: a public provider, and an
// installation's provider with no directory to bound it, default to INVITED_ONLY; everything else to
// what this installation did before the column existed.
func resolvedProvisioning(stated string, preset ProviderPreset, installation bool) (Provisioning, error) {
	if strings.TrimSpace(stated) == "" {
		if preset.Public || (installation && preset.DirectoryClaim == "") {
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

// Arriving is what a provider vouched for about the person at the door (ADR-0071 §1).
//
// Four facts, and the two new ones are the point. An address is a **name**: `verified` says the
// provider confirmed delivery to it, and nothing more. A **directory** is a fact the provider
// vouched for — `tid` at Microsoft, `hd` at Google — and it is the one that says which organisation
// this person belongs to. Both providers this product presets say in their own documentation that
// the address is the wrong thing to authorise on.
type Arriving struct {
	// Email is the address the token carried.
	Email string
	// EmailVerified is the provider's confirmation that mail reaches it.
	EmailVerified bool
	// Directory is the provider's identifier for the organisation, empty where the token named
	// none — a personal account, or an issuer with no such claim.
	Directory string
	// AddressAuthoritative is whether the provider hosts the mailbox the address names, in its own
	// terms (ADR-0078 §5): Microsoft's `xms_edov` exactly `true`; Google for its consumer domains or
	// a hosted domain equal to the address's; no other issuer, a self-hosted one included.
	//
	// It is what `INVITED_ONLY` needs, because claiming an account that already exists is done on
	// the strength of an address, and an address nobody owns the domain of is an assertion. It is
	// also one of the two second proofs that activate an invited account (ADR-0078 §1).
	AddressAuthoritative bool
}

// MayAdmit answers whether this provider may bring an arriving subject into the workspace at all.
//
// The first gate, and the one that decides between a refusal and everything else. `MayClaim` and
// `MayProvision` below say what happens to somebody who got through it.
//
// **Under `DOMAINS` the question is the directory where the preset has one** (ADR-0071 §2). Not
// "prefers" — instead of. Two lists that both admit are two doors, and the weaker one decides which
// is why the domains list is not consulted at all for a preset that knows better. What it keeps is
// the one case where it is all there is: a `GENERIC` issuer, whose token offers no directory claim
// and never will.
func (p IdentityProvider) MayAdmit(arriving Arriving) bool {
	if !arriving.EmailVerified || emailDomain(arriving.Email) == "" {
		return false
	}
	switch p.Provisioning {
	case ProvisionInvitedOnly:
		// Claiming an account that already exists is done on the strength of an address, so the
		// address has to be one the provider owns the domain of. A personal account with a
		// verified address at somebody else's domain is precisely the case this refuses.
		return arriving.AddressAuthoritative
	case ProvisionAny:
		return true
	case ProvisionDomains:
		return p.admitsDirectoryOf(arriving)
	default:
		// A mode this build does not understand admits nobody. A row written by a newer one is a
		// row this one does not act on, which is the only safe reading of it.
		return false
	}
}

// MayAdmitWithProof answers whether this provider may bring an arriving subject to a door it opens
// only with a second proof of its own (ADR-0078 §1, §5): the invitation's own link bound to the
// flow, for the one invited account it names, or an existing account's own proof at the LINK step.
//
// Everything `MayAdmit` admits, and under `INVITED_ONLY` and `DOMAINS` one more: an address the
// provider verified but would not admit on its own word - not authoritative for it, or outside the
// directory or domain list. Authority and the list decide who comes in *new*, on the provider's
// word alone - a new account, an invitation activated without its link. An existing account that
// brings its own proof - its password at the LINK step, its mailbox through the connect link, an
// invitation's link - is not coming in new: it is a member connecting a provider, with the
// provider's verified address equal to its own (the caller checks that), and nothing is created
// through this (identity.md §10.4). `ANY` admits every verified address already, and a mode this
// build does not know admits nobody.
func (p IdentityProvider) MayAdmitWithProof(arriving Arriving) bool {
	if p.MayAdmit(arriving) {
		return true
	}
	return p.AdmitsOwnProof() && arriving.EmailVerified && emailDomain(arriving.Email) != ""
}

// AdmitsOwnProof answers whether this provider can connect an existing account that brings its own
// proof at all: every mode this build knows does, for a verified address. A mailed connect link is
// pointless through a provider that never would - one whose mode a newer build wrote.
func (p IdentityProvider) AdmitsOwnProof() bool {
	switch p.Provisioning {
	case ProvisionInvitedOnly, ProvisionDomains, ProvisionAny:
		return true
	default:
		return false
	}
}

// MayAdmitInvitation answers whether this provider may bring in the one invited account a sign-in
// started from - the invitation's own link, bound to the flow (ADR-0078 §1).
//
// The invitation is an administrator's explicit choice of this person, so neither authority nor the
// directory or domain list overrules it - and it widens nothing beyond the invitation itself: the
// caller has checked that the verified address is the invited account's, and the link proves that
// one account only. It reads as `MayAdmitWithProof` - the link is the invited account's own proof -
// and keeps its name so that the invitation's door says which proof it relies on. An unverified
// address is never admitted, and a mode this build does not know admits nobody.
func (p IdentityProvider) MayAdmitInvitation(arriving Arriving) bool {
	return p.MayAdmitWithProof(arriving)
}

// admitsDirectoryOf is `DOMAINS`, read against whichever thing this preset can be sure of.
func (p IdentityProvider) admitsDirectoryOf(arriving Arriving) bool {
	preset, known := PresetOf(p.Kind)
	if known && preset.DirectoryClaim != "" {
		// The preset has a directory claim, so the directory list is the gate — and an empty list
		// admits nobody, exactly as an empty domains list does. A row migrated from before
		// ADR-0071 has domains and no directories and therefore admits nobody until an
		// administrator names one; failing closed is the only direction that does not hand out
		// accounts, and the screen says so rather than leaving it to be discovered.
		return arriving.Directory != "" && containsFold(p.AllowedDirectories, arriving.Directory)
	}
	return p.linksDomain(arriving.Email)
}

// containsFold is the comparison both lists use: a directory identifier is a GUID or a host, and
// neither has a case somebody typed on purpose.
func containsFold(list []string, value string) bool {
	for _, each := range list {
		if strings.EqualFold(each, value) {
			return true
		}
	}
	return false
}

// MayClaim answers whether an admitted subject may take over an account that already exists.
//
// True in every mode, because admission already asked the harder question: under `INVITED_ONLY`
// that the provider owns the address's domain, and under `DOMAINS` that the person comes from a
// directory this row names. What differs between the modes is who is admitted, not what an
// admitted person may do - which is the whole of the concept's reading.
func (p IdentityProvider) MayClaim(arriving Arriving) bool {
	return p.MayAdmit(arriving)
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

// OidcFlowPrefix labels the state a sign-in flow hands the browser, so a value found in a log or a
// bug report says what it was without anybody having to guess (the `hbt_` prefixes of
// api-guidelines.md §7).
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
	// SessionID binds the flow to the session that asked for a step-up at the provider (ADR-0075
	// §2). Zero is a sign-in flow, which finishes a sign-in and no step-up.
	SessionID shared.ID
	// InvitedAccountID is the invited account a sign-in started from the invitation's own link
	// accepts (ADR-0078 §1): the second proof that lets a provider activate it. Zero is every
	// other sign-in. Checked when the flow opened, and spent only by an arrival that succeeds.
	InvitedAccountID shared.ID
	// PendingID is the CONNECT link a sign-in started from, in a workspace that switched the
	// password off (ADR-0078 §1): the mailbox half of the account's proof. Zero is every other
	// sign-in. Checked when the flow opened, and spent only by an arrival that connects.
	PendingID shared.ID
}

// NewOidcFlowInput is what starting a sign-in needs.
type NewOidcFlowInput struct {
	ID         shared.ID
	TenantID   shared.ID
	ProviderID shared.ID
	Nonce      string
	Verifier   string
	Now        time.Time
	// SessionID makes the flow a step-up of that session. Zero for a sign-in.
	SessionID shared.ID
	// InvitedAccountID is the invitation a sign-in started from. Zero for every other flow, and
	// never beside a session: a step-up belongs to somebody already signed in.
	InvitedAccountID shared.ID
	// PendingID is the CONNECT link a sign-in started from. Zero for every other flow; never beside
	// a session or an invitation - an invited account holds no link to connect, and a step-up
	// belongs to somebody already signed in.
	PendingID shared.ID
}

// NewOidcFlow opens one.
//
// The verifier's length is RFC 7636's, and it is checked here rather than trusted: a verifier
// short enough to guess makes PKCE decorative, and the one place that would notice is a test
// nobody wrote.
func NewOidcFlow(in NewOidcFlowInput) (OidcFlow, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() || in.ProviderID.IsZero() || in.Now.IsZero() ||
		in.Nonce == "" || len(in.Verifier) < 43 || len(in.Verifier) > 128 ||
		(!in.SessionID.IsZero() && !in.InvitedAccountID.IsZero()) ||
		(!in.PendingID.IsZero() && (!in.SessionID.IsZero() || !in.InvitedAccountID.IsZero())) {
		return OidcFlow{}, shared.ErrInternal.WithDetail("identity_provider.flow_incomplete")
	}
	return OidcFlow{
		ID: in.ID, TenantID: in.TenantID, ProviderID: in.ProviderID,
		Nonce: in.Nonce, Verifier: in.Verifier, SessionID: in.SessionID,
		InvitedAccountID: in.InvitedAccountID, PendingID: in.PendingID,
		CreatedAt: in.Now.UTC(), ExpiresAt: in.Now.Add(OidcFlowLifetime).UTC(),
	}, nil
}

// ParseOidcFlowState reads a presented state back into its token, and with it the workspace the
// flow belongs to. The tenant travels inside the handle rather than being taken from the request
// on the return leg: the browser comes back from somebody else's site, and what it carries is
// the only thing about that leg this installation minted itself.
func ParseOidcFlowState(raw string) (Token, error) { return parsePrefixed(raw, OidcFlowPrefix) }

// ProvisionExternal builds the account a subject gets on its first arrival (identity.md §10.4).
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
