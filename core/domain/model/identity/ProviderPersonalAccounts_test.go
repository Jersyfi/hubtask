// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A provider's personal accounts are admitted only by invitation (UC-ID-09 check 1, UC-ID-11
// check 2): no admission mode gives a personal Microsoft account an account on arrival. Microsoft's
// personal accounts all come from one documented directory, and three kinds of issuer serve them:
// `/consumers`, `/common`, and that directory's own.

const personalMicrosoft = "9188040d-6c67-4c5b-b112-36a304b66dad"

func TestAConfigurationThatWouldAdmitPersonalMicrosoftAccountsNewIsRefused(t *testing.T) {
	const company = "72f988bf-86f1-41af-91ab-2d7cd011db47"
	const login = "https://login.microsoftonline.com/"
	cases := []struct {
		name         string
		installation bool
		issuer       string
		mode         Provisioning
		directories  []string
		refusal      string
	}{
		{name: "DOMAINS naming the personal directory",
			issuer: login + "common/v2.0", mode: ProvisionDomains, directories: []string{company, personalMicrosoft},
			refusal: "identity_provider.directory_personal"},
		{name: "DOMAINS naming it in capitals",
			issuer: login + "common/v2.0", mode: ProvisionDomains, directories: []string{"9188040D-6C67-4C5B-B112-36A304B66DAD"},
			refusal: "identity_provider.directory_personal"},
		{name: "DOMAINS naming it on the installation's row", installation: true,
			issuer: login + "common/v2.0", mode: ProvisionDomains, directories: []string{personalMicrosoft},
			refusal: "identity_provider.directory_personal"},
		{name: "ANY naming it",
			issuer: login + "organizations/v2.0", mode: ProvisionAny, directories: []string{personalMicrosoft},
			refusal: "identity_provider.directory_personal"},
		{name: "ANY on the personal-account endpoint",
			issuer: login + "consumers/v2.0", mode: ProvisionAny,
			refusal: "identity_provider.provisioning_personal"},
		{name: "ANY on the endpoint for everybody",
			issuer: login + "common/v2.0", mode: ProvisionAny, directories: []string{company},
			refusal: "identity_provider.provisioning_personal"},
		{name: "ANY on the personal directory's own issuer",
			issuer: login + personalMicrosoft + "/v2.0", mode: ProvisionAny,
			refusal: "identity_provider.provisioning_personal"},
		{name: "ANY on the legacy token service of the personal directory",
			issuer: "https://sts.windows.net/" + personalMicrosoft + "/", mode: ProvisionAny,
			refusal: "identity_provider.provisioning_personal"},
		{name: "ANY on the personal-account endpoint, offered to every workspace", installation: true,
			issuer: login + "consumers/v2.0", mode: ProvisionAny,
			refusal: "identity_provider.provisioning_personal"},

		{name: "INVITED_ONLY naming the personal directory: a list only narrows",
			issuer: login + "common/v2.0", mode: ProvisionInvitedOnly, directories: []string{personalMicrosoft}},
		{name: "INVITED_ONLY on the personal-account endpoint",
			issuer: login + "consumers/v2.0", mode: ProvisionInvitedOnly},
		{name: "DOMAINS on the endpoint for everybody, naming companies",
			issuer: login + "common/v2.0", mode: ProvisionDomains, directories: []string{company}},
		{name: "DOMAINS on the personal-account endpoint, naming a company",
			issuer: login + "consumers/v2.0", mode: ProvisionDomains, directories: []string{company}},
		{name: "ANY on one company's own issuer",
			issuer: login + company + "/v2.0", mode: ProvisionAny},
		{name: "ANY on the endpoint for work accounts",
			issuer: login + "organizations/v2.0", mode: ProvisionAny, directories: []string{company}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := providerInput()
			if tc.installation {
				in.TenantID = shared.ID("")
			}
			in.Issuer = tc.issuer
			in.Kind = string(KindMicrosoft)
			in.Provisioning = string(tc.mode)
			in.AllowedDirectories = tc.directories
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

// A row stored before configuration refused it is read as the repository builds it - no
// validation - and admits nobody new: a personal account is read under INVITED_ONLY, with the
// row's list narrowing it, whatever the row says. A company account is not touched.
func TestAStoredRowAdmitsNoPersonalMicrosoftAccountNew(t *testing.T) {
	const company = "72f988bf-86f1-41af-91ab-2d7cd011db47"
	stored := func(issuer string, mode Provisioning, directories ...string) IdentityProvider {
		return IdentityProvider{
			ID: providerRow, TenantID: sessionTenant, Kind: KindMicrosoft,
			Issuer: issuer, Provisioning: mode, AllowedDirectories: directories,
		}
	}
	listed := stored("https://login.microsoftonline.com/common/v2.0", ProvisionDomains, company, personalMicrosoft)
	consumers := stored("https://login.microsoftonline.com/consumers/v2.0", ProvisionAny)
	everybody := stored("https://login.microsoftonline.com/common/v2.0", ProvisionAny, company)
	from := func(directory string, authoritative bool) Arriving {
		return Arriving{
			Email: "ada@outlook.example", EmailVerified: true,
			Directory: directory, AddressAuthoritative: authoritative,
		}
	}

	cases := []struct {
		name      string
		provider  IdentityProvider
		arriving  Arriving
		admitted  bool
		provision bool
	}{
		{"DOMAINS listing it: a personal account meeting its account here", listed, from(personalMicrosoft, true), true, false},
		{"DOMAINS listing it: a personal account the provider is not authoritative for", listed, from(personalMicrosoft, false), false, false},
		{"DOMAINS listing it: a company account", listed, from(company, false), true, true},
		{"ANY on consumers: a personal account meeting its account here", consumers, from(personalMicrosoft, true), true, false},
		{"ANY on consumers: written in capitals", consumers, from("9188040D-6C67-4C5B-B112-36A304B66DAD", true), true, false},
		{"ANY on common: a personal account outside its list", everybody, from(personalMicrosoft, true), false, false},
		{"ANY on common: a company account", everybody, from(company, false), true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.provider.MayAdmit(tc.arriving); got != tc.admitted {
				t.Errorf("MayAdmit = %v, want %v", got, tc.admitted)
			}
			if got := tc.provider.MayProvision(tc.arriving); got != tc.provision {
				t.Errorf("MayProvision = %v, want %v", got, tc.provision)
			}
			// The invitation's own link, and an existing account's own proof, are untouched.
			if !tc.provider.MayAdmitInvitation(tc.arriving) {
				t.Error("an invitation's own link was refused")
			}
		})
	}
}
