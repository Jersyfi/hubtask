// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import "github.com/Jersyfi/hubtask/core/domain/model/setting"

// The eight structure settings as entries of the settings catalogue (domain-model.md §2, "The settings catalogue"). The
// value a layer stores for one setting is a StructureValues deciding that setting alone: the
// shapes stay this package's, and the catalogue's resolver says which layer's value is in force
// and which lock binds it. A lock on one capability of one type (`fields.TASK.DUE_DATE`) is a
// refinement inside the setting `fields` and stays here (fixCapabilities).

// StructureArea is the area the structure settings are keyed under.
const StructureArea = "structure"

// StructureKey is a structure setting's catalogue key.
func StructureKey(name StructureSetting) setting.Key {
	return setting.Key(StructureArea + "." + string(name))
}

// StructureDefinition is one structure setting's catalogue entry: the installation locks it
// through its default preset, the workspace sets and locks it, a hub and a collection set it.
func StructureDefinition(name StructureSetting) setting.Definition {
	return setting.Definition{
		Key: StructureKey(name), Kind: setting.KindStructure,
		Levels: []setting.Level{
			setting.LevelInstance, setting.LevelWorkspace, setting.LevelHub, setting.LevelCollection,
		},
		Lockable: true, May: setting.MayReplace,
		Writers: map[setting.Level]string{
			setting.LevelInstance: "admin:instance", setting.LevelWorkspace: "STRUCTURE",
			setting.LevelHub: "STRUCTURE", setting.LevelCollection: "STRUCTURE",
		},
		Preview: "PreviewContainerStructure",
	}
}

// only is these values with every setting but one left to the layer above.
func (v StructureValues) only(name StructureSetting) StructureValues {
	return StructureValues{}.Take(name, v)
}

// structureLayerAt is one layer's values, clipped to the bound, as catalogue entries.
func structureLayerAt(level setting.Level, bound StructureBound, layer StructureLayer) setting.Layer {
	clipped := bound.Clip(layer.Values)
	entries := map[setting.Key]setting.Entry{}
	for _, name := range clipped.Decided() {
		entries[StructureKey(name)] = setting.Entry{Value: clipped.only(name), Hold: setting.HoldSet}
	}
	return setting.Layer{Level: level, Entries: entries}
}
