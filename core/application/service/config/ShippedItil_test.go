// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/servicedesk"
	"github.com/Jersyfi/hubtask/core/application/service/workflow"
	"github.com/Jersyfi/hubtask/core/domain/event"
	automationmodel "github.com/Jersyfi/hubtask/core/domain/model/automation"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	desk "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	viewmodel "github.com/Jersyfi/hubtask/core/domain/model/view"
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
	if template.Origin != domain.OriginHubtask || template.Version != 6 || !reservedKey(template.Key) {
		t.Fatalf("the template is %v %d %v", template.Origin, template.Version, template.Key)
	}
	// Its hub part (configuration.md §6): the hub's switches and which of the template's workflows
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
	// Its queues (UC-SVC-03, UC-SVC-07) are the hub's views, anchored on the hub that takes the
	// part, and its macros (UC-SVC-06) the hub's manual rules, switched on.
	tenant, id, now := shared.ID("01936f2a-7c1e-7000-8000-000000000001"), shared.ID("01936f2a-7c1e-7000-8000-000000000002"), time.Now()
	hubPlaces := places{key: map[string]string{}, id: map[string]string{}, hub: lintHub.String()}
	for _, key := range []string{"mine", "unassigned", "about_to_breach", "unhappy"} {
		block := template.Hub[sectionViews][key]
		if block == nil {
			t.Errorf("the queue %s is missing", key)
			continue
		}
		query, _ := mapQuery(block["query"], hubPlaces.in).(map[string]any)
		if query["scope_container_id"] != lintHub.String() {
			t.Errorf("the queue %s is not anchored on the hub: %v", key, query)
		}
		if _, err := viewmodel.ValidatedViewQuery(query); err != nil {
			t.Errorf("the queue %s: %v", key, err)
		}
		if !viewmodel.Layout(text(block["layout"])).Valid() {
			t.Errorf("the queue %s's layout %v", key, block["layout"])
		}
	}
	if len(template.Hub[sectionRules]) == 0 {
		t.Error("the desk brings no macro")
	}
	// Its one timed rule (configuration.md §6): a resolved incident is closed a week after it was done,
	// asked again when it is due, so one reopened or closed by hand is left alone.
	closing := template.Hub[sectionRules]["close_resolved"]
	if trigger, _ := closing["trigger"].(map[string]any); trigger["kind"] != "RELATIVE_DATE" || trigger["anchor"] != "COMPLETED_AT" {
		t.Errorf("the closing rule is %v", closing)
	} else if _, err := automationmodel.NewRule(automationmodel.NewRuleInput{
		ID: id, TenantID: tenant, Key: "close_resolved", Name: text(closing["name"]), Scope: automationmodel.Scope{Type: automationmodel.ScopeHub, ID: lintHub},
		RunAs: id, Trigger: automationmodel.Trigger{Kind: automationmodel.TriggerRelativeDate, Anchor: automationmodel.AnchorCompletedAt, Offset: text(trigger["offset"])},
		Conditions: []automationmodel.Condition{{Expr: "item.entry_type == 'incident' && item.status == 'resolved'"}},
		Actions:    []automationmodel.Action{{Kind: "TAKE_STEP", Params: map[string]any{"to_status": "closed"}}},
		OnError:    automationmodel.OnErrorStop, CreatedBy: id, Now: now, Text: textport.Composing{},
	}); err != nil {
		t.Errorf("the closing rule: %v", err)
	}
	// Its escalation (configuration.md §6): an SLA goal at risk tells the assignee by an internal
	// comment, a breached one also raises an incident's priority by one step.
	escalation := map[string]string{"sla_at_risk": "de.hubtask.work.item.sla_at_risk.v1", "sla_breached": "de.hubtask.work.item.sla_breached.v1"}
	for key, block := range template.Hub[sectionRules] {
		if key == "close_resolved" {
			continue
		}
		trigger, _ := block["trigger"].(map[string]any)
		kept := automationmodel.Trigger{Kind: automationmodel.TriggerManual}
		if key == "p1_escalation" {
			// "Which group answers first" is asked when the hub takes the template; the rule sets
			// the incident's responding group to the answer once its priority becomes P1.
			if trigger["kind"] != "EVENT" || trigger["event_type"] != string(event.ItemUpdated) {
				t.Errorf("the P1 escalation is on %v", trigger)
			}
			if !strings.Contains(domain.Canonical(block["actions"]), `"value":"answer:first_group"`) || len(template.Questions) != 1 ||
				template.Questions[0].Key != "first_group" || template.Questions[0].Kind != domain.QuestionGroup || template.Questions[0].Required {
				t.Errorf("the P1 escalation does not hand the incident to the group asked for: %v %v", block, template.Questions)
			}
			if text(template.Document[sectionFields]["responding_group"]["kind"]) != "GROUP" {
				t.Error("the desk defines no responding group")
			}
			kept = automationmodel.Trigger{Kind: automationmodel.TriggerEvent, EventType: event.ItemUpdated, ChangedFields: []string{"custom_fields.priority"}}
		} else if eventType, escalates := escalation[key]; escalates {
			if trigger["kind"] != "EVENT" || trigger["event_type"] != eventType {
				t.Errorf("the escalation %s is not on %s: %v", key, eventType, trigger)
			}
			kept = automationmodel.Trigger{Kind: automationmodel.TriggerEvent, EventType: event.Type(eventType)}
			if !strings.Contains(domain.Canonical(block["actions"]), `"internal":true`) {
				t.Errorf("the escalation %s tells nobody by an internal comment: %v", key, block)
			}
		} else if trigger["kind"] != "MANUAL" {
			t.Errorf("the macro %s is not a manual rule: %v", key, block)
		}
		if enabled, _ := block["enabled"].(bool); !enabled {
			t.Errorf("the rule %s is not switched on: %v", key, block)
		}
		var actions []automationmodel.Action
		for _, raw := range block["actions"].([]any) {
			action := raw.(map[string]any)
			params, _ := action["params"].(map[string]any)
			actions = append(actions, automationmodel.Action{Kind: text(action["kind"]), Params: params})
		}
		var conditions []automationmodel.Condition
		for _, raw := range asList(block["conditions"]) {
			conditions = append(conditions, automationmodel.Condition{Expr: text(raw.(map[string]any)["expr"])})
		}
		if _, err := automationmodel.NewRule(automationmodel.NewRuleInput{
			ID: id, TenantID: tenant, Key: key, Name: text(block["name"]), Scope: automationmodel.Scope{Type: automationmodel.ScopeHub, ID: lintHub},
			RunAs: id, Trigger: kept, Conditions: conditions, Actions: actions,
			OnError: automationmodel.OnError(text(block["on_error"])), CreatedBy: id, Now: now, Text: textport.Composing{},
		}); err != nil {
			t.Errorf("the rule %s: %v", key, err)
		}
	}
	if !strings.Contains(domain.Canonical(template.Hub[sectionRules]["sla_breached"]), `"value":"P1"`) {
		t.Error("a breached P2 is not raised to P1")
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
			_, err = desk.NewSLAPolicy(desk.NewSLAPolicyInput{ID: id, TenantID: tenant, Key: key, Name: block["name"].(string),
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

func asList(v any) []any {
	list, _ := v.([]any)
	return list
}
