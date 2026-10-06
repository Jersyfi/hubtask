// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package oidc

import (
	"encoding/json"
	"testing"
	"time"

	port "github.com/Jersyfi/hubtask/core/port/identityprovider"
)

// What "authoritative for the address" means, per claim shape (ADR-0078 §5, SC-32).
//
// It decides whether a provider may activate an invited account without the invitation's own link,
// so the table is the reading, written out: the provider's own statement in the shape its preset
// names, and nothing else. Before SC-32 a Microsoft token counted whenever its domain-ownership
// claim was present at all, and every issuer without a directory claim - every self-hosted one, and
// Google without `hd` - counted always.

// The presets' rules as the application hands them over (relyingConfig): Microsoft names a claim,
// Google its own domains and a directory that is a domain, a generic issuer nothing.
var (
	microsoftRule = port.Authority{Claim: "xms_edov"}
	googleRule    = port.Authority{OwnDomains: []string{"gmail.com", "googlemail.com"}, DirectoryIsDomain: true}
	genericRule   = port.Authority{}
)

func TestTheAuthorityTablePerClaimShape(t *testing.T) {
	cases := []struct {
		name      string
		rule      port.Authority
		email     string
		verified  bool
		directory string
		asserted  string // the raw JSON of the rule's claim, empty for absent
		want      bool
	}{
		// Microsoft: the domain-ownership claim, exactly `true`.
		{"microsoft, xms_edov true", microsoftRule, "ada@contoso.com", true, "tid-1", `true`, true},
		{"microsoft, xms_edov false", microsoftRule, "ada@contoso.com", true, "tid-1", `false`, false},
		{"microsoft, xms_edov absent", microsoftRule, "ada@contoso.com", true, "tid-1", ``, false},
		{"microsoft, xms_edov as a string", microsoftRule, "ada@contoso.com", true, "tid-1", `"true"`, false},
		{"microsoft, xms_edov null", microsoftRule, "ada@contoso.com", true, "tid-1", `null`, false},
		{"microsoft, the email claim alone", microsoftRule, "ada@contoso.com", true, "", ``, false},
		{"microsoft, a directory equal to the domain is no statement", microsoftRule, "ada@contoso.com", true, "contoso.com", ``, false},
		{"microsoft, unverified", microsoftRule, "ada@contoso.com", false, "tid-1", `true`, false},

		// Google: its own consumer domains, or a hosted domain equal to the address's.
		{"google, gmail.com", googleRule, "ada@gmail.com", true, "", ``, true},
		{"google, googlemail.com", googleRule, "ada@googlemail.com", true, "", ``, true},
		{"google, gmail.com in capitals", googleRule, "Ada@GMail.com", true, "", ``, true},
		{"google, hd equal to the domain", googleRule, "ada@acme.example", true, "acme.example", ``, true},
		{"google, hd equal in another case", googleRule, "ada@Acme.Example", true, "acme.example", ``, true},
		{"google, hd of another domain", googleRule, "ada@acme.example", true, "other.example", ``, false},
		{"google, no hd and a foreign domain", googleRule, "ada@acme.example", true, "", ``, false},
		{"google, a subdomain of hd", googleRule, "ada@mail.acme.example", true, "acme.example", ``, false},
		{"google, gmail.com unverified", googleRule, "ada@gmail.com", false, "", ``, false},
		{"google, a lookalike of gmail.com", googleRule, "ada@gmail.com.evil.example", true, "", ``, false},

		// Any other issuer: never, whatever it sends.
		{"generic, verified", genericRule, "ada@example.org", true, "", ``, false},
		{"generic, sending xms_edov true", genericRule, "ada@example.org", true, "", `true`, false},
		{"generic, a directory equal to the domain", genericRule, "ada@example.org", true, "example.org", ``, false},

		// No address to be authoritative for.
		{"no address", microsoftRule, "", true, "tid-1", `true`, false},
		{"no domain", googleRule, "ada@", true, "", ``, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var asserted json.RawMessage
			if c.asserted != "" {
				asserted = json.RawMessage(c.asserted)
			}
			if got := authoritative(c.email, c.verified, c.directory, asserted, c.rule); got != c.want {
				t.Errorf("authoritative = %v, want %v", got, c.want)
			}
		})
	}
}

// Through a verified token: a self-hosted issuer that mints Microsoft's claim is still not
// authoritative. The rule comes from the preset, which is bound to the issuer's host, never from
// what the token chose to say about itself.
func TestAnIssuerWithoutARuleIsNotAuthoritativeWhateverItClaims(t *testing.T) {
	now := time.Now()
	idp := newFakeIDP(t)
	idp.extraClaims = map[string]any{"xms_edov": true, "hd": "example.org"}
	idp.token = idp.wellFormed(now)

	identity, err := relyingParty(idp, now).Exchange(t.Context(), configFor(idp), exchange())
	if err != nil {
		t.Fatalf("exchanging: %v", err)
	}
	if identity.AddressAuthoritative {
		t.Error("a generic issuer was read as authoritative for the address")
	}
}

// And through a verified token the other way: Google's hosted domain equal to the address's.
func TestAHostedDomainEqualToTheAddressIsAuthoritative(t *testing.T) {
	now := time.Now()
	idp := newFakeIDP(t)
	idp.extraClaims = map[string]any{"hd": "example.org"}
	idp.token = idp.wellFormed(now)

	cfg := configFor(idp)
	cfg.DirectoryClaim = "hd"
	cfg.Authority = googleRule
	identity, err := relyingParty(idp, now).Exchange(t.Context(), cfg, exchange())
	if err != nil {
		t.Fatalf("exchanging: %v", err)
	}
	if !identity.AddressAuthoritative {
		t.Error("a hosted domain equal to the address's was not read as authority")
	}
}
