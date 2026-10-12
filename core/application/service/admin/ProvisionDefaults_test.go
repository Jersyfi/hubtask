// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
)

// A workspace provisioned without a language or a zone of its own starts with the installation's
// (UC-INS-09: what every workspace starts with); one that names its own keeps them; and with no
// installation default, the product's `en` and `UTC`.
func TestANewWorkspaceStartsWithTheInstallationsLanguageAndZone(t *testing.T) {
	for _, tc := range []struct {
		name               string
		installation       identity.LocalisationDefaults
		locale, zone       string
		wantLocale, wantTZ string
	}{
		{"the installation's", identity.LocalisationDefaults{Locale: "fr", TimeZone: "Europe/Paris"}, "", "", "fr", "Europe/Paris"},
		{"its own wins", identity.LocalisationDefaults{Locale: "fr", TimeZone: "Europe/Paris"}, "de", "Europe/Berlin", "de", "Europe/Berlin"},
		{"one of each", identity.LocalisationDefaults{Locale: "fr", TimeZone: "Europe/Paris"}, "de", "", "de", "Europe/Paris"},
		{"nothing decided anywhere", identity.LocalisationDefaults{}, "", "", "en", "UTC"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newProvisionFixture()
			store := newInstanceStore()
			store.level.Localisation = tc.installation
			f.handler.Instance = store

			command := provisionCommand()
			command.DefaultLocale, command.DefaultTimeZone = tc.locale, tc.zone
			provisioned, err := f.handler.Execute(t.Context(), operator(), command)
			if err != nil {
				t.Fatalf("provisioning: %v", err)
			}
			if provisioned.Tenant.DefaultLocale != tc.wantLocale || provisioned.Tenant.DefaultTimeZone != tc.wantTZ {
				t.Errorf("the workspace starts with %s / %s, want %s / %s",
					provisioned.Tenant.DefaultLocale, provisioned.Tenant.DefaultTimeZone, tc.wantLocale, tc.wantTZ)
			}
		})
	}
}
