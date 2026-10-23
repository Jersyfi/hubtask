// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"slices"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// MaxInstallationPresets bounds the installation's presets: a catalogue every workspace is
// offered, not a store.
const MaxInstallationPresets = 50

// InstallationPreset is a preset the installation offers every workspace (UC-INS-18): one of them
// may be the default a hub starts from where its workspace chose none, and only the default locks
// settings - to its own values, for every workspace, hub and collection.
type InstallationPreset struct {
	Preset
	IsDefault bool
	Locks     []LockKey
}

// ErrLocksNeedDefault refuses locks on a preset that is not the installation's default: a lock
// fixes a setting to the default's value, and another preset has no say over anybody.
func ErrLocksNeedDefault() error {
	return shared.ErrValidation.WithDetail("presets.locks_need_default").
		WithFields(shared.FieldError{Path: "/locks", Code: "presets.locks_need_default"})
}

// ErrInstallationDefault refuses withdrawing the installation's default preset.
func ErrInstallationDefault() error {
	return shared.ErrConflict.WithDetail("presets.installation_default")
}

// InstallationHold is what the installation holds every workspace to beside its bound: the key of
// its default preset, the default's values, and the settings the default locks to them. The
// values are a layer only where a lock fixes them; otherwise they reach a hub by being copied at
// its creation, so choosing a default changes no hub that exists.
type InstallationHold struct {
	Default PresetKey
	Values  StructureValues
	Locks   []LockKey
}

// Hold is the hold the preset makes: the default's key, values and locks; nothing for any other.
func (p InstallationPreset) Hold() InstallationHold {
	if !p.IsDefault {
		return InstallationHold{}
	}
	return InstallationHold{Default: p.Key, Values: p.Values, Locks: slices.Clone(p.Locks)}
}

// WithInstallation answers the bound with the installation's hold, its values held to the bound
// like every layer's.
func (b StructureBound) WithInstallation(hold InstallationHold) StructureBound {
	hold.Values = b.Clip(hold.Values)
	b.installation = hold
	return b
}

// Installation answers the installation's hold.
func (b StructureBound) Installation() InstallationHold { return b.installation }

// ParseLockKeys reads a set of locks against the bound, each once, in order.
func ParseLockKeys(raw []string, bound StructureBound) ([]LockKey, error) {
	keys := make([]LockKey, 0, len(raw))
	for _, one := range raw {
		key, err := ParseLockKey(one, bound)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(keys, key) {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys, nil
}
