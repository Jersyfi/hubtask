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

type hiddenFixture struct {
	Bound []struct {
		Type              domain.ItemType     `json:"type"`
		Capabilities      []domain.Capability `json:"capabilities"`
		AllowedChildTypes []domain.ItemType   `json:"allowed_child_types"`
		MaxDepth          int                 `json:"max_depth"`
	} `json:"bound"`
	Cases []struct {
		Name      string `json:"name"`
		Structure struct {
			Levels []struct {
				Type domain.ItemType `json:"type"`
			} `json:"levels"`
			Fields map[domain.ItemType]struct {
				Capabilities []domain.Capability `json:"capabilities"`
			} `json:"fields"`
		} `json:"structure"`
		Type      domain.ItemType   `json:"type"`
		Ancestors []domain.ItemType `json:"ancestors"`
		Hidden    struct {
			Level        bool                `json:"level"`
			Capabilities []domain.Capability `json:"capabilities"`
		} `json:"hidden"`
	} `json:"cases"`
	Below []struct {
		Name    string            `json:"name"`
		Levels  []domain.ItemType `json:"levels"`
		Entries []struct {
			ID      string          `json:"id"`
			Type    domain.ItemType `json:"type"`
			Parent  *string         `json:"parent"`
			Trashed bool            `json:"trashed"`
		} `json:"entries"`
		Below map[string]int `json:"below"`
	} `json:"below"`
}

// The domain decides the mark and the client derives it from the same table (replica.test.ts in
// the web app): a case added on either side turns both red or neither.
func TestTheHiddenMarkAgreesWithTheSharedFixture(t *testing.T) {
	raw, err := os.ReadFile("../../../../api/fixtures/hidden-entries.json")
	if err != nil {
		t.Fatalf("the fixture could not be read: %v", err)
	}
	var fixture hiddenFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("the fixture does not parse: %v", err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("the fixture holds no cases")
	}
	profiles := make([]domain.CapabilityProfile, 0, len(fixture.Bound))
	for _, row := range fixture.Bound {
		profiles = append(profiles, domain.CapabilityProfile{
			Type: row.Type, Capabilities: row.Capabilities,
			AllowedChildTypes: row.AllowedChildTypes, MaxDepth: row.MaxDepth,
		})
	}
	bound := domain.NewStructureBound(profiles, profiles)

	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			values := domain.StructureValues{Fields: map[domain.ItemType]domain.LevelFields{}}
			for _, level := range tc.Structure.Levels {
				values.Levels = append(values.Levels, domain.Level{Type: level.Type})
			}
			for itemType, fields := range tc.Structure.Fields {
				values.Fields[itemType] = domain.LevelFields{Capabilities: fields.Capabilities}
			}
			offered := domain.NewOffered(bound, domain.EffectiveStructure{Values: values})

			got := offered.Hidden(tc.Type, tc.Ancestors)
			if got.Level != tc.Hidden.Level {
				t.Errorf("level hidden = %v, the fixture says %v", got.Level, tc.Hidden.Level)
			}
			if !slices.Equal(got.Capabilities, tc.Hidden.Capabilities) {
				t.Errorf("capabilities hidden = %v, the fixture says %v", got.Capabilities, tc.Hidden.Capabilities)
			}

			// The refusal is the same predicate as the mark (P-05).
			if refused := offered.RequireLevel(tc.Type, tc.Ancestors, ""); (refused != nil) != tc.Hidden.Level {
				t.Errorf("RequireLevel = %v, the mark says level hidden = %v", refused, tc.Hidden.Level)
			}
			profile, _ := bound.Profile(tc.Type)
			narrowed := offered.Narrow(profile)
			for _, capability := range profile.Capabilities {
				hidden := slices.Contains(tc.Hidden.Capabilities, capability)
				if err := narrowed.Require(capability, "/x"); (err != nil) != hidden {
					t.Errorf("Require(%s) = %v, the mark says hidden = %v", capability, err, hidden)
				}
			}
		})
	}
}

// UC-WRK-24 check 4, the same table as replica.test.ts: what the gate counts below a shown entry -
// the types met on the way down, each group asked whether the collection hides any of them.
func TestWhatIsHiddenBelowAgreesWithTheSharedFixture(t *testing.T) {
	raw, err := os.ReadFile("../../../../api/fixtures/hidden-entries.json")
	if err != nil {
		t.Fatalf("the fixture could not be read: %v", err)
	}
	var fixture hiddenFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("the fixture does not parse: %v", err)
	}
	if len(fixture.Below) == 0 {
		t.Fatal("the fixture holds no trees")
	}
	bound := domain.NewStructureBound(matrixProfiles(), matrixProfiles())
	for _, tc := range fixture.Below {
		t.Run(tc.Name, func(t *testing.T) {
			values := domain.StructureValues{}
			for _, level := range tc.Levels {
				values.Levels = append(values.Levels, domain.Level{Type: level})
			}
			offered := domain.NewOffered(bound, domain.EffectiveStructure{Values: values})
			types := map[string]domain.ItemType{}
			parents := map[string]string{}
			live := map[string]bool{}
			for _, entry := range tc.Entries {
				types[entry.ID] = entry.Type
				if entry.Parent != nil {
					parents[entry.ID] = *entry.Parent
				}
				live[entry.ID] = !entry.Trashed
			}
			ancestorsOf := func(id string) []domain.ItemType {
				chain := []domain.ItemType{}
				for at, held := parents[id]; held; at, held = parents[at] {
					chain = append(chain, types[at])
				}
				return chain
			}
			for _, entry := range tc.Entries {
				want := tc.Below[entry.ID]
				got := 0
				if offered.HidesALevel() && !offered.Hidden(entry.Type, ancestorsOf(entry.ID)).Level {
					for _, other := range tc.Entries {
						seen := []domain.ItemType{}
						under := false
						for at := other.ID; ; {
							if !live[at] {
								break
							}
							if !slices.Contains(seen, types[at]) {
								seen = append(seen, types[at])
							}
							parent, held := parents[at]
							if !held {
								break
							}
							if parent == entry.ID {
								under = true
								break
							}
							at = parent
						}
						if under && offered.HidesAnyOf(seen) {
							got++
						}
					}
				}
				if got != want {
					t.Errorf("%s: %d hidden below, the fixture says %d", entry.ID, got, want)
				}
			}
		})
	}
}
