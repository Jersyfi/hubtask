// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// managedSwitch reads the switch as the workspace's settings answer it.
func managedSwitch(t *testing.T, registry *usecase.Registry) usecase.Output {
	t.Helper()
	out, err := registry.Invoke(t.Context(), ReadWorkspaceName, workspaceActor(), usecase.Input{})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	policy, _ := out["sign_in_policy"].(usecase.Output)
	row, _ := policy["managed_accounts"].(usecase.Output)
	if row == nil {
		t.Fatal("the workspace's rule answers no managed_accounts")
	}
	return row
}

// UC-ID-20 check 8: *Managed accounts allowed* is a switch of the sign-in rule - on by default, the
// workspace may switch it off with the proof every switch asks, and the answer says where its value
// came from and whether a level above locked it.
func TestManagedAccountsIsASwitchOfTheRule(t *testing.T) {
	fixture := newWorkspacePolicyFixture(now)
	registry := workspaceRegistry(t, fixture)

	row := managedSwitch(t, registry)
	if row["value"] != true || row["source"] != string(domain.SourceDefault) {
		t.Errorf("the default reads %v from %v, want true from DEFAULT", row["value"], row["source"])
	}

	off := usecase.Input{"sign_in_policy": map[string]any{"managed_accounts": false}}
	if _, err := registry.Invoke(t.Context(), UpdateWorkspaceName, workspaceActor(), off); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("switching it off without a step-up answered %v", err)
	}
	off["step_up_token"] = "hbt_stp_x"
	if _, err := registry.Invoke(t.Context(), UpdateWorkspaceName, workspaceActor(), off); err != nil {
		t.Fatalf("switching it off: %v", err)
	}
	if row := managedSwitch(t, registry); row["value"] != false || row["source"] != string(domain.SourceWorkspace) {
		t.Errorf("after switching off it reads %v from %v", row["value"], row["source"])
	}

	// Back on is allowed while the level above says on: a workspace tightens against the level
	// above, not against its own last word.
	on := usecase.Input{"sign_in_policy": map[string]any{"managed_accounts": true}, "step_up_token": "hbt_stp_x"}
	if _, err := registry.Invoke(t.Context(), UpdateWorkspaceName, workspaceActor(), on); err != nil {
		t.Errorf("switching it back on: %v", err)
	}
}

// The installation's "off" and its lock hold: a workspace cannot switch on what the level above
// switched off, and a locked switch says who locked it.
func TestManagedAccountsFollowTheInstallation(t *testing.T) {
	fixture := newWorkspacePolicyFixture(now)
	registry := workspaceRegistry(t, fixture)
	fixture.instance.level.Policy.Patch = domain.PolicyPatch{ManagedAccounts: boolOf(false)}

	on := usecase.Input{"sign_in_policy": map[string]any{"managed_accounts": true}, "step_up_token": "hbt_stp_x"}
	_, err := registry.Invoke(t.Context(), UpdateWorkspaceName, workspaceActor(), on)
	if err == nil || !strings.Contains(err.Error(), "auth.policy_loosens") {
		t.Errorf("switching on under the installation's off answered %v", err)
	}

	fixture.instance.level.Policy.Locks = map[domain.PolicySwitch]bool{domain.SwitchManagedAccounts: true}
	row := managedSwitch(t, registry)
	if row["value"] != false || row["installation"] != false || row["lock"] != string(domain.LockInstance) {
		t.Errorf("the locked switch reads %v", row)
	}
	// A family plan's "on", locked: the workspace cannot switch it off either.
	fixture.instance.level.Policy.Patch = domain.PolicyPatch{ManagedAccounts: boolOf(true)}
	off := usecase.Input{"sign_in_policy": map[string]any{"managed_accounts": false}, "step_up_token": "hbt_stp_x"}
	_, err = registry.Invoke(t.Context(), UpdateWorkspaceName, workspaceActor(), off)
	if err == nil || !strings.Contains(err.Error(), "auth.policy_locked") {
		t.Errorf("writing a locked switch answered %v", err)
	}
	if len(fixture.store.updates) != 0 {
		t.Errorf("%d writes from refused changes", len(fixture.store.updates))
	}
}
