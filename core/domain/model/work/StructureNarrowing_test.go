// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"reflect"
	"testing"
)

func projectStructure() EffectiveStructure {
	return EffectiveStructure{Values: StructureValues{
		Levels: []Level{{Type: ItemTask}, {Type: ItemWorkPackage}, {Type: ItemActivity}},
		Views:  &Views{Offered: []ViewLayout{ViewListCollapsed, ViewKanban}, Default: ViewListCollapsed},
	}}
}

func listStructure() EffectiveStructure {
	return EffectiveStructure{Values: StructureValues{
		Levels: []Level{{Type: ItemTask}},
		Fields: map[ItemType]LevelFields{
			ItemTask: {Capabilities: []Capability{CapabilityCompletion, CapabilityHistory, CapabilityAmount}},
		},
		Views: &Views{Offered: []ViewLayout{ViewListCollapsed}, Default: ViewListCollapsed},
	}}
}

// The summer party's entries: two tasks, one of them with a work package of two activities and a
// due date, the other with notes; an activity directly under the first task.
func partyEntries() []EntryShape {
	return []EntryShape{
		{Type: ItemTask, Holds: []Capability{CapabilityDueDate}},
		{Type: ItemTask, Holds: []Capability{CapabilityNotes, CapabilityDueDate}},
		{Type: ItemWorkPackage, Ancestors: []ItemType{ItemTask}, Holds: []Capability{CapabilityDueDate}},
		{Type: ItemActivity, Ancestors: []ItemType{ItemWorkPackage, ItemTask}},
		{Type: ItemActivity, Ancestors: []ItemType{ItemWorkPackage, ItemTask}},
		{Type: ItemActivity, Ancestors: []ItemType{ItemTask}},
	}
}

func TestNarrowingNamesWhatAChangeStopsShowing(t *testing.T) {
	cases := []struct {
		name          string
		before, after EffectiveStructure
		entries       []EntryShape
		want          Narrowing
	}{
		{
			name:   "project to list: deeper levels with counts, values on entries still shown, views",
			before: projectStructure(), after: listStructure(), entries: partyEntries(),
			want: Narrowing{
				Levels: []LevelNarrowing{{Type: ItemWorkPackage, Entries: 1}, {Type: ItemActivity, Entries: 3}},
				Fields: []FieldNarrowing{
					{Type: ItemTask, Capability: CapabilityDueDate, Entries: 2},
					{Type: ItemTask, Capability: CapabilityNotes, Entries: 1},
				},
				Views: []ViewLayout{ViewKanban},
			},
		},
		{
			name:   "list to project offers more and narrows nothing",
			before: listStructure(), after: projectStructure(), entries: partyEntries(),
			want: Narrowing{},
		},
		{
			name:   "dropping work packages hides the activities under them, not the one under a task",
			before: projectStructure(),
			after: EffectiveStructure{Values: StructureValues{
				Levels: []Level{{Type: ItemTask}, {Type: ItemActivity}},
				Views:  projectStructure().Values.Views,
			}},
			entries: partyEntries(),
			want: Narrowing{
				Levels: []LevelNarrowing{{Type: ItemWorkPackage, Entries: 1}, {Type: ItemActivity, Entries: 2}},
			},
		},
		{
			name:   "an empty collection narrows only its views",
			before: projectStructure(), after: listStructure(),
			want: Narrowing{Views: []ViewLayout{ViewKanban}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NarrowingOf(testBound(), tc.before, tc.after, tc.entries)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("narrowing = %+v, want %+v", got, tc.want)
			}
			if got.Narrows() != tc.want.Narrows() {
				t.Errorf("narrows = %v", got.Narrows())
			}
		})
	}
}

func TestNarrowingsOfTwoCollectionsAddUp(t *testing.T) {
	one := NarrowingOf(testBound(), projectStructure(), listStructure(), partyEntries())
	sum := one.Add(one)
	if sum.Levels[1].Entries != 6 || sum.Fields[0].Entries != 4 || len(sum.Views) != 1 {
		t.Errorf("sum = %+v", sum)
	}
	if one.Levels[1].Entries != 3 {
		t.Error("adding changed the operand")
	}
}
