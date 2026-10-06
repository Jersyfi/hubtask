// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// How a session was opened, and through which provider, reaches the body (UC-ID-06 check 2). The
// projection is written by hand, and a member it does not copy is a member that never arrives.
func TestASessionCarriesHowItWasOpenedAndThroughWhichProvider(t *testing.T) {
	row := usecase.Output{
		"id": "0192f000-0000-7000-8000-0000000000aa", "created_at": time.Now().UTC(),
		"signed_in_with": "OIDC", "signed_in_provider": "Contoso Entra ID", "current": false,
	}
	answer := sessionResponse(row)
	if answer.SignedInWith == nil || *answer.SignedInWith != "OIDC" {
		t.Errorf("signed_in_with = %v", answer.SignedInWith)
	}
	if answer.SignedInProvider == nil || *answer.SignedInProvider != "Contoso Entra ID" {
		t.Errorf("signed_in_provider = %v", answer.SignedInProvider)
	}

	delete(row, "signed_in_provider")
	if bare := sessionResponse(row); bare.SignedInProvider != nil {
		t.Errorf("a session without a provider names %q", *bare.SignedInProvider)
	}
}
