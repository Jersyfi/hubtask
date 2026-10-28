// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package quota

import (
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
)

// The quotas as entries of the settings catalogue (domain-model.md §2, "The settings catalogue"):
// the installation and a plan set and lock a ceiling, and setting.Resolve answers which of them is
// in force. The operator's exception for one workspace is no level of the catalogue but the last
// word over it: a lock says what a workspace may not ask for, not what the operator may not grant.

// SettingArea is the area the quotas are keyed under.
const SettingArea = "quotas"

// SettingKey is a quota's catalogue key.
func SettingKey(name string) setting.Key { return setting.Key(SettingArea + "." + name) }

// Definition is one quota's catalogue entry; the mode's table is its default, so the declaration
// carries none.
func Definition(name string) setting.Definition {
	declaration := Declaration()
	return setting.Definition{
		Key: SettingKey(name), Kind: setting.Kind(declaration.Kind),
		Minimum: declaration.Minimum, Maximum: declaration.Maximum,
		Levels:   []setting.Level{setting.LevelInstance, setting.LevelPlan},
		Lockable: true, May: setting.May(declaration.May),
		Writers:  map[setting.Level]string{setting.LevelInstance: "admin:instance", setting.LevelPlan: "admin:instance"},
	}
}

// layerAt is one level's ceiling and lock for one quota as a catalogue layer.
func layerAt(level setting.Level, name string, value *int64, locked bool) setting.Layer {
	entry := setting.Entry{Hold: setting.HoldSet}
	if value != nil {
		entry.Value = *value
	}
	if locked {
		entry.Hold = setting.HoldLock
	}
	entries := map[setting.Key]setting.Entry{}
	if value != nil || locked {
		entries[SettingKey(name)] = entry
	}
	return setting.Layer{Level: level, Entries: entries}
}

// resolve answers one quota above the workspace: the installation's, the plan's where the
// installation does not lock it.
func (l Levels) resolve(name string) setting.Resolution {
	return setting.Resolve(Definition(name),
		layerAt(setting.LevelInstance, name, valueOf(l.Instance, name), l.InstanceLocks[name]),
		layerAt(setting.LevelPlan, name, valueOf(l.Plan.Overrides, name), l.Plan.Locks[name]),
	)
}
