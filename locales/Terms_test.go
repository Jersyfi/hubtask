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
