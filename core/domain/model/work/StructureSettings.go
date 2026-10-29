// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// StructureSetting is one of the nine things a preset decides for a collection (domain-model.md
// §2, "Levels and presets"). The spelling is the contract's key.
type StructureSetting string

const (
	// SettingLevels is which item types are offered, top-down, and what each is called.
	SettingLevels StructureSetting = "levels"
	// SettingFields is which capabilities each offered type carries.
	SettingFields StructureSetting = "fields"
	// SettingViews is which layouts a client offers, and which it opens with.
	SettingViews StructureSetting = "views"
	// SettingCounts is the figure a client shows in the collection's head.
	SettingCounts StructureSetting = "counts"
	// SettingCompletion is what completing a child does to its parent (I-W5).
	SettingCompletion StructureSetting = "completion"
	// SettingCapture is what a capture field recognises when it targets the collection.
	SettingCapture StructureSetting = "capture"
	// SettingCelebration is how success is shown.
	SettingCelebration StructureSetting = "celebration"
	// SettingAutomation is what the collection applies by itself.
	SettingAutomation StructureSetting = "automation"
	// SettingEntryTypes is which entry types each level offers (UC-FLW-01 check 3).
	SettingEntryTypes StructureSetting = "entry_types"
)

var structureSettings = [...]StructureSetting{
	SettingLevels, SettingFields, SettingViews, SettingCounts,
	SettingCompletion, SettingCapture, SettingCelebration, SettingAutomation, SettingEntryTypes,
}

// StructureSettingNames is the nine, in the order the contract lists them.
func StructureSettingNames() []StructureSetting { return structureSettings[:] }

// ParseStructureSetting reads a setting's name.
func ParseStructureSetting(raw string) (StructureSetting, error) {
	for _, known := range structureSettings {
		if string(known) == raw {
			return known, nil
		}
	}
	return "", settingInvalid("setting", "")
}

// ViewLayout is a layout a collection offers. The same spellings as a saved view's layout
// (view.Layout); declared here because the view package depends on this one, and held equal to it
// by a test there.
type ViewLayout string

const (
	ViewListCollapsed ViewLayout = "LIST_COLLAPSED"
	ViewListExpanded  ViewLayout = "LIST_EXPANDED"
	ViewKanban        ViewLayout = "KANBAN"
	ViewTimeline      ViewLayout = "TIMELINE"
)

var viewLayouts = [...]ViewLayout{ViewListCollapsed, ViewListExpanded, ViewKanban, ViewTimeline}

// ViewLayouts is the declared set.
func ViewLayouts() []ViewLayout { return viewLayouts[:] }

// Figure is what counts in a collection (UC-WRK-29 check 1).
type Figure string

const (
	FigureOpen     Figure = "OPEN"
	FigureFieldSum Figure = "FIELD_SUM"
	FigureDue      Figure = "DUE"
	FigureRhythm   Figure = "RHYTHM"
	FigureProgress Figure = "PROGRESS"
	FigureFlow     Figure = "FLOW"
	FigureNone     Figure = "NONE"
)

var figures = [...]Figure{
	FigureOpen, FigureFieldSum, FigureDue, FigureRhythm, FigureProgress, FigureFlow, FigureNone,
}

// Figures is the declared set.
func Figures() []Figure { return figures[:] }

// CaptureToken is one thing a capture field may recognise (UC-WRK-26 check 2, UC-WRK-25 checks
// 1-2).
type CaptureToken string

const (
	CaptureLines  CaptureToken = "LINES"
	CaptureAmount CaptureToken = "AMOUNT"
	CaptureDate   CaptureToken = "DATE"
	CapturePerson CaptureToken = "PERSON"
	CaptureLabel  CaptureToken = "LABEL"
)

var captureTokens = [...]CaptureToken{CaptureLines, CaptureAmount, CaptureDate, CapturePerson, CaptureLabel}

// CaptureTokens is the declared set.
func CaptureTokens() []CaptureToken { return captureTokens[:] }

// CelebrationStyle is how success is shown (design-system.md §7): every tier, or the subtle first
// one alone. Switching celebrations off is the person's own preference, not the collection's.
type CelebrationStyle string

const (
	CelebrationAll    CelebrationStyle = "ALL"
	CelebrationSubtle CelebrationStyle = "SUBTLE"
)

var celebrationStyles = [...]CelebrationStyle{CelebrationAll, CelebrationSubtle}

// CelebrationStyles is the declared set.
func CelebrationStyles() []CelebrationStyle { return celebrationStyles[:] }

// AutomationBehaviour is something a collection applies by itself. The set is closed: a value is
// declared only once something honours it, because a value accepted before that would be a setting
// nothing keeps.
type AutomationBehaviour string

const (
	// AutomationRequiredBeyondFirstColumn lets an entry enter a column after the board's first
	// only with every required custom field of its type set (UC-WRK-31 check 2). The server
	// applies it on every writer that places an entry in a column (`FlowGate`).
	AutomationRequiredBeyondFirstColumn AutomationBehaviour = "REQUIRED_BEYOND_FIRST_COLUMN"
)

var automationBehaviours = [...]AutomationBehaviour{AutomationRequiredBeyondFirstColumn}

// AutomationBehaviours is the declared set.
func AutomationBehaviours() []AutomationBehaviour { return automationBehaviours[:] }

// MaxLevelNameLength bounds a level's own name, in code points.
const MaxLevelNameLength = 60

// Level is one offered item type and, where a person chose one, its name. The name is that
// person's content (rule 10): it is never logged, never a problem parameter, and hashed in the
// audit trail. Without one, the client shows the translated default.
type Level struct {
	Type ItemType
	Name string
	// DoneName is what the level's done entries are shown under - "In the trolley" - where a
	// person chose one (UC-WRK-25 check 3). The same kind of content as Name, under the same rule.
	DoneName string
}

// LevelFields is what one offered type carries. An object rather than a bare list, so that field
// definitions can sit beside the capabilities when a setting needs them.
type LevelFields struct {
	Capabilities []Capability
	// Categories are the labels the collection groups this type's entries by, in their order
	// (UC-WRK-25 check 5): an entry sits under the first of them it carries. Only beside LABELS,
	// and only on a collection's own layer - labels belong to a collection, so a hub's or the
	// workspace's layer naming one would name nothing; the use case checks that every one is a
	// label of the collection. A label deleted afterwards stays named, and a client skips it.
	Categories []shared.ID
}

// MaxCategories bounds how many labels a type's categories name: the aisles of a shop, not a
// taxonomy.
const MaxCategories = 50

// Views is which layouts a client offers, and the one it opens with.
type Views struct {
	Offered []ViewLayout
	Default ViewLayout
}

// Counts is the figure a client shows, with the custom field it sums for FIELD_SUM.
type Counts struct {
	Figure   Figure
	FieldKey string
}

// StructureValues is a layer's say about the nine settings: a nil or empty value is a setting
// this layer leaves to the one above. Resolved, every one of them is set.
type StructureValues struct {
	Levels      []Level
	Fields      map[ItemType]LevelFields
	Views       *Views
	Counts      *Counts
	Completion  CompletionPolicy
	Capture     []CaptureToken
	Celebration CelebrationStyle
	// Automation is set when not nil; an empty, non-nil list is "nothing runs by itself".
	Automation []AutomationBehaviour
	// EntryTypes is set when not nil: per level, the type keys offered, in order. A level absent
	// from a set map offers every type of its level, so the empty map is "every type".
	EntryTypes map[ItemType][]string
}

// MaxOfferedEntryTypes bounds how many types one level of a collection offers.
const MaxOfferedEntryTypes = 50

// Has reports whether this layer decides the setting.
func (v StructureValues) Has(setting StructureSetting) bool {
	switch setting {
	case SettingLevels:
		return v.Levels != nil
	case SettingFields:
		return v.Fields != nil
	case SettingViews:
		return v.Views != nil
	case SettingCounts:
		return v.Counts != nil
	case SettingCompletion:
		return v.Completion != ""
	case SettingCapture:
		return v.Capture != nil
	case SettingCelebration:
		return v.Celebration != ""
	case SettingAutomation:
		return v.Automation != nil
	case SettingEntryTypes:
		return v.EntryTypes != nil
	}
	return false
}

// Decided lists the settings this layer decides, in the contract's order.
func (v StructureValues) Decided() []StructureSetting {
	var decided []StructureSetting
	for _, setting := range structureSettings {
		if v.Has(setting) {
			decided = append(decided, setting)
		}
	}
	return decided
}

// Without leaves the setting to the layer above.
func (v StructureValues) Without(setting StructureSetting) StructureValues {
	switch setting {
	case SettingLevels:
		v.Levels = nil
	case SettingFields:
		v.Fields = nil
	case SettingViews:
		v.Views = nil
	case SettingCounts:
		v.Counts = nil
	case SettingCompletion:
		v.Completion = ""
	case SettingCapture:
		v.Capture = nil
	case SettingCelebration:
		v.Celebration = ""
	case SettingAutomation:
		v.Automation = nil
	case SettingEntryTypes:
		v.EntryTypes = nil
	}
	return v
}

// Take copies one setting's value from another set of values.
func (v StructureValues) Take(setting StructureSetting, from StructureValues) StructureValues {
	switch setting {
	case SettingLevels:
		v.Levels = slices.Clone(from.Levels)
	case SettingFields:
		v.Fields = cloneFields(from.Fields)
	case SettingViews:
		v.Views = cloneViews(from.Views)
	case SettingCounts:
		if from.Counts != nil {
			counts := *from.Counts
			v.Counts = &counts
		} else {
			v.Counts = nil
		}
	case SettingCompletion:
		v.Completion = from.Completion
	case SettingCapture:
		v.Capture = slices.Clone(from.Capture)
	case SettingCelebration:
		v.Celebration = from.Celebration
	case SettingAutomation:
		v.Automation = slices.Clone(from.Automation)
	case SettingEntryTypes:
		v.EntryTypes = cloneEntryTypes(from.EntryTypes)
	}
	return v
}

// Overlay takes every setting the other values decide.
func (v StructureValues) Overlay(other StructureValues) StructureValues {
	for _, setting := range other.Decided() {
		v = v.Take(setting, other)
	}
	return v
}

// SameSetting reports whether two sets of values say the same about one setting.
func (v StructureValues) SameSetting(setting StructureSetting, other StructureValues) bool {
	switch setting {
	case SettingLevels:
		return slices.Equal(v.Levels, other.Levels) && (v.Levels == nil) == (other.Levels == nil)
	case SettingFields:
		if (v.Fields == nil) != (other.Fields == nil) || len(v.Fields) != len(other.Fields) {
			return false
		}
		for itemType, fields := range v.Fields {
			theirs, held := other.Fields[itemType]
			if !held || !sameCapabilities(fields.Capabilities, theirs.Capabilities) ||
				!slices.Equal(fields.Categories, theirs.Categories) {
				return false
			}
		}
		return true
	case SettingViews:
		if v.Views == nil || other.Views == nil {
			return v.Views == other.Views
		}
		return v.Views.Default == other.Views.Default && slices.Equal(v.Views.Offered, other.Views.Offered)
	case SettingCounts:
		if v.Counts == nil || other.Counts == nil {
			return v.Counts == other.Counts
		}
		return *v.Counts == *other.Counts
	case SettingCompletion:
		return v.Completion == other.Completion
	case SettingCapture:
		return (v.Capture == nil) == (other.Capture == nil) && sameTokens(v.Capture, other.Capture)
	case SettingCelebration:
		return v.Celebration == other.Celebration
	case SettingAutomation:
		return (v.Automation == nil) == (other.Automation == nil) && slices.Equal(v.Automation, other.Automation)
	case SettingEntryTypes:
		if (v.EntryTypes == nil) != (other.EntryTypes == nil) || len(v.EntryTypes) != len(other.EntryTypes) {
			return false
		}
		for level, keys := range v.EntryTypes {
			theirs, held := other.EntryTypes[level]
			if !held || !slices.Equal(keys, theirs) {
				return false
			}
		}
		return true
	}
	return false
}

// Moved lists the settings on which two sets of values differ.
func (v StructureValues) Moved(other StructureValues) []StructureSetting {
	var moved []StructureSetting
	for _, setting := range structureSettings {
		if !v.SameSetting(setting, other) {
			moved = append(moved, setting)
		}
	}
	return moved
}

func sameCapabilities(a, b []Capability) bool {
	if len(a) != len(b) {
		return false
	}
	for _, capability := range a {
		if !slices.Contains(b, capability) {
			return false
		}
	}
	return true
}

func sameTokens(a, b []CaptureToken) bool {
	if len(a) != len(b) {
		return false
	}
	for _, token := range a {
		if !slices.Contains(b, token) {
			return false
		}
	}
	return true
}

// withoutOptIn is a type's capabilities as a layer that does not name them carries them: all of
// them but the opt-in ones (Capability.OptIn).
func withoutOptIn(capabilities []Capability) []Capability {
	return slices.DeleteFunc(slices.Clone(capabilities), Capability.OptIn)
}

func cloneFields(fields map[ItemType]LevelFields) map[ItemType]LevelFields {
	if fields == nil {
		return nil
	}
	clone := make(map[ItemType]LevelFields, len(fields))
	for itemType, level := range fields {
		clone[itemType] = LevelFields{
			Capabilities: slices.Clone(level.Capabilities), Categories: slices.Clone(level.Categories),
		}
	}
	return clone
}

func cloneEntryTypes(offered map[ItemType][]string) map[ItemType][]string {
	if offered == nil {
		return nil
	}
	clone := make(map[ItemType][]string, len(offered))
	for level, keys := range offered {
		clone[level] = slices.Clone(keys)
	}
	return clone
}

func cloneViews(views *Views) *Views {
	if views == nil {
		return nil
	}
	return &Views{Offered: slices.Clone(views.Offered), Default: views.Default}
}

// StructureLayer is what one level of the inheritance stores: values, and the preset they were
// copied from. The preset is provenance only - choosing one copies its values, and a later change
// to the preset changes nothing stored here.
type StructureLayer struct {
	Preset PresetKey
	Values StructureValues
}

// IsEmpty reports whether the layer decides nothing and names no preset.
func (l StructureLayer) IsEmpty() bool {
	return l.Preset == "" && len(l.Values.Decided()) == 0
}

// StructureBound is what the installation allows: the system profiles intersected with the
// profiles in force (an installation's own narrowing in item_capability_profile), with the
// system's topology. Every setting at every layer is held inside it - a layer never widens it -
// and every stored value is clipped to it when resolved, because the installation may narrow after
// a value was stored.
type StructureBound struct {
	profiles map[ItemType]CapabilityProfile
	// order is the bound's types, top-down: the roots first, then each type after a parent.
	order []ItemType
	// parents are each type's system parents.
	parents map[ItemType][]ItemType
	// installation is what the installation holds every workspace to beside the profiles: its
	// default preset and the settings that preset locks (InstallationPresets.go).
	installation InstallationHold
}

// NewStructureBound builds the bound. A type the profiles in force no longer carry is not in it.
func NewStructureBound(inForce, system []CapabilityProfile) StructureBound {
	held := make(map[ItemType]CapabilityProfile, len(inForce))
	for _, profile := range inForce {
		held[profile.Type] = profile
	}

	bound := StructureBound{
		profiles: make(map[ItemType]CapabilityProfile, len(system)),
		parents:  make(map[ItemType][]ItemType, len(system)),
	}
	for _, profile := range system {
		ours, present := held[profile.Type]
		if !present {
			continue
		}
		capabilities := make([]Capability, 0, len(profile.Capabilities))
		for _, capability := range profile.Capabilities {
			if ours.Allows(capability) {
				capabilities = append(capabilities, capability)
			}
		}
		bound.profiles[profile.Type] = CapabilityProfile{
			Type: profile.Type, Capabilities: capabilities,
			AllowedChildTypes: slices.Clone(profile.AllowedChildTypes), MaxDepth: profile.MaxDepth,
		}
	}
	for _, profile := range system {
		if _, present := bound.profiles[profile.Type]; !present {
			continue
		}
		for _, child := range profile.AllowedChildTypes {
			if child == profile.Type {
				continue
			}
			bound.parents[child] = append(bound.parents[child], profile.Type)
		}
	}

	// Top-down: a root first, then every type whose parents are placed. The system's order breaks
	// ties, so the answer is stable.
	placed := map[ItemType]bool{}
	for len(bound.order) < len(bound.profiles) {
		progressed := false
		for _, profile := range system {
			if _, present := bound.profiles[profile.Type]; !present || placed[profile.Type] {
				continue
			}
			ready := true
			for _, parent := range bound.parents[profile.Type] {
				if !placed[parent] && parent != profile.Type {
					ready = false
				}
			}
			if ready {
				bound.order = append(bound.order, profile.Type)
				placed[profile.Type] = true
				progressed = true
			}
		}
		if !progressed {
			// A cycle in the system's topology: NewHierarchy refuses that installation already.
			break
		}
	}
	return bound
}

// Types is the bound's types, top-down.
func (b StructureBound) Types() []ItemType { return slices.Clone(b.order) }

// Profile answers the bound's profile of one type.
func (b StructureBound) Profile(itemType ItemType) (CapabilityProfile, bool) {
	profile, held := b.profiles[itemType]
	return profile, held
}

func (b StructureBound) isRoot(itemType ItemType) bool {
	for _, parent := range b.parents[itemType] {
		if _, held := b.profiles[parent]; held {
			return false
		}
	}
	return true
}

// Default is Hubtask's own value of every setting under this bound: today's behaviour, so that a
// container nobody configured answers exactly what it did before settings existed.
func (b StructureBound) Default() StructureValues {
	levels := make([]Level, 0, len(b.order))
	fields := make(map[ItemType]LevelFields, len(b.order))
	for _, itemType := range b.order {
		levels = append(levels, Level{Type: itemType})
		fields[itemType] = LevelFields{Capabilities: withoutOptIn(b.profiles[itemType].Capabilities)}
	}
	return StructureValues{
		Levels: levels,
		Fields: fields,
		Views: &Views{
			Offered: []ViewLayout{ViewListCollapsed, ViewListExpanded, ViewKanban, ViewTimeline},
			Default: ViewListCollapsed,
		},
		Counts:      &Counts{Figure: FigureNone},
		Completion:  DefaultCompletionPolicy,
		Capture:     []CaptureToken{CaptureDate, CapturePerson, CaptureLabel},
		Celebration: CelebrationAll,
		Automation:  []AutomationBehaviour{},
		EntryTypes:  map[ItemType][]string{},
	}
}

// Clip holds stored values inside the bound. It never fails: a value stored before the
// installation narrowed is shown as narrowed, not refused on read.
func (b StructureBound) Clip(values StructureValues) StructureValues {
	if values.Levels != nil {
		values.Levels = b.closeLevels(values.Levels)
	}
	if values.Fields != nil {
		clipped := make(map[ItemType]LevelFields, len(b.order))
		for _, itemType := range b.order {
			profile := b.profiles[itemType]
			level, set := values.Fields[itemType]
			if !set {
				clipped[itemType] = LevelFields{Capabilities: withoutOptIn(profile.Capabilities)}
				continue
			}
			kept := make([]Capability, 0, len(level.Capabilities))
			for _, capability := range profile.Capabilities {
				if slices.Contains(level.Capabilities, capability) || capability == CapabilityCompletion {
					kept = append(kept, capability)
				}
			}
			clipped[itemType] = LevelFields{Capabilities: kept, Categories: categoriesBeside(kept, level.Categories)}
		}
		values.Fields = clipped
	}
	if values.Automation != nil {
		// Every declared behaviour is within every installation's bound; what is dropped is only a
		// value this build no longer declares.
		kept := make([]AutomationBehaviour, 0, len(values.Automation))
		for _, behaviour := range values.Automation {
			if slices.Contains(automationBehaviours[:], behaviour) && !slices.Contains(kept, behaviour) {
				kept = append(kept, behaviour)
			}
		}
		values.Automation = kept
	}
	if values.EntryTypes != nil {
		kept := make(map[ItemType][]string, len(values.EntryTypes))
		for level, keys := range values.EntryTypes {
			if _, held := b.profiles[level]; held && len(keys) > 0 {
				kept[level] = slices.Clone(keys)
			}
		}
		values.EntryTypes = kept
	}
	return values
}

// closeLevels keeps the levels the bound holds whose parent level is kept, in the bound's order,
// and a root where none would be left.
func (b StructureBound) closeLevels(levels []Level) []Level {
	wanted := map[ItemType]Level{}
	for _, level := range levels {
		if _, held := b.profiles[level.Type]; held {
			wanted[level.Type] = level
		}
	}
	kept := map[ItemType]bool{}
	closed := make([]Level, 0, len(wanted))
	for _, itemType := range b.order {
		level, asked := wanted[itemType]
		if !asked {
			continue
		}
		if !b.isRoot(itemType) && !b.anyParentKept(itemType, kept) {
			continue
		}
		kept[itemType] = true
		closed = append(closed, level)
	}
	if len(closed) == 0 {
		for _, itemType := range b.order {
			if b.isRoot(itemType) {
				closed = append(closed, Level{Type: itemType})
			}
		}
	}
	return closed
}

func (b StructureBound) anyParentKept(itemType ItemType, kept map[ItemType]bool) bool {
	for _, parent := range b.parents[itemType] {
		if kept[parent] {
			return true
		}
	}
	return false
}

// Validate is the check a write makes: every value inside the bound, the levels closed, names
// normalised and bounded, every vocabulary closed. It answers the values as they are stored.
//
// A value beyond the bound is refused as capability_not_supported (UC-WRK-23 check 5): it asks for
// more than the installation offers, which is the same answer an entry asking for an absent
// capability gets.
func (b StructureBound) Validate(values StructureValues, form text.Normalizer) (StructureValues, error) {
	if values.Levels != nil {
		levels, err := b.validateLevels(values.Levels, form)
		if err != nil {
			return StructureValues{}, err
		}
		values.Levels = levels
	}
	if values.Fields != nil {
		fields, err := b.validateFields(values.Fields)
		if err != nil {
			return StructureValues{}, err
		}
		values.Fields = fields
	}
	if values.Views != nil {
		if err := validateViews(*values.Views); err != nil {
			return StructureValues{}, err
		}
	}
	if values.Counts != nil {
		counts, err := validateCounts(*values.Counts)
		if err != nil {
			return StructureValues{}, err
		}
		values.Counts = &counts
	}
	if values.Completion != "" && !values.Completion.Valid() {
		return StructureValues{}, settingInvalid(string(SettingCompletion), "")
	}
	if values.Capture != nil {
		seen := map[CaptureToken]bool{}
		for _, token := range values.Capture {
			if !slices.Contains(captureTokens[:], token) || seen[token] {
				return StructureValues{}, settingInvalid(string(SettingCapture), "")
			}
			seen[token] = true
		}
	}
	if values.Celebration != "" && !slices.Contains(celebrationStyles[:], values.Celebration) {
		return StructureValues{}, settingInvalid(string(SettingCelebration), "")
	}
	if values.Automation != nil {
		seen := map[AutomationBehaviour]bool{}
		for _, behaviour := range values.Automation {
			if !slices.Contains(automationBehaviours[:], behaviour) {
				// A behaviour this installation does not have is asking for more than it offers.
				return StructureValues{}, widens(SettingAutomation, "", "")
			}
			if seen[behaviour] {
				return StructureValues{}, settingInvalid(string(SettingAutomation), "")
			}
			seen[behaviour] = true
		}
	}
	if values.EntryTypes != nil {
		offered, err := b.validateEntryTypes(values.EntryTypes)
		if err != nil {
			return StructureValues{}, err
		}
		values.EntryTypes = offered
	}
	return values, nil
}

// validateEntryTypes holds the offered types to their shape: a level the bound holds, one to
// MaxOfferedEntryTypes keys, each a type key, none twice. Whether each names a type of the
// workspace is answered when an entry chooses one: a key that names nothing offers nothing.
func (b StructureBound) validateEntryTypes(offered map[ItemType][]string) (map[ItemType][]string, error) {
	validated := make(map[ItemType][]string, len(offered))
	for level, keys := range offered {
		if _, held := b.profiles[level]; !held {
			return nil, widens(SettingEntryTypes, level, "")
		}
		if len(keys) == 0 || len(keys) > MaxOfferedEntryTypes {
			return nil, settingInvalid(string(SettingEntryTypes), level)
		}
		seen := map[string]bool{}
		for _, key := range keys {
			if !entryTypeKeyPattern.MatchString(key) || seen[key] {
				return nil, settingInvalid(string(SettingEntryTypes), level)
			}
			seen[key] = true
		}
		validated[level] = slices.Clone(keys)
	}
	return validated, nil
}

func (b StructureBound) validateLevels(levels []Level, form text.Normalizer) ([]Level, error) {
	asked := map[ItemType]Level{}
	for _, level := range levels {
		if _, held := b.profiles[level.Type]; !held {
			return nil, widens(SettingLevels, level.Type, "")
		}
		if _, twice := asked[level.Type]; twice {
			return nil, settingInvalid(string(SettingLevels), level.Type)
		}
		name, err := levelName(level.Type, level.Name, form)
		if err != nil {
			return nil, err
		}
		doneName, err := levelName(level.Type, level.DoneName, form)
		if err != nil {
			return nil, err
		}
		asked[level.Type] = Level{Type: level.Type, Name: name, DoneName: doneName}
	}

	kept := map[ItemType]bool{}
	ordered := make([]Level, 0, len(asked))
	rooted := false
	for _, itemType := range b.order {
		level, wanted := asked[itemType]
		if !wanted {
			continue
		}
		if b.isRoot(itemType) {
			rooted = true
		} else if !b.anyParentKept(itemType, kept) {
			return nil, levelsNotClosed(itemType)
		}
		kept[itemType] = true
		ordered = append(ordered, level)
	}
	if !rooted {
		return nil, levelsNotClosed("")
	}
	return ordered, nil
}

// levelName normalises a name a person gave a level - its own, or its done entries' - trimmed,
// NFC, 1 to 60 code points. An empty name is no name - the translated default applies. The refusal
// never carries the name: it is the person's content (rule 10).
func levelName(itemType ItemType, raw string, form text.Normalizer) (string, error) {
	if raw == "" {
		return "", nil
	}
	normalised, err := shared.NFC(strings.TrimSpace(raw), form)
	if err != nil {
		return "", err
	}
	length := utf8.RuneCountInString(normalised)
	if length == 0 || length > MaxLevelNameLength {
		return "", shared.ErrValidation.
			WithDetail("structure.level_name_invalid").
			WithParams(map[string]string{
				"item_type": string(itemType), "max": "60",
			}).
			WithFields(shared.FieldError{
				Path: "/settings/levels", Code: "structure.level_name_invalid",
				Params: map[string]string{"item_type": string(itemType)},
			})
	}
	return normalised, nil
}

func (b StructureBound) validateFields(fields map[ItemType]LevelFields) (map[ItemType]LevelFields, error) {
	validated := make(map[ItemType]LevelFields, len(fields))
	for itemType, level := range fields {
		profile, held := b.profiles[itemType]
		if !held {
			return nil, widens(SettingFields, itemType, "")
		}
		seen := map[Capability]bool{}
		for _, capability := range level.Capabilities {
			if seen[capability] {
				return nil, settingInvalid(string(SettingFields), itemType)
			}
			seen[capability] = true
			if !profile.Allows(capability) {
				return nil, widens(SettingFields, itemType, capability)
			}
		}
		if !seen[CapabilityCompletion] && profile.Allows(CapabilityCompletion) {
			// Completion is mandatory for every type (domain-model.md §2): an entry that cannot be
			// done is not an entry.
			return nil, settingInvalid(string(SettingFields), itemType)
		}
		ordered := make([]Capability, 0, len(level.Capabilities))
		for _, capability := range profile.Capabilities {
			if seen[capability] {
				ordered = append(ordered, capability)
			}
		}
		if err := validateCategories(itemType, level.Categories, seen[CapabilityLabels]); err != nil {
			return nil, err
		}
		validated[itemType] = LevelFields{Capabilities: ordered, Categories: slices.Clone(level.Categories)}
	}
	return validated, nil
}

// validateCategories holds a type's categories to their shape: beside LABELS, at most
// MaxCategories, each an identifier, none twice. Whether each is a label of the collection is the
// use case's question, which can read the labels.
func validateCategories(itemType ItemType, categories []shared.ID, labelled bool) error {
	if len(categories) == 0 {
		return nil
	}
	if !labelled || len(categories) > MaxCategories {
		return settingInvalid(string(SettingFields), itemType)
	}
	seen := map[shared.ID]bool{}
	for _, category := range categories {
		if _, err := shared.ParseID(category.String()); err != nil || seen[category] {
			return settingInvalid(string(SettingFields), itemType)
		}
		seen[category] = true
	}
	return nil
}

// categoriesBeside keeps categories only where their type still carries LABELS: without labels an
// entry carries none to be grouped by.
func categoriesBeside(capabilities []Capability, categories []shared.ID) []shared.ID {
	if !slices.Contains(capabilities, CapabilityLabels) {
		return nil
	}
	return slices.Clone(categories)
}

func validateViews(views Views) error {
	if len(views.Offered) == 0 || !slices.Contains(views.Offered, views.Default) {
		return settingInvalid(string(SettingViews), "")
	}
	seen := map[ViewLayout]bool{}
	for _, layout := range views.Offered {
		if !slices.Contains(viewLayouts[:], layout) || seen[layout] {
			return settingInvalid(string(SettingViews), "")
		}
		seen[layout] = true
	}
	return nil
}

// maxFieldKeyLength matches a custom field key's own bound.
const maxFieldKeyLength = 64

func validateCounts(counts Counts) (Counts, error) {
	if !slices.Contains(figures[:], counts.Figure) {
		return Counts{}, settingInvalid(string(SettingCounts), "")
	}
	key := strings.TrimSpace(counts.FieldKey)
	if counts.Figure == FigureFieldSum {
		if key == "" || len(key) > maxFieldKeyLength {
			return Counts{}, settingInvalid(string(SettingCounts), "")
		}
		return Counts{Figure: counts.Figure, FieldKey: key}, nil
	}
	if key != "" {
		return Counts{}, settingInvalid(string(SettingCounts), "")
	}
	return Counts{Figure: counts.Figure}, nil
}

func widens(setting StructureSetting, itemType ItemType, capability Capability) error {
	params := map[string]string{"setting": string(setting)}
	if itemType != "" {
		params["item_type"] = string(itemType)
	}
	if capability != "" {
		params["capability"] = string(capability)
	}
	return shared.ErrCapabilityNotSupported.
		WithDetail("structure.widens").
		WithParams(params).
		WithFields(shared.FieldError{Path: "/settings/" + string(setting), Code: "structure.widens", Params: params})
}

func settingInvalid(setting string, itemType ItemType) error {
	params := map[string]string{"setting": setting}
	if itemType != "" {
		params["item_type"] = string(itemType)
	}
	return shared.ErrValidation.
		WithDetail("structure.setting_invalid").
		WithParams(params).
		WithFields(shared.FieldError{Path: "/settings/" + setting, Code: "structure.setting_invalid", Params: params})
}

func levelsNotClosed(itemType ItemType) error {
	params := map[string]string{}
	if itemType != "" {
		params["item_type"] = string(itemType)
	}
	return shared.ErrValidation.
		WithDetail("structure.levels_not_closed").
		WithParams(params).
		WithFields(shared.FieldError{Path: "/settings/levels", Code: "structure.levels_not_closed", Params: params})
}
