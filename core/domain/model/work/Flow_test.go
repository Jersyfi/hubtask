// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-WRK-31 check 1: Flow opens on the board, keeps its states as the board's columns, and lets an
// entry past the first column only with its required fields - a value that survives the
// installation's bound and is accepted on a write.
func TestFlowOpensOnTheBoardAndGuardsItsFirstColumn(t *testing.T) {
	bound := seededBound()
	flow, err := FindPreset(PresetFlow, bound)
	if err != nil {
		t.Fatalf("Flow: %v", err)
	}
	values := flow.Values
	if values.Views == nil || values.Views.Default != ViewKanban {
		t.Errorf("Flow opens on %+v", values.Views)
	}
	if !slices.Contains(values.Fields[ItemTask].Capabilities, CapabilityBucket) {
		t.Error("Flow's entries carry no column")
	}
	if values.Counts == nil || values.Counts.Figure != FigureFlow {
		t.Errorf("Flow counts %+v", values.Counts)
	}
	if !slices.Equal(values.Automation, []AutomationBehaviour{AutomationRequiredBeyondFirstColumn}) {
		t.Errorf("Flow applies %v", values.Automation)
	}
	if _, err := bound.Validate(values, nil); err != nil {
		t.Errorf("Flow's own values are refused: %v", err)
	}
	for _, preset := range []PresetKey{PresetList, PresetTasks, PresetProject} {
		other, _ := FindPreset(preset, bound)
		if len(other.Values.Automation) != 0 {
			t.Errorf("%s applies %v", preset, other.Values.Automation)
		}
	}
	// A value a later build no longer declares is dropped on read, a declared one kept once.
	clipped := bound.Clip(StructureValues{Automation: []AutomationBehaviour{
		"GONE", AutomationRequiredBeyondFirstColumn, AutomationRequiredBeyondFirstColumn,
	}})
	if !slices.Equal(clipped.Automation, []AutomationBehaviour{AutomationRequiredBeyondFirstColumn}) {
		t.Errorf("clipped to %v", clipped.Automation)
	}
}

func TestTheFirstColumnIsTheLeftmostOnTheBoard(t *testing.T) {
	gone := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	removed := Bucket{ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000a1"), DeletedAt: &gone}
	first := Bucket{ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000a2")}
	second := Bucket{ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000a3")}

	cases := []struct {
		name   string
		board  []Bucket
		bucket shared.ID
		want   bool
	}{
		{"the leftmost", []Bucket{first, second}, first.ID, true},
		{"one to its right", []Bucket{first, second}, second.ID, false},
		{"after a deleted one", []Bucket{removed, first, second}, first.ID, true},
		{"a deleted one is not first", []Bucket{removed, first}, removed.ID, false},
		{"an empty board", nil, second.ID, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FirstColumn(tc.board, tc.bucket); got != tc.want {
				t.Errorf("FirstColumn = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTheRequiredFieldsAnEntryLacks(t *testing.T) {
	collection := shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")
	gone := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	definition := func(key string, required bool, scope shared.ID, types ...ItemType) CustomFieldDefinition {
		if len(types) == 0 {
			types = []ItemType{ItemTask}
		}
		return CustomFieldDefinition{Key: key, IsRequired: required, CollectionID: scope, AppliesTo: types}
	}
	deleted := definition("phone", true, collection)
	deleted.DeletedAt = &gone

	cases := []struct {
		name        string
		definitions []CustomFieldDefinition
		values      map[string]any
		want        []string
	}{
		{"nothing required", []CustomFieldDefinition{definition("customer", false, collection)}, nil, nil},
		{"a required field set", []CustomFieldDefinition{definition("customer", true, collection)},
			map[string]any{"customer": "ACME"}, nil},
		{"a required field missing", []CustomFieldDefinition{definition("customer", true, collection)}, nil,
			[]string{"customer"}},
		{"an empty text is no value", []CustomFieldDefinition{definition("customer", true, collection)},
			map[string]any{"customer": "  "}, []string{"customer"}},
		{"an empty choice is no value", []CustomFieldDefinition{definition("tags", true, collection)},
			map[string]any{"tags": []any{}}, []string{"tags"}},
		{"false is a value", []CustomFieldDefinition{definition("signed", true, collection)},
			map[string]any{"signed": false}, nil},
		{"zero is a value", []CustomFieldDefinition{definition("hours", true, collection)},
			map[string]any{"hours": float64(0)}, nil},
		{"several, sorted", []CustomFieldDefinition{
			definition("zone", true, collection), definition("customer", true, shared.ID("")),
		}, nil, []string{"customer", "zone"}},
		{"a deleted definition asks nothing", []CustomFieldDefinition{deleted}, nil, nil},
		{"another type's field asks nothing", []CustomFieldDefinition{
			definition("customer", true, collection, ItemActivity),
		}, nil, nil},
		{"the collection's own wins over the workspace's", []CustomFieldDefinition{
			definition("customer", true, shared.ID("")), definition("customer", false, collection),
		}, nil, nil},
		{"whichever order they arrive in", []CustomFieldDefinition{
			definition("customer", false, collection), definition("customer", true, shared.ID("")),
		}, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MissingRequiredFields(tc.definitions, WorkItem{Type: ItemTask, CustomFields: tc.values})
			if !slices.Equal(got, tc.want) {
				t.Errorf("missing %v, want %v", got, tc.want)
			}
		})
	}
}

// The refusal names every missing field, so that a form can mark each one (UC-WRK-31 check 2).
func TestTheRefusalNamesTheFields(t *testing.T) {
	err := RequiredFieldsMissing([]string{"customer", "phone"})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("refused as %v", err)
	}
	problem := shared.AsError(err)
	if problem.DetailCode != "items.required_fields_missing" || problem.Params["keys"] != "customer, phone" ||
		problem.Params["count"] != "2" {
		t.Errorf("refused with %s %v", problem.DetailCode, problem.Params)
	}
	if len(problem.Fields) != 2 || problem.Fields[0].Path != "/custom_fields/customer" ||
		problem.Fields[1].Code != "fields.value_required" {
		t.Errorf("fields %+v", problem.Fields)
	}
}
