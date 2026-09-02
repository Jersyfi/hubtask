// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func providerInput() NewIdentityProviderInput {
	return NewIdentityProviderInput{
		TenantID: sessionTenant,
		Issuer:   "https://login.example.org",
		ClientID: "hubtask",
		Enabled:  true,
		Now:      time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
	}
}

// The issuer is the anchor of every check that follows - the metadata, the keys, the signature -
// so what may be one is narrow on purpose.
func TestWhatMayBeAnIssuer(t *testing.T) {
	cases := []struct {
		name    string
		issuer  string
		want    string
		refused bool
	}{
		{name: "a plain https issuer", issuer: "https://login.example.org", want: "https://login.example.org"},
		{name: "a path is allowed - providers use them", issuer: "https://example.org/auth/realms/staff", want: "https://example.org/auth/realms/staff"},
		{name: "the trailing slash is not part of it", issuer: "https://login.example.org/", want: "https://login.example.org"},
		{name: "surrounding space is typing", issuer: "  https://login.example.org  ", want: "https://login.example.org"},
		{name: "plain http is refused - anybody on the path could answer for it", issuer: "http://login.example.org", refused: true},
		{name: "no scheme at all", issuer: "login.example.org", refused: true},
		{name: "a query string is not part of an issuer", issuer: "https://login.example.org?realm=staff", refused: true},
		{name: "nor is a fragment", issuer: "https://login.example.org#staff", refused: true},
		{name: "credentials in the URL are refused", issuer: "https://user:pass@login.example.org", refused: true},
		{name: "empty", issuer: "   ", refused: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := providerInput()
			in.Issuer = c.issuer
			provider, err := NewIdentityProvider(in)
			if c.refused {
				if err == nil {
					t.Fatalf("%q was accepted as an issuer", c.issuer)
				}
				if shared.AsError(err).Category != shared.CategoryValidation {
					t.Errorf("the refusal is %v, want a validation error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("%q was refused: %v", c.issuer, err)
			}
			if provider.Issuer != c.want {
				t.Errorf("the issuer normalised to %q, want %q", provider.Issuer, c.want)
			}
		})
	}
}

// The domain list is what decides whether an arriving address may claim an account that already
// exists, so a sloppy entry is a way in rather than a typo.
func TestWhatMayBeALinkingDomain(t *testing.T) {
	cases := []struct {
		name    string
		domains []string
		want    []string
		refused bool
	}{
		{name: "lowercased and de-duplicated", domains: []string{"Example.org", "example.org"}, want: []string{"example.org"}},
		{name: "a leading at sign is how people write it", domains: []string{"@example.org"}, want: []string{"example.org"}},
		{name: "none at all is a workspace that links nothing", domains: nil, want: []string{}},
		{name: "a wildcard is refused rather than interpreted", domains: []string{"*.example.org"}, refused: true},
		{name: "an address is not a domain", domains: []string{"someone@example.org"}, refused: true},
		{name: "a bare label is not a domain", domains: []string{"example"}, refused: true},
		{name: "a leading dot", domains: []string{".example.org"}, refused: true},
		{name: "eleven is more than the bound", domains: []string{"a.org", "b.org", "c.org", "d.org", "e.org", "f.org", "g.org", "h.org", "i.org", "j.org", "k.org"}, refused: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := providerInput()
			in.AllowedEmailDomains = c.domains
			provider, err := NewIdentityProvider(in)
			if c.refused {
				if err == nil {
					t.Fatalf("%v was accepted", c.domains)
				}
				return
			}
			if err != nil {
				t.Fatalf("%v was refused: %v", c.domains, err)
			}
			if len(provider.AllowedEmailDomains) != len(c.want) {
				t.Fatalf("normalised to %v, want %v", provider.AllowedEmailDomains, c.want)
			}
			for i, want := range c.want {
				if provider.AllowedEmailDomains[i] != want {
					t.Errorf("domain %d is %q, want %q", i, provider.AllowedEmailDomains[i], want)
				}
			}
		})
	}
}

// Linking hands somebody an account that already exists. Both conditions guard that door, and
// the subdomain case is the one that looks harmless and is not: `example.org.evil.net` ends in
// nothing this list contains, and `staff.example.org` is a different organisation's mail.
func TestWhenAnArrivingAddressMayClaimAnExistingAccount(t *testing.T) {
	in := providerInput()
	in.AllowedEmailDomains = []string{"example.org", "example.net"}
	provider, err := NewIdentityProvider(in)
	if err != nil {
		t.Fatalf("configuring: %v", err)
	}

	cases := []struct {
		name     string
		email    string
		verified bool
		want     bool
	}{
		{name: "a verified address in a configured domain", email: "ada@example.org", verified: true, want: true},
		{name: "the second domain counts too", email: "ada@example.net", verified: true, want: true},
		{name: "case is not part of a domain", email: "Ada@EXAMPLE.org", verified: true, want: true},
		{name: "unverified is somebody typing", email: "ada@example.org", verified: false},
		{name: "a subdomain is a different organisation", email: "ada@staff.example.org", verified: true},
		{name: "a suffix match is the way in this refuses", email: "ada@example.org.evil.net", verified: true},
		{name: "another domain entirely", email: "ada@elsewhere.org", verified: true},
		{name: "not an address", email: "ada", verified: true},
		{name: "an address with nothing after the at sign", email: "ada@", verified: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := provider.LinksAddress(c.email, c.verified); got != c.want {
				t.Errorf("LinksAddress(%q, %v) = %v, want %v", c.email, c.verified, got, c.want)
			}
		})
	}
}

// A workspace that configured no domains links nobody, however verified the address is.
func TestNoConfiguredDomainsLinksNothing(t *testing.T) {
	provider, err := NewIdentityProvider(providerInput())
	if err != nil {
		t.Fatalf("configuring: %v", err)
	}
	if provider.LinksAddress("ada@example.org", true) {
		t.Error("an address linked against an empty domain list")
	}
}

// A client id is not optional, and neither is knowing which workspace this belongs to.
func TestAProviderNeedsItsClientAndItsWorkspace(t *testing.T) {
	blank := providerInput()
	blank.ClientID = "  "
	if _, err := NewIdentityProvider(blank); err == nil {
		t.Error("a provider without a client id was accepted")
	}

	homeless := providerInput()
	homeless.TenantID = shared.ID("")
	if _, err := NewIdentityProvider(homeless); err == nil {
		t.Error("a provider without a workspace was accepted")
	}
}
