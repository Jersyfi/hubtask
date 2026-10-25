// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package voice

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestSettingsRefuseWhatCannotBeSpokenOrStored(t *testing.T) {
	good := Settings{Enabled: true, SkillID: "amzn1.ask.skill.1", Aliases: []Alias{{Phrase: "Kühlschrank", CollectionID: "c1"}}}
	if err := good.Validate(); err != nil {
		t.Fatalf("good settings refused: %v", err)
	}
	many := make([]Alias, MaxAliases+1)
	for index := range many {
		many[index] = Alias{Phrase: strings.Repeat("a", index+1)[:min(index+1, MaxAliasLength)], CollectionID: "c1"}
	}
	for name, bad := range map[string]Settings{
		"too many":    {Aliases: many},
		"empty":       {Aliases: []Alias{{Phrase: " ", CollectionID: "c1"}}},
		"too long":    {Aliases: []Alias{{Phrase: strings.Repeat("x", MaxAliasLength+1), CollectionID: "c1"}}},
		"no place":    {Aliases: []Alias{{Phrase: "A"}}},
		"twice":       {Aliases: []Alias{{Phrase: "Büro", CollectionID: "c1"}, {Phrase: "buero", CollectionID: "c2"}}},
		"not a skill": {SkillID: "skill"},
	} {
		if err := bad.Validate(); !errors.Is(err, shared.ErrValidation) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
