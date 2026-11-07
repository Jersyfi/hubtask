// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// UC-FLW-01 check 3: the types a level offers are stored and read back in their order, and the
// empty offer - every type - stays a decision rather than reading as "inherit".
func TestTheOfferedEntryTypesSurviveTheDocument(t *testing.T) {
	cases := map[string]map[work.ItemType][]string{
		"narrowed":   {work.ItemTask: {"feature", "bug"}},
		"every type": {},
	}
	for name, offered := range cases {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(valuesDocument(work.StructureValues{EntryTypes: offered}))
			if err != nil {
				t.Fatal(err)
			}
			var document structureSettingsDocument
			if err := json.Unmarshal(encoded, &document); err != nil {
				t.Fatal(err)
			}
			read := valuesFrom(&document)
			if !read.Has(work.SettingEntryTypes) || len(read.EntryTypes) != len(offered) ||
				!slices.Equal(read.EntryTypes[work.ItemTask], offered[work.ItemTask]) {
				t.Fatalf("read back %v from %s", read.EntryTypes, encoded)
			}
		})
	}
	if valuesFrom(&structureSettingsDocument{Completion: "MANUAL"}).Has(work.SettingEntryTypes) {
		t.Error("a document without the key decides the offer")
	}
}
