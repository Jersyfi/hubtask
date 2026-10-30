// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/servicedesk"
	"github.com/Jersyfi/hubtask/core/application/service/workflow"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	desk "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	textport "github.com/Jersyfi/hubtask/core/port/text"
)

func itilTemplate(t *testing.T) domain.Template {
	t.Helper()
	templates, err := bundledTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, template := range templates {
		if template.Key == "itil_service_desk" {
			return template
		}
	}
	t.Fatal("the ITIL service desk template is not shipped")
	return domain.Template{}
}

// The ITIL 4 service desk is a Hubtask template: incident, service request, problem and change
// with their forms (incident and service request open the portal) and workflows, the change
// approved by a CAB member, the office calendar, P1-P4 policies and the desk's canned replies -
// every block one the use cases that apply it accept, and every section one the document has.
func TestTheItilTemplateIsAWholeDesk(t *testing.T) {
	template := itilTemplate(t)
	if template.Origin != domain.OriginHubtask || template.Version != 2 || !reservedKey(template.Key) {
		t.Fatalf("the template is %v %d %v", template.Origin, template.Version, template.Key)
	}
	// Its hub part (templates.md §1): the hub's switches and which of the template's workflows
	// each of its types follows in the hub - every section one a hub's document has.
	hubKnown := map[string]bool{}
	for _, s := range hubSections(lintHub) {
		hubKnown[s.name] = true
	}
	for name := range template.Hub {
		if !hubKnown[name] {
			t.Errorf("hub section %s is not one of a hub's document", name)
		}
	}
	for entryType, block := range template.Hub[sectionScheme] {
		if template.Document[sectionTypes][entryType] == nil || template.Document[sectionWorkflows][text(block[workflowAttribute])] == nil {
			t.Errorf("the scheme's %s names a type or workflow the template does not bring: %v", entryType, block)
		}
	}
	if len(template.Hub[sectionScheme]) != 4 {
		t.Errorf("the scheme is %v", template.Hub[sectionScheme])
	}
	known := map[string]bool{sectionSettings: true}
	for _, s := range sections() {
		known[s.name] = true
	}
	for name := range template.Document {
		if !known[name] {
			t.Errorf("section %s is not one of the document's", name)
		}
	}
	tenant, id, now := shared.ID("01936f2a-7c1e-7000-8000-000000000001"), shared.ID("01936f2a-7c1e-7000-8000-000000000002"), time.Now()
	for key, block := range template.Document[sectionWorkflows] {
		shape, err := workflow.ShapeFromInput(block["shape"])
		if err == nil {
			_, err = work.NewWorkflowShape(shape, false, textport.Composing{})
		}
		if err != nil {
			t.Errorf("workflow %s: %v", key, err)
		}
	}
	portal := 0
	for key, block := range template.Document[sectionTypes] {
		form, err := workflow.FormFromInput(block["form"])
		if err == nil {
			_, err = work.NewForm(form, textport.Composing{})
		}
		if err != nil {
			t.Errorf("type %s's form: %v", key, err)
		}
		if strings.Contains(domain.Canonical(block["form"]), "PORTAL") {
			portal++
		}
	}
	if portal != 2 {
		t.Errorf("%d types open the portal, want incident and service request", portal)
	}
	if !strings.Contains(domain.Canonical(template.Document[sectionWorkflows]["change"]), "cab_member") {
		t.Error("the change is not approved by a CAB member")
	}
	for key, block := range template.Document[sectionCalendars] {
		holidays := []string{}
		spec, err := servicedesk.SpecFromInput(block["zone"].(string), block["hours"], holidays)
		if err == nil {
			_, err = desk.NewCalendar(desk.NewCalendarInput{ID: id, TenantID: tenant, Key: key, Name: block["name"].(string), Spec: spec, Now: now, Text: textport.Composing{}})
		}
		if err != nil {
			t.Errorf("calendar %s: %v", key, err)
		}
	}
	for key, block := range template.Document[sectionPolicies] {
		goals, err := servicedesk.GoalsFromInput(block["goals"])
		if err == nil {
			types := []string{}
			for _, each := range block["entry_types"].([]any) {
				types = append(types, each.(string))
			}
			field, _ := block["priority_field"].(string)
			_, err = desk.NewSlaPolicy(desk.NewSlaPolicyInput{ID: id, TenantID: tenant, Key: key, Name: block["name"].(string),
				CalendarKey: block["calendar"].(string), EntryTypes: types, PriorityField: field, Goals: goals, Enabled: true, Now: now, Text: textport.Composing{}})
		}
		if err != nil {
			t.Errorf("policy %s: %v", key, err)
		}
	}
	for key, block := range template.Document[sectionReplies] {
		if _, err := desk.NewReplyTemplate(desk.NewReplyTemplateInput{ID: id, TenantID: tenant, Key: key,
			Name: block["name"].(string), Body: block["body"].(string), Now: now, Text: textport.Composing{}}); err != nil {
			t.Errorf("reply %s: %v", key, err)
		}
	}
}
