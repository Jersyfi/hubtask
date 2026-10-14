// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// The projections here are written by hand, and a hand-written projection is where a new contract
// field goes missing without any test noticing. These pin the fields the LINK step added
// (ADR-0071's addendum): the card cannot say whose account it is asking about without them.
func TestTheLinkChallengeCarriesWhoseAccountAndWhichProvider(t *testing.T) {
	answered := mfaChallengeResponse(usecase.Output{
		"mfa_required": true, "pending_token": "hbt_pnd_x", "expires_at": time.Now().UTC(),
		"methods": []any{"LINK"}, "email": "bert@example.org", "provider_name": "Contoso",
	})
	if len(answered.Methods) != 1 || answered.Methods[0] != "LINK" {
		t.Errorf("methods = %v", answered.Methods)
	}
	if answered.Email == nil || *answered.Email != "bert@example.org" {
		t.Errorf("email = %v", answered.Email)
	}
	if answered.ProviderName == nil || *answered.ProviderName != "Contoso" {
		t.Errorf("provider_name = %v", answered.ProviderName)
	}

	// And a second factor's step says nothing it was not given.
	plain := mfaChallengeResponse(usecase.Output{
		"mfa_required": true, "pending_token": "hbt_pnd_y", "expires_at": time.Now().UTC(),
		"methods": []any{"TOTP", "RECOVERY"},
	})
	if plain.Email != nil || plain.ProviderName != nil {
		t.Errorf("a TOTP challenge carried %v / %v", plain.Email, plain.ProviderName)
	}

	// The code step a reset answers names whose account it is (UC-ID-04 check 5): the person
	// arrived from the mail's link and typed no address.
	afterReset := mfaChallengeResponse(usecase.Output{
		"mfa_required": true, "pending_token": "hbt_pnd_z", "expires_at": time.Now().UTC(),
		"methods": []any{"TOTP", "RECOVERY"}, "email": "anna@example.org",
	})
	if afterReset.Email == nil || *afterReset.Email != "anna@example.org" || afterReset.ProviderName != nil {
		t.Errorf("the reset's step answered %v / %v", afterReset.Email, afterReset.ProviderName)
	}
}

// The TERMS step names the terms, their version and who set them (UC-ID-19); terms without a
// version answer none rather than an empty one.
func TestTheTermsChallengeCarriesTheTerms(t *testing.T) {
	answered := mfaChallengeResponse(usecase.Output{
		"mfa_required": true, "pending_token": "hbt_pnd_t", "expires_at": time.Now().UTC(),
		"methods": []any{"TERMS"},
		"terms":   usecase.Output{"url": "https://host.example/terms", "version": "3", "set_by": "INSTALLATION"},
	})
	terms := answered.Terms
	if terms == nil || terms.Url != "https://host.example/terms" || terms.Version == nil ||
		*terms.Version != "3" || terms.SetBy != "INSTALLATION" {
		t.Fatalf("terms = %+v", terms)
	}
	unversioned := mfaChallengeResponse(usecase.Output{
		"mfa_required": true, "pending_token": "hbt_pnd_u", "expires_at": time.Now().UTC(),
		"methods": []any{"TERMS"},
		"terms":   usecase.Output{"url": "https://acme.example/terms", "set_by": "WORKSPACE"},
	})
	if unversioned.Terms == nil || unversioned.Terms.Version != nil {
		t.Errorf("unversioned terms = %+v", unversioned.Terms)
	}
	if plain := mfaChallengeResponse(usecase.Output{"methods": []any{"TOTP"}}); plain.Terms != nil {
		t.Errorf("a TOTP challenge carried terms %+v", plain.Terms)
	}
}

func TestAPresetAnswersTheModesOfBothLevels(t *testing.T) {
	answered := identityProviderPresetResponse(usecase.Output{
		"kind": "GENERIC", "scopes": []any{"openid"},
		"provisioning":              []any{"INVITED_ONLY", "DOMAINS", "ANY"},
		"installation_provisioning": []any{"INVITED_ONLY"},
	})
	if len(answered.Provisioning) != 3 {
		t.Errorf("provisioning = %v", answered.Provisioning)
	}
	if len(answered.InstallationProvisioning) != 1 || answered.InstallationProvisioning[0] != "INVITED_ONLY" {
		t.Errorf("installation_provisioning = %v", answered.InstallationProvisioning)
	}
}

// A form asks for the directory list wherever the server requires it: the shared issuer paths
// travel to the response, and a preset with none answers an empty list rather than null.
func TestAPresetAnswersItsSharedIssuerPaths(t *testing.T) {
	microsoft := identityProviderPresetResponse(usecase.Output{
		"kind": "MICROSOFT", "shared_issuer_paths": []any{"common", "organizations"},
	})
	if len(microsoft.SharedIssuerPaths) != 2 || microsoft.SharedIssuerPaths[1] != "organizations" {
		t.Errorf("shared_issuer_paths = %v", microsoft.SharedIssuerPaths)
	}
	personal := identityProviderPresetResponse(usecase.Output{
		"kind": "MICROSOFT", "personal_issuer_paths": []any{"common", "consumers"},
	})
	if len(personal.PersonalIssuerPaths) != 2 || personal.PersonalIssuerPaths[1] != "consumers" {
		t.Errorf("personal_issuer_paths = %v", personal.PersonalIssuerPaths)
	}
	if generic := identityProviderPresetResponse(usecase.Output{"kind": "GENERIC"}); generic.PersonalIssuerPaths == nil {
		t.Error("a preset without personal issuers answered null")
	}
	if generic := identityProviderPresetResponse(usecase.Output{"kind": "GENERIC"}); generic.SharedIssuerPaths == nil {
		t.Error("a preset without shared issuers answered null")
	}
}

func TestAPresetAnswersItsOneIssuerAndOnlyThen(t *testing.T) {
	google := identityProviderPresetResponse(usecase.Output{"kind": "GOOGLE", "issuer": "https://accounts.google.com"})
	if google.Issuer == nil || *google.Issuer != "https://accounts.google.com" {
		t.Errorf("issuer = %v", google.Issuer)
	}
	if generic := identityProviderPresetResponse(usecase.Output{"kind": "GENERIC"}); generic.Issuer != nil {
		t.Errorf("a preset without one issuer answered %q", *generic.Issuer)
	}
}

// The personal directories travel to the response, and a preset with none answers an empty list
// rather than null.
func TestAPresetAnswersItsPersonalDirectories(t *testing.T) {
	microsoft := identityProviderPresetResponse(usecase.Output{
		"kind": "MICROSOFT", "personal_directories": []any{"9188040d-6c67-4c5b-b112-36a304b66dad"},
	})
	if len(microsoft.PersonalDirectories) != 1 || microsoft.PersonalDirectories[0] != "9188040d-6c67-4c5b-b112-36a304b66dad" {
		t.Errorf("personal_directories = %v", microsoft.PersonalDirectories)
	}
	if generic := identityProviderPresetResponse(usecase.Output{"kind": "GENERIC"}); generic.PersonalDirectories == nil {
		t.Error("a preset without personal directories answered null")
	}
}
