// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"reflect"
	"testing"
)

// UC-WRK-31 check 1: the preset *Flow* brings its states as columns, with a limit and a column that
// completes; *Project* brings the columns without a limit; a preset without a board brings none.
func TestANewCollectionStartsWithTheColumnsItsBoardNeeds(t *testing.T) {
	presets := map[PresetKey]Preset{}
	for _, preset := range BuiltInPresets() {
		presets[preset.Key] = preset
	}
	cases := []struct {
		name    string
		values  StructureValues
		columns int
		limit   int
	}{
		{"flow", presets[PresetFlow].Values, 3, StartingWipLimit},
		// The board is offered but not the default view: a list without columns (L0) until someone
		// shows it as a board (domain-model.md §3.5a).
		{"project", presets[PresetProject].Values, 0, 0},
		{"tasks", presets[PresetTasks].Values, 0, 0},
		{"list", presets[PresetList].Values, 0, 0},
		{"nothing said", StructureValues{}, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			columns := StartingColumns(c.values)
			if len(columns) != c.columns {
				t.Fatalf("%d columns, want %d", len(columns), c.columns)
			}
			if c.columns == 0 {
				return
			}
			if columns[1].WipLimit != c.limit {
				t.Errorf("the work in progress is limited to %d, want %d", columns[1].WipLimit, c.limit)
			}
			if columns[0].WipLimit != 0 || columns[2].WipLimit != 0 {
				t.Errorf("a column other than the work in progress is limited: %+v", columns)
			}
			if columns[0].IsDoneBucket || columns[1].IsDoneBucket || !columns[2].IsDoneBucket {
				t.Errorf("only the last column completes: %+v", columns)
			}
			for _, column := range columns {
				if _, err := NewBucket(NewBucketInput{
					ID: "0190d3f2-0000-7000-8000-000000000001", TenantID: "0190d3f2-0000-7000-8000-000000000002",
					CollectionID: "0190d3f2-0000-7000-8000-000000000003", Name: column.NameCode, OrderKey: "a0",
					WipLimit: &column.WipLimit, IsDoneBucket: column.IsDoneBucket, ColorToken: column.ColorToken,
				}); err != nil {
					t.Errorf("column %s is no valid bucket: %v", column.NameCode, err)
				}
			}
		})
	}
}

// UC-WRK-31 check 5: a collection created under a workflow starts with one column per status.
func TestACollectionUnderAWorkflowStartsWithAColumnPerStatus(t *testing.T) {
	cases := []struct {
		name  string
		shape WorkflowShape
		want  []StartingColumn
	}{
		{"named statuses keep their names and colours", WorkflowShape{Statuses: []WorkflowStatus{
			{Key: "new", Name: "Neu", Category: CategoryTodo},
			{Key: "doing", Name: "In Arbeit", Category: CategoryInProgress, ColorToken: "amber"},
			{Key: "done", Name: "Erledigt", Category: CategoryDone},
			{Key: "dropped", Name: "Verworfen", Category: CategoryDone},
		}}, []StartingColumn{
			{Name: "Neu", Status: "new", ColorToken: "slate"},
			{Name: "In Arbeit", Status: "doing", ColorToken: "amber"},
			{Name: "Erledigt", Status: "done", ColorToken: "green", IsDoneBucket: true},
			{Name: "Verworfen", Status: "dropped", ColorToken: "green"},
		}},
		{"the system workflow's statuses are named by category", WorkflowShape{Statuses: []WorkflowStatus{
			{Key: "todo", Category: CategoryTodo}, {Key: "done", Category: CategoryDone},
		}}, []StartingColumn{
			{NameCode: "seed.bucket.todo", Status: "todo", ColorToken: "slate"},
			{NameCode: "seed.bucket.done", Status: "done", ColorToken: "green", IsDoneBucket: true},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := WorkflowStartingColumns(c.shape); !reflect.DeepEqual(got, c.want) {
				t.Errorf("columns %+v, want %+v", got, c.want)
			}
		})
	}
}
