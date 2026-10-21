// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package i18n

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"golang.org/x/text/language"
)

// The faces the installation ships, as the design system lists them (design-system.md §3). The
// design system's own test holds each listed script to a face that covers it in the build; this
// holds every language of the product to a script in that list, so that the two lists cannot
// drift apart: a twelfth row in the table below, or a catalogue in a script nothing covers, fails
// here before a reader sees boxes. A catalogue an operator adds through HUBTASK_LOCALE_DIR is not
// held to it - its script falls to the device's fonts, which design-system.md §3 says.
const facesFile = "../../packages/design-system/fonts/faces.json"

func shippedScripts(t *testing.T) map[string][]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.FromSlash(facesFile))
	if err != nil {
		t.Fatalf("reading the design system's faces: %v", err)
	}
	var list struct {
		Faces []struct {
			Family  string            `json:"family"`
			Scripts map[string]string `json:"scripts"`
		} `json:"faces"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("parsing %s: %v", facesFile, err)
	}
	scripts := map[string][]string{}
	for _, face := range list.Faces {
		for script := range face.Scripts {
			scripts[script] = append(scripts[script], face.Family)
		}
	}
	return scripts
}

func TestEveryLanguageOfTheProductHasAShippedFace(t *testing.T) {
	scripts := shippedScripts(t)

	tags := map[string]string{}
	for tag := range weekAndDecimal {
		tags[tag] = "the 1.0 set"
	}
	embedded, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	for tag := range embedded {
		tags[tag] = "a catalogue the binary embeds"
	}

	names := make([]string, 0, len(tags))
	for tag := range tags {
		names = append(names, tag)
	}
	sort.Strings(names)
	for _, tag := range names {
		// The script CLDR's likely subtags infer, as the direction is inferred (Locale.go); the
		// client asks the browser's CLDR the same question for the heading face.
		script, _ := language.Make(tag).Script()
		if len(scripts[script.String()]) == 0 {
			t.Errorf("%s (%s) is written in %s, and no shipped face covers it", tag, tags[tag], script)
		}
	}
}

// The inference the test above rests on, for the tags whose script is the point.
func TestTheScriptsOfTheOneZeroSetAreTheExpectedOnes(t *testing.T) {
	want := map[string]string{"zh-hans": "Hans", "hi": "Deva", "ar": "Arab", "bn": "Beng", "ru": "Cyrl", "en": "Latn", "de": "Latn"}
	for tag, expected := range want {
		if script, _ := language.Make(tag).Script(); script.String() != expected {
			t.Errorf("%s infers %s, expected %s", tag, script, expected)
		}
	}
}
