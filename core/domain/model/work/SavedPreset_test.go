// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

const savedID = shared.ID("0190a6f2-3c4d-7e8f-9a0b-1c2d3e4f5a6b")

func TestNewSavedPreset(t *testing.T) {
	values := work.StructureValues{
		Levels: []work.Level{{Type: work.ItemTask}},
		Fields: map[work.ItemType]work.LevelFields{work.ItemTask: {
			Capabilities: []work.Capability{work.CapabilityCompletion, work.CapabilityLabels},
			Categories:   []shared.ID{"0190a6f2-3c4d-7e8f-9a0b-1c2d3e4f5a6c"},
		}},
	}
	tests := []struct {
		name     string
		in       work.NewSavedPresetInput
		wantCode string
	}{
		{"the workspace's", work.NewSavedPresetInput{Origin: work.PresetOriginWorkspace, Name: " Client project "}, ""},
		{"one's own", work.NewSavedPresetInput{Origin: work.PresetOriginOwn, OwnerID: "owner", Name: "Weekly shop"}, ""},
		{"Hubtask's is not saved", work.NewSavedPresetInput{Origin: work.PresetOriginHubtask, Name: "x"}, "presets.origin_unknown"},
		{"no name", work.NewSavedPresetInput{Origin: work.PresetOriginOwn, Name: "  "}, "presets.name_empty"},
		{"too long", work.NewSavedPresetInput{Origin: work.PresetOriginOwn, Name: strings.Repeat("a", 101)}, "presets.name_too_long"},
		{"two lines", work.NewSavedPresetInput{Origin: work.PresetOriginOwn, Name: "a\nb"}, "presets.name_malformed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.in.ID, tc.in.Values = savedID, values
			preset, err := work.NewSavedPreset(tc.in)
			if tc.wantCode != "" {
				var problem *shared.Error
				if !errors.As(err, &problem) || problem.DetailCode != tc.wantCode {
					t.Fatalf("err = %v, want %s", err, tc.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if preset.Key != work.SavedPresetKey(savedID) || preset.Name != strings.TrimSpace(tc.in.Name) {
				t.Errorf("key %q name %q", preset.Key, preset.Name)
			}
			if (preset.OwnerID != "") != (tc.in.Origin == work.PresetOriginOwn) {
				t.Errorf("owner %q for origin %s", preset.OwnerID, tc.in.Origin)
			}
			if got := preset.Values.Fields[work.ItemTask].Categories; len(got) != 0 {
				t.Errorf("categories kept: %v", got)
			}
		})
	}
}

func TestPresetKeyNamesBuiltInOrSaved(t *testing.T) {
	if !work.PresetList.BuiltIn() || work.PresetKey("NOPE").BuiltIn() {
		t.Error("BuiltIn wrong")
	}
	if _, saved := work.PresetFlow.SavedID(); saved {
		t.Error("a built-in key named a saved preset")
	}
	if id, saved := work.SavedPresetKey(savedID).SavedID(); !saved || id != savedID {
		t.Errorf("saved id %q %v", id, saved)
	}
	if _, saved := work.PresetKey("not-an-id").SavedID(); saved {
		t.Error("a malformed key named a saved preset")
	}
}
