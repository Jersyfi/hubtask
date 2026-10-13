// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package locales

import (
	"encoding/json"
	"regexp"
	"testing"
)

// UC-ID-04 check 8, UC-ID-10 check 1, P-12: the mail a sign-in link sends where the password is off
// connects a provider, and the mail an account without a password gets sends nothing to reset. Both
// can answer *Get a sign-in link by mail* on a workspace that has no password, so neither opens by
// saying somebody asked to reset one.
var resetWords = regexp.MustCompile(`(?i)asked to reset|zurückzusetzen`)

func TestTheMailsWithoutAPasswordDoNotSpeakOfAReset(t *testing.T) {
	for _, name := range []string{"en.json", "de.json"} {
		raw, err := Files.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		var messages map[string]string
		if err := json.Unmarshal(raw, &messages); err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, code := range []string{"email.password_connect.body", "email.password_reset_provider.body"} {
			text, ok := messages[code]
			if !ok {
				continue // not translated: the reader gets the English text, which this test also reads
			}
			if word := resetWords.FindString(text); word != "" {
				t.Errorf("%s %s says %q: %s", name, code, word, text)
			}
		}
	}
}
