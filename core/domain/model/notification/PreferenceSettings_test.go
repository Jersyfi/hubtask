// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/setting"
)

// Every category a person may switch off is a PERSON setting of the catalogue (F-5, model.md §4),
// and its default is what somebody who has said nothing gets - never a second "on" that could one
// day disagree with DefaultPreference.
func TestEverySwitchableCategoryIsAPersonSettingDefaultingAsThePreferenceDoes(t *testing.T) {
	definitions := PreferenceDefinitions()
	byKey := map[setting.Key]setting.Definition{}
	for _, definition := range definitions {
		byKey[definition.Key] = definition
	}
	for _, category := range Categories() {
		definition, declared := byKey[PreferenceKey(category)]
		if declared != category.Suppressible() {
			t.Errorf("%s declared %v, switchable %v", category, declared, category.Suppressible())
			continue
		}
		if !declared {
			continue
		}
		if len(definition.Levels) != 1 || definition.Levels[0] != setting.LevelPerson {
			t.Errorf("%s is decided at %v, want the person alone", category, definition.Levels)
		}
		if definition.Writers[setting.LevelPerson] != setting.PersonOwnWriter {
			t.Errorf("%s is written by %q", category, definition.Writers[setting.LevelPerson])
		}
		resolved := setting.Resolve(definition)
		if resolved.Effective != DefaultPreference("", "", category, ChannelEmail).Enabled || resolved.Source != setting.LevelDefault {
			t.Errorf("%s resolves to %v from %s", category, resolved.Effective, resolved.Source)
		}
	}
	if PreferenceKey(CategoryRequestReply) != "notifications.request_reply" {
		t.Errorf("a key reads %s", PreferenceKey(CategoryRequestReply))
	}
}
