// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"slices"

	"github.com/Jersyfi/hubtask/core/domain/model/setting"
)

// The AI rule's three switches as entries of the settings catalogue (domain-model.md §2, "The
// settings catalogue"). The installation and a plan resolve through setting.Resolve; the
// workspace's own say stays EffectiveAi's, because a lock holds a workspace out of a source and
// never in one (ADR-0072 §2) - narrowing to NONE under a lock is a rule no lock in the catalogue
// expresses.

// AiArea is the area the AI switches are keyed under.
const AiArea = "ai"

// AiKey is a switch's catalogue key.
func AiKey(name AiSwitch) setting.Key { return setting.Key(AiArea + "." + string(name)) }

// AiDefinition is one switch's catalogue entry: the installation decides it freely, a plan only
// narrows it, and both may lock it.
func AiDefinition(name AiSwitch) setting.Definition {
	declaration, _ := DeclarationOfAiSwitch(name)
	definition := setting.Definition{
		Key: AiKey(name), Kind: setting.Kind(declaration.Kind), Options: declaration.Options,
		Levels:   []setting.Level{setting.LevelInstance, setting.LevelPlan, setting.LevelWorkspace},
		Lockable: true, May: setting.May(declaration.May),
		Writers: map[setting.Level]string{
			setting.LevelInstance: "admin:instance", setting.LevelPlan: "admin:instance",
			setting.LevelWorkspace: "STRUCTURE",
		},
	}
	switch name {
	case AiSwitchSources:
		definition.Default = AiSourcesOwn
		definition.Tightens = func(inForce, proposed any) bool {
			above, _ := inForce.(AiSources)
			narrowed, _ := proposed.(AiSources)
			return narrowed.Within(above)
		}
	case AiSwitchMinJurisdiction:
		// A workspace has no own model below it to hold to a minimum.
		definition.Levels = definition.Levels[:2]
		delete(definition.Writers, setting.LevelWorkspace)
		definition.Default = AiMinAny
		definition.Tightens = func(inForce, proposed any) bool {
			above, _ := inForce.(AiMinJurisdiction)
			floor, _ := proposed.(AiMinJurisdiction)
			return slices.Index(aiMinOrder, floor) >= slices.Index(aiMinOrder, above)
		}
	case AiSwitchPersonOptOut:
		definition.Default = AiOptOutOffered
	}
	return definition
}

// aiLayerAt is one level above the workspace as catalogue entries: a valid value it decided, and
// its locks. Only OFFERED is honoured above the workspace - a level that cannot name a legal basis
// cannot make AI part of everybody's work (milestone PH, decision 4).
func aiLayerAt(level setting.Level, layer AiLayer) setting.Layer {
	entries := map[setting.Key]setting.Entry{}
	if layer.Sources != nil && layer.Sources.Valid() {
		entries[AiKey(AiSwitchSources)] = setting.Entry{Value: *layer.Sources, Hold: setting.HoldSet}
	}
	if layer.MinJurisdiction != nil && layer.MinJurisdiction.Valid() {
		entries[AiKey(AiSwitchMinJurisdiction)] = setting.Entry{Value: *layer.MinJurisdiction, Hold: setting.HoldSet}
	}
	if layer.PersonOptOut != nil && *layer.PersonOptOut == AiOptOutOffered {
		entries[AiKey(AiSwitchPersonOptOut)] = setting.Entry{Value: AiOptOutOffered, Hold: setting.HoldSet}
	}
	for _, name := range AiSwitches() {
		if layer.Locks[name] {
			entry := entries[AiKey(name)]
			entry.Hold = setting.HoldLock
			entries[AiKey(name)] = entry
		}
	}
	return setting.Layer{Level: level, Entries: entries}
}
