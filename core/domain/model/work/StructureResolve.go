// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"slices"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// LockKey names what a lock fixes: a whole setting (`views`), or inside the fields one capability
// of one type (`fields.TASK.DUE_DATE` - "dates always on").
type LockKey string

// ParseLockKey reads a lock's key against the bound: a setting, or `fields.<TYPE>.<CAPABILITY>`
// naming a capability the bound carries for that type.
func ParseLockKey(raw string, bound StructureBound) (LockKey, error) {
	if setting, err := ParseStructureSetting(raw); err == nil {
		return LockKey(setting), nil
	}
	parts := strings.Split(raw, ".")
	if len(parts) == 3 && parts[0] == string(SettingFields) {
		if profile, held := bound.Profile(ItemType(parts[1])); held && profile.Allows(Capability(parts[2])) {
			return LockKey(raw), nil
		}
	}
	return "", shared.ErrValidation.
		WithDetail("structure.lock_unknown").
		WithFields(shared.FieldError{Path: "/locks", Code: "structure.lock_unknown"})
}

// Setting is the setting a lock fixes, whole or in part.
func (k LockKey) Setting() StructureSetting {
	setting, _, _ := strings.Cut(string(k), ".")
	return StructureSetting(setting)
}

// capability answers the type and capability of a fields lock, or false for a whole setting.
func (k LockKey) capability() (ItemType, Capability, bool) {
	parts := strings.Split(string(k), ".")
	if len(parts) != 3 {
		return "", "", false
	}
	return ItemType(parts[1]), Capability(parts[2]), true
}

// StructureLock is where a lock came from and who set it, so that a screen sends a person to the
// right one (P-06).
type StructureLock struct {
	Origin   identity.LockOrigin
	LockedBy shared.ID
}

// WorkspaceStructure is the workspace's layer: values every hub and collection inherits, the locks
// that bind them, and the preset a new hub starts from.
type WorkspaceStructure struct {
	Layer StructureLayer
	// Locks maps each lock to who set it.
	Locks map[LockKey]shared.ID
	// DefaultPreset is copied into a hub created without a preset. It applies to hubs only: a
	// collection follows its hub.
	DefaultPreset PresetKey
}

// LayerAt is one layer below the workspace with the source it answers as: a hub's or a
// collection's.
type LayerAt struct {
	Source identity.PolicySource
	Layer  StructureLayer
}

// EffectiveStructure is what applies to one container: every setting's value, where it came from,
// and which locks bind it. The locks travel with the values because every reader needs both - the
// screen to draw the lock, the writer to refuse the change (the shape of identity.EffectivePolicy).
type EffectiveStructure struct {
	Values  StructureValues
	Sources map[StructureSetting]identity.PolicySource
	Locks   map[LockKey]StructureLock
	// Preset is the provenance of the nearest layer that names one, empty where none does.
	Preset PresetKey
}

// SourceOf answers which level decided a setting's value in force.
func (e EffectiveStructure) SourceOf(setting StructureSetting) identity.PolicySource {
	if source, held := e.Sources[setting]; held {
		return source
	}
	return identity.SourceDefault
}

// ResolveStructure resolves the layers into what applies, nearest last: the workspace, then a hub,
// then a collection. A layer's value is a default for the layers below, held only by the bound and
// by a lock (N2): a collection may switch dates on under a hub without them.
//
// A locked setting takes the value the workspace resolved and ignores every lower layer's - a row
// stored before the lock landed is shown with the lock rather than refused here; refusing is the
// writer's job (identity.Effective works the same way).
func ResolveStructure(bound StructureBound, workspace WorkspaceStructure, below ...LayerAt) EffectiveStructure {
	values := bound.Default()
	sources := map[StructureSetting]identity.PolicySource{}
	preset := PresetKey("")

	apply := func(layer StructureLayer, source identity.PolicySource, locked func(StructureSetting) bool) {
		clipped := bound.Clip(layer.Values)
		for _, setting := range clipped.Decided() {
			if locked(setting) {
				continue
			}
			values = values.Take(setting, clipped)
			sources[setting] = source
		}
		if layer.Preset != "" {
			preset = layer.Preset
		}
	}

	apply(workspace.Layer, identity.SourceWorkspace, func(StructureSetting) bool { return false })
	atWorkspace := values

	locks := map[LockKey]StructureLock{}
	wholeLocked := map[StructureSetting]bool{}
	for key, by := range workspace.Locks {
		locks[key] = StructureLock{Origin: identity.LockWorkspace, LockedBy: by}
		if _, _, partial := key.capability(); !partial {
			wholeLocked[key.Setting()] = true
		}
	}

	for _, layer := range below {
		apply(layer.Layer, layer.Source, func(setting StructureSetting) bool { return wholeLocked[setting] })
	}

	// A lock on one capability fixes it to what the workspace resolved, on or off, wherever the
	// type is offered.
	for key := range workspace.Locks {
		itemType, capability, partial := key.capability()
		if !partial || wholeLocked[SettingFields] {
			continue
		}
		values.Fields = fixCapability(values.Fields, atWorkspace.Fields, itemType, capability)
	}

	return EffectiveStructure{Values: values, Sources: sources, Locks: locks, Preset: preset}
}

func fixCapability(
	fields, locked map[ItemType]LevelFields, itemType ItemType, capability Capability,
) map[ItemType]LevelFields {
	fixed := cloneFields(fields)
	level, offered := fixed[itemType]
	if !offered {
		return fixed
	}
	on := slices.Contains(locked[itemType].Capabilities, capability)
	has := slices.Contains(level.Capabilities, capability)
	switch {
	case on && !has:
		level.Capabilities = append(level.Capabilities, capability)
	case !on && has:
		level.Capabilities = slices.DeleteFunc(level.Capabilities, func(c Capability) bool { return c == capability })
	}
	fixed[itemType] = level
	return fixed
}

// LockedAgainst lists the locks a layer's own values break, under the workspace's locks: a whole
// setting the layer decides differently from the locked value, or a capability it turns the other
// way. `above` is what the layers above resolve to; the layer is the one being written.
func LockedAgainst(bound StructureBound, above EffectiveStructure, layer StructureValues) []LockKey {
	var broken []LockKey
	clipped := bound.Clip(layer)
	for key := range above.Locks {
		itemType, capability, partial := key.capability()
		if !partial {
			if clipped.Has(key.Setting()) && !clipped.SameSetting(key.Setting(), above.Values) {
				broken = append(broken, key)
			}
			continue
		}
		if !clipped.Has(SettingFields) {
			continue
		}
		level, set := clipped.Fields[itemType]
		if !set {
			continue
		}
		want := slices.Contains(above.Values.Fields[itemType].Capabilities, capability)
		if slices.Contains(level.Capabilities, capability) != want {
			broken = append(broken, key)
		}
	}
	slices.Sort(broken)
	return broken
}

// KeepLocked adjusts values a person did not type - a preset chosen - to the locks: a locked
// setting is left to the layer above, a locked capability set the way it is locked. It answers the
// values and the locks that changed them, which the answer names (kept_by_lock).
func KeepLocked(
	bound StructureBound, above EffectiveStructure, values StructureValues,
) (StructureValues, []LockKey) {
	broken := LockedAgainst(bound, above, values)
	for _, key := range broken {
		itemType, capability, partial := key.capability()
		if !partial {
			values = values.Without(key.Setting())
			continue
		}
		values.Fields = fixCapability(values.Fields, above.Values.Fields, itemType, capability)
	}
	return values, broken
}

// ErrSettingLocked refuses a write against a lock, naming the setting and where the lock came
// from.
func ErrSettingLocked(key LockKey, lock StructureLock) error {
	return shared.ErrValidation.
		WithDetail("structure.setting_locked").
		WithParams(map[string]string{"setting": string(key), "origin": string(lock.Origin)}).
		WithFields(shared.FieldError{
			Path: "/settings/" + string(key.Setting()), Code: "structure.setting_locked",
		})
}
