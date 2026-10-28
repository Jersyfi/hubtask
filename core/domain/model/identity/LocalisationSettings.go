// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/setting"
)

// The localisation defaults as entries of the settings catalogue (domain-model.md §2, "The
// settings catalogue"): the installation sets a default, never a lock (identity.md §18), and a
// workspace starts from it where it names none of its own.

// LocalisationArea is the area the localisation defaults are keyed under.
const LocalisationArea = "localisation"

// LocalisationKey is a localisation default's catalogue key.
func LocalisationKey(name string) setting.Key { return setting.Key(LocalisationArea + "." + name) }

// LocalisationDefinition is one localisation default's catalogue entry. Empty is the product's
// default: what the tenant's own constructor reads as `en`, `UTC` and the locale's week.
func LocalisationDefinition(name string) setting.Definition {
	declaration, _ := DeclarationOfLocalisation(name)
	return setting.Definition{
		Key: LocalisationKey(name), Kind: setting.Kind(declaration.Kind), Options: declaration.Options,
		Levels: []setting.Level{setting.LevelInstance, setting.LevelWorkspace},
		May:    setting.May(declaration.May),
		Writers: map[setting.Level]string{
			setting.LevelInstance: "admin:instance", setting.LevelWorkspace: "STRUCTURE",
		},
		Default: "",
	}
}

// StartingLocalisation answers a new workspace's language and zone: the ones it named, or the
// installation's defaults where it named none.
func StartingLocalisation(defaults LocalisationDefaults, locale, zone string) (string, string) {
	resolve := func(name, installation, own string) string {
		layers := []setting.Layer{
			{Level: setting.LevelInstance, Entries: map[setting.Key]setting.Entry{}},
			{Level: setting.LevelWorkspace, Entries: map[setting.Key]setting.Entry{}},
		}
		if installation != "" {
			layers[0].Entries[LocalisationKey(name)] = setting.Entry{Value: installation, Hold: setting.HoldSet}
		}
		if strings.TrimSpace(own) != "" {
			layers[1].Entries[LocalisationKey(name)] = setting.Entry{Value: own, Hold: setting.HoldSet}
		}
		value, _ := setting.Resolve(LocalisationDefinition(name), layers...).Effective.(string)
		return value
	}
	return resolve(LocalisationLocale, defaults.Locale, locale), resolve(LocalisationTimeZone, defaults.TimeZone, zone)
}
