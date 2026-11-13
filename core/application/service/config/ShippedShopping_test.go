// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import "testing"

// The shopping template starts as a list (L0, domain-model.md §3.5a): its three-status workflow is
// installed and offered, and nothing runs it until somebody chooses it.
func TestTheShoppingTemplateOffersItsWorkflowWithoutRunningIt(t *testing.T) {
	templates, err := bundledTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, template := range templates {
		if template.Key != "shopping" {
			continue
		}
		if template.Version < 3 {
			t.Errorf("shopping is still version %d", template.Version)
		}
		workflow, installed := template.Document["workflows"]["shopping_item"]
		if !installed {
			t.Fatal("the workflow is not installed")
		}
		shape, _ := workflow["shape"].(map[string]any)
		if statuses, _ := shape["statuses"].([]any); len(statuses) != 3 {
			t.Errorf("the workflow has %d statuses, want 3", len(statuses))
		}
		if scheme := template.Hub["scheme"]; len(scheme) != 0 {
			t.Errorf("the hub runs %v", scheme)
		}
		if module := template.Hub["modules"]["workflows"]; module["switch"] != "ON" {
			t.Errorf("the workflows module is %v, so the workflow is not offered", module)
		}
		return
	}
	t.Fatal("no shopping template is shipped")
}
