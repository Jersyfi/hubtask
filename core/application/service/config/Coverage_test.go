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
