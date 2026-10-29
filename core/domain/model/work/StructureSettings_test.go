// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// seededMatrix is the system matrix as db/migrations/0002 seeds it: a task holds work packages, a
// work package activities.
func seededMatrix() []CapabilityProfile {
	return []CapabilityProfile{
		{Type: ItemTask, Capabilities: []Capability{
			CapabilityCompletion, CapabilityDueDate, CapabilityReminder, CapabilityAssignment,
			CapabilityMembers, CapabilityBucket, CapabilityNotes, CapabilityLabels, CapabilityComments,
			CapabilityCover, CapabilityAttachments, CapabilityHistory, CapabilityRecurrence,
			CapabilityCustomFields,
		}, AllowedChildTypes: []ItemType{ItemWorkPackage}, MaxDepth: 3},
		{Type: ItemWorkPackage, Capabilities: []Capability{
			CapabilityCompletion, CapabilityDueDate, CapabilityReminder, CapabilityAssignment,
			CapabilityMembers, CapabilityNotes, CapabilityLabels, CapabilityComments,
			CapabilityAttachments, CapabilityHistory, CapabilityCustomFields,
		}, AllowedChildTypes: []ItemType{ItemActivity}, MaxDepth: 2},
		{Type: ItemActivity, Capabilities: []Capability{
			CapabilityCompletion, CapabilityDueDate, CapabilityReminder, CapabilityAssignment,
			CapabilityHistory,
		}, MaxDepth: 1},
	}
}

// withTaskHoldingActivities is the matrix once a task may hold an activity directly (domain-model.md
// §2): the activity then has two parents.
func withTaskHoldingActivities() []CapabilityProfile {
	matrix := seededMatrix()
	matrix[0].AllowedChildTypes = []ItemType{ItemWorkPackage, ItemActivity}
	return matrix
}

func seededBound() StructureBound { return NewStructureBound(seededMatrix(), seededMatrix()) }

func detailOf(t *testing.T, err error) string {
	t.Helper()
	refusal := shared.AsError(err)
	if refusal == nil {
		t.Fatalf("not a domain error: %v", err)
	}
	return refusal.DetailCode
}

func levelTypes(levels []Level) []ItemType {
	types := make([]ItemType, 0, len(levels))
	for _, level := range levels {
		types = append(types, level.Type)
	}
	return types
}

// The default is today's behaviour: every level and capability the installation has, every view,
// no figure, manual completion.
func TestHubtasksDefaultIsTodaysBehaviour(t *testing.T) {
	values := seededBound().Default()

	if got := levelTypes(values.Levels); !slices.Equal(got, []ItemType{ItemTask, ItemWorkPackage, ItemActivity}) {
		t.Errorf("levels %v", got)
	}
	if len(values.Fields[ItemTask].Capabilities) != 14 || len(values.Fields[ItemActivity].Capabilities) != 5 {
		t.Errorf("fields %v", values.Fields)
	}
	if values.Views.Default != ViewListCollapsed || len(values.Views.Offered) != 4 {
		t.Errorf("views %+v", values.Views)
	}
	if values.Counts.Figure != FigureNone || values.Completion != CompletionManual ||
		values.Celebration != CelebrationAll || values.Automation == nil || len(values.Automation) != 0 {
		t.Errorf("values %+v", values)
	}
	if values.EntryTypes == nil || len(values.EntryTypes) != 0 {
		t.Errorf("the default does not offer every type: %v", values.EntryTypes)
	}
	if got := len(values.Decided()); got != 9 {
		t.Errorf("the default decides %d settings, want 9", got)
	}
}

// UC-WRK-23 check 2: every preset is values for all eight settings beside the entry types, which
// name a workspace's own types and so stay the layer above's.
func TestEveryPresetDecidesAllEightSettings(t *testing.T) {
	bound := NewStructureBound(withTaskHoldingActivities(), withTaskHoldingActivities())
	for _, preset := range BuiltInPresets() {
		offered := preset.Offered(bound)
		if got := len(offered.Values.Decided()); got != 8 || offered.Values.Has(SettingEntryTypes) {
			t.Errorf("%s decides %d settings", preset.Key, got)
		}
		if _, err := bound.Validate(offered.Values, nil); err != nil {
			t.Errorf("%s is not valid in its own installation: %v", preset.Key, err)
		}
	}
}

// A list has no date, person, label, note editor or child (UC-WRK-23 check 4) and offers no board
// or timeline.
func TestAListOffersATitleAndNothingElse(t *testing.T) {
	list, err := FindPreset(PresetList, seededBound())
	if err != nil {
		t.Fatal(err)
	}
	if got := levelTypes(list.Values.Levels); !slices.Equal(got, []ItemType{ItemTask}) {
		t.Errorf("levels %v", got)
	}
	for _, absent := range []Capability{
		CapabilityDueDate, CapabilityAssignment, CapabilityMembers, CapabilityLabels, CapabilityNotes,
	} {
		if slices.Contains(list.Values.Fields[ItemTask].Capabilities, absent) {
			t.Errorf("a list entry carries %s", absent)
		}
	}
	if slices.Contains(list.Values.Views.Offered, ViewKanban) || slices.Contains(list.Values.Views.Offered, ViewTimeline) {
		t.Errorf("a list offers %v", list.Values.Views.Offered)
	}
}

// Tasks offers an activity under a task only where the installation's task holds one.
func TestTasksOffersStepsOnlyWhereATaskHoldsThem(t *testing.T) {
	before, _ := FindPreset(PresetTasks, seededBound())
	if got := levelTypes(before.Values.Levels); !slices.Equal(got, []ItemType{ItemTask}) {
		t.Errorf("before: %v", got)
	}
	after, _ := FindPreset(PresetTasks, NewStructureBound(withTaskHoldingActivities(), withTaskHoldingActivities()))
	if got := levelTypes(after.Values.Levels); !slices.Equal(got, []ItemType{ItemTask, ItemActivity}) {
		t.Errorf("after: %v", got)
	}
}

func TestAnUnknownPresetIsRefused(t *testing.T) {
	_, err := FindPreset("SHOPPING", seededBound())
	if detailOf(t, err) != "presets.unknown" {
		t.Errorf("refused as %v", err)
	}
}

// UC-WRK-23 check 5: a setting narrows what the installation allows and never widens it.
func TestASettingNeverWidensTheInstallation(t *testing.T) {
	narrowed := seededMatrix()
	narrowed[0].Capabilities = slices.DeleteFunc(narrowed[0].Capabilities,
		func(c Capability) bool { return c == CapabilityDueDate })
	bound := NewStructureBound(narrowed, seededMatrix())

	cases := []struct {
		name   string
		values StructureValues
		detail string
		kind   *shared.Error
	}{
		{"a capability the installation narrowed away", StructureValues{Fields: map[ItemType]LevelFields{
			ItemTask: {Capabilities: []Capability{CapabilityCompletion, CapabilityDueDate}},
		}}, "structure.widens", shared.ErrCapabilityNotSupported},
		{"a capability the type never had", StructureValues{Fields: map[ItemType]LevelFields{
			ItemActivity: {Capabilities: []Capability{CapabilityCompletion, CapabilityNotes}},
		}}, "structure.widens", shared.ErrCapabilityNotSupported},
		{"a type the installation lacks", StructureValues{Levels: []Level{{Type: ItemTask}, {Type: "MILESTONE"}}},
			"structure.widens", shared.ErrCapabilityNotSupported},
		{"a behaviour this installation does not have", StructureValues{Automation: []AutomationBehaviour{"DONE_COLUMN_COMPLETES"}},
			"structure.widens", shared.ErrCapabilityNotSupported},
		{"a behaviour twice", StructureValues{Automation: []AutomationBehaviour{
			AutomationRequiredBeyondFirstColumn, AutomationRequiredBeyondFirstColumn,
		}}, "structure.setting_invalid", shared.ErrValidation},
		{"a level without its parent", StructureValues{Levels: []Level{{Type: ItemTask}, {Type: ItemActivity}}},
			"structure.levels_not_closed", shared.ErrValidation},
		{"no top level", StructureValues{Levels: []Level{{Type: ItemWorkPackage}}},
			"structure.levels_not_closed", shared.ErrValidation},
		{"a level twice", StructureValues{Levels: []Level{{Type: ItemTask}, {Type: ItemTask}}},
			"structure.setting_invalid", shared.ErrValidation},
		{"an entry that cannot be done", StructureValues{Fields: map[ItemType]LevelFields{
			ItemTask: {Capabilities: []Capability{CapabilityNotes}},
		}}, "structure.setting_invalid", shared.ErrValidation},
		{"a default view not offered", StructureValues{Views: &Views{
			Offered: []ViewLayout{ViewListCollapsed}, Default: ViewKanban,
		}}, "structure.setting_invalid", shared.ErrValidation},
		{"a sum of nothing", StructureValues{Counts: &Counts{Figure: FigureFieldSum}},
			"structure.setting_invalid", shared.ErrValidation},
		{"a key beside a figure that takes none", StructureValues{Counts: &Counts{Figure: FigureOpen, FieldKey: "hours"}},
			"structure.setting_invalid", shared.ErrValidation},
		{"an unknown capture", StructureValues{Capture: []CaptureToken{"PRICE"}},
			"structure.setting_invalid", shared.ErrValidation},
		{"an unknown celebration", StructureValues{Celebration: "CONFETTI"},
			"structure.setting_invalid", shared.ErrValidation},
		{"an unknown completion", StructureValues{Completion: "AUTOMATIC"},
			"structure.setting_invalid", shared.ErrValidation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := bound.Validate(tc.values, nil)
			if err == nil {
				t.Fatal("accepted")
			}
			if !errors.Is(err, tc.kind) || detailOf(t, err) != tc.detail {
				t.Errorf("refused as %v, want %s", err, tc.detail)
			}
		})
	}
}

// An activity has two parents once a task holds one: either offered is enough (any, not all).
func TestAnActivityNeedsAnyOfItsParents(t *testing.T) {
	bound := NewStructureBound(withTaskHoldingActivities(), withTaskHoldingActivities())
	values, err := bound.Validate(StructureValues{Levels: []Level{{Type: ItemActivity}, {Type: ItemTask}}}, nil)
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if got := levelTypes(values.Levels); !slices.Equal(got, []ItemType{ItemTask, ItemActivity}) {
		t.Errorf("stored top-down as %v", got)
	}
}

type composing struct{}

func (composing) NFC(text string) string  { return strings.ReplaceAll(text, "é", "é") }
func (composing) NFKC(text string) string { return text }

// UC-WRK-23 check 6: a level's name is the person's, normalised and bounded; the refusal never
// carries it.
func TestALevelNameIsNormalisedAndBounded(t *testing.T) {
	bound := seededBound()

	values, err := bound.Validate(StructureValues{Levels: []Level{{Type: ItemTask, Name: "  Caf" + "é run "}}}, composing{})
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if values.Levels[0].Name != "Café run" {
		t.Errorf("stored as %q", values.Levels[0].Name)
	}

	for _, name := range []string{"   ", strings.Repeat("x", MaxLevelNameLength+1)} {
		_, err := bound.Validate(StructureValues{Levels: []Level{{Type: ItemTask, Name: name}}}, nil)
		refusal := shared.AsError(err)
		if refusal == nil || refusal.DetailCode != "structure.level_name_invalid" {
			t.Fatalf("refused as %v", err)
		}
		for _, value := range refusal.Params {
			if strings.Contains(value, "xxxx") {
				t.Error("the refusal carries the name")
			}
		}
	}
}

// Values are inherited setting by setting: a hub's value applies below unless the collection has
// its own, and a collection may widen what its hub narrowed - only the installation and a lock
// bind it.
func TestASettingIsInheritedOnItsOwn(t *testing.T) {
	bound := seededBound()
	list, _ := FindPreset(PresetList, bound)
	hub := LayerAt{Source: identity.SourceHub, Layer: StructureLayer{Preset: PresetList, Values: list.Values}}
	collection := LayerAt{Source: identity.SourceCollection, Layer: StructureLayer{Values: StructureValues{
		Fields: map[ItemType]LevelFields{ItemTask: {Capabilities: []Capability{
			CapabilityCompletion, CapabilityDueDate, CapabilityHistory, CapabilityCustomFields,
		}}},
	}}}

	effective := ResolveStructure(bound, WorkspaceStructure{}, hub, collection)

	if !slices.Contains(effective.Values.Fields[ItemTask].Capabilities, CapabilityDueDate) {
		t.Error("the collection's own dates are not in force")
	}
	if effective.SourceOf(SettingFields) != identity.SourceCollection {
		t.Errorf("fields from %s", effective.SourceOf(SettingFields))
	}
	for _, setting := range []StructureSetting{SettingLevels, SettingViews, SettingCounts, SettingCapture} {
		if effective.SourceOf(setting) != identity.SourceHub {
			t.Errorf("%s from %s, want HUB", setting, effective.SourceOf(setting))
		}
	}
	if effective.Preset != PresetList {
		t.Errorf("provenance %q", effective.Preset)
	}

	untouched := ResolveStructure(bound, WorkspaceStructure{})
	for _, setting := range StructureSettingNames() {
		if untouched.SourceOf(setting) != identity.SourceDefault {
			t.Errorf("%s from %s on an untouched container", setting, untouched.SourceOf(setting))
		}
	}
}

// A collection made with its hub's preset stores nothing and follows the hub when it switches.
func TestACollectionMadeWithTheHubsPresetFollowsALaterHubSwitch(t *testing.T) {
	bound := seededBound()
	list, _ := FindPreset(PresetList, bound)
	project, _ := FindPreset(PresetProject, bound)
	collection := LayerAt{Source: identity.SourceCollection} // made with the hub's preset: no values

	before := ResolveStructure(bound, WorkspaceStructure{},
		LayerAt{Source: identity.SourceHub, Layer: StructureLayer{Preset: PresetList, Values: list.Values}}, collection)
	after := ResolveStructure(bound, WorkspaceStructure{},
		LayerAt{Source: identity.SourceHub, Layer: StructureLayer{Preset: PresetProject, Values: project.Values}}, collection)

	if got := levelTypes(before.Values.Levels); !slices.Equal(got, []ItemType{ItemTask}) {
		t.Errorf("before %v", got)
	}
	if got := levelTypes(after.Values.Levels); len(got) != 3 || after.Values.Completion != CompletionRollup {
		t.Errorf("after %v, %s", got, after.Values.Completion)
	}
}

// A value stored before the installation narrowed is shown narrowed, not refused on read.
func TestTheInstallationNarrowedAfterTheCollectionStoredDueDates(t *testing.T) {
	narrowed := seededMatrix()
	narrowed[0].Capabilities = slices.DeleteFunc(narrowed[0].Capabilities,
		func(c Capability) bool { return c == CapabilityDueDate })
	bound := NewStructureBound(narrowed, seededMatrix())

	stored := LayerAt{Source: identity.SourceCollection, Layer: StructureLayer{Values: StructureValues{
		Fields: map[ItemType]LevelFields{ItemTask: {Capabilities: []Capability{CapabilityCompletion, CapabilityDueDate}}},
	}}}
	effective := ResolveStructure(bound, WorkspaceStructure{
		Locks: map[LockKey]shared.ID{"fields.TASK.DUE_DATE": shared.MustParseID("018f0000-0000-7000-8000-0000000000a2")},
	}, stored)

	if slices.Contains(effective.Values.Fields[ItemTask].Capabilities, CapabilityDueDate) {
		t.Error("a date the installation no longer offers is in force")
	}
	if _, err := ParseLockKey("fields.TASK.DUE_DATE", bound); err == nil {
		t.Error("a lock on a capability the installation lacks is accepted")
	}
}

// A workspace lock fixes the value for every layer below; a lower layer's own value is ignored
// when resolving, refused when written, and kept aside when a preset is chosen.
func TestALockBindsEveryLayerBelow(t *testing.T) {
	bound := seededBound()
	admin := shared.MustParseID("018f0000-0000-7000-8000-0000000000a1")
	workspace := WorkspaceStructure{
		Layer: StructureLayer{Values: StructureValues{Views: &Views{
			Offered: []ViewLayout{ViewKanban}, Default: ViewKanban,
		}}},
		Locks: map[LockKey]shared.ID{"views": admin, "fields.TASK.DUE_DATE": admin},
	}
	list, _ := FindPreset(PresetList, bound)
	collection := LayerAt{Source: identity.SourceCollection, Layer: StructureLayer{Values: list.Values}}

	effective := ResolveStructure(bound, workspace, collection)

	if effective.Values.Views.Default != ViewKanban || effective.SourceOf(SettingViews) != identity.SourceWorkspace {
		t.Errorf("views %+v from %s", effective.Values.Views, effective.SourceOf(SettingViews))
	}
	if !slices.Contains(effective.Values.Fields[ItemTask].Capabilities, CapabilityDueDate) {
		t.Error("the locked date is off")
	}
	if lock := effective.Locks["views"]; lock.Origin != identity.LockWorkspace || lock.LockedBy != admin {
		t.Errorf("lock %+v", lock)
	}

	above := ResolveStructure(bound, workspace)
	if broken := LockedAgainst(bound, above, list.Values); !slices.Equal(broken, []LockKey{"fields.TASK.DUE_DATE", "views"}) {
		t.Errorf("broken %v", broken)
	}
	kept, keptBy := KeepLocked(bound, above, list.Values)
	if kept.Has(SettingViews) || !slices.Contains(kept.Fields[ItemTask].Capabilities, CapabilityDueDate) {
		t.Errorf("kept %+v", kept)
	}
	if len(keptBy) != 2 {
		t.Errorf("kept by %v", keptBy)
	}
	if err := ErrSettingLocked("views", above.Locks["views"]); detailOf(t, err) != "structure.setting_locked" {
		t.Errorf("refused as %v", err)
	}
}

func TestALockKeyIsASettingOrATypesCapability(t *testing.T) {
	bound := seededBound()
	for _, good := range []string{"views", "levels", "fields.TASK.DUE_DATE", "fields.ACTIVITY.REMINDER"} {
		if _, err := ParseLockKey(good, bound); err != nil {
			t.Errorf("%s refused: %v", good, err)
		}
	}
	for _, bad := range []string{"", "colour", "fields.TASK", "fields.ACTIVITY.NOTES", "fields.MILESTONE.DUE_DATE"} {
		if _, err := ParseLockKey(bad, bound); err == nil || detailOf(t, err) != "structure.lock_unknown" {
			t.Errorf("%q accepted or refused as %v", bad, err)
		}
	}
}

func TestMovedNamesTheSettingsThatDiffer(t *testing.T) {
	bound := seededBound()
	list, _ := FindPreset(PresetList, bound)
	changed := list.Values
	changed.Celebration = CelebrationAll
	changed = changed.Without(SettingCapture)
	if got := list.Values.Moved(changed); !slices.Equal(got, []StructureSetting{SettingCapture, SettingCelebration}) {
		t.Errorf("moved %v", got)
	}
	if got := list.Values.Moved(list.Values); len(got) != 0 {
		t.Errorf("moved %v against itself", got)
	}
}

// UC-FLW-01 check 3: the types a level offers are held to their shape, never beyond the bound's
// levels, inherited on their own and clipped where the installation dropped a level.
func TestTheEntryTypesALevelOffers(t *testing.T) {
	bound := seededBound()

	valid, err := bound.Validate(StructureValues{EntryTypes: map[ItemType][]string{ItemTask: {"bug", "task"}}}, nil)
	if err != nil || !slices.Equal(valid.EntryTypes[ItemTask], []string{"bug", "task"}) {
		t.Fatalf("a valid offer was not kept in its order: %v, %v", valid.EntryTypes, err)
	}
	refused := []struct {
		name    string
		offered map[ItemType][]string
		code    string
	}{
		{"an empty list", map[ItemType][]string{ItemTask: {}}, "structure.setting_invalid"},
		{"a key twice", map[ItemType][]string{ItemTask: {"bug", "bug"}}, "structure.setting_invalid"},
		{"not a key", map[ItemType][]string{ItemTask: {"Bug Report"}}, "structure.setting_invalid"},
		{"a level the installation lacks", map[ItemType][]string{ItemType("EPIC"): {"bug"}}, "structure.widens"},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			_, err := bound.Validate(StructureValues{EntryTypes: tc.offered}, nil)
			if err == nil || detailOf(t, err) != tc.code {
				t.Fatalf("want %s, got %v", tc.code, err)
			}
		})
	}

	hub := StructureValues{EntryTypes: map[ItemType][]string{ItemTask: {"bug"}}}
	collection := StructureValues{Celebration: CelebrationSubtle}
	inForce := bound.Default().Overlay(hub).Overlay(collection)
	if !slices.Equal(inForce.EntryTypes[ItemTask], []string{"bug"}) {
		t.Errorf("the hub's offer did not reach the collection: %v", inForce.EntryTypes)
	}
	if hub.SameSetting(SettingEntryTypes, StructureValues{EntryTypes: map[ItemType][]string{ItemTask: {"task", "bug"}}}) {
		t.Error("an offer in another order reads as the same")
	}
	if got := hub.Without(SettingEntryTypes); got.Has(SettingEntryTypes) {
		t.Error("the offer was not left to the layer above")
	}

	narrowed := NewStructureBound(seededMatrix()[:2], seededMatrix())
	clipped := narrowed.Clip(StructureValues{EntryTypes: map[ItemType][]string{ItemActivity: {"call"}, ItemTask: {"bug"}}})
	if _, kept := clipped.EntryTypes[ItemActivity]; kept || len(clipped.EntryTypes[ItemTask]) != 1 {
		t.Errorf("a level the installation dropped kept its offer: %v", clipped.EntryTypes)
	}
}
