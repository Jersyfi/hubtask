// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package expression_test

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/condition"
	port "github.com/Jersyfi/hubtask/core/port/expression"
	"github.com/Jersyfi/hubtask/infrastructure/expression"
)

type itemActivation map[string]any

func (a itemActivation) Resolve(_ context.Context, name string) (any, bool, error) {
	if name == "item" {
		return map[string]any(a), true, nil
	}
	return nil, false, nil
}

type shippedRule struct {
	Enabled bool `json:"enabled"`
	Trigger struct {
		EventType     string   `json:"event_type"`
		ChangedFields []string `json:"changed_fields"`
	} `json:"trigger"`
	Conditions []struct {
		Expr string `json:"expr"`
	} `json:"conditions"`
	Actions []shippedAction `json:"actions"`
}

type shippedAction struct {
	Kind   string `json:"kind"`
	Params struct {
		Condition string          `json:"condition"`
		Then      []shippedAction `json:"then"`
		Else      []shippedAction `json:"else"`
		Key       string          `json:"key"`
		Value     string          `json:"value"`
	} `json:"params"`
}

func holds(t *testing.T, text string, item itemActivation) bool {
	t.Helper()
	program, err := expression.New().Compile(text, condition.RuleEnvironment(), port.Boolean)
	if err != nil {
		t.Fatalf("%q does not compile: %v", text, err)
	}
	value, err := program.Evaluate(context.Background(), item)
	if err != nil {
		t.Fatalf("%q: %v", text, err)
	}
	return value.Bool
}

// run walks the actions as the engine does and answers the priority they set, empty for none.
func run(t *testing.T, actions []shippedAction, item itemActivation) string {
	t.Helper()
	for _, action := range actions {
		switch action.Kind {
		case "SET_CUSTOM_FIELD":
			if action.Params.Key == "priority" {
				return action.Params.Value
			}
		case "BRANCH":
			next := action.Params.Else
			if holds(t, action.Params.Condition, item) {
				next = action.Params.Then
			}
			if set := run(t, next, item); set != "" {
				return set
			}
		}
	}
	return ""
}

// UC-SVC-02 check 14 (the owner's decision of 2026-11-12): the ITIL desk ships two rules, switched
// on, that set the priority from the impact × urgency matrix when a request comes in and when its
// impact or urgency changes; without both, they set nothing.
func TestTheItilDeskSetsPriorityFromImpactAndUrgency(t *testing.T) {
	raw, err := os.ReadFile("../../core/application/service/config/shipped/itil_service_desk.json")
	if err != nil {
		t.Fatal(err)
	}
	var template struct {
		Hub struct {
			Rules map[string]shippedRule `json:"rules"`
		} `json:"hub"`
	}
	if err := json.Unmarshal(raw, &template); err != nil {
		t.Fatal(err)
	}
	created, changed := template.Hub.Rules["priority_on_create"], template.Hub.Rules["priority_on_change"]
	if !created.Enabled || created.Trigger.EventType != "de.hubtask.work.item.created.v1" {
		t.Fatalf("the rule on a new request is %+v", created)
	}
	if !changed.Enabled || changed.Trigger.EventType != "de.hubtask.work.item.updated.v1" ||
		!slices.Equal(changed.Trigger.ChangedFields, []string{"custom_fields.impact", "custom_fields.urgency"}) {
		t.Fatalf("the rule on a change is %+v", changed)
	}
	matrix := map[[2]string]string{
		{"High", "High"}: "P1", {"High", "Medium"}: "P2", {"Medium", "High"}: "P2",
		{"High", "Low"}: "P3", {"Medium", "Medium"}: "P3", {"Low", "High"}: "P3",
		{"Medium", "Low"}: "P4", {"Low", "Medium"}: "P4", {"Low", "Low"}: "P4",
	}
	for _, rule := range []shippedRule{created, changed} {
		for pair, want := range matrix {
			item := itemActivation{"entry_type": "incident", "custom_fields": map[string]any{"impact": pair[0], "urgency": pair[1]}}
			if !holds(t, rule.Conditions[0].Expr, item) {
				t.Fatalf("%v does not meet the rule's condition", pair)
			}
			if got := run(t, rule.Actions, item); got != want {
				t.Errorf("impact %s × urgency %s sets %q, want %s", pair[0], pair[1], got, want)
			}
		}
		for _, item := range []itemActivation{
			{"entry_type": "incident", "custom_fields": map[string]any{"impact": "High"}},
			{"entry_type": "incident"},
			{"entry_type": "task", "custom_fields": map[string]any{"impact": "High", "urgency": "High"}},
		} {
			if holds(t, rule.Conditions[0].Expr, item) {
				t.Errorf("the rule acts on %v", item)
			}
		}
	}
}
