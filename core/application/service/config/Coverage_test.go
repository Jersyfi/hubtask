// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"strings"
	"testing"

	settingservice "github.com/Jersyfi/hubtask/core/application/service/setting"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
)

// Every catalogue setting a workspace may decide is a block of its document (P-18): through a
// `settings` area, the `structure` section or the `modules` section. A new workspace setting that
// joins none of them fails here rather than being configurable only on its screen.
func TestEveryWorkspaceSettingIsInTheDocument(t *testing.T) {
	catalogue, err := settingservice.Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	// rotation_from is a moment the rule is measured from, not configuration (signInPaths).
	outside := map[string]bool{"sign_in.rotation_from": true}
	for _, definition := range catalogue.At(setting.LevelWorkspace) {
		key := string(definition.Key)
		if outside[key] {
			continue
		}
		_, held := areaOf(key)
		held = held || key == settingKey(domain.AgentApplyKey) ||
			strings.HasPrefix(key, "structure.") || strings.HasPrefix(key, "module.")
		if !held {
			t.Errorf("%s is decided by the workspace and is in no section of its document", key)
		}
	}
}

// Every catalogue setting a hub or a collection may decide is a block of the hub's document
// (configuration.md §1): the hub's own in a section of its own, a collection's in a collection
// section keyed by its handle. A new setting decided there that joins none of them fails here
// rather than being left out of an export, a rollback and an apply to a second hub.
func TestEveryHubAndCollectionSettingIsInTheHubsDocument(t *testing.T) {
	catalogue, err := settingservice.Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	held := map[string]bool{}
	collections := map[string]bool{}
	for _, s := range hubSections(supportHub) {
		held[s.name] = true
		collections[s.name] = s.collection
	}
	at := map[setting.Level]map[string]string{
		setting.LevelHub:        {"structure.": sectionStructure, "module.": sectionModules, planningPrefix: sectionTolerance},
		setting.LevelCollection: {"structure.": sectionCollectionStructure, planningPrefix: sectionCollectionTolerance},
	}
	for level, prefixes := range at {
		for _, definition := range catalogue.At(level) {
			key, section := string(definition.Key), ""
			for prefix, name := range prefixes {
				if strings.HasPrefix(key, prefix) {
					section = name
				}
			}
			if section == "" || !held[section] || collections[section] != (level == setting.LevelCollection) {
				t.Errorf("%s is decided by a %s and is in no section of the hub's document", key, level)
			}
		}
	}
}
