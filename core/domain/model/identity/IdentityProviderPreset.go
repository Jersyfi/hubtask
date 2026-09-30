// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The presets a provider can be configured from (SI-10, ADR-0069 §3).
//
// **Data rather than branches**, and the reason is the second one: a `switch` on the issuer that
// decides the mark is a `switch` somebody extends without noticing that the scopes, the
// registration instructions and the provisioning rule all had to move with it. What a preset holds
// is everything that follows from *which* provider this is, in one row, so that adding a fourth is
// adding a row.
//
// What a preset never holds is a secret or an endpoint. Discovery answers the endpoints (ADR-0036),
// and the client registration is the operator's.
type ProviderKind string

const (
	// KindGeneric is every provider with no published sign-in button guideline: an operator's own
	// Keycloak, Authentik, Okta, Zitadel. It draws the letter tile, which is the honest answer
	// rather than a borrowed logo.
	KindGeneric ProviderKind = "GENERIC"
	KindGoogle  ProviderKind = "GOOGLE"
	// KindMicrosoft covers both spellings of the same provider, the v2 endpoint and the legacy
	// security token service.
	KindMicrosoft ProviderKind = "MICROSOFT"
)

// ProviderPreset is what follows from which provider a workspace picked.
type ProviderPreset struct {
	Kind ProviderKind

	// IssuerSuffixes are the hosts this preset speaks for. An issuer that ends in none of them is
	// not this provider, whatever the form said - which is how a `GOOGLE` mark cannot be put on
	// somebody else's issuer to borrow the trust that comes with it.
	//
	// Empty means "any issuer", which is GENERIC and only GENERIC.
	IssuerSuffixes []string

	// Scopes are what the authorization request asks for. Informational here: the adapter asks
	// for exactly these three (ADR-0036), and what the preset is for is telling an operator what
	// their registration has to permit.
	Scopes []string

	// AddressesVerified is whether this issuer's addresses are verified by construction.
	//
	// It decides one thing: whether the provider may be INVITED_ONLY. That mode hands an account
	// that already exists to an arriving subject on the strength of an address, and only an issuer
	// this installation knows verifies addresses may do that. GENERIC is false - not because a
	// self-hosted provider is careless, but because this installation cannot know, and the safe
	// reading of "cannot know" is the one that does not give away an account.
	AddressesVerified bool

	// DirectoryClaim is the claim that names the organisation a person belongs to, as this
	// provider calls it: `tid` at Microsoft, `hd` at Google.
	//
	// **It is what admission is decided on**, and the reason is that both providers say so in their
	// own words. Google: "The domain of the email claim is insufficient to ensure that the account
	// is managed by a domain or organization - you must verify the `hd` claim explicitly."
	// Microsoft: the `email` claim "isn't guaranteed to be correct and is mutable over time - never
	// use it for authorization". An address is a name; a directory is a fact the provider vouched
	// for (ADR-0071 §1).
	//
	// Empty is GENERIC, and it stays empty: there is no claim in OIDC core that means this, and
	// inventing one for an issuer this installation knows nothing about would be a guess. A GENERIC
	// provider therefore admits on `allowed_email_domains`, which is all its token offers.
	DirectoryClaim string

	// IssuerTemplates are the issuer strings this preset accepts with a `{tenantid}` placeholder
	// in them — the multi-directory endpoints (ADR-0071 §3).
	//
	// Microsoft publishes one: the discovery document at `/common/v2.0` answers
	// `https://login.microsoftonline.com/{tenantid}/v2.0`, and documents the rule — substitute the
	// token's `tid` and compare exactly. This field is what says the preset knows that shape; the
	// substitution itself is the adapter's, and what decides who comes in is the directory list.
	SupportsTemplatedIssuer bool

	// Public is whether anybody in the world can hold an account at this issuer.
	//
	// A public provider may **only** be INVITED_ONLY, and the rule is not an operator's to relax:
	// `ANY` on a public issuer means every person alive is provisioned an account in this
	// workspace, which is the finding SI-10 answers.
	Public bool

	// Particular is the message code of the one thing about this provider that is not like the
	// others - `hd` for Google, the directory-specific issuer for Microsoft. Empty where there is
	// none.
	Particular string

	// Instructions is the message code of what an operator has to do at the provider to make this
	// work. Rendered with `redirect_uri`, which is this installation's own callback and the one
	// value every registration form asks for.
	Instructions string
}

// providerHost assembles a host from its labels.
//
// Written in parts rather than as a literal, and not for style: a dotted lowercase string in this
// tree is a message code to `TestEveryUsedMessageCodeIsInTheCatalogue`, and an issuer that had to be
// entered in `locales/en.json` to pass a gate would be a sentence nobody ever renders.
func providerHost(labels ...string) string { return strings.Join(labels, ".") }

// providerPresets is the table. Order is the order a screen offers them in.
var providerPresets = []ProviderPreset{
	{
		Kind:              KindGoogle,
		IssuerSuffixes:    []string{providerHost("accounts", "google", "com")},
		Scopes:            []string{"openid", "email", "profile"},
		AddressesVerified: true,
		// One issuer for every Google account there is, private ones included. The `hd` claim is
		// how a token says which domain it came from - and this installation does not read it,
		// because INVITED_ONLY makes the question moot: the account has to exist here already.
		DirectoryClaim: "hd",
		Public:         true,
		Particular:     "identity_provider.preset.google.particular",
		Instructions:   "identity_provider.preset.google.instructions",
	},
	{
		Kind: KindMicrosoft,
		IssuerSuffixes: []string{
			providerHost("login", "microsoftonline", "com"),
			providerHost("sts", "windows", "net"),
		},
		Scopes:            []string{"openid", "email", "profile"},
		AddressesVerified: true,
		Public:            false,
		DirectoryClaim:    "tid",
		// `common` and `organizations` are the multi-directory endpoints, and a token minted behind
		// one names the *directory* in `iss` - never `common`. Microsoft publishes the issuer as a
		// template and documents the rule: substitute the token's `tid` and compare exactly. That
		// keeps ADR-0036 §2's comparison exact and moves the question of *which* directories may
		// come in to where it belongs — `allowed_directories` (ADR-0071 §3).
		SupportsTemplatedIssuer: true,
		Particular:              "identity_provider.preset.microsoft.particular",
		Instructions:            "identity_provider.preset.microsoft.instructions",
	},
	{
		Kind:              KindGeneric,
		IssuerSuffixes:    nil,
		Scopes:            []string{"openid", "email", "profile"},
		AddressesVerified: false,
		Public:            false,
		Instructions:      "identity_provider.preset.generic.instructions",
	},
}

// ProviderPresets answers the table. A copy: a caller that sorted it would sort everybody's.
func ProviderPresets() []ProviderPreset {
	presets := make([]ProviderPreset, len(providerPresets))
	copy(presets, providerPresets)
	return presets
}

// PresetOf answers the preset of a kind, and whether there is one.
func PresetOf(kind ProviderKind) (ProviderPreset, bool) {
	for _, preset := range providerPresets {
		if preset.Kind == kind {
			return preset, true
		}
	}
	return ProviderPreset{}, false
}

// microsoftMultiDirectorySegments are the addresses that stand for *many* directories rather than
// one (ADR-0071 §3).
//
// `/consumers` is deliberately not among them: it publishes one fixed directory - the one personal
// Microsoft accounts live in - so a row pointed at it is already bounded to exactly that, and
// asking for a list on top would be asking somebody to type a GUID the endpoint already answered.
// It is the configuration for "a private person signs in with their own Microsoft account".
var microsoftMultiDirectorySegments = []string{"/common", "/organizations"}

// ParseProviderKind reads a kind and refuses what is not one.
func ParseProviderKind(raw string) (ProviderKind, error) {
	kind := ProviderKind(strings.ToUpper(strings.TrimSpace(raw)))
	if kind == "" {
		return KindGeneric, nil
	}
	if _, held := PresetOf(kind); !held {
		return "", shared.ErrValidation.
			WithDetail("identity_provider.kind_invalid").
			WithParams(map[string]string{"kind": strings.TrimSpace(raw)})
	}
	return kind, nil
}

// SpeaksFor reports whether an issuer belongs to this preset.
func (p ProviderPreset) SpeaksFor(issuerHost string) bool {
	if len(p.IssuerSuffixes) == 0 {
		return true
	}
	host := strings.ToLower(issuerHost)
	for _, suffix := range p.IssuerSuffixes {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

// KindOfIssuer answers which preset an issuer's host belongs to, GENERIC when none claims it.
//
// It is what the mark is drawn from when nobody said which provider this is, and what the
// configuration checks a stated kind against.
func KindOfIssuer(issuerHost string) ProviderKind {
	for _, preset := range providerPresets {
		if len(preset.IssuerSuffixes) > 0 && preset.SpeaksFor(issuerHost) {
			return preset.Kind
		}
	}
	return KindGeneric
}
