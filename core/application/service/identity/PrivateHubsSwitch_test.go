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

// readWorkspace answers the workspace as the catalogue does.
func readWorkspace(t *testing.T, registry *usecase.Registry) usecase.Output {
	t.Helper()
	out, err := registry.Invoke(t.Context(), ReadWorkspaceName, workspaceActor(), usecase.Input{})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	return out
}

func privateHubsRow(t *testing.T, out usecase.Output) usecase.Output {
	t.Helper()
	policy, _ := out["sign_in_policy"].(usecase.Output)
	row, _ := policy["private_hubs"].(usecase.Output)
	if row == nil {
		t.Fatal("the workspace's rule answers no private_hubs")
	}
	return row
}

// UC-ID-16 check 7: *Private hubs allowed* is a switch of the rule - on by default, switched off by
// the workspace with the proof every switch asks, its source and lock answered - and a client is
// offered private hubs only while it is on and somebody else is here (check 8).
func TestPrivateHubsIsASwitchOfTheRule(t *testing.T) {
	fixture := newWorkspacePolicyFixture(now)
	fixture.writer.People = people(2)
	registry := workspaceRegistry(t, fixture)

	out := readWorkspace(t, registry)
	if row := privateHubsRow(t, out); row["value"] != true || row["source"] != string(domain.SourceDefault) {
		t.Errorf("the default reads %v from %v, want true from DEFAULT", row["value"], row["source"])
	}
	if out["private_hubs_offered"] != true || out["several_people"] != true {
		t.Errorf("offered %v, several %v; want both", out["private_hubs_offered"], out["several_people"])
	}

	off := usecase.Input{"sign_in_policy": map[string]any{"private_hubs": false}}
	if _, err := registry.Invoke(t.Context(), UpdateWorkspaceName, workspaceActor(), off); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("switching it off without a step-up answered %v", err)
	}
	off["step_up_token"] = "hbt_stp_x"
	if _, err := registry.Invoke(t.Context(), UpdateWorkspaceName, workspaceActor(), off); err != nil {
		t.Fatalf("switching it off: %v", err)
	}
	out = readWorkspace(t, registry)
	if row := privateHubsRow(t, out); row["value"] != false || row["source"] != string(domain.SourceWorkspace) {
		t.Errorf("after switching off it reads %v from %v", row["value"], row["source"])
	}
	// Off: nothing is offered, and the setting's place stays findable.
	if out["private_hubs_offered"] != false || out["several_people"] != true {
		t.Errorf("offered %v, several %v; want not offered among several", out["private_hubs_offered"], out["several_people"])
	}
}

// The installation's lock holds, and says who set it.
func TestPrivateHubsFollowTheInstallation(t *testing.T) {
	fixture := newWorkspacePolicyFixture(now)
	registry := workspaceRegistry(t, fixture)
	fixture.instance.level.Policy.Patch = domain.PolicyPatch{PrivateHubs: boolOf(false)}
	fixture.instance.level.Policy.Locks = map[domain.PolicySwitch]bool{domain.SwitchPrivateHubs: true}

	row := privateHubsRow(t, readWorkspace(t, registry))
	if row["value"] != false || row["installation"] != false || row["lock"] != string(domain.LockInstance) {
		t.Errorf("the locked switch reads %v", row)
	}
	on := usecase.Input{"sign_in_policy": map[string]any{"private_hubs": true}, "step_up_token": "hbt_stp_x"}
	_, err := registry.Invoke(t.Context(), UpdateWorkspaceName, workspaceActor(), on)
	if err == nil || !strings.Contains(err.Error(), "auth.policy_locked") {
		t.Errorf("writing a locked switch answered %v", err)
	}
	if len(fixture.store.updates) != 0 {
		t.Errorf("%d writes from a refused change", len(fixture.store.updates))
	}
}
