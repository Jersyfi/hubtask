// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package setting

import (
	"strings"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/setting"
)

// The one registry holds every area's declarations, each valid and under one key only, each with
// a writer at every level that decides it.
func TestTheCatalogueHoldsEveryArea(t *testing.T) {
	catalogue, err := Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	areas := map[string]int{}
	for _, definition := range catalogue.Definitions() {
		area, _, _ := strings.Cut(string(definition.Key), ".")
		areas[area]++
		for _, level := range definition.Levels {
			if definition.Writers[level] == "" {
				t.Errorf("%s names no writer at %s", definition.Key, level)
			}
		}
	}
	for _, area := range []string{"sign_in", "legal", "localisation", "quotas", "ai", "structure", "module", "config"} {
		if areas[area] == 0 {
			t.Errorf("the area %s is missing", area)
		}
	}
	if got := len(catalogue.At(domain.LevelHub)); got == 0 {
		t.Error("nothing is decided at a hub")
	}
}
