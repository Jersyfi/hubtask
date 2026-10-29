// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"slices"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// UC-FLW-01 check 3: `entry_types` reads as every channel sends it and answers in the same shape.
func TestTheEntryTypesSettingReadsAndAnswersInTheContractsWords(t *testing.T) {
	values, err := structureValuesFrom(map[string]any{
		"entry_types": map[string]any{"TASK": []any{"feature", "bug"}},
	})
	if err != nil || !slices.Equal(values.EntryTypes[domain.ItemTask], []string{"feature", "bug"}) {
		t.Fatalf("read %v, %v", values.EntryTypes, err)
	}
	out := structureValuesOutput(values)
	offered, held := out["entry_types"].(map[string]any)
	if !held || len(offered["TASK"].([]any)) != 2 {
		t.Fatalf("answered %v", out)
	}
	for _, malformed := range []any{[]any{"bug"}, map[string]any{"TASK": "bug"}, map[string]any{"TASK": []any{1}}} {
		if _, err := structureValuesFrom(map[string]any{"entry_types": malformed}); err == nil {
			t.Errorf("%v was read", malformed)
		}
	}
}
