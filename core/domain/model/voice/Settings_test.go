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

func TestTheInvocationIsTheWorkspacesOrTheLanguagesDefault(t *testing.T) {
	if got := (Settings{}).Invocation(German); got != "listen helfer" {
		t.Errorf("German default %q", got)
	}
	if got := (Settings{}).Invocation(English); got != "list helper" {
		t.Errorf("English default %q", got)
	}
	if got := (Settings{InvocationName: "  Meine   Küche "}).Invocation(German); got != "meine küche" {
		t.Errorf("own name %q", got)
	}
	for name, valid := range map[string]bool{
		"meine küche": true, "mom's list": true, "x": false, "alexa listen": false, "listen skill": false,
		"liste 2": false, "3 listen": false, strings.Repeat("a", 51): false,
	} {
		err := Settings{InvocationName: name}.Validate()
		if (err == nil) != valid {
			t.Errorf("%q: valid %v, got %v", name, valid, err)
		}
	}
}
