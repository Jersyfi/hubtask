// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import "testing"

// *Private hubs allowed* (ADR-0073 §6, UC-ID-16 check 7): on by default, off the stricter value. A
// workspace may switch it off under an open "on", never on under an "off" above; the installation's
// lock holds whatever it decided, and the plan's lock holds the level above's value.
func TestPrivateHubsSwitchOffIsStricter(t *testing.T) {
	inForce := DefaultSignInPolicy()
	if !inForce.PrivateHubs {
		t.Fatal("the product's default is off")
	}
	if err := Tighten(SwitchPrivateHubs, inForce, PolicyPatch{PrivateHubs: boolOf(false)}); err != nil {
		t.Errorf("switching off under an open on: %v", err)
	}
	off := inForce
	off.PrivateHubs = false
	if err := Tighten(SwitchPrivateHubs, off, PolicyPatch{PrivateHubs: boolOf(true)}); err == nil {
		t.Error("a workspace switched on what the level above switched off")
	}

	ownOff := Effective(PolicyLayer{}, PolicyLayer{}, PolicyLayer{Patch: PolicyPatch{PrivateHubs: boolOf(false)}})
	if ownOff.Policy.PrivateHubs || ownOff.SourceOf(SwitchPrivateHubs) != SourceWorkspace {
		t.Errorf("the workspace's own off: %v from %q", ownOff.Policy.PrivateHubs, ownOff.SourceOf(SwitchPrivateHubs))
	}

	instance := PolicyLayer{
		Patch: PolicyPatch{PrivateHubs: boolOf(false)},
		Locks: map[PolicySwitch]bool{SwitchPrivateHubs: true},
	}
	locked := Effective(instance, PolicyLayer{}, PolicyLayer{Patch: PolicyPatch{PrivateHubs: boolOf(true)}})
	if locked.Policy.PrivateHubs || locked.LockOf(SwitchPrivateHubs) != LockInstance ||
		locked.SourceOf(SwitchPrivateHubs) != SourceInstance {
		t.Errorf("the installation's lock did not hold: %v, lock %q", locked.Policy.PrivateHubs, locked.LockOf(SwitchPrivateHubs))
	}

	plan := PolicyLayer{Locks: map[PolicySwitch]bool{SwitchPrivateHubs: true}}
	byPlan := Effective(PolicyLayer{}, plan, PolicyLayer{Patch: PolicyPatch{PrivateHubs: boolOf(false)}})
	if !byPlan.Policy.PrivateHubs || byPlan.LockOf(SwitchPrivateHubs) != LockPlan {
		t.Errorf("the plan's lock did not hold the default: %v, lock %q", byPlan.Policy.PrivateHubs, byPlan.LockOf(SwitchPrivateHubs))
	}

	if _, _, err := locked.Tightened(locked.Policy, PolicyPatch{PrivateHubs: boolOf(true)}); err == nil {
		t.Error("a workspace wrote a locked switch")
	}
	if SwitchText(off, SwitchPrivateHubs) != "false" {
		t.Errorf("the trail spells it %q", SwitchText(off, SwitchPrivateHubs))
	}
}
