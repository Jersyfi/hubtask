// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import "testing"

// Admission, decided on the directory rather than on the text of an address (ADR-0071).
//
// The case that matters most has no assertion about lists in it at all: **a Microsoft account could
// not sign in**, in any mode, on any configuration. Entra ID issues no `email_verified`, the
// adapter answered false, and the first gate refused everybody. The preset said
// `AddressesVerified: true` the whole time, which is a statement about the issuer that nothing
// consulted.

func microsoftProvider(t *testing.T, mode string, directories, domains []string) IdentityProvider {
	t.Helper()
	in := providerInput()
	in.Issuer = "https://login.microsoftonline.com/9188040d-6c67-4c5b-b112-36a304b66dad/v2.0"
	in.Kind = string(KindMicrosoft)
	in.Provisioning = mode
	in.AllowedDirectories = directories
	in.AllowedEmailDomains = domains
	configured, err := NewIdentityProvider(in)
	if err != nil {
		t.Fatalf("configuring: %v", err)
	}
	return configured
}

// The defect, as a test. `xms_edov` is what Microsoft answers the question with, and a token that
// carries it is admitted exactly as a Google one with `email_verified` is.
func TestAMicrosoftAccountIsAdmittedOnWhatMicrosoftActuallySends(t *testing.T) {
	const acmeDirectory = "72f988bf-86f1-41af-91ab-2d7cd011db47"
	configured := microsoftProvider(t, string(ProvisionDomains), []string{acmeDirectory}, nil)

	admitted := Arriving{
		Email: "ada@acme.example", EmailVerified: true,
		Directory: acmeDirectory, AddressAuthoritative: true,
	}
	if !configured.MayAdmit(admitted) {
		t.Error("a verified account from a named directory was refused")
	}

	// And the shape the adapter produced before ADR-0071: no `email_verified`, so nothing verified,
	// so nobody in. The whole provider was unusable and no test said so.
	unverified := admitted
	unverified.EmailVerified = false
	unverified.AddressAuthoritative = false
	if configured.MayAdmit(unverified) {
		t.Error("an unverified address was admitted")
	}
}

// The directory is the gate, and the address's domain is not consulted at all. Both halves are
// asserted, because "prefers the directory" and "reads only the directory" are different products.
func TestUnderDomainsTheDirectoryDecidesAndTheAddressDoesNot(t *testing.T) {
	const ours = "72f988bf-86f1-41af-91ab-2d7cd011db47"
	const theirs = "11111111-2222-3333-4444-555555555555"
	// The domains list says acme.example and is deliberately wrong about everything below.
	configured := microsoftProvider(t, string(ProvisionDomains), []string{ours}, []string{"acme.example"})

	// Our directory, an address at a domain the list never heard of: admitted.
	if !configured.MayAdmit(Arriving{
		Email: "ada@somewhere-else.example", EmailVerified: true,
		Directory: ours, AddressAuthoritative: true,
	}) {
		t.Error("the address's domain was allowed to refuse somebody from a named directory")
	}

	// Another directory, an address in the domains list: refused. This is the case the whole
	// decision is about — anybody at any provider can verify an address at a domain they do not
	// own, and before this the list said yes to them.
	if configured.MayAdmit(Arriving{
		Email: "stranger@acme.example", EmailVerified: true,
		Directory: theirs, AddressAuthoritative: true,
	}) {
		t.Error("a stranger's directory was admitted because the address looked right")
	}

	// No directory at all — a personal account at a provider that has organisations.
	if configured.MayAdmit(Arriving{
		Email: "ada@acme.example", EmailVerified: true, AddressAuthoritative: false,
	}) {
		t.Error("an account belonging to no directory was admitted")
	}
}

// A row migrated from before ADR-0071 has domains and no directories, and admits nobody. Failing
// closed is the only direction that does not hand out accounts, and it is a migration note rather
// than an accident.
func TestAPresetWithADirectoryClaimAndNoDirectoriesAdmitsNobody(t *testing.T) {
	configured := microsoftProvider(t, string(ProvisionDomains), nil, []string{"acme.example"})
	if configured.MayAdmit(Arriving{
		Email: "ada@acme.example", EmailVerified: true,
		Directory: "72f988bf-86f1-41af-91ab-2d7cd011db47", AddressAuthoritative: true,
	}) {
		t.Error("an empty directory list admitted somebody")
	}
}

// GENERIC has no directory claim and never will, so the domains list keeps its job there. That is
// the one place it still decides anything.
func TestAGenericProviderStillAdmitsOnTheAddressDomain(t *testing.T) {
	in := providerInput()
	in.Provisioning = string(ProvisionDomains)
	in.AllowedEmailDomains = []string{"example.org"}
	configured, err := NewIdentityProvider(in)
	if err != nil {
		t.Fatalf("configuring: %v", err)
	}
	if !configured.MayAdmit(Arriving{
		Email: "ada@example.org", EmailVerified: true, AddressAuthoritative: true,
	}) {
		t.Error("a generic provider refused an address inside its list")
	}
	if configured.MayAdmit(Arriving{
		Email: "ada@elsewhere.org", EmailVerified: true, AddressAuthoritative: true,
	}) {
		t.Error("a generic provider admitted an address outside its list")
	}
}

// `INVITED_ONLY` hands over an account that already exists, so it asks the harder question: does
// the provider own the domain of the address it is presenting. A personal account with a verified
// address at somebody else's domain is exactly what that refuses.
func TestInvitedOnlyNeedsAnAuthoritativeAddress(t *testing.T) {
	configured := microsoftProvider(t, string(ProvisionInvitedOnly), nil, nil)

	if !configured.MayAdmit(Arriving{
		Email: "ada@acme.example", EmailVerified: true, AddressAuthoritative: true,
	}) {
		t.Error("an authoritative address was refused")
	}
	if configured.MayAdmit(Arriving{
		Email: "ada@acme.example", EmailVerified: true, AddressAuthoritative: false,
	}) {
		t.Error("an account that merely verified somebody else's domain claimed one here")
	}
}

// A directory list on a preset that cannot read one is refused rather than stored and ignored: a
// list nobody consults is worse than no list, because somebody wrote it believing it bounded
// something.
func TestAGenericProviderRefusesADirectoryList(t *testing.T) {
	in := providerInput()
	in.AllowedDirectories = []string{"acme.example"}
	if _, err := NewIdentityProvider(in); err == nil {
		t.Fatal("a directory list was accepted for a preset with no directory claim")
	}
}

// The multi-directory endpoint is configurable now, and bounded: without a list it is every
// organisation in the world, which is a decision rather than a default.
func TestAMultiDirectoryIssuerNeedsItsDirectoriesNamed(t *testing.T) {
	in := providerInput()
	in.Issuer = "https://login.microsoftonline.com/common/v2.0"
	in.Kind = string(KindMicrosoft)
	in.Provisioning = string(ProvisionDomains)

	if _, err := NewIdentityProvider(in); err == nil {
		t.Fatal("the shared endpoint was configured with nothing bounding it")
	}

	in.AllowedDirectories = []string{"72f988bf-86f1-41af-91ab-2d7cd011db47"}
	configured, err := NewIdentityProvider(in)
	if err != nil {
		t.Fatalf("configuring the shared endpoint with a directory list: %v", err)
	}
	if configured.Issuer != "https://login.microsoftonline.com/common/v2.0" {
		t.Errorf("the issuer was rewritten to %q", configured.Issuer)
	}
}

// And the personal-account endpoint needs no list, because it *is* one directory: that is the
// configuration for "a private person signs in with their own Microsoft account".
func TestThePersonalAccountEndpointIsOneDirectoryAndNeedsNoList(t *testing.T) {
	in := providerInput()
	in.Issuer = "https://login.microsoftonline.com/consumers/v2.0"
	in.Kind = string(KindMicrosoft)
	in.Provisioning = string(ProvisionInvitedOnly)
	if _, err := NewIdentityProvider(in); err != nil {
		t.Fatalf("configuring the personal-account endpoint: %v", err)
	}
}

// The invitation's own link admits one more arrival, and only under INVITED_ONLY: a verified
// address the provider is not authoritative for (ADR-0078 §1). It never admits an unverified one,
// and under DOMAINS the list stays the gate.
func TestTheInvitationLinkAdmitsOnlyWhatInvitedOnlyNeeds(t *testing.T) {
	verified := Arriving{Email: "ada@example.org", EmailVerified: true}
	unverified := Arriving{Email: "ada@example.org"}

	in := providerInput()
	in.Provisioning = string(ProvisionInvitedOnly)
	invitedOnly, err := NewIdentityProvider(in)
	if err != nil {
		t.Fatalf("configuring: %v", err)
	}
	if invitedOnly.MayAdmit(verified) {
		t.Fatal("a non-authoritative address was admitted under INVITED_ONLY without the link")
	}
	if !invitedOnly.MayAdmitInvited(verified) {
		t.Error("the invitation's link did not admit a verified address under INVITED_ONLY")
	}
	if invitedOnly.MayAdmitInvited(unverified) {
		t.Error("the invitation's link admitted an address the provider did not verify")
	}

	in = providerInput()
	in.Provisioning = string(ProvisionDomains)
	in.AllowedEmailDomains = []string{"elsewhere.org"}
	domains, err := NewIdentityProvider(in)
	if err != nil {
		t.Fatalf("configuring: %v", err)
	}
	if domains.MayAdmitInvited(verified) {
		t.Error("the invitation's link widened a DOMAINS list")
	}
}
