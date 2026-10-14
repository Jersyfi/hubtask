// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package locales

import (
	"encoding/json"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
)

// P-12, UC-LIF-04: every class of data a retention rule can name is drawn in words, in each
// catalogue the screen ships. The screen takes the classes from the manifest and keeps no list, so
// a class added to the catalogue without its words would reach the owner as its constant.
func TestEveryRetentionKindHasItsWords(t *testing.T) {
	for _, file := range []string{"en.json", "de.json"} {
		raw, err := Files.ReadFile(file)
		if err != nil {
			t.Fatalf("reading %s: %v", file, err)
		}
		var messages map[string]string
		if err := json.Unmarshal(raw, &messages); err != nil {
			t.Fatalf("parsing %s: %v", file, err)
		}
		for _, kind := range lifecycle.Catalogue() {
			if messages["app.retention.kind."+string(kind.Name)] == "" {
				t.Errorf("%s has no words for the retention kind %s", file, kind.Name)
			}
		}
	}
}
