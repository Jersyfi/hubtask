// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// PresetKey names a preset: one of Hubtask's own by its key, which a client translates, or a saved
// one by its id (SavedPresetKey). Stable either way: a layer keeps it as provenance and the
// workspace as its default preset.
type PresetKey string

// BuiltIn reports whether the key names one of Hubtask's presets.
func (k PresetKey) BuiltIn() bool {
	switch k {
	case PresetList, PresetTasks, PresetProject, PresetFlow:
		return true
	}
	return false
}

// SavedPresetKey is the key a saved preset is named by: its id.
func SavedPresetKey(id shared.ID) PresetKey { return PresetKey(id.String()) }

// SavedID answers the id of the saved preset the key names; false for Hubtask's keys and for
// anything that is not an id.
func (k PresetKey) SavedID() (shared.ID, bool) {
	if k == "" || k.BuiltIn() {
		return "", false
	}
	id, err := shared.ParseID(string(k))
	if err != nil {
		return "", false
	}
	return id, true
}

// PresetOrigin says where a preset comes from (UC-WRK-30 check 2, UC-INS-18 check 2).
type PresetOrigin string

const (
	// PresetOriginHubtask is one of Hubtask's four, build data.
	PresetOriginHubtask PresetOrigin = "HUBTASK"
	// PresetOriginInstallation is the installation's: offered in every workspace, kept by its
	// operator.
	PresetOriginInstallation PresetOrigin = "INSTALLATION"
	// PresetOriginWorkspace is the workspace's: offered to everyone in it, kept by whoever may
	// change the workspace's shape.
	PresetOriginWorkspace PresetOrigin = "WORKSPACE"
	// PresetOriginOwn is one person's: offered to them alone.
	PresetOriginOwn PresetOrigin = "OWN"
)

// The bounds of the saved presets: a catalogue to choose from, not a store.
const (
	MaxPresetNameLength = 100
	MaxWorkspacePresets = 100
	MaxOwnPresets       = 50
)

const (
	PresetList    PresetKey = "LIST"
	PresetTasks   PresetKey = "TASKS"
	PresetProject PresetKey = "PROJECT"
	PresetFlow    PresetKey = "FLOW"
)

// Preset is a starting point: values for all eight settings (domain-model.md §2). Choosing one
// copies its values into a layer; it is never consulted again.
//
// Hubtask's presets carry their key alone. A saved one - the workspace's or a person's own - is
// keyed by its id and carries the name a person gave it (their content, rule 10), its origin and,
// for a person's own, whose it is.
type Preset struct {
	Key    PresetKey
	Origin PresetOrigin
	Values StructureValues

	ID      shared.ID
	Name    string
	OwnerID shared.ID
	Version int
}

// BuiltInPresets are Hubtask's four presets, as build data rather than rows: nothing writes them,
// and an installation's own presets are rows of their own (InstallationPreset). Their values are
// held to the installation's bound when they are offered (Offered), so a preset never offers what
// the installation does not.
//
// Each type's fields are written out from the system matrix rather than derived, because a preset
// is a decision about what a kind of collection needs - *List* has no date, person, label, note
// editor or child entry (UC-WRK-23 check 4) - and a decision belongs in one readable place.
func BuiltInPresets() []Preset {
	// Every capability but the opt-in ones (Capability.OptIn): *Project* and *Flow* are the bound
	// minus what a collection has to ask for.
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
		{Key: PresetList, Origin: PresetOriginHubtask, Values: StructureValues{
			Levels: []Level{{Type: ItemTask}},
			Fields: map[ItemType]LevelFields{ItemTask: {Capabilities: []Capability{
				CapabilityCompletion, CapabilityHistory, CapabilityCustomFields, CapabilityAmount,
			}}},
			Views:       &Views{Offered: []ViewLayout{ViewListCollapsed}, Default: ViewListCollapsed},
			Counts:      &Counts{Figure: FigureOpen},
			Completion:  CompletionManual,
			Capture:     []CaptureToken{CaptureLines, CaptureAmount},
			Celebration: CelebrationSubtle,
			Automation:  []AutomationBehaviour{},
		}},
		{Key: PresetTasks, Origin: PresetOriginHubtask, Values: StructureValues{
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
		{Key: PresetProject, Origin: PresetOriginHubtask, Values: StructureValues{
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
		{Key: PresetFlow, Origin: PresetOriginHubtask, Values: StructureValues{
			Levels: []Level{{Type: ItemTask}},
			Fields: map[ItemType]LevelFields{ItemTask: {Capabilities: all}},
			Views: &Views{
				Offered: []ViewLayout{ViewKanban, ViewListCollapsed}, Default: ViewKanban,
			},
			Counts:      &Counts{Figure: FigureFlow},
			Completion:  CompletionManual,
			Capture:     captureDated,
			Celebration: CelebrationAll,
			Automation:  []AutomationBehaviour{AutomationRequiredBeyondFirstColumn},
		}},
	}
}

// Offered answers the preset as this installation offers it: clipped to the bound.
func (p Preset) Offered(bound StructureBound) Preset {
	offered := p
	offered.Values = bound.Clip(p.Values)
	return offered
}

// NewSavedPresetInput is what a saved preset is made of.
type NewSavedPresetInput struct {
	ID      shared.ID
	Origin  PresetOrigin
	OwnerID shared.ID
	Name    string
	// Values are the eight settings it copies - a collection's structure in force, as a rule.
	Values StructureValues
	Text   text.Normalizer
}

// NewSavedPreset is a preset of the installation's, the workspace's or one person's own (UC-WRK-30
// check 1, UC-INS-18 check 1). Its values lose their categories: those name labels of one
// collection, which a preset offered for every collection cannot name.
func NewSavedPreset(in NewSavedPresetInput) (Preset, error) {
	switch in.Origin {
	case PresetOriginInstallation, PresetOriginWorkspace, PresetOriginOwn:
	default:
		return Preset{}, shared.ErrValidation.
			WithDetail("presets.origin_unknown").
			WithFields(shared.FieldError{Path: "/origin", Code: "presets.origin_unknown"})
	}
	name, err := PresetName(in.Name, in.Text)
	if err != nil {
		return Preset{}, err
	}
	owner := shared.ID("")
	if in.Origin == PresetOriginOwn {
		owner = in.OwnerID
	}
	return Preset{
		Key: SavedPresetKey(in.ID), Origin: in.Origin, ID: in.ID, Name: name, OwnerID: owner,
		Values: WithoutCategories(in.Values), Version: 1,
	}, nil
}

// PresetName normalises, trims and checks a saved preset's name: one line, like a container's.
func PresetName(raw string, form text.Normalizer) (string, error) {
	name, err := shared.NFC(strings.TrimSpace(raw), form)
	if err != nil {
		return "", err
	}
	invalid := func(code string) error {
		return shared.ErrValidation.WithDetail(code).
			WithFields(shared.FieldError{Path: "/name", Code: code})
	}
	switch {
	case name == "":
		return "", invalid("presets.name_empty")
	case utf8.RuneCountInString(name) > MaxPresetNameLength:
		return "", shared.ErrValidation.WithDetail("presets.name_too_long").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxPresetNameLength)}).
			WithFields(shared.FieldError{Path: "/name", Code: "presets.name_too_long"})
	case strings.IndexFunc(name, unicode.IsControl) >= 0:
		return "", invalid("presets.name_malformed")
	}
	return name, nil
}

// WithoutCategories answers the values with every type's categories left out.
func WithoutCategories(values StructureValues) StructureValues {
	if values.Fields == nil {
		return values
	}
	fields := make(map[ItemType]LevelFields, len(values.Fields))
	for itemType, level := range values.Fields {
		fields[itemType] = LevelFields{Capabilities: slices.Clone(level.Capabilities)}
	}
	values.Fields = fields
	return values
}

// ErrPresetsLimitReached refuses a saved preset beyond the bound of its origin.
func ErrPresetsLimitReached(origin PresetOrigin) error {
	maximum := MaxWorkspacePresets
	switch origin {
	case PresetOriginOwn:
		maximum = MaxOwnPresets
	case PresetOriginInstallation:
		maximum = MaxInstallationPresets
	}
	return shared.ErrConflict.WithDetail("presets.limit_reached").
		WithParams(map[string]string{"maximum": strconv.Itoa(maximum)})
}

// FindPreset answers one of Hubtask's presets by its key, as this installation offers it.
func FindPreset(key PresetKey, bound StructureBound) (Preset, error) {
	for _, preset := range BuiltInPresets() {
		if preset.Key == key {
			return preset.Offered(bound), nil
		}
	}
	return Preset{}, ErrPresetUnknown(key)
}

// ErrPresetUnknown refuses a key that names no preset the chooser is offered.
func ErrPresetUnknown(key PresetKey) error {
	return shared.ErrValidation.
		WithDetail("presets.unknown").
		WithParams(map[string]string{"preset": string(key)}).
		WithFields(shared.FieldError{Path: "/preset", Code: "presets.unknown"})
}
