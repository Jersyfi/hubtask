// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import "testing"

// UC-INS-22 check 11: *the workspace may show its own name and logo* is on by default and off is
// the stricter value. The installation and a plan switch it off and may lock it, the lock naming its
// origin; a workspace never turns it back on under an off above.
func TestOwnBrandingIsOnByDefaultAndTheLevelsAboveSwitchItOff(t *testing.T) {
	inForce := DefaultSignInPolicy()
	if !inForce.OwnBranding {
		t.Fatal("the product's default is off")
	}
	declared, held := DeclarationOfSwitch(SwitchOwnBranding)
	if !held || declared.Kind != KindFlag || declared.May != MayTighten {
		t.Errorf("declared as %+v, %v", declared, held)
	}
	off := inForce
	off.OwnBranding = false
	if err := Tighten(SwitchOwnBranding, off, PolicyPatch{OwnBranding: boolOf(true)}); err == nil {
		t.Error("a workspace switched on what the level above switched off")
	}
	if err := Tighten(SwitchOwnBranding, inForce, PolicyPatch{OwnBranding: boolOf(false)}); err != nil {
		t.Errorf("switching off under an open on: %v", err)
	}

	cases := []struct {
		name       string
		instance   PolicyLayer
		plan       PolicyLayer
		wantLock   LockOrigin
		wantSource PolicySource
	}{
		{"the installation's off, locked", PolicyLayer{
			Patch: PolicyPatch{OwnBranding: boolOf(false)},
			Locks: map[PolicySwitch]bool{SwitchOwnBranding: true},
		}, PolicyLayer{}, LockInstance, SourceInstance},
		{"the installation's off, open", PolicyLayer{
			Patch: PolicyPatch{OwnBranding: boolOf(false)},
		}, PolicyLayer{}, LockNone, SourceInstance},
		{"the plan's off, locked", PolicyLayer{}, PolicyLayer{
			Patch: PolicyPatch{OwnBranding: boolOf(false)},
			Locks: map[PolicySwitch]bool{SwitchOwnBranding: true},
		}, LockPlan, SourcePlan},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workspace := PolicyLayer{Patch: PolicyPatch{OwnBranding: boolOf(true)}}
			resolved := Effective(tc.instance, tc.plan, workspace)
			if resolved.Policy.OwnBranding {
				t.Error("the workspace turned back on what the level above switched off")
			}
			if resolved.LockOf(SwitchOwnBranding) != tc.wantLock || resolved.SourceOf(SwitchOwnBranding) != tc.wantSource {
				t.Errorf("lock %q, source %q; want %q, %q", resolved.LockOf(SwitchOwnBranding),
					resolved.SourceOf(SwitchOwnBranding), tc.wantLock, tc.wantSource)
			}
		})
	}
	if SwitchText(off, SwitchOwnBranding) != "false" {
		t.Errorf("the trail spells it %q", SwitchText(off, SwitchOwnBranding))
	}
}
