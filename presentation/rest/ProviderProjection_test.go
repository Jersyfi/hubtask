// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The provider's projection is written by hand, so the field the installation's lock adds is pinned
// here: without it the workspace's screen would draw a mandated provider as a switch it may turn off.
func TestTheProviderProjectionCarriesTheInstallationLock(t *testing.T) {
	row := usecase.Output{
		"id": "01936f2a-7c1e-7000-8000-0000000000e5", "issuer": "https://login.platform.example",
		"display_name": "The platform", "kind": "GENERIC", "provisioning": "INVITED_ONLY",
		"scope": "installation", "enabled": true, "offered_here": true, "created_at": time.Now().UTC(),
		"installation_lock": "TURNS_ON",
	}
	answered := identityProviderResponse(row)
	if answered.InstallationLock == nil || string(*answered.InstallationLock) != "TURNS_ON" {
		t.Errorf("the projection dropped installation_lock: %v", answered.InstallationLock)
	}

	delete(row, "installation_lock")
	if answered := identityProviderResponse(row); answered.InstallationLock != nil {
		t.Errorf("a workspace's own row carries installation_lock %v", *answered.InstallationLock)
	}
}

// The refusal of switching a mandated provider off answers 422, as a locked rule does.
func TestTheMandatedRefusalIsAValidationProblem(t *testing.T) {
	err := shared.ErrValidation.WithDetail("identity_provider.mandated").
		WithParams(map[string]string{"origin": "INSTANCE"})
	if status := statusByCategory[shared.AsError(err).Category]; status != http.StatusUnprocessableEntity {
		t.Errorf("identity_provider.mandated answers %d, want 422", status)
	}
}
