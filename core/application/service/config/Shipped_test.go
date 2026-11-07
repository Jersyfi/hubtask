// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestEveryPresetIsOfferedAsATemplateOfItsOrigin(t *testing.T) {
	f := newFixture()
	rows := invoke(t, ListConfigTemplates{f.desk}.Descriptor(), person, nil)["data"].([]usecase.Output)
	origins := map[string]string{}
	for _, row := range rows {
		origins[row["key"].(string)] = row["origin"].(string)
	}
	for _, key := range []string{"list", "tasks", "project", "flow"} {
		if origins[key] != "HUBTASK" {
			t.Errorf("%s is not offered as Hubtask's template: %v", key, origins)
		}
	}
	if origins["installation_0190a8f00000700080000000000000cc"] != "INSTALLATION" {
		t.Errorf("the installation's preset is not offered as its template: %v", origins)
	}
}

func TestTakingAHubtaskTemplatePlansItsStructureAndRecordsItsProvenance(t *testing.T) {
	f := newFixture()
	f.world.structure = map[string]any{}
	plan := invoke(t, PlanConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{"key": "list"})
	if plan["template"] != "list@1" {
		t.Fatalf("the provenance is %v", plan["template"])
	}
	document := plan["document"].(map[string]any)
	if _, held := document["structure"].(map[string]any)["completion"]; !held {
		t.Fatalf("the template does not set the preset's structure: %v", document)
	}
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": document, "plan_hash": plan["plan_hash"], "template": "list@1",
	})
	if f.world.structure["completion"] == nil {
		t.Errorf("the preset's completion was not set: %v", f.world.structure)
	}
	if len(f.store.uses) != 1 || f.store.uses[0].TemplateKey != "list" {
		t.Errorf("the template taken was not recorded: %+v", f.store.uses)
	}
}

func TestAWorkspaceTemplateMayNotTakeAPresetsKey(t *testing.T) {
	for _, key := range []string{"tasks", "installation_x", "preset_x", "own_x"} {
		f := newFixture()
		_, err := PublishConfigTemplate{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{"key": key, "name": "Ours"})
		if !hasField(err, "config.template_key_shipped") {
			t.Errorf("%s was taken: %v", key, err)
		}
	}
}

func hasField(err error, code string) bool {
	var e *shared.Error
	if !errors.As(err, &e) {
		return false
	}
	for _, field := range e.Fields {
		if field.Code == code {
			return true
		}
	}
	return false
}
