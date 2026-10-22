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
