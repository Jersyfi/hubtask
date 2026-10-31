// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/workflow"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	textport "github.com/Jersyfi/hubtask/core/port/text"
)

// The registers - IT assets, risks, contacts and organisations, vendors and contracts - are Hubtask
// templates (research products-2026-10 §8, templates.md §4): each brings record types, never done
// and never scheduled, with a form whose every field the template defines, each definition one
// DefineCustomField accepts, on the level its types are. The risk's score is the likelihood times
// the impact, from the template's own numbers.
func TestTheRegisterTemplatesAreWholeRegisters(t *testing.T) {
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
	tenant, id := shared.ID("01936f2a-7c1e-7000-8000-000000000001"), shared.ID("01936f2a-7c1e-7000-8000-000000000002")
	expected := map[string][]string{
		"it_asset_register": {"asset"},
		"risk_register":     {"risk_record"},
		"contact_register":  {"organisation", "contact"},
		"vendor_contracts":  {"vendor", "contract"},
	}
	fieldOwner := map[string]string{}
	for key, types := range expected {
		template, ok := byKey[key]
		if !ok {
			t.Errorf("%s is not shipped", key)
			continue
		}
		if template.Origin != domain.OriginHubtask || template.Version != 1 || !reservedKey(key) || template.ForHubs() {
			t.Errorf("%s is %v %d, for hubs %v", key, template.Origin, template.Version, template.ForHubs())
		}
		for name := range template.Document {
			if !known[name] {
				t.Errorf("%s: section %s is not one of the document's", key, name)
			}
		}
		if text(template.Document[sectionModules]["workflows"][switchAttribute]) != "ON" {
			t.Errorf("%s does not switch on the module its types need", key)
		}
		if len(template.Document[sectionTypes]) != len(types) {
			t.Errorf("%s brings types %v", key, template.Document[sectionTypes])
		}

		fields := template.Document[sectionFields]
		for field, block := range fields {
			if other, taken := fieldOwner[field]; taken {
				t.Errorf("%s and %s both define %s: one workspace holds both registers", other, key, field)
			}
			fieldOwner[field] = key
			if _, err := work.NewCustomFieldDefinition(work.NewCustomFieldInput{
				ID: id, TenantID: tenant, Key: field, Kind: work.CustomFieldKind(text(block["kind"])),
				Options: optionTexts(block["options"]), Currency: text(block["currency"]), Formula: text(block["formula"]),
				AppliesTo: levels(block["applies_to"]), Now: time.Now(), Text: textport.Composing{},
			}); err != nil {
				t.Errorf("%s: field %s: %v", key, field, err)
			}
			for _, input := range formulaInputs(text(block["formula"])) {
				kind := work.CustomFieldKind(text(fields[input]["kind"]))
				if fields[input] == nil || !kind.IsFormulaInput() {
					t.Errorf("%s: %s's formula reads %s, which is no number of the template", key, field, input)
				}
			}
		}

		for _, entryType := range types {
			block := template.Document[sectionTypes][entryType]
			if block == nil {
				t.Errorf("%s does not bring the type %s", key, entryType)
				continue
			}
			if block["record"] != true || block["completable"] != false || block["schedulable"] != false || block["workflow"] != false {
				t.Errorf("%s: %s is not a record: %v", key, entryType, block)
			}
			form, err := workflow.FormFromInput(block["form"])
			if err == nil {
				_, err = work.NewForm(form, textport.Composing{})
			}
			if err != nil {
				t.Errorf("%s: type %s's form: %v", key, entryType, err)
				continue
			}
			level := work.ItemType(text(block["level"]))
			for _, placed := range form.Fields {
				custom, isCustom := strings.CutPrefix(placed.Key, "custom_fields.")
				if !isCustom {
					continue
				}
				definition := fields[custom]
				if definition == nil {
					t.Errorf("%s: %s's form places %s, which the template does not define", key, entryType, custom)
					continue
				}
				if !containsLevel(levels(definition["applies_to"]), level) {
					t.Errorf("%s: %s is not carried by %s's level %s", key, custom, entryType, level)
				}
			}
		}
	}
	risk := byKey["risk_register"].Document[sectionFields]["risk_score"]
	if text(risk["kind"]) != "CALCULATED" || !strings.Contains(text(risk["formula"]), "risk_likelihood * entry.custom_fields.risk_impact") {
		t.Errorf("the risk's score is %v", risk)
	}
	contract := byKey["vendor_contracts"].Document[sectionFields]
	if text(contract["contract_document"]["kind"]) != "FILE" || text(contract["contract_vendor"]["kind"]) != "REFERENCE" {
		t.Errorf("a contract does not hold its document and its vendor: %v", contract)
	}
}

var formulaInput = regexp.MustCompile(`entry\.custom_fields\.([a-z][a-z0-9_]*)`)

func formulaInputs(formula string) []string {
	var out []string
	for _, match := range formulaInput.FindAllStringSubmatch(formula, -1) {
		out = append(out, match[1])
	}
	return out
}

func optionTexts(raw any) []string {
	var out []string
	for _, each := range asList(raw) {
		out = append(out, text(each))
	}
	return out
}

func levels(raw any) []work.ItemType {
	var out []work.ItemType
	for _, each := range asList(raw) {
		out = append(out, work.ItemType(text(each)))
	}
	return out
}

func containsLevel(levels []work.ItemType, level work.ItemType) bool {
	for _, each := range levels {
		if each == level {
			return true
		}
	}
	return false
}
