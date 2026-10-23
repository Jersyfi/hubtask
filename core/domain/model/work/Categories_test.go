// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"slices"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// A type's categories (UC-WRK-25 check 5) and a level's done section name (check 3), held through
// every place that builds a LevelFields or a Level: the clone, the comparison, Hubtask's default,
// the clip, the validation and the lock.

var (
	dairy   = shared.MustParseID("0192f000-0000-7000-8000-0000000000e1")
	produce = shared.MustParseID("0192f000-0000-7000-8000-0000000000e2")
)

func labelled(categories ...shared.ID) LevelFields {
	return LevelFields{
		Capabilities: []Capability{CapabilityCompletion, CapabilityLabels},
		Categories:   categories,
	}
}

func TestCloneFieldsCopiesTheCategories(t *testing.T) {
	original := map[ItemType]LevelFields{ItemTask: labelled(dairy, produce)}
	clone := cloneFields(original)
	clone[ItemTask].Categories[0] = produce
	if original[ItemTask].Categories[0] != dairy {
		t.Error("the clone shares the categories with the original")
	}
}

func TestTheOrderOfCategoriesIsPartOfTheSetting(t *testing.T) {
	a := StructureValues{Fields: map[ItemType]LevelFields{ItemTask: labelled(dairy, produce)}}
	b := StructureValues{Fields: map[ItemType]LevelFields{ItemTask: labelled(produce, dairy)}}
	if a.SameSetting(SettingFields, b) {
		t.Error("two orders of the same categories compare as one")
	}
	if !a.SameSetting(SettingFields, StructureValues{Fields: cloneFields(a.Fields)}) {
		t.Error("the same categories compare as different")
	}
	if got := a.Moved(b); !slices.Equal(got, []StructureSetting{SettingFields}) {
		t.Errorf("moved %v", got)
	}
}

func TestHubtasksDefaultAndThePresetsNameNoCategories(t *testing.T) {
	for itemType, fields := range seededBound().Default().Fields {
		if len(fields.Categories) != 0 {
			t.Errorf("the default names categories for %s", itemType)
		}
	}
	for _, preset := range BuiltInPresets() {
		for itemType, fields := range preset.Values.Fields {
			if len(fields.Categories) != 0 {
				t.Errorf("%s names categories for %s", preset.Key, itemType)
			}
		}
	}
}

func TestTheClipKeepsCategoriesOnlyBesideLabels(t *testing.T) {
	bound := seededBound()
	kept := bound.Clip(StructureValues{Fields: map[ItemType]LevelFields{ItemTask: labelled(dairy)}})
	if !slices.Equal(kept.Fields[ItemTask].Categories, []shared.ID{dairy}) {
		t.Errorf("kept %v", kept.Fields[ItemTask].Categories)
	}

	// The installation stopped offering labels on an activity: an activity's categories go with them.
	dropped := bound.Clip(StructureValues{Fields: map[ItemType]LevelFields{ItemActivity: labelled(dairy)}})
	if len(dropped.Fields[ItemActivity].Categories) != 0 {
		t.Errorf("categories survived without labels: %v", dropped.Fields[ItemActivity].Categories)
	}
	if len(dropped.Fields[ItemTask].Categories) != 0 {
		t.Error("a type the layer left out gained categories")
	}
}

func TestCategoriesAreValidatedForTheirShape(t *testing.T) {
	bound := seededBound()
	many := make([]shared.ID, MaxCategories+1)
	for i := range many {
		many[i] = shared.ID("0192f000-0000-7000-8000-0000000001" + string(rune('a'+i/26)) + string(rune('a'+i%26)))
	}
	cases := map[string]struct {
		fields LevelFields
		want   string
	}{
		"beside labels":         {fields: labelled(dairy, produce)},
		"without labels":        {fields: LevelFields{Capabilities: []Capability{CapabilityCompletion}, Categories: []shared.ID{dairy}}, want: "structure.setting_invalid"},
		"twice":                 {fields: labelled(dairy, dairy), want: "structure.setting_invalid"},
		"not an identifier":     {fields: labelled("milk"), want: "structure.setting_invalid"},
		"over the bound":        {fields: labelled(many...), want: "structure.setting_invalid"},
		"none beside no labels": {fields: LevelFields{Capabilities: []Capability{CapabilityCompletion}}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			validated, err := bound.Validate(StructureValues{Fields: map[ItemType]LevelFields{ItemTask: c.fields}}, nil)
			if c.want != "" {
				if err == nil || detailOf(t, err) != c.want {
					t.Fatalf("error %v, want %s", err, c.want)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(validated.Fields[ItemTask].Categories, c.fields.Categories) {
				t.Errorf("validated %v", validated.Fields[ItemTask].Categories)
			}
		})
	}
}

func TestALockTurningLabelsOffLeavesNoCategories(t *testing.T) {
	bound := seededBound()
	admin := shared.MustParseID("018f0000-0000-7000-8000-0000000000a1")
	workspace := WorkspaceStructure{
		Layer: StructureLayer{Values: StructureValues{Fields: map[ItemType]LevelFields{
			ItemTask: {Capabilities: []Capability{CapabilityCompletion}},
		}}},
		Locks: map[LockKey]shared.ID{"fields.TASK.LABELS": admin},
	}
	collection := LayerAt{Source: identity.SourceCollection, Layer: StructureLayer{Values: StructureValues{
		Fields: map[ItemType]LevelFields{ItemTask: labelled(dairy)},
	}}}
	effective := ResolveStructure(bound, workspace, collection)
	if slices.Contains(effective.Values.Fields[ItemTask].Capabilities, CapabilityLabels) ||
		len(effective.Values.Fields[ItemTask].Categories) != 0 {
		t.Errorf("fields in force %+v", effective.Values.Fields[ItemTask])
	}
}

// UC-WRK-25 check 3: the done section's name is a level's name in all but meaning - normalised,
// bounded, kept beside the level's own.
func TestADoneSectionsNameIsALevelName(t *testing.T) {
	bound := seededBound()
	levels := func(done string) StructureValues {
		return StructureValues{Levels: []Level{{Type: ItemTask, Name: "item", DoneName: done}}}
	}
	validated, err := bound.Validate(levels(" In der Tüte "), text.Composing{})
	if err != nil {
		t.Fatal(err)
	}
	if got := validated.Levels[0]; got.DoneName != "In der Tüte" || got.Name != "item" {
		t.Errorf("validated %+v", got)
	}
	if _, err := bound.Validate(levels(strings.Repeat("x", MaxLevelNameLength+1)), text.Composing{}); err == nil ||
		detailOf(t, err) != "structure.level_name_invalid" {
		t.Errorf("a long done name answered %v", err)
	}
	if levels("In the trolley").SameSetting(SettingLevels, levels("Done")) {
		t.Error("two done names compare as one")
	}
	clipped := bound.Clip(levels("In the trolley"))
	if clipped.Levels[0].DoneName != "In the trolley" {
		t.Errorf("the clip lost the done name: %+v", clipped.Levels)
	}
}
