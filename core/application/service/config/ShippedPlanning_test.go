// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/workflow"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	textport "github.com/Jersyfi/hubtask/core/port/text"
)

// The Scrum team, the Kanban team and the Classic project are Hubtask templates (configuration.md §9,
// standards.md): each brings its types with their forms and workflows, the modules it needs, the
// roles it asks for, and a hub part whose scheme runs each type in its own workflow - every block one
// the use cases that apply it accept, and every section one the document has.
func TestThePlanningTemplatesAreWholeWaysOfWorking(t *testing.T) {
	templates, err := bundledTemplates()
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]domain.Template{}
	for _, template := range templates {
		byKey[template.Key] = template
	}
	known := map[string]bool{sectionSettings: true}
	for _, s := range sections() {
		known[s.name] = true
	}
	hubKnown := map[string]bool{}
	for _, s := range hubSections(lintHub) {
		hubKnown[s.name] = true
	}
	tenant, id := shared.ID("01936f2a-7c1e-7000-8000-000000000001"), shared.ID("01936f2a-7c1e-7000-8000-000000000002")
	for key, modules := range map[string][]string{
		"scrum_team":      {"workflows", "planning", "knowledge"},
		"kanban_team":     {"workflows"},
		"classic_project": {"workflows", "planning", "knowledge"},
	} {
		template, ok := byKey[key]
		if !ok {
			t.Errorf("%s is not shipped", key)
			continue
		}
		if template.Origin != domain.OriginHubtask || template.Version < 1 || !reservedKey(key) {
			t.Errorf("%s is %v %d", key, template.Origin, template.Version)
		}
		for name := range template.Document {
			if !known[name] {
				t.Errorf("%s: section %s is not one of the document's", key, name)
			}
		}
		for name := range template.Hub {
			if !hubKnown[name] {
				t.Errorf("%s: hub section %s is not one of a hub's document", key, name)
			}
		}
		for _, module := range modules {
			if text(template.Document[sectionModules][module][switchAttribute]) != "ON" || text(template.Hub[sectionModules][module][switchAttribute]) != "ON" {
				t.Errorf("%s does not switch %s on", key, module)
			}
		}
		for flow, block := range template.Document[sectionWorkflows] {
			shape, err := workflow.ShapeFromInput(block["shape"])
			if err == nil {
				_, err = work.NewWorkflowShape(shape, false, textport.Composing{})
			}
			if err != nil {
				t.Errorf("%s: workflow %s: %v", key, flow, err)
			}
		}
		for entryType, block := range template.Document[sectionTypes] {
			form, err := workflow.FormFromInput(block["form"])
			if err == nil {
				_, err = work.NewForm(form, textport.Composing{})
			}
			if err != nil {
				t.Errorf("%s: type %s's form: %v", key, entryType, err)
			}
			if template.Hub[sectionScheme][entryType] == nil {
				t.Errorf("%s: the hub's scheme leaves %s out", key, entryType)
			}
		}
		for entryType, block := range template.Hub[sectionScheme] {
			if template.Document[sectionTypes][entryType] == nil || template.Document[sectionWorkflows][text(block[workflowAttribute])] == nil {
				t.Errorf("%s: the scheme's %s names a type or workflow the template does not bring", key, entryType)
			}
		}
		for role, block := range template.Document[sectionRoles] {
			permissions := []string{}
			for _, p := range block["permissions"].([]any) {
				permissions = append(permissions, p.(string))
			}
			if _, err := identity.NewCustomRole(id, tenant, role, text(block["name"]), permissions); err != nil {
				t.Errorf("%s: role %s: %v", key, role, err)
			}
		}
	}
}

// A template opens its collections on the view its way of working lives in (user-experience.md
// §7.5): the Kanban and the Scrum team on the board, the Classic project on its timeline. The value
// is the hub's structure, so a collection made later inherits it and may set its own.
func TestThePlanningTemplatesOpenOnTheirOwnView(t *testing.T) {
	templates, err := bundledTemplates()
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]domain.Template{}
	for _, template := range templates {
		byKey[template.Key] = template
	}
	for key, opening := range map[string]work.ViewLayout{
		"kanban_team": work.ViewKanban, "scrum_team": work.ViewKanban, "classic_project": work.ViewTimeline,
	} {
		views, _ := byKey[key].Hub[sectionStructure][string(work.SettingViews)][settingValueField].(map[string]any)
		offered, _ := views["offered"].([]any)
		if text(views["default"]) != string(opening) {
			t.Errorf("%s opens on %v, want %s", key, views["default"], opening)
		}
		found := false
		for _, layout := range offered {
			found = found || text(layout) == string(opening)
		}
		if !found {
			t.Errorf("%s does not offer the view it opens on: %v", key, offered)
		}
	}
}

// A hub made from a team, project or desk template shows what counts on its collections' cards
// and rows (user-experience.md §2.5, §7.9; UC-WRK-29 check 1): the hub's structure names a figure,
// which its collections inherit, instead of the default that counts nothing.
func TestTheWorkTemplatesCountSomethingOnTheirCollections(t *testing.T) {
	templates, err := bundledTemplates()
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]domain.Template{}
	for _, template := range templates {
		byKey[template.Key] = template
	}
	for key, figure := range map[string]work.Figure{
		"kanban_team": work.FigureFlow, "scrum_team": work.FigureOpen, "classic_project": work.FigureProgress, "itil_service_desk": work.FigureOpen,
	} {
		counts, _ := byKey[key].Hub[sectionStructure][string(work.SettingCounts)][settingValueField].(map[string]any)
		if text(counts["figure"]) != string(figure) {
			t.Errorf("%s counts %v on its collections, want %s", key, counts["figure"], figure)
		}
	}
}
