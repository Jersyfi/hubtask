// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import "github.com/Jersyfi/hubtask/core/domain/model/setting"

// The sign-in rule's switches as entries of the settings catalogue (domain-model.md §2.4). The
// value a level stores for one switch is a patch deciding that switch alone (PolicyPatch.Only):
// the typed pointers stay the rule's own, and the catalogue's resolver only says which level's
// patch is in force, which lock binds it, and whether a lower patch tightens.

// SettingArea is the area the sign-in switches are keyed under.
const SettingArea = "sign_in"

// SwitchKey is a switch's catalogue key.
func SwitchKey(name PolicySwitch) setting.Key { return setting.Key(SettingArea + "." + string(name)) }

// SwitchDefinition is one switch's catalogue entry. The installation decides it freely, a plan
// and a workspace only tighten it; the installation and a plan may lock it.
func SwitchDefinition(name PolicySwitch) setting.Definition {
	kind := setting.KindStructure
	declaration, held := DeclarationOfSwitch(name)
	if held {
		kind = setting.Kind(declaration.Kind)
	}
	return setting.Definition{
		Key:  SwitchKey(name),
		Kind: kind, Options: declaration.Options, Minimum: declaration.Minimum, Maximum: declaration.Maximum,
		Levels:   []setting.Level{setting.LevelInstance, setting.LevelPlan, setting.LevelWorkspace},
		Lockable: true, May: setting.MayTighten,
		Writers: map[setting.Level]string{
			setting.LevelInstance: "admin:instance", setting.LevelPlan: "admin:instance",
			setting.LevelWorkspace: "MANAGE_MEMBERS",
		},
		Tightens: func(inForce, proposed any) bool {
			patch, isPatch := proposed.(PolicyPatch)
			if !isPatch || !patch.Decides(name) {
				return false
			}
			policy := DefaultSignInPolicy()
			if above, decided := inForce.(PolicyPatch); decided {
				policy = applySwitch(policy, name, above)
			}
			return tightens(name, policy, patch)
		},
	}
}

// policyLayerAt is one level's sign-in layer as catalogue entries.
func policyLayerAt(level setting.Level, layer PolicyLayer, locks bool) setting.Layer {
	entries := map[setting.Key]setting.Entry{}
	for _, name := range layer.Patch.Decided() {
		entries[SwitchKey(name)] = setting.Entry{Value: layer.Patch.Only(name), Hold: setting.HoldSet}
	}
	if locks {
		for name, locked := range layer.Locks {
			if !locked {
				continue
			}
			entry := entries[SwitchKey(name)]
			entry.Hold = setting.HoldLock
			entries[SwitchKey(name)] = entry
		}
	}
	return setting.Layer{Level: level, Entries: entries}
}
