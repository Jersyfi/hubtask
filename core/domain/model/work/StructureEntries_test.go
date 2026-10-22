// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// testBound is the installation's matrix as domain-model.md §2 ships it. The shared table with the
// client is read by the application's test (Hidden_test.go), since the domain does not decode.
func testBound() StructureBound {
	profiles := []CapabilityProfile{
		{Type: ItemTask, Capabilities: []Capability{
			CapabilityCompletion, CapabilityDueDate, CapabilityReminder, CapabilityAssignment,
			CapabilityMembers, CapabilityBucket, CapabilityNotes, CapabilityLabels, CapabilityComments,
			CapabilityCover, CapabilityAttachments, CapabilityHistory, CapabilityRecurrence,
			CapabilityCustomFields,
		}, AllowedChildTypes: []ItemType{ItemWorkPackage, ItemActivity}, MaxDepth: 3},
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
	return NewStructureBound(profiles, profiles)
}

func TestAnActivityUnderAHiddenWorkPackageIsHiddenThoughItsTypeIsOffered(t *testing.T) {
	bound := testBound()
	offered := NewOffered(bound, EffectiveStructure{Values: StructureValues{
		Levels: []Level{{Type: ItemTask}, {Type: ItemActivity}},
	}})
	if offered.Hidden(ItemActivity, []ItemType{ItemTask}).Level {
		t.Error("an activity directly under a task is hidden")
	}
	if !offered.Hidden(ItemActivity, []ItemType{ItemWorkPackage, ItemTask}).Level {
		t.Error("an activity under a work package is shown although the work package is hidden")
	}
}

func listOffered(t *testing.T) (Offered, StructureBound) {
	t.Helper()
	bound := testBound()
	values := StructureValues{
		Levels: []Level{{Type: ItemTask}},
		Fields: map[ItemType]LevelFields{
			ItemTask: {Capabilities: []Capability{CapabilityCompletion, CapabilityHistory, CapabilityCustomFields}},
		},
	}
	return NewOffered(bound, EffectiveStructure{Values: values}), bound
}

func TestAHiddenValueIsRefusedAsHiddenAndAMissingOneAsUnsupported(t *testing.T) {
	offered, bound := listOffered(t)
	task, _ := bound.Profile(ItemTask)
	narrowed := offered.Narrow(task)

	err := narrowed.Require(CapabilityDueDate, "/due_at")
	var refusal *shared.Error
	if !errors.As(err, &refusal) || !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Fatalf("a hidden due date answered %v, want capability_not_supported", err)
	}
	if refusal.DetailCode != "items.capability_hidden" {
		t.Errorf("detail = %q, want items.capability_hidden", refusal.DetailCode)
	}
	if refusal.Params["capability"] != "DUE_DATE" || refusal.Params["item_type"] != "TASK" {
		t.Errorf("params = %v, want the type and the capability only", refusal.Params)
	}

	activity, _ := bound.Profile(ItemActivity)
	err = offered.Narrow(activity).Require(CapabilityNotes, "/notes")
	if !errors.As(err, &refusal) || refusal.DetailCode != "items.capability_not_supported" {
		t.Errorf("notes on an activity answered %v, want the installation's items.capability_not_supported", err)
	}
}

func TestCompletionIsNeverHidden(t *testing.T) {
	bound := testBound()
	values := StructureValues{
		Levels: []Level{{Type: ItemTask}},
		Fields: map[ItemType]LevelFields{ItemTask: {Capabilities: []Capability{CapabilityHistory}}},
	}
	offered := NewOffered(bound, EffectiveStructure{Values: values})
	task, _ := bound.Profile(ItemTask)
	if err := offered.Narrow(task).Require(CapabilityCompletion, "/completion"); err != nil {
		t.Errorf("completion refused: %v", err)
	}
	if slices.Contains(offered.Hidden(ItemTask, nil).Capabilities, CapabilityCompletion) {
		t.Error("completion is marked hidden")
	}
}

// The history keeps the installation's form: a task's history does not turn compact because its
// collection stopped offering notes, labels, comments and fields.
func TestTheHistoryFormIgnoresTheStructure(t *testing.T) {
	bound := testBound()
	values := StructureValues{
		Levels: []Level{{Type: ItemTask}},
		Fields: map[ItemType]LevelFields{ItemTask: {Capabilities: []Capability{CapabilityCompletion}}},
	}
	task, _ := bound.Profile(ItemTask)
	narrowed := NewOffered(bound, EffectiveStructure{Values: values}).Narrow(task)
	if narrowed.HistoryIsCompact() {
		t.Error("a task's history turned compact under a narrower structure")
	}
	activity, _ := bound.Profile(ItemActivity)
	if !OfferedByBound(bound).Narrow(activity).HistoryIsCompact() {
		t.Error("an activity's history is no longer compact")
	}
}

func TestTheBoundOffersEverything(t *testing.T) {
	bound := testBound()
	offered := OfferedByBound(bound)
	for _, itemType := range bound.Types() {
		got := offered.Hidden(itemType, []ItemType{ItemWorkPackage, ItemTask})
		if got.Level || len(got.Capabilities) != 0 {
			t.Errorf("%s: the bound hides %+v", itemType, got)
		}
	}
}

func TestALevelRefusalNamesTheTypeAndTheField(t *testing.T) {
	offered, _ := listOffered(t)
	err := offered.RequireLevel(ItemWorkPackage, []ItemType{ItemTask}, "/type")
	var refusal *shared.Error
	if !errors.As(err, &refusal) || !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Fatalf("answered %v, want capability_not_supported", err)
	}
	if refusal.DetailCode != "items.level_hidden" || refusal.Params["item_type"] != "WORK_PACKAGE" {
		t.Errorf("refusal = %s %v", refusal.DetailCode, refusal.Params)
	}
	if len(refusal.Fields) != 1 || refusal.Fields[0].Path != "/type" {
		t.Errorf("fields = %v, want /type", refusal.Fields)
	}
	// A type the installation does not hold is the installation's question, not the structure's.
	if err := offered.RequireLevel(ItemType("MILESTONE"), nil, ""); err != nil {
		t.Errorf("an unknown type answered %v", err)
	}
}

// Counted asks Hidden's level question the way a statement over an entry's path does: an entry is
// shown exactly where its type and every type above it are counted. Every combination of levels,
// under the full bound and under one that no longer holds the activity, over every chain the
// matrix allows.
func TestCountedTypesAnswerWhatHiddenAnswers(t *testing.T) {
	full := testBound()
	profiles := []CapabilityProfile{}
	for _, itemType := range []ItemType{ItemTask, ItemWorkPackage} {
		profile, _ := full.Profile(itemType)
		profile.AllowedChildTypes = slices.DeleteFunc(slices.Clone(profile.AllowedChildTypes),
			func(child ItemType) bool { return child == ItemActivity })
		profiles = append(profiles, profile)
	}
	withoutActivities := NewStructureBound(profiles, profiles)

	chains := [][]ItemType{
		{ItemTask}, {ItemWorkPackage, ItemTask}, {ItemActivity, ItemTask},
		{ItemActivity, ItemWorkPackage, ItemTask},
	}
	for _, bound := range []StructureBound{full, withoutActivities} {
		for mask := 1; mask < 8; mask++ {
			var levels []Level
			for i, itemType := range ItemTypes() {
				if mask&(1<<i) != 0 {
					levels = append(levels, Level{Type: itemType})
				}
			}
			offered := NewOffered(bound, EffectiveStructure{Values: StructureValues{Levels: levels}})
			counted := offered.Counted()
			for _, chain := range chains {
				shown := true
				for _, itemType := range chain {
					shown = shown && slices.Contains(counted, itemType)
				}
				if hidden := offered.Hidden(chain[0], chain[1:]).Level; hidden == shown {
					t.Errorf("levels %v, chain %v: Hidden says %v, Counted shows %v", levels, chain, hidden, shown)
				}
			}
		}
	}
}
