// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A shared Microsoft endpoint - `/organizations`, `/common` - stands for every directory at the
// provider, so the directory list is what bounds it in every admission mode (identity.md §10.3):
// configuration refuses one without a list, and admission reads the list - `ANY` included, which
// would otherwise admit every work account in the world.

const (
	listedCompany = "72f988bf-86f1-41af-91ab-2d7cd011db47"
	otherCompany  = "0f1e2d3c-4b5a-4968-8776-655443322110"
	microsoftHost = "https://login.microsoftonline.com/"
)

func TestASharedMicrosoftEndpointIsNotSavedWithoutItsDirectories(t *testing.T) {
	cases := []struct {
		name         string
		installation bool
		endpoint     string
		mode         Provisioning
		refusal      string
	}{
		{name: "organizations, ANY", endpoint: "organizations", mode: ProvisionAny,
			refusal: "identity_provider.directories_required"},
		{name: "organizations, DOMAINS", endpoint: "organizations", mode: ProvisionDomains,
			refusal: "identity_provider.directories_required"},
		{name: "organizations, INVITED_ONLY", endpoint: "organizations", mode: ProvisionInvitedOnly,
			refusal: "identity_provider.directories_required"},
		{name: "organizations, ANY, offered to every workspace", installation: true,
			endpoint: "organizations", mode: ProvisionAny, refusal: "identity_provider.directories_required"},
		{name: "organizations, INVITED_ONLY, offered to every workspace", installation: true,
			endpoint: "organizations", mode: ProvisionInvitedOnly, refusal: "identity_provider.directories_required"},
		// ANY on /common serves personal accounts, which is refused before the list is asked for.
		{name: "common, ANY", endpoint: "common", mode: ProvisionAny,
			refusal: "identity_provider.provisioning_personal"},
		{name: "common, DOMAINS", endpoint: "common", mode: ProvisionDomains,
			refusal: "identity_provider.directories_required"},
		{name: "common, INVITED_ONLY", endpoint: "common", mode: ProvisionInvitedOnly,
			refusal: "identity_provider.directories_required"},
		{name: "common, DOMAINS, offered to every workspace", installation: true,
			endpoint: "common", mode: ProvisionDomains, refusal: "identity_provider.directories_required"},
		{name: "one company's own issuer, ANY: the issuer is the bound", endpoint: listedCompany,
			mode: ProvisionAny},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := providerInput()
			if tc.installation {
				in.TenantID = shared.ID("")
			}
			in.Issuer = microsoftHost + tc.endpoint + "/v2.0"
			in.Kind = string(KindMicrosoft)
			in.Provisioning = string(tc.mode)
			_, err := NewIdentityProvider(in)
			switch {
			case tc.refusal == "" && err != nil:
				t.Fatalf("refused: %v", err)
			case tc.refusal != "" && err == nil:
				t.Fatalf("accepted, want %s", tc.refusal)
			case tc.refusal != "" && shared.AsError(err).DetailCode != tc.refusal:
				t.Fatalf("refused with %v, want %s", err, tc.refusal)
			}
		})
	}
}

func TestASharedMicrosoftEndpointAdmitsOnlyItsDirectoriesInEveryMode(t *testing.T) {
	// Read as the repository builds a row - no validation - so that a row stored before
	// configuration refused it is held too.
	row := func(endpoint string, mode Provisioning, directories ...string) IdentityProvider {
		return IdentityProvider{
			ID: providerRow, TenantID: sessionTenant, Kind: KindMicrosoft,
			Issuer: microsoftHost + endpoint + "/v2.0", Provisioning: mode, AllowedDirectories: directories,
		}
	}
	offered := func(endpoint string, mode Provisioning, directories ...string) IdentityProvider {
		installation := row(endpoint, mode, directories...)
		installation.TenantID = shared.ID("")
		return installation
	}
	from := func(directory string) Arriving {
		// Authoritative, so that INVITED_ONLY is decided by the list alone.
		return Arriving{
			Email: "ada@contoso.example", EmailVerified: true,
			Directory: directory, AddressAuthoritative: true,
		}
	}

	cases := []struct {
		name     string
		provider IdentityProvider
		arriving Arriving
		admitted bool
	}{
		{"organizations, ANY: a listed directory", row("organizations", ProvisionAny, listedCompany), from(listedCompany), true},
		{"organizations, ANY: a listed directory in capitals", row("organizations", ProvisionAny, listedCompany),
			from("72F988BF-86F1-41AF-91AB-2D7CD011DB47"), true},
		{"organizations, ANY: another directory", row("organizations", ProvisionAny, listedCompany), from(otherCompany), false},
		{"organizations, ANY: no directory named", row("organizations", ProvisionAny, listedCompany), from(""), false},
		{"organizations, ANY, stored without a list: nobody", row("organizations", ProvisionAny), from(otherCompany), false},
		{"organizations, ANY, offered by the installation: a listed directory", offered("organizations", ProvisionAny, listedCompany), from(listedCompany), true},
		{"organizations, ANY, offered by the installation: another directory", offered("organizations", ProvisionAny, listedCompany), from(otherCompany), false},
		{"organizations, DOMAINS: a listed directory", row("organizations", ProvisionDomains, listedCompany), from(listedCompany), true},
		{"organizations, DOMAINS: another directory", row("organizations", ProvisionDomains, listedCompany), from(otherCompany), false},
		{"organizations, INVITED_ONLY: a listed directory", row("organizations", ProvisionInvitedOnly, listedCompany), from(listedCompany), true},
		{"organizations, INVITED_ONLY: another directory", row("organizations", ProvisionInvitedOnly, listedCompany), from(otherCompany), false},
		{"common, ANY, stored before it was refused: a listed directory", row("common", ProvisionAny, listedCompany), from(listedCompany), true},
		{"common, ANY, stored before it was refused: another directory", row("common", ProvisionAny, listedCompany), from(otherCompany), false},
		{"common, DOMAINS: a listed directory", row("common", ProvisionDomains, listedCompany), from(listedCompany), true},
		{"common, DOMAINS: another directory", row("common", ProvisionDomains, listedCompany), from(otherCompany), false},
		{"common, INVITED_ONLY: a listed directory", row("common", ProvisionInvitedOnly, listedCompany), from(listedCompany), true},
		{"common, INVITED_ONLY: another directory", row("common", ProvisionInvitedOnly, listedCompany), from(otherCompany), false},
		{"one company's own issuer, ANY: its directory, no list", row(listedCompany, ProvisionAny), from(listedCompany), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.provider.MayAdmit(tc.arriving); got != tc.admitted {
				t.Errorf("MayAdmit = %v, want %v", got, tc.admitted)
			}
			if got := tc.provider.MayClaim(tc.arriving); got != tc.admitted {
				t.Errorf("MayClaim = %v, want %v", got, tc.admitted)
			}
			// A member bringing their own proof, or an invitation's own link, is not coming in new
			// and is not bounded by the list (identity.md §10.4).
			if !tc.provider.MayAdmitWithProof(tc.arriving) || !tc.provider.MayAdmitInvitation(tc.arriving) {
				t.Error("an arrival with its own proof was refused")
			}
		})
	}
}
