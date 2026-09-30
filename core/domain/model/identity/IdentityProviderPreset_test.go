// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The mark a provider draws is a piece of borrowed trust, so an issuer has to belong to the preset
// that claims it (ADR-0069 §3).
func TestWhichIssuerBelongsToWhichPreset(t *testing.T) {
	cases := []struct {
		name   string
		issuer string
		want   ProviderKind
	}{
		{name: "Google's one issuer", issuer: "https://accounts.google.com", want: KindGoogle},
		{name: "a Microsoft directory", issuer: "https://login.microsoftonline.com/9188040d/v2.0", want: KindMicrosoft},
		{name: "the legacy token service", issuer: "https://sts.windows.net/9188040d/", want: KindMicrosoft},
		{name: "somebody's own Keycloak", issuer: "https://login.example.org/realms/staff", want: KindGeneric},
		{name: "a lookalike is not the provider", issuer: "https://accounts.google.com.evil.net", want: KindGeneric},
		{name: "nor is a prefix", issuer: "https://notaccounts.google.com", want: KindGeneric},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := KindOfIssuer(IssuerHost(c.issuer)); got != c.want {
				t.Errorf("KindOfIssuer(%q) = %q, want %q", c.issuer, got, c.want)
			}
		})
	}
}

// A stated kind the issuer does not belong to is refused rather than corrected: the operator meant
// something, and drawing a Google mark over somebody else's issuer is the outcome this prevents.
func TestAStatedKindMustMatchTheIssuer(t *testing.T) {
	in := providerInput()
	in.Kind = "GOOGLE"
	if _, err := NewIdentityProvider(in); err == nil {
		t.Error("a GOOGLE mark was accepted on an issuer that is not Google's")
	}

	in.Kind = "INVENTED"
	if _, err := NewIdentityProvider(in); err == nil {
		t.Error("a kind that is no preset was accepted")
	}

	in.Kind = "generic"
	configured, err := NewIdentityProvider(in)
	if err != nil {
		t.Fatalf("a lowercased kind was refused: %v", err)
	}
	if configured.Kind != KindGeneric {
		t.Errorf("the kind is %q, want GENERIC", configured.Kind)
	}
}

// `common` is Microsoft's multi-directory endpoint, and a token minted behind it names the
// directory in `iss`. ADR-0036 compares that exactly and is not weakened for one provider, so the
// configuration is refused where somebody is looking at the form.
func TestTheMultiDirectoryIssuerIsRefused(t *testing.T) {
	in := providerInput()
	in.Kind = "MICROSOFT"
	in.Issuer = "https://login.microsoftonline.com/common/v2.0"
	if _, err := NewIdentityProvider(in); err == nil {
		t.Error("the shared endpoint was accepted as an issuer")
	}

	in.Issuer = "https://login.microsoftonline.com/9188040d-6c67-4c5b-b112-36a304b66dad/v2.0"
	if _, err := NewIdentityProvider(in); err != nil {
		t.Errorf("a directory's own issuer was refused: %v", err)
	}
}

// The rule the preset puts on the mode, and the one it no longer does.
func TestWhatAPresetPermitsAsProvisioning(t *testing.T) {
	google := providerInput()
	google.Issuer = "https://accounts.google.com"
	google.Kind = "GOOGLE"

	// Everybody in the world holds an account at this issuer, so anything but INVITED_ONLY
	// provisions all of them a desk here.
	for _, mode := range []string{"DOMAINS", "ANY"} {
		attempt := google
		attempt.Provisioning = mode
		if _, err := NewIdentityProvider(attempt); err == nil {
			t.Errorf("%s was accepted on a public provider", mode)
		}
	}

	// And with nothing stated it is the safe value rather than the previous default.
	configured, err := NewIdentityProvider(google)
	if err != nil {
		t.Fatalf("configuring Google: %v", err)
	}
	if configured.Provisioning != ProvisionInvitedOnly {
		t.Errorf("a public provider defaults to %q, want INVITED_ONLY", configured.Provisioning)
	}

	// A self-hosted issuer may be INVITED_ONLY since ADR-0071's addendum (E2): the strictest mode
	// for every preset. What once made it dangerous - connecting an existing account on the
	// strength of an address - now asks for the account's own proof whatever the mode.
	generic := providerInput()
	generic.Provisioning = "INVITED_ONLY"
	invitedOnly, err := NewIdentityProvider(generic)
	if err != nil {
		t.Fatalf("INVITED_ONLY was refused on a self-hosted issuer: %v", err)
	}
	if invitedOnly.MayProvision() {
		t.Error("an INVITED_ONLY provider may create accounts")
	}

	unknown := providerInput()
	unknown.Provisioning = "SOMETIMES"
	if _, err := NewIdentityProvider(unknown); err == nil {
		t.Error("a mode that is not one was accepted")
	}
}

// The axis the concept fixes: **who comes in**. The difference between DOMAINS and ANY is one case
// - an address outside the configured list - and getting it wrong makes the two modes the same
// thing under two names.
func TestWhoEachModeAdmits(t *testing.T) {
	invited := providerInput()
	invited.Issuer = "https://login.microsoftonline.com/9188040d/v2.0"
	invited.Kind = "MICROSOFT"
	invited.Provisioning = "INVITED_ONLY"
	invitedOnly, err := NewIdentityProvider(invited)
	if err != nil {
		t.Fatalf("configuring INVITED_ONLY: %v", err)
	}

	domains := providerInput()
	domains.AllowedEmailDomains = []string{"example.org"}
	byDomain, err := NewIdentityProvider(domains)
	if err != nil {
		t.Fatalf("configuring DOMAINS: %v", err)
	}

	loose := providerInput()
	loose.Provisioning = "ANY"
	anybody, err := NewIdentityProvider(loose)
	if err != nil {
		t.Fatalf("configuring ANY: %v", err)
	}

	cases := []struct {
		name         string
		provider     IdentityProvider
		email        string
		verified     bool
		admitted     bool
		mayProvision bool
	}{
		{name: "INVITED_ONLY admits a verified address and creates nothing",
			provider: invitedOnly, email: "ada@elsewhere.org", verified: true, admitted: true},
		{name: "INVITED_ONLY refuses an unverified one",
			provider: invitedOnly, email: "ada@elsewhere.org"},
		{name: "DOMAINS admits inside the list and may create there",
			provider: byDomain, email: "ada@example.org", verified: true, admitted: true, mayProvision: true},
		{name: "DOMAINS refuses outside the list - the case that makes it not ANY",
			provider: byDomain, email: "ada@elsewhere.org", verified: true, mayProvision: true},
		{name: "ANY admits any verified address",
			provider: anybody, email: "ada@elsewhere.org", verified: true, admitted: true, mayProvision: true},
		{name: "ANY refuses an unverified one too",
			provider: anybody, email: "ada@elsewhere.org", mayProvision: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.provider.MayAdmit(Arriving{Email: c.email, EmailVerified: c.verified, AddressAuthoritative: c.verified}); got != c.admitted {
				t.Errorf("MayAdmit = %v, want %v", got, c.admitted)
			}
			// Whoever is admitted may claim; the modes differ in who is admitted, not in what an
			// admitted person may do.
			if got := c.provider.MayClaim(Arriving{Email: c.email, EmailVerified: c.verified, AddressAuthoritative: c.verified}); got != c.admitted {
				t.Errorf("MayClaim = %v, want %v", got, c.admitted)
			}
			if got := c.provider.MayProvision(); got != c.mayProvision {
				t.Errorf("MayProvision = %v, want %v", got, c.mayProvision)
			}
		})
	}
}

// An installation's provider with no directory to bound it is offered to every workspace, so it may
// admit only the people each workspace invited (ADR-0071's addendum, E2). It replaces "never the
// installation's provider", which left a platform with its own directory no way to offer it.
func TestAnInstallationProviderWithoutADirectoryAdmitsOnlyTheInvited(t *testing.T) {
	for _, mode := range []string{"DOMAINS", "ANY"} {
		offered := providerInput()
		offered.TenantID = shared.ID("")
		offered.Provisioning = mode
		if _, err := NewIdentityProvider(offered); err == nil {
			t.Errorf("a self-hosted issuer was offered to every workspace as %s", mode)
		}
	}

	offered := providerInput()
	offered.TenantID = shared.ID("")
	configured, err := NewIdentityProvider(offered)
	if err != nil {
		t.Fatalf("a self-hosted issuer was refused as the installation's provider: %v", err)
	}
	if configured.Provisioning != ProvisionInvitedOnly {
		t.Errorf("with nothing stated it is %q, want INVITED_ONLY", configured.Provisioning)
	}

	// The same provider is fine as one workspace's own.
	if _, err := NewIdentityProvider(providerInput()); err != nil {
		t.Errorf("a workspace's own GENERIC provider was refused: %v", err)
	}

	// And a preset that does vouch for its addresses may be the installation's.
	verified := providerInput()
	verified.TenantID = shared.ID("")
	verified.Issuer = "https://accounts.google.com"
	verified.Kind = "GOOGLE"
	if _, err := NewIdentityProvider(verified); err != nil {
		t.Errorf("Google was refused as the installation's provider: %v", err)
	}
}

// A row written by a newer build, read by this one: an unknown mode admits nobody rather than
// guessing which of the three it resembles.
func TestAnUnknownModeAdmitsNobody(t *testing.T) {
	provider := IdentityProvider{Provisioning: "TOMORROWS_MODE", AllowedEmailDomains: []string{"example.org"}}
	if provider.MayAdmit(Arriving{Email: "ada@example.org", EmailVerified: true, AddressAuthoritative: true}) {
		t.Error("a mode this build does not know admitted an address")
	}
	if provider.MayProvision() {
		t.Error("a mode this build does not know provisioned an account")
	}
}

// The name on the button, and its fallback: the issuer's host, which the button's own flow sends
// the person to the moment it is pressed.
func TestTheNameOnTheButton(t *testing.T) {
	configured, err := NewIdentityProvider(providerInput())
	if err != nil {
		t.Fatalf("configuring: %v", err)
	}
	if configured.DisplayName != "login.example.org" {
		t.Errorf("the name is %q, want the issuer's host", configured.DisplayName)
	}

	named := providerInput()
	named.DisplayName = "  Staff directory  "
	configured, err = NewIdentityProvider(named)
	if err != nil {
		t.Fatalf("configuring: %v", err)
	}
	if configured.DisplayName != "Staff directory" {
		t.Errorf("the name is %q, want it trimmed", configured.DisplayName)
	}

	long := providerInput()
	long.DisplayName = string(make([]rune, MaxProviderDisplayName+1))
	if _, err := NewIdentityProvider(long); err == nil {
		t.Error("a name past the bound was accepted")
	}
}

// Every preset a row may name has to be one this build can find, or the mark it declares is a mark
// nothing draws.
func TestEveryPresetIsReachableByItsKind(t *testing.T) {
	for _, preset := range ProviderPresets() {
		found, held := PresetOf(preset.Kind)
		if !held {
			t.Errorf("%q is in the table and PresetOf cannot find it", preset.Kind)
			continue
		}
		if found.Instructions == "" {
			t.Errorf("%q has no registration instructions", preset.Kind)
		}
		if preset.Public && !preset.AddressesVerified {
			t.Errorf("%q is public and its addresses are not verified, which leaves it no legal mode",
				preset.Kind)
		}
	}
}
