// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package module

import (
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/setting"
)

// Decision is what the installation or a plan decides about one module: on or off, and whether
// it locks that for every level below. A plan that does not offer a module locks it off.
type Decision struct {
	On     bool
	Locked bool
}

// Decisions is one level's decisions above the workspace, by module; a module it does not name
// is left to the levels below.
type Decisions map[Key]Decision

// Name is a module's name inside the `module` area of the installation's and a plan's document:
// `service_desk`.
func (k Key) Name() string { return strings.ToLower(string(k)) }

// OfName answers the module a document's name inside the area names.
func OfName(name string) (Key, bool) {
	key := Key(strings.ToUpper(name))
	return key, key.Valid() && name == key.Name()
}

// Layer answers the decisions as the catalogue's layer for that level.
func (d Decisions) Layer(level setting.Level) setting.Layer {
	entries := make(map[setting.Key]setting.Entry, len(d))
	for key, decision := range d {
		hold := setting.HoldSet
		if decision.Locked {
			hold = setting.HoldLock
		}
		entries[key.SettingKey()] = setting.Entry{Value: decision.On, Hold: hold}
	}
	return setting.Layer{Level: level, Entries: entries}
}

// FitsUnder refuses a plan's decision on a module the installation locks, naming the first in
// Keys' order.
func (d Decisions) FitsUnder(above Decisions) (Key, bool) {
	for _, key := range keys {
		if _, decided := d[key]; decided && above[key].Locked {
			return key, false
		}
	}
	return "", true
}
