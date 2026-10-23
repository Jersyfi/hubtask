// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import "testing"

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
		{"project", presets[PresetProject].Values, 3, 0},
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
