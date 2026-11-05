// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package locales

import (
	"encoding/json"
	"strings"
	"testing"
)

// UC-ID-14 check 3: the mailed invitation states how long its link works - fourteen days, the
// redemption token's lifetime (identity.RedemptionLifetime), as the link handed over by hand says.
func TestTheInvitationMailStatesHowLongItsLinkWorks(t *testing.T) {
	stated := map[string]string{"en.json": "fourteen days", "de.json": "vierzehn Tage"}
	for name, words := range stated {
		raw, err := Files.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		var messages map[string]string
		if err := json.Unmarshal(raw, &messages); err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, code := range []string{"email.invitation.body", "email.invitation.body.withheld"} {
			if text := messages[code]; !strings.Contains(text, words) {
				t.Errorf("%s %s does not say %q: %s", name, code, words, text)
			}
		}
	}
}
