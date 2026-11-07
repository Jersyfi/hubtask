// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package locales

import "testing"

// A module's switch says the product's name the column, Today and the products band say, so a
// newcomer switching it on knows what appears.
func TestAModuleSwitchSaysItsProductsName(t *testing.T) {
	pairs := [][2]string{
		{"app.modules.name_knowledge", "app.nav.pages"},
		{"app.modules.name_whiteboard", "app.nav.whiteboards"},
		{"app.instance.setting.module.knowledge", "app.nav.pages"},
		{"app.instance.setting.module.whiteboard", "app.nav.whiteboards"},
	}
	for _, name := range []string{"en.json", "de.json"} {
		messages := catalogue(t, name)
		for _, pair := range pairs {
			if messages[pair[0]] != messages[pair[1]] {
				t.Errorf("%s: %s says %q, the product is %q", name, pair[0], messages[pair[0]], messages[pair[1]])
			}
		}
	}
}
