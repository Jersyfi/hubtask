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
