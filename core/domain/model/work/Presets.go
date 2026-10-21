// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"slices"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// PresetKey names one of Hubtask's own presets. Stable: a layer keeps it as provenance, and a
// client translates it.
type PresetKey string

const (
	PresetList    PresetKey = "LIST"
	PresetTasks   PresetKey = "TASKS"
	PresetProject PresetKey = "PROJECT"
	PresetFlow    PresetKey = "FLOW"
)

// Preset is a starting point: values for all eight settings (domain-model.md §2). Choosing one
// copies its values into a layer; it is never consulted again.
type Preset struct {
	Key    PresetKey
	Values StructureValues
}

// BuiltInPresets are Hubtask's four presets, as build data rather than rows: nothing writes them,
// and an installation's own presets arrive in a table of their own. Their values are held to the
// installation's bound when they are offered (Offered), so a preset never offers what the
// installation does not.
//
// Each type's fields are written out from the system matrix rather than derived, because a preset
// is a decision about what a kind of collection needs - *List* has no date, person, label, note
// editor or child entry (UC-WRK-23 check 4) - and a decision belongs in one readable place.
func BuiltInPresets() []Preset {
	all := []Capability{
		CapabilityCompletion, CapabilityDueDate, CapabilityReminder, CapabilityAssignment,
		CapabilityMembers, CapabilityBucket, CapabilityNotes, CapabilityLabels, CapabilityComments,
		CapabilityCover, CapabilityAttachments, CapabilityHistory, CapabilityRecurrence,
		CapabilityCustomFields,
	}
	without := func(dropped ...Capability) []Capability {
		return slices.DeleteFunc(slices.Clone(all), func(c Capability) bool { return slices.Contains(dropped, c) })
	}
	captureDated := []CaptureToken{CaptureDate, CapturePerson, CaptureLabel}

	return []Preset{
		{Key: PresetList, Values: StructureValues{
			Levels: []Level{{Type: ItemTask}},
			Fields: map[ItemType]LevelFields{ItemTask: {Capabilities: []Capability{
				CapabilityCompletion, CapabilityHistory, CapabilityCustomFields,
			}}},
			Views:       &Views{Offered: []ViewLayout{ViewListCollapsed}, Default: ViewListCollapsed},
			Counts:      &Counts{Figure: FigureOpen},
			Completion:  CompletionManual,
			Capture:     []CaptureToken{CaptureLines, CaptureAmount},
			Celebration: CelebrationSubtle,
			Automation:  []AutomationBehaviour{},
		}},
		{Key: PresetTasks, Values: StructureValues{
			// The activity is clipped away where the installation's task does not accept one: no
			// level is offered that nothing could hold.
			Levels: []Level{{Type: ItemTask}, {Type: ItemActivity}},
			Fields: map[ItemType]LevelFields{
				ItemTask: {Capabilities: without(CapabilityMembers, CapabilityBucket, CapabilityCover)},
				ItemActivity: {Capabilities: []Capability{
					CapabilityCompletion, CapabilityDueDate, CapabilityReminder, CapabilityAssignment,
					CapabilityHistory,
				}},
			},
			Views: &Views{
				Offered: []ViewLayout{ViewListCollapsed, ViewListExpanded}, Default: ViewListCollapsed,
			},
			Counts:      &Counts{Figure: FigureDue},
			Completion:  CompletionManual,
			Capture:     captureDated,
			Celebration: CelebrationAll,
			Automation:  []AutomationBehaviour{},
		}},
		{Key: PresetProject, Values: StructureValues{
			Levels: []Level{{Type: ItemTask}, {Type: ItemWorkPackage}, {Type: ItemActivity}},
			Fields: map[ItemType]LevelFields{
				ItemTask:        {Capabilities: all},
				ItemWorkPackage: {Capabilities: all},
				ItemActivity:    {Capabilities: all},
			},
			Views: &Views{
				Offered: []ViewLayout{ViewListExpanded, ViewListCollapsed, ViewKanban, ViewTimeline},
				Default: ViewListExpanded,
			},
			Counts:      &Counts{Figure: FigureProgress},
			Completion:  CompletionRollup,
			Capture:     captureDated,
			Celebration: CelebrationAll,
			Automation:  []AutomationBehaviour{},
		}},
		{Key: PresetFlow, Values: StructureValues{
			Levels: []Level{{Type: ItemTask}},
			Fields: map[ItemType]LevelFields{ItemTask: {Capabilities: all}},
			Views: &Views{
				Offered: []ViewLayout{ViewKanban, ViewListCollapsed}, Default: ViewKanban,
			},
			Counts:      &Counts{Figure: FigureFlow},
			Completion:  CompletionManual,
			Capture:     captureDated,
			Celebration: CelebrationAll,
			Automation:  []AutomationBehaviour{},
		}},
	}
}

// Offered answers the preset as this installation offers it: clipped to the bound.
func (p Preset) Offered(bound StructureBound) Preset {
	return Preset{Key: p.Key, Values: bound.Clip(p.Values)}
}

// FindPreset answers one of Hubtask's presets by its key, as this installation offers it.
func FindPreset(key PresetKey, bound StructureBound) (Preset, error) {
	for _, preset := range BuiltInPresets() {
		if preset.Key == key {
			return preset.Offered(bound), nil
		}
	}
	return Preset{}, shared.ErrValidation.
		WithDetail("presets.unknown").
		WithParams(map[string]string{"preset": string(key)}).
		WithFields(shared.FieldError{Path: "/preset", Code: "presets.unknown"})
}
