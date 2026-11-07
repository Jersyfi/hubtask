// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package setting

import (
	"strings"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/service"
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

// A writer is one the access model can answer for (model.md §4, ui.md A11): the operator's scope
// at the installation and a plan, the person at their own, and a permission of the catalogue a
// role can carry everywhere in a workspace - so *who may do what* can name the roles holding it.
func TestEveryWriterIsOneTheAccessModelKnows(t *testing.T) {
	catalogue, err := Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, entry := range service.PermissionCatalogue() {
		known[string(entry.Permission)] = true
	}
	for _, definition := range catalogue.Definitions() {
		for level, writer := range definition.Writers {
			switch level {
			case domain.LevelInstance, domain.LevelPlan:
				if writer != "admin:instance" {
					t.Errorf("%s at %s: writer %q is not the operator's scope", definition.Key, level, writer)
				}
			case domain.LevelPerson:
				if writer != domain.PersonOwnWriter {
					t.Errorf("%s at %s: writer %q is not the person", definition.Key, level, writer)
				}
			default:
				if !known[writer] {
					t.Errorf("%s at %s: writer %q is no permission of the catalogue", definition.Key, level, writer)
				}
			}
		}
	}
}
