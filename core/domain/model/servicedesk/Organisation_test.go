// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-SVC-11 check 1: an organisation is named by its mail domains, checked and normalised; a
// malformed, repeated or missing domain is refused, and sharing is off unless it is switched on.
func TestAnOrganisationIsNamedByItsMailDomains(t *testing.T) {
	cases := []struct {
		name    string
		domains []string
		want    []string
		code    string
	}{
		{"normalised and sorted", []string{" Example.ORG ", "@corp.example.com"}, []string{"corp.example.com", "example.org"}, ""},
		{"none", nil, nil, "organisations.domains_required"},
		{"a single label", []string{"localhost"}, nil, "organisations.domain_malformed"},
		{"a wildcard", []string{"*.example.org"}, nil, "organisations.domain_malformed"},
		{"an address", []string{"anna@example.org"}, nil, "organisations.domain_malformed"},
		{"a hyphen at the edge", []string{"-example.org"}, nil, "organisations.domain_malformed"},
		{"repeated", []string{"example.org", "EXAMPLE.org"}, nil, "organisations.domain_repeated"},
		{"too many", strings.Split(strings.Repeat("a.org,", MaxOrganisationDomains+1), ",")[:MaxOrganisationDomains+1], nil, "organisations.domains_too_many"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NewOrganisation(NewOrganisationInput{
				ID: shared.MustParseID("0192f000-0000-7000-8000-000000000001"), TenantID: shared.MustParseID("0192f000-0000-7000-8000-000000000002"),
				Key: "acme", Name: "Acme", Domains: c.domains, Now: time.Unix(0, 0),
			})
			if c.code != "" {
				var refusal *shared.Error
				if !errors.As(err, &refusal) || refusal.DetailCode != c.code {
					t.Fatalf("error %v, want %s", err, c.code)
				}
				return
			}
			if err != nil || !slices.Equal(got.Domains, c.want) || got.SharesRequests {
				t.Fatalf("got %+v, %v", got, err)
			}
		})
	}
}

// UC-SVC-11 check 2: a person belongs by the domain of their address, exactly - a subdomain or a
// look-alike is another domain.
func TestAPersonBelongsByTheDomainOfTheirAddress(t *testing.T) {
	o := Organisation{Domains: []string{"example.org"}}
	for address, want := range map[string]bool{
		"anna@example.org": true, "Anna@EXAMPLE.org": true, "anna@mail.example.org": false,
		"anna@example.org.evil": false, "anna": false, "": false,
	} {
		if got := o.Holds(address); got != want {
			t.Errorf("Holds(%q) = %v, want %v", address, got, want)
		}
	}
}
