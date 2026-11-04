// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
)

// germanWords knows a few of the templates' words in German and nothing in any other language.
type germanWords map[string]string

func (g germanWords) Render(locale, code string, _ map[string]string) string {
	if locale != "de" {
		return code
	}
	for english, german := range g {
		if TemplateWordCode(english) == code {
			return german
		}
	}
	return code
}

var someGerman = germanWords{"Card": "Karte", "To do": "Zu erledigen", "Done": "Erledigt"}

// A Hubtask template is planned in the applying person's language and stored as that plain text;
// a word no catalogue holds stays as shipped, and the shipped template itself is not touched.
func TestAHubtaskTemplateIsPlannedInTheApplyingPersonsLanguage(t *testing.T) {
	f := newFixture()
	f.desk.Words = someGerman
	german := person
	german.Locale = "de"

	plan := invoke(t, PlanConfigTemplate{f.desk}.Descriptor(), german, usecase.Input{"key": "kanban_team"})
	document := plan["document"].(map[string]any)
	card := document["types"].(map[string]any)["card"].(map[string]any)
	if card["name"] != "Karte" {
		t.Errorf("the type is named %v, want Karte", card["name"])
	}
	statuses := document["workflows"].(map[string]any)["card"].(map[string]any)["shape"].(map[string]any)["statuses"].([]any)
	names := map[string]any{}
	for _, status := range statuses {
		row := status.(map[string]any)
		names[row["key"].(string)] = row["name"]
	}
	if names["todo"] != "Zu erledigen" || names["done"] != "Erledigt" || names["doing"] != "Doing" {
		t.Errorf("statuses %v, want the known words in German and the rest as shipped", names)
	}

	english := invoke(t, PlanConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{"key": "kanban_team"})
	if name := english["document"].(map[string]any)["types"].(map[string]any)["card"].(map[string]any)["name"]; name != "Card" {
		t.Errorf("planned without a locale the type is named %v, want Card", name)
	}
	templates, err := bundledTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, template := range templates {
		if template.Key == "kanban_team" && template.Document["types"]["card"]["name"] != "Card" {
			t.Error("planning in German changed the shipped template")
		}
	}
}

// A workspace's own template is its own words: it is never translated.
func TestAWorkspaceTemplateKeepsItsOwnWords(t *testing.T) {
	own := domain.Template{Key: "mine", Origin: domain.OriginWorkspace, Document: domain.Document{
		"types": domain.Section{"card": domain.Block{"name": "Card"}},
	}}
	if got := localizedTemplate(own, someGerman, "de").Document["types"]["card"]["name"]; got != "Card" {
		t.Errorf("a workspace template's word became %v", got)
	}
}
