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

// The golden test of a person's own localisation on the catalogue (F-5, model.md §4): the chain
// account → workspace → installation as i18n-l10n.md §2 states it, kept as it was written before
// the catalogue resolved it, answers the value OwnLocalisation does, for every combination.
func chainBefore(account, workspace, installation string) string {
	for _, value := range []string{account, workspace, installation} {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func TestOwnLocalisationResolvesTheChainAndNamesItsSource(t *testing.T) {
	values := []string{"", " ", "de"}
	zones := []string{"", "Europe/Berlin"}
	days := []string{"", "SUNDAY"}
	for _, own := range values {
		for _, held := range values {
			for _, installed := range values {
				for _, zone := range zones {
					for _, day := range days {
						account := Account{Locale: own, TimeZone: zone, WeekStart: day}
						workspace := Workspace{DefaultLocale: held, DefaultTimeZone: zone}
						workspace.Settings.WeekStart = WeekStart(day)
						installation := LocalisationDefaults{Locale: installed, TimeZone: zone, WeekStart: WeekStart(day)}
						got := OwnLocalisation(installation, workspace, account)
						if len(got) != 3 {
							t.Fatalf("answered %d settings, want 3", len(got))
						}
						want := []string{chainBefore(own, held, installed), zone, day}
						for i, resolution := range got {
							value, _ := resolution.Effective.(string)
							if value != want[i] {
								t.Errorf("%s of (%q, %q, %q) = %q, want %q", resolution.Key, own, held, installed, value, want[i])
							}
						}
					}
				}
			}
		}
	}
}

func TestOwnLocalisationNamesWhereEachValueComesFrom(t *testing.T) {
	workspace := Workspace{DefaultLocale: "de", DefaultTimeZone: "Europe/Berlin"}
	got := OwnLocalisation(LocalisationDefaults{WeekStart: WeekStartSunday}, workspace, Account{TimeZone: "Asia/Tokyo"})
	want := map[string]string{
		"localisation.locale": "WORKSPACE", "localisation.time_zone": "PERSON", "localisation.week_start": "INSTANCE",
	}
	for _, resolution := range got {
		if string(resolution.Source) != want[string(resolution.Key)] {
			t.Errorf("%s comes from %s, want %s", resolution.Key, resolution.Source, want[string(resolution.Key)])
		}
	}
	if got := OwnLocalisation(LocalisationDefaults{}, Workspace{}, Account{}); got[2].Source != "DEFAULT" {
		t.Errorf("a week start nobody decided comes from %s, want DEFAULT", got[2].Source)
	}
}
