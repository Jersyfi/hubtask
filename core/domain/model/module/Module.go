// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package module holds the switches for the suite's families of building blocks (principle P-17,
// UC-INS-21): which modules exist, what each switch is as an entry of the settings catalogue, and
// what a place makes of the switches resolved for it.
//
// A module is off until a place switches it on; the installation and a plan only offer and lock.
// Switching off hides and never deletes: a module's data stays stored, a backup and an export carry
// it, and switching on again shows it unchanged.
package module

import (
	"slices"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Key is one module, spelled as the contract spells it.
type Key string

const (
	Workflows   Key = "WORKFLOWS"
	Planning    Key = "PLANNING"
	Knowledge   Key = "KNOWLEDGE"
	Whiteboard  Key = "WHITEBOARD"
	ServiceDesk Key = "SERVICE_DESK"
	Assets      Key = "ASSETS"
	Time        Key = "TIME"
	Goals       Key = "GOALS"
	Agents      Key = "AGENTS"
)

var keys = [...]Key{Workflows, Planning, Knowledge, Whiteboard, ServiceDesk, Assets, Time, Goals, Agents}

// Keys is every module, in the order a screen draws them.
func Keys() []Key { return keys[:] }

// Valid reports whether the key names a module.
func (k Key) Valid() bool { return slices.Contains(keys[:], k) }

// Parse reads a module's key.
func Parse(raw string) (Key, error) {
	if key := Key(raw); key.Valid() {
		return key, nil
	}
	return "", shared.ErrValidation.WithDetail("module.unknown").
		WithFields(shared.FieldError{Path: "/module", Code: "module.unknown"})
}

// Area is the area the switches are keyed under in the catalogue and the stores.
const Area = "module"

// SettingKey is a module's catalogue key: `module.service_desk`.
func (k Key) SettingKey() setting.Key { return setting.Key(Area + "." + strings.ToLower(string(k))) }

// OfSettingKey answers the module a catalogue key names.
func OfSettingKey(key setting.Key) (Key, bool) {
	name, held := strings.CutPrefix(string(key), Area+".")
	if !held {
		return "", false
	}
	module := Key(strings.ToUpper(name))
	return module, module.Valid()
}

// Definition is a module switch's catalogue entry: off by default; the installation and a plan
// lock it on or off (a module a plan does not offer is locked off); the workspace switches it
// freely inside that; a hub only narrows - it switches off what the workspace has on, never on
// what the workspace has off (UC-INS-21 check 3).
func (k Key) Definition() setting.Definition {
	return setting.Definition{
		Key: k.SettingKey(), Kind: setting.KindFlag,
		Levels:   []setting.Level{setting.LevelInstance, setting.LevelPlan, setting.LevelWorkspace, setting.LevelHub},
		Lockable: true, May: setting.MayTighten, TightenFrom: setting.LevelHub,
		Default: false,
		Writers: map[setting.Level]string{
			setting.LevelInstance: "admin:instance", setting.LevelPlan: "admin:instance",
			setting.LevelWorkspace: "STRUCTURE", setting.LevelHub: "STRUCTURE",
		},
		Tightens: func(inForce, proposed any) bool {
			on, _ := proposed.(bool)
			above, _ := inForce.(bool)
			return !on || above
		},
	}
}

// NarrowsReplica reports whether a device's copy holds the module's records only while the module
// is on at the workspace (offline-sync.md §8): the goals, read by no other module's screen. A
// module whose records others draw from - a workflow's statuses, a sprint on an entry - stays in
// the copy and is hidden by the client.
func (k Key) NarrowsReplica() bool { return k == Goals }

// Definitions is every module's entry, in Keys' order.
func Definitions() []setting.Definition {
	out := make([]setting.Definition, 0, len(keys))
	for _, key := range keys {
		out = append(out, key.Definition())
	}
	return out
}

// State is one module as a place resolves it.
type State struct {
	Module     Key
	Resolution setting.Resolution
}

// On reports whether the module is on.
func (s State) On() bool {
	on, _ := s.Resolution.Effective.(bool)
	return on
}

// Resolve answers every module at a place from the layers above and at it, top-down.
func Resolve(layers ...setting.Layer) []State {
	states := make([]State, 0, len(keys))
	for _, key := range keys {
		states = append(states, State{Module: key, Resolution: setting.Resolve(key.Definition(), layers...)})
	}
	return states
}

// Set is the modules on at a place.
type Set []Key

// SetOf is the modules the states have on.
func SetOf(states []State) Set {
	on := Set{}
	for _, state := range states {
		if state.On() {
			on = append(on, state.Module)
		}
	}
	return on
}

// Has reports whether the module is on.
func (s Set) Has(key Key) bool { return slices.Contains(s, key) }

// OffCode is the refusal of using a module that is off, the same through every door
// (UC-INS-21 check 5).
const OffCode = "module.off"

// Require refuses a module that is off. Forbidden rather than invalid, for the reason a plan's
// feature is (entitlement.NotIncluded): the request is well formed, and what would change the
// answer is the switch.
func (s Set) Require(key Key) error {
	if s.Has(key) {
		return nil
	}
	return shared.ErrForbidden.WithDetail(OffCode).WithParams(map[string]string{"module": string(key)})
}
