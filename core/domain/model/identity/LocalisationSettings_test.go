// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"testing"
)

// The golden test of the localisation defaults' move onto the settings catalogue (F-5):
// startingLocalisationBefore is the resolution as it stood before, kept verbatim but for its name.
func startingLocalisationBefore(defaults LocalisationDefaults, locale, zone string) (string, string) {
	if strings.TrimSpace(locale) == "" {
		locale = defaults.Locale
	}
	if strings.TrimSpace(zone) == "" {
		zone = defaults.TimeZone
	}
	return locale, zone
}

func TestStartingLocalisationResolvesAsBeforeTheCatalogue(t *testing.T) {
	locales := []string{"", " ", "de", "en-GB"}
	zones := []string{"", "  ", "UTC", "Europe/Berlin"}
	for _, installationLocale := range locales {
		for _, installationZone := range zones {
			defaults := LocalisationDefaults{Locale: installationLocale, TimeZone: installationZone}
			for _, locale := range locales {
				for _, zone := range zones {
					gotLocale, gotZone := StartingLocalisation(defaults, locale, zone)
					wantLocale, wantZone := startingLocalisationBefore(defaults, locale, zone)
					if gotLocale != wantLocale || gotZone != wantZone {
						t.Fatalf("%+v %q %q: got %q %q, want %q %q",
							defaults, locale, zone, gotLocale, gotZone, wantLocale, wantZone)
					}
				}
			}
		}
	}
}
