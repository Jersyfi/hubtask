// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package locales

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// P-12 and UC-ID-02 check 7 (SC-09): one concept has one name everywhere - *second factor* - and
// nothing internal reaches a reader: no field name, no "enrolment", no "armed". Held as a test so
// that the catalogue grep the task was accepted on stays empty.
var retired = regexp.MustCompile(`(?i)2FA|TOTP|two-step|enrol|armed|recovery_code|scharf|entschärf|Zwei-Faktor`)

func TestNoRetiredWordReachesAReader(t *testing.T) {
	for _, name := range []string{"en.json", "de.json"} {
		raw, err := Files.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		var messages map[string]string
		if err := json.Unmarshal(raw, &messages); err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for code, text := range messages {
			// The palette's search words are matched against what a person types, never shown:
			// "2FA" is exactly what someone looking for the second factor types.
			if strings.HasPrefix(code, "app.palette.words_") {
				continue
			}
			if word := retired.FindString(text); word != "" {
				t.Errorf("%s %s says %q: %s", name, code, word, text)
			}
		}
	}
}

// user-experience.md §2.1 and §2.7: the navigation's rows and the headings they open say the same
// plain words, so a card on the overview is not named differently from the row it leads to.
func TestARowAndItsHeadingSayTheDirectionsWords(t *testing.T) {
	en, de := catalogue(t, "en.json"), catalogue(t, "de.json")
	for code, want := range map[string][2]string{
		"app.nav.approvals":           {"Waiting for your yes", "Wartet auf dein Ja"},
		"app.approvals.mine_title":    {"Waiting for your yes", "Wartet auf dein Ja"},
		"app.admin.privacy":           {"Privacy requests", "Datenschutzanfragen"},
		"app.admin_overview.requests": {"Privacy requests", "Datenschutzanfragen"},
	} {
		if en[code] != want[0] {
			t.Errorf("en.json %s says %q, not %q", code, en[code], want[0])
		}
		if de[code] != want[1] {
			t.Errorf("de.json %s says %q, not %q", code, de[code], want[1])
		}
	}
}
