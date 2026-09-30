// SPDX-License-Identifier: BUSL-1.1
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

// UC-ID-12 checks 1 and 2 (SC-06): `require_admin_totp` and `mfa_required_for` were two stored
// values, one of them writable without a step-up, a lock or the "no loosening" check - and they
// disagreed in both directions. The old field is now read from the rule in force, and a write to it
// is the same change to the rule, through every check the rule has.

// workspaceRegistry is the two workspace doors as a client reaches them.
func workspaceRegistry(t *testing.T, fixture *workspacePolicyFixture) *usecase.Registry {
	t.Helper()
	registry, err := usecase.NewRegistry(nil,
		ReadWorkspace{Writer: fixture.writer}.Descriptor(),
		UpdateWorkspace{Writer: fixture.writer}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	return registry
}

// bothFields answers the old boolean and the rule's value as a client reads them.
func bothFields(t *testing.T, registry *usecase.Registry) (bool, string) {
	t.Helper()
	out, err := registry.Invoke(t.Context(), ReadWorkspaceName, workspaceActor(), usecase.Input{})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	flag, _ := out["require_admin_totp"].(bool)
	policy, _ := out["sign_in_policy"].(usecase.Output)
	rule, _ := policy["mfa_required_for"].(usecase.Output)
	value, _ := rule["value"].(string)
	return flag, value
}

func TestTheOldFlagAndTheRuleCanNoLongerDisagree(t *testing.T) {
	fixture := newWorkspacePolicyFixture(now)
	registry := workspaceRegistry(t, fixture)
	write := func(in usecase.Input) error {
		in["step_up_token"] = "hbt_stp_x"
		_, err := registry.Invoke(t.Context(), UpdateWorkspaceName, workspaceActor(), in)
		return err
	}

	// The flag switched on is the rule set to administrators.
	if err := write(usecase.Input{"require_admin_totp": true}); err != nil {
		t.Fatalf("switching the flag on: %v", err)
	}
	if flag, rule := bothFields(t, registry); !flag || rule != string(domain.MfaForAdmins) {
		t.Errorf("after the flag went on they read %v / %q, want true / ADMINS", flag, rule)
	}

	// And off is the rule set to nobody.
	if err := write(usecase.Input{"require_admin_totp": false}); err != nil {
		t.Fatalf("switching the flag off: %v", err)
	}
	if flag, rule := bothFields(t, registry); flag || rule != string(domain.MfaForNobody) {
		t.Errorf("after the flag went off they read %v / %q, want false / NONE", flag, rule)
	}

	// The other direction: the rule moves, and the flag follows it.
	if err := write(usecase.Input{"sign_in_policy": map[string]any{"mfa_required_for": "EVERYONE"}}); err != nil {
		t.Fatalf("setting the rule: %v", err)
	}
	if flag, rule := bothFields(t, registry); !flag || rule != string(domain.MfaForEveryone) {
		t.Errorf("after the rule went to everyone they read %v / %q, want true / EVERYONE", flag, rule)
	}

	// "Administrators need one" is already true of "everyone needs one": the flag switched on again
	// does not loosen the rule to administrators, and moves nothing.
	writes := len(fixture.store.updates)
	if err := write(usecase.Input{"require_admin_totp": true}); err != nil {
		t.Fatalf("the flag sent as it stands: %v", err)
	}
	if _, rule := bothFields(t, registry); rule != string(domain.MfaForEveryone) {
		t.Errorf("the flag loosened the rule to %q", rule)
	}
	if len(fixture.store.updates) != writes {
		t.Error("a flag that moved nothing wrote the workspace")
	}

	// A row stored before this, where the two had already come apart: the flag on, the rule off.
	// What is answered is the rule, which is what signing in has always obeyed.
	fixture.store.row.Settings.RequireAdminTotp = true
	fixture.store.row.Settings.SignIn.MfaRequiredFor = requirementOf(domain.MfaForNobody)
	if flag, rule := bothFields(t, registry); flag || rule != string(domain.MfaForNobody) {
		t.Errorf("a row that disagreed reads %v / %q, want the rule's false / NONE", flag, rule)
	}
}

// The flag is a change to the rule, so it meets every check the rule has: the proof, the lock and
// the level above.
func TestTheOldFlagMeetsEveryCheckTheRuleHas(t *testing.T) {
	fixture := newWorkspacePolicyFixture(now)
	registry := workspaceRegistry(t, fixture)

	// No proof, no change - and nothing written.
	_, err := registry.Invoke(t.Context(), UpdateWorkspaceName, workspaceActor(),
		usecase.Input{"require_admin_totp": true})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("the flag without a step-up answered %v", err)
	}
	if len(fixture.store.updates) != 0 {
		t.Fatal("the flag was written without a proof")
	}

	// The installation demands a factor of administrators: switching the flag off would loosen it.
	fixture.instance.level.Policy.Patch = domain.PolicyPatch{
		MfaRequiredFor: requirementOf(domain.MfaForAdmins),
	}
	_, err = registry.Invoke(t.Context(), UpdateWorkspaceName, workspaceActor(),
		usecase.Input{"require_admin_totp": false, "step_up_token": "hbt_stp_x"})
	if err == nil || !strings.Contains(err.Error(), "auth.policy_loosens") {
		t.Errorf("loosening the installation's rule through the flag answered %v", err)
	}

	// The installation locked the rule where it is: the flag cannot move it either.
	fixture.instance.level.Policy.Patch = domain.PolicyPatch{}
	fixture.instance.level.Policy.Locks = map[domain.PolicySwitch]bool{domain.SwitchMfaRequiredFor: true}
	_, err = registry.Invoke(t.Context(), UpdateWorkspaceName, workspaceActor(),
		usecase.Input{"require_admin_totp": true, "step_up_token": "hbt_stp_x"})
	if err == nil || !strings.Contains(err.Error(), "auth.policy_locked") {
		t.Errorf("moving a locked rule through the flag answered %v", err)
	}

	// Both fields in one body, saying different things, is refused rather than guessed at.
	fixture.instance.level.Policy.Locks = map[domain.PolicySwitch]bool{}
	_, err = registry.Invoke(t.Context(), UpdateWorkspaceName, workspaceActor(), usecase.Input{
		"require_admin_totp": false, "step_up_token": "hbt_stp_x",
		"sign_in_policy": map[string]any{"mfa_required_for": "ADMINS"},
	})
	var refusal *shared.Error
	if !errors.As(err, &refusal) || len(refusal.Fields) != 1 || refusal.Fields[0].Path != "/require_admin_totp" {
		t.Errorf("two fields saying different things answered %v", err)
	}
	if len(fixture.store.updates) != 0 {
		t.Errorf("%d writes from refused changes", len(fixture.store.updates))
	}
}
