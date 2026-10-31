// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/setting"
)

// PreferenceKey is the catalogue's key of a category's switch: `notifications.<category>`.
func PreferenceKey(category Category) setting.Key {
	return setting.Key("notifications." + strings.ToLower(string(category)))
}

// PreferenceDefinitions declares every category a person may switch off as a setting of their own
// (F-5, model.md §4). The person alone decides it - no level above holds a default to inherit -
// and the default is DefaultPreference's, read from it so the two cannot disagree. The value
// itself stays a preference row, its absence the default (Preference).
func PreferenceDefinitions() []setting.Definition {
	out := []setting.Definition{}
	for _, category := range Categories() {
		if !category.Suppressible() {
			continue
		}
		out = append(out, setting.Definition{
			Key: PreferenceKey(category), Kind: setting.KindFlag,
			Levels:  []setting.Level{setting.LevelPerson},
			May:     setting.MayReplace,
			Writers: map[setting.Level]string{setting.LevelPerson: setting.PersonOwnWriter},
			Default: DefaultPreference("", "", category, ChannelEmail).Enabled,
		})
	}
	return out
}
