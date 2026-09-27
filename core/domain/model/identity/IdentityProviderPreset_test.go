// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import "testing"

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

// The two rules the preset puts on the mode, each of them a hole somebody would otherwise
// configure by accident.
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

	// The other way round: INVITED_ONLY gives an account that already exists to whoever arrives
	// with its address, so an issuer this installation cannot vouch for may not use it.
	generic := providerInput()
	generic.Provisioning = "INVITED_ONLY"
	if _, err := NewIdentityProvider(generic); err == nil {
		t.Error("INVITED_ONLY was accepted on a provider whose addresses nobody vouches for")
	}

	unknown := providerInput()
	unknown.Provisioning = "SOMETIMES"
	if _, err := NewIdentityProvider(unknown); err == nil {
		t.Error("a mode that is not one was accepted")
	}
}

// The ladder, on the one axis that has a security answer: how freely an arriving subject may claim
// an account that already exists here.
func TestTheProvisioningLadder(t *testing.T) {
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
		mayLink      bool
		mayProvision bool
	}{
		{name: "INVITED_ONLY claims outside the list and creates nothing",
			provider: invitedOnly, email: "ada@elsewhere.org", verified: true, mayLink: true},
		{name: "INVITED_ONLY still refuses an unverified address",
			provider: invitedOnly, email: "ada@elsewhere.org"},
		{name: "DOMAINS claims inside the list and creates outside it",
			provider: byDomain, email: "ada@example.org", verified: true, mayLink: true, mayProvision: true},
		{name: "DOMAINS claims nothing outside the list",
			provider: byDomain, email: "ada@elsewhere.org", verified: true, mayProvision: true},
		{name: "ANY claims on any verified address",
			provider: anybody, email: "ada@elsewhere.org", verified: true, mayLink: true, mayProvision: true},
		{name: "ANY refuses an unverified one too",
			provider: anybody, email: "ada@elsewhere.org", mayProvision: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.provider.MayLink(c.email, c.verified); got != c.mayLink {
				t.Errorf("MayLink = %v, want %v", got, c.mayLink)
			}
			if got := c.provider.MayProvision(); got != c.mayProvision {
				t.Errorf("MayProvision = %v, want %v", got, c.mayProvision)
			}
		})
	}
}

// A row written by a newer build, read by this one: an unknown mode links nothing rather than
// guessing which of the three it resembles.
func TestAnUnknownModeLinksNothing(t *testing.T) {
	provider := IdentityProvider{Provisioning: "TOMORROWS_MODE", AllowedEmailDomains: []string{"example.org"}}
	if provider.MayLink("ada@example.org", true) {
		t.Error("a mode this build does not know linked an address")
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
