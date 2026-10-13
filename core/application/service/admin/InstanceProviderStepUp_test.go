// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"testing"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// UC-ID-10 check 7, at the installation's level: configuring, changing and removing the provider
// every workspace is offered each ask the operator for a fresh proof. Without it nothing is stored,
// discovery is never called, and nothing is removed.
func TestTheInstallationsProviderChangesAskForAFreshProof(t *testing.T) {
	writer, store, relying, _ := newInstanceProviderWriter(newRegister(operatorID))
	proof := &stepUpFake{expect: "hbt_stu_fresh"}
	writer.Configure.StepUp = proof

	command := identityservice.ConfigureIdentityProviderCommand{
		Issuer: "https://accounts.google.com", ClientID: "hubtask", Kind: "GOOGLE",
		ClientSecret: secret.New("s3cr3t"),
	}
	if _, err := (ConfigureInstanceIdentityProvider{Writer: writer}).
		Execute(t.Context(), operator(), command); err == nil {
		t.Fatal("the installation's provider was configured without a fresh proof")
	}
	if len(store.rows) != 0 || len(relying.checked) != 0 {
		t.Fatal("a refused configuration reached the store or the provider")
	}

	command.StepUpToken = "hbt_stu_fresh"
	stored, err := (ConfigureInstanceIdentityProvider{Writer: writer}).Execute(t.Context(), operator(), command)
	if err != nil {
		t.Fatalf("configuring with the proof: %v", err)
	}

	change := command
	change.ID = stored.ID
	change.DisplayName = "Renamed"
	change.StepUpToken = ""
	if _, err := (ConfigureInstanceIdentityProvider{Writer: writer}).
		Execute(t.Context(), operator(), change); err == nil {
		t.Error("the installation's provider was changed without a fresh proof")
	}
	if store.rows[0].DisplayName == "Renamed" {
		t.Error("a refused change was written")
	}
	change.StepUpToken = "hbt_stu_fresh"
	if _, err := (ConfigureInstanceIdentityProvider{Writer: writer}).
		Execute(t.Context(), operator(), change); err != nil {
		t.Fatalf("changing with the proof: %v", err)
	}

	remove := RemoveInstanceIdentityProvider{Writer: writer}
	if err := remove.Execute(t.Context(), operator(), stored.ID, ""); err == nil {
		t.Error("the installation's provider was removed without a fresh proof")
	}
	if len(store.rows) != 1 {
		t.Fatalf("a refused removal left %d rows", len(store.rows))
	}
	if err := remove.Execute(t.Context(), operator(), stored.ID, "hbt_stu_fresh"); err != nil {
		t.Fatalf("removing with the proof: %v", err)
	}
	if len(store.rows) != 0 {
		t.Errorf("the proven removal left %d rows", len(store.rows))
	}
	if proof.consumed != 3 {
		t.Errorf("the proof was spent %d times, want once per proven change", proof.consumed)
	}
}
