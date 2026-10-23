// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// presetFixture is api/fixtures/structure-presets.json: what the web app's capture tests read as
// the presets' levels, fields and capture tokens. One table, two readers - a preset changed in the
// domain and not in the fixture turns this red, and the client's tests read the fixture.
type presetFixture struct {
	Presets []presetEntry `json:"presets"`
}

type presetEntry struct {
	Key     string              `json:"key"`
	Levels  []string            `json:"levels"`
	Fields  map[string][]string `json:"fields"`
	Capture []string            `json:"capture"`
}

func sortedStrings[T ~string](values []T) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	slices.Sort(out)
	return out
}

func TestThePresetsAgreeWithTheSharedFixture(t *testing.T) {
	raw, err := os.ReadFile("../../../../api/fixtures/structure-presets.json")
	if err != nil {
		t.Fatalf("the fixture could not be read: %v", err)
	}
	var fixture presetFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("the fixture does not parse: %v", err)
	}

	built := domain.BuiltInPresets()
	if len(fixture.Presets) != len(built) {
		t.Fatalf("the fixture holds %d presets, the domain %d", len(fixture.Presets), len(built))
	}
	for _, preset := range built {
		t.Run(string(preset.Key), func(t *testing.T) {
			index := slices.IndexFunc(fixture.Presets, func(each presetEntry) bool { return each.Key == string(preset.Key) })
			if index < 0 {
				t.Fatalf("the fixture has no %s", preset.Key)
			}
			want := fixture.Presets[index]

			levels := make([]string, 0, len(preset.Values.Levels))
			for _, level := range preset.Values.Levels {
				levels = append(levels, string(level.Type))
			}
			if !slices.Equal(levels, want.Levels) {
				t.Errorf("levels: the domain has %v, the fixture %v", levels, want.Levels)
			}
			if len(preset.Values.Fields) != len(want.Fields) {
				t.Errorf("fields: the domain has %d types, the fixture %d", len(preset.Values.Fields), len(want.Fields))
			}
			for itemType, fields := range preset.Values.Fields {
				got := sortedStrings(fields.Capabilities)
				expected := slices.Clone(want.Fields[string(itemType)])
				slices.Sort(expected)
				if !slices.Equal(got, expected) {
					t.Errorf("fields of %s: the domain has %v, the fixture %v", itemType, got, expected)
				}
			}
			capture := sortedStrings(preset.Values.Capture)
			expected := slices.Clone(want.Capture)
			slices.Sort(expected)
			if !slices.Equal(capture, expected) {
				t.Errorf("capture: the domain has %v, the fixture %v", capture, expected)
			}
		})
	}
}
