// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"maps"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/workflow"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	automationmodel "github.com/Jersyfi/hubtask/core/domain/model/automation"
	cmdbmodel "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	textport "github.com/Jersyfi/hubtask/core/port/text"
)

// catalogueKeys is configuration.md §9's catalogue as Hubtask ships it, family by family. *List* and
// *Tasks* are the presets of the same names, offered as templates by presetTemplates.
var catalogueKeys = map[string][]string{
	"personal":   {"household", "shopping", "habits", "travel"},
	"teams":      {"kanban_team", "scrum_team", "bug_tracking", "content_calendar", "event_planning", "team_wiki"},
	"projects":   {"classic_project", "hybrid_project", "client_project"},
	"programmes": {"portfolio", "okr", "scaled_agile"},
	"service":    {"itil_service_desk", "internal_it", "facility_requests", "hr_requests", "customer_support"},
	"processes":  {"approvals", "onboarding", "procurement", "capa", "risk_controls", "records_of_processing"},
	"registers":  {"it_asset_register", "risk_register", "contact_register", "vendor_contracts", "configuration_items"},
}

// The whole catalogue is shipped and nothing beside it, and every template in it is one a workspace
// can take: each block one the use cases that apply it accept, the configuration check finding no
// error in it (configuration.md §11), and a key two templates share meaning the same thing in both -
// one workspace may take any two of them.
func TestEveryShippedTemplateIsOneAWorkspaceCanTake(t *testing.T) {
	templates, err := bundledTemplates()
	if err != nil {
		t.Fatal(err)
	}
	var want, got []string
	for _, keys := range catalogueKeys {
		want = append(want, keys...)
	}
	for _, template := range templates {
		got = append(got, template.Key)
	}
	sort.Strings(want)
	sort.Strings(got)
	if !slices.Equal(want, got) {
		t.Errorf("shipped %v, the catalogue is %v", got, want)
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
	firstSeen := map[string]string{}
	seen := map[string]domain.Block{}

	for _, template := range templates {
		key := template.Key
		if template.Origin != domain.OriginHubtask || template.Version < 1 || !reservedKey(key) {
			t.Errorf("%s is %v %d", key, template.Origin, template.Version)
		}
		if err := checkTemplate(template.Document, template.Hub); err != nil {
			t.Errorf("%s: the configuration check refuses it: %v", key, err)
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
		for _, section := range []string{sectionTypes, sectionFields, sectionWorkflows, sectionRoles} {
			for blockKey, block := range template.Document[section] {
				at := section + "." + blockKey
				if other, taken := seen[at]; taken && !reflect.DeepEqual(other, block) {
					t.Errorf("%s and %s both bring %s, differently: one workspace may take both", firstSeen[at], key, at)
				}
				if _, taken := seen[at]; !taken {
					seen[at], firstSeen[at] = block, key
				}
			}
		}

		fields := template.Document[sectionFields]
		for field, block := range fields {
			if _, err := work.NewCustomFieldDefinition(work.NewCustomFieldInput{
				ID: id, TenantID: tenant, Key: field, Kind: work.CustomFieldKind(text(block["kind"])),
				Options: optionTexts(block["options"]), Currency: text(block["currency"]), Formula: text(block["formula"]),
				AppliesTo: levels(block["applies_to"]), Now: time.Now(), Text: textport.Composing{},
			}); err != nil {
				t.Errorf("%s: field %s: %v", key, field, err)
			}
			for _, input := range formulaInputs(text(block["formula"])) {
				if fields[input] == nil || !work.CustomFieldKind(text(fields[input]["kind"])).IsFormulaInput() {
					t.Errorf("%s: %s's formula reads %s, which is no number of the template", key, field, input)
				}
			}
		}

		for flow, block := range template.Document[sectionWorkflows] {
			shape, err := workflow.ShapeFromInput(block["shape"])
			if err == nil {
				_, err = work.NewWorkflowShape(shape, false, textport.Composing{})
			}
			if err != nil {
				t.Errorf("%s: workflow %s: %v", key, flow, err)
				continue
			}
			for _, step := range shape.Steps {
				if step.Approval == nil {
					continue
				}
				for _, level := range step.Approval.Levels {
					for _, role := range level.Roles {
						if template.Document[sectionRoles][role] == nil {
							t.Errorf("%s: %s's step %s waits on %s, a role the template does not bring", key, flow, step.Key, role)
						}
					}
				}
			}
		}
		for entryType, block := range template.Document[sectionTypes] {
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
				if fields[custom] == nil {
					t.Errorf("%s: %s's form places %s, which the template does not define", key, entryType, custom)
				} else if !containsLevel(levels(fields[custom]["applies_to"]), level) {
					t.Errorf("%s: %s is not carried by %s's level %s", key, custom, entryType, level)
				}
			}
			runs := block["workflow"] == true
			if runs != (template.Hub[sectionScheme][entryType] != nil) {
				t.Errorf("%s: %s runs a workflow %v, the hub's scheme names it %v", key, entryType, runs, !runs)
			}
			if block["record"] == true && (block["completable"] != false || block["schedulable"] != false || runs) {
				t.Errorf("%s: the record type %s is done or scheduled: %v", key, entryType, block)
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
		// A template's own CI classes are defined one by one beneath the shipped hierarchy, a
		// parent before its children, as DefineCiClass defines them.
		classes := cmdbmodel.NewClasses(nil)
		var own []cmdbmodel.Class
		for _, classKey := range slices.Sorted(maps.Keys(template.Document[sectionCiClasses])) {
			block := template.Document[sectionCiClasses][classKey]
			in := usecase.Input{"key": classKey}
			ops, err := ciClassOps(domain.BlockChange{Section: sectionCiClasses, Key: classKey, Op: domain.OpAdd, After: block}, nil)
			if err != nil || len(ops) != 1 || ops[0].UseCase != "DefineCiClass" {
				t.Errorf("%s: class %s: %+v, %v", key, classKey, ops, err)
				continue
			}
			for name, value := range ops[0].Input {
				in[name] = value
			}
			class, err := cmdbmodel.NewClass(ciClassInput(in, id, tenant), classes)
			if err != nil {
				t.Errorf("%s: class %s: %v", key, classKey, err)
				continue
			}
			own = append(own, class)
			classes = cmdbmodel.NewClasses(own)
		}
		for rule, block := range template.Document[sectionRules] {
			trigger, _ := block["trigger"].(map[string]any)
			if trigger["anchor"] == "FIELD" && text(fields[text(trigger["anchor_field"])]["kind"]) != "DATE" {
				t.Errorf("%s: rule %s measures from %v, which is no date of the template", key, rule, trigger["anchor_field"])
			}
			if _, err := automationmodel.NewRule(automationmodel.NewRuleInput{
				ID: id, TenantID: tenant, Key: rule, Name: text(block["name"]),
				Scope: automationmodel.Scope{Type: automationmodel.ScopeTenant}, RunAs: id,
				Trigger: automationmodel.Trigger{
					Kind: automationmodel.TriggerKind(text(trigger["kind"])), Anchor: automationmodel.DateAnchor(text(trigger["anchor"])),
					AnchorField: text(trigger["anchor_field"]), Offset: text(trigger["offset"]),
				},
				Conditions: []automationmodel.Condition{{Expr: "item.entry_type != ''"}},
				Actions:    []automationmodel.Action{{Kind: "ADD_COMMENT", Params: map[string]any{"body": "x"}}},
				OnError:    automationmodel.OnErrorStop, CreatedBy: id, Now: time.Now(), Text: textport.Composing{},
			}); err != nil {
				t.Errorf("%s: rule %s: %v", key, rule, err)
			}
		}
	}
}

// A template taken into an empty workspace raises no finding of the configuration check, a
// warning included: an administrator's first look at the overview must not say the product's own
// template is misconfigured.
func TestEveryShippedTemplatePassesTheConfigurationCheck(t *testing.T) {
	templates, err := bundledTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, template := range templates {
		in := checkedBase()
		if in.Workspace, err = asJSON(template.Document); err != nil {
			t.Fatal(err)
		}
		if template.Hub != nil {
			if in.Hub, err = asJSON(template.Hub); err != nil {
				t.Fatal(err)
			}
		}
		for _, finding := range domain.CheckDocument(in) {
			t.Errorf("%s: %s at %s %v", template.Key, finding.Code, finding.Path, finding.Params)
		}
	}
}

// A service template's hub offers its request types by name, so the hub's collections are desks
// and nothing beside them is: a request type of the workspace is usable everywhere, and a client
// counting that alone made every Scrum collection a desk (UC-SVC-05).
func TestEveryServiceTemplateMakesItsHubsCollectionsDesks(t *testing.T) {
	templates, err := bundledTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, template := range templates {
		var portal []string
		for key, block := range template.Document[sectionTypes] {
			form, _ := block["form"].(map[string]any)
			channels, _ := form["channels"].([]any)
			for _, raw := range channels {
				rule, _ := raw.(map[string]any)
				if rule["channel"] == "PORTAL" && rule["closed"] != true {
					portal = append(portal, key)
				}
			}
		}
		if len(portal) == 0 || template.Hub == nil {
			continue
		}
		setting := template.Hub[sectionStructure][string(work.SettingEntryTypes)]
		value, _ := setting[settingValueField].(map[string]any)
		offered, _ := value[string(work.ItemTask)].([]any)
		names := map[string]bool{}
		for _, each := range offered {
			names[each.(string)] = true
		}
		for _, key := range portal {
			if !names[key] {
				t.Errorf("%s: the hub does not offer the request type %s by name (offers %v)", template.Key, key, offered)
			}
		}
	}
}

// A template that opens a portal ships the role that raises requests there, so a desk takes its
// first request without anybody defining a role first (user-experience.md §4: a portal is there
// on first use).
func TestEveryServiceTemplateShipsARequesterRole(t *testing.T) {
	templates, err := bundledTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, template := range templates {
		opens := false
		for _, block := range template.Document[sectionTypes] {
			form, _ := block["form"].(map[string]any)
			channels, _ := form["channels"].([]any)
			for _, raw := range channels {
				rule, _ := raw.(map[string]any)
				opens = opens || (rule["channel"] == "PORTAL" && rule["closed"] != true)
			}
		}
		if !opens {
			continue
		}
		role := template.Document[sectionRoles]["requester"]
		permissions, _ := role["permissions"].([]any)
		if len(permissions) != 1 || permissions[0] != "REQUEST" {
			t.Errorf("%s: the requester role = %v, want one holding REQUEST alone", template.Key, role)
		}
	}
}

// ciClassInput reads DefineCiClass's input as its handler does.
func ciClassInput(in usecase.Input, id, tenant shared.ID) cmdbmodel.ClassInput {
	out := cmdbmodel.ClassInput{ID: id, TenantID: tenant, Key: text(in["key"]), Parent: text(in["parent"]), Name: text(in["name"]),
		Icon: text(in["icon"]), StalenessDays: number(in["staleness_days"]), Now: time.Now()}
	out.Abstract, _ = in["abstract"].(bool)
	for _, a := range rowsOfAny(in["attributes"]) {
		identifier, _ := a["identifier"].(bool)
		out.Attributes = append(out.Attributes, cmdbmodel.Attribute{Key: text(a["key"]), Kind: cmdbmodel.AttributeKind(text(a["kind"])),
			Need: cmdbmodel.Need(text(a["need"])), Identifier: identifier, Options: optionTexts(a["options"])})
	}
	for _, r := range rowsOfAny(in["identifier_rules"]) {
		out.Rules = append(out.Rules, cmdbmodel.IdentifierRule{Attributes: optionTexts(r["attributes"])})
	}
	for _, r := range rowsOfAny(in["required_relations"]) {
		outgoing, _ := r["outgoing"].(bool)
		out.Required = append(out.Required, cmdbmodel.RequiredRelation{Types: optionTexts(r["types"]), Outgoing: outgoing})
	}
	return out
}
