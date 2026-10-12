// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package locales

import (
	"encoding/json"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/quota"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
)

// UC-INS-09 check 1: every key the installation's level declares is named in words, and every value
// of a closed set too - in each catalogue the screen ships. The screen draws a key by the kind the
// server declares and keeps no list, so a key added to the domain without its words would reach a
// reader as `area.key`; this is where that is caught.
func TestEveryInstanceSettingHasItsWords(t *testing.T) {
	type key struct {
		area, name string
		options    []string
	}
	var keys []key
	for _, name := range identity.PolicySwitches() {
		if declared, held := identity.DeclarationOfSwitch(name); held {
			keys = append(keys, key{"sign_in", string(name), declared.Options})
		}
	}
	for _, link := range identity.LegalLinkNames() {
		keys = append(keys, key{"legal", string(link), nil})
	}
	for _, name := range identity.LocalisationKeys() {
		declared, _ := identity.DeclarationOfLocalisation(name)
		keys = append(keys, key{"localisation", name, declared.Options})
	}
	for _, name := range quota.OperatorNames() {
		keys = append(keys, key{"quotas", name, nil})
	}
	for _, name := range integration.AiSwitches() {
		if declared, held := integration.DeclarationOfAiSwitch(name); held {
			keys = append(keys, key{"ai", string(name), declared.Options})
		}
	}

	for _, file := range []string{"en.json", "de.json"} {
		raw, err := Files.ReadFile(file)
		if err != nil {
			t.Fatalf("reading %s: %v", file, err)
		}
		var messages map[string]string
		if err := json.Unmarshal(raw, &messages); err != nil {
			t.Fatalf("parsing %s: %v", file, err)
		}
		for _, each := range keys {
			if messages["app.instance.setting."+each.area+"."+each.name] == "" {
				t.Errorf("%s has no name for %s.%s", file, each.area, each.name)
			}
			for _, option := range each.options {
				if messages["app.instance.choice."+each.area+"."+each.name+"."+option] == "" {
					t.Errorf("%s has no words for %s.%s = %s", file, each.area, each.name, option)
				}
			}
		}
	}
}
