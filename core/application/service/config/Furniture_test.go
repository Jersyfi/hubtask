// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"fmt"
	"maps"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const (
	deskHub        = supportHub
	networkQueue   = "0190a8f0-0000-7000-8000-0000000000c1"
	createdRuleID  = "0190a8f0-0000-7000-8000-0000000000d1"
	createdViewID  = "0190a8f0-0000-7000-8000-0000000000d2"
	existingRuleID = "0190a8f0-0000-7000-8000-0000000000d3"
)

// furniture is the fake doors of views, rules and the places they name.
func furniture(w *world, name string, in usecase.Input) (usecase.Output, error) {
	if w.inputs == nil {
		w.inputs = map[string][]usecase.Input{}
	}
	switch name {
	case "ListContainers":
		var rows []any
		for _, raw := range w.containers {
			row := raw.(map[string]any)
			if (in.String("parent_id") == "" && row["type"] == "HUB") || (in.String("parent_id") != "" && row["parent_id"] == in.String("parent_id")) {
				rows = append(rows, row)
			}
		}
		return usecase.Output{"data": rows, "page": map[string]any{"has_more": false}}, nil
	case "ListSavedViews":
		if w.views == nil {
			return nil, shared.ErrValidation.WithDetail("module.off")
		}
		return usecase.Output{"data": rowsOf(w.views)}, nil
	case "ListRules":
		if w.rules == nil {
			return nil, shared.ErrValidation.WithDetail("module.off")
		}
		return usecase.Output{"data": rowsOf(w.rules), "page": map[string]any{"has_more": false}}, nil
	}
	w.calls = append(w.calls, name)
	w.inputs[name] = append(w.inputs[name], maps.Clone(in))
	switch name {
	case "CreateSavedView":
		row := map[string]any{"id": createdViewID, "scope_id": nil}
		maps.Copy(row, in)
		w.views[in.String("key")] = row
		return usecase.Output(row), nil
	case "CreateRule":
		scope := in["scope"]
		row := map[string]any{"id": createdRuleID, "enabled": false}
		maps.Copy(row, in)
		row["scope"] = scope
		w.rules[in.String("key")] = row
		return usecase.Output(row), nil
	case "EnableRule", "DisableRule":
		for _, row := range w.rules {
			if row["id"] == in.String("rule_id") {
				row["enabled"] = name == "EnableRule"
				return usecase.Output(row), nil
			}
		}
		return nil, shared.ErrNotFound.WithDetail("automation.rule_not_found")
	}
	return usecase.Output{}, nil
}

func furnitureFixture() fixture {
	f := hubFixture()
	f.world.containers = []any{
		map[string]any{"id": deskHub, "type": "HUB", "handle": "desk"},
		map[string]any{"id": networkQueue, "type": "COLLECTION", "parent_id": deskHub, "handle": "network"},
	}
	f.world.views = map[string]map[string]any{
		// Shared workspace-wide: the workspace's block.
		"breaches": {"id": "0190a8f0-0000-7000-8000-0000000000e1", "key": "breaches", "scope_type": "TENANT", "scope_id": nil,
			"sharing": "SCOPE", "name": "Breaches", "layout": "LIST_EXPANDED", "grouping": map[string]any{}, "visible_fields": []any{},
			"query": map[string]any{"scope_container_id": networkQueue,
				"filter": map[string]any{"op": "AND", "nodes": []any{map[string]any{"field": "collection_id", "op": "IN", "value": []any{networkQueue}}}}}},
		// Somebody's own bookmark: in no document.
		"private": {"id": "0190a8f0-0000-7000-8000-0000000000e2", "key": "private", "scope_type": "TENANT", "scope_id": nil,
			"sharing": "PRIVATE", "name": "Mine", "layout": "KANBAN", "query": map[string]any{}},
		// The hub's queue: the hub's block.
		"open": {"id": "0190a8f0-0000-7000-8000-0000000000e3", "key": "open", "scope_type": "HUB", "scope_id": deskHub,
			"sharing": "SCOPE", "name": "Open", "layout": "LIST_EXPANDED", "query": map[string]any{"scope_container_id": deskHub}},
	}
	f.world.rules = map[string]map[string]any{
		"nightly": {"id": existingRuleID, "key": "nightly", "scope": map[string]any{"type": "TENANT"}, "name": "Nightly",
			"enabled": true, "run_as": "0190a8f0-0000-7000-8000-00000000cccc", "trigger": map[string]any{"kind": "MANUAL"},
			"conditions": []any{}, "throttle": map[string]any{}, "on_error": "STOP",
			"actions": []any{map[string]any{"kind": "MOVE_WORK_ITEM", "params": map[string]any{"target_collection_id": networkQueue}}}},
	}
	return f
}

func block(t *testing.T, document map[string]any, section, key string) map[string]any {
	t.Helper()
	rows, _ := document[section].(map[string]any)
	row, _ := rows[key].(map[string]any)
	if row == nil {
		t.Fatalf("%s.%s is not in the document: %v", section, key, document[section])
	}
	return row
}

// accepted.md D, configuration.md §1: the workspace's shared views and rules are sections of
// its document by their stable keys, and the places they name are named by handle, so the
// document applies to a second workspace; a person's own bookmark is in none.
func TestViewsAndRulesAreSectionsByTheirKeys(t *testing.T) {
	f := furnitureFixture()
	out := invoke(t, ExportConfig{f.desk}.Descriptor(), person, usecase.Input{})
	document := out["document"].(map[string]any)
	views := document["views"].(map[string]any)
	if len(views) != 1 {
		t.Fatalf("the views section holds %v", views)
	}
	query := block(t, document, "views", "breaches")["query"].(map[string]any)
	if query["scope_container_id"] != "network" {
		t.Errorf("the anchor is not named by handle: %v", query)
	}
	if fmt.Sprint(query["filter"]) != fmt.Sprint(map[string]any{"op": "AND", "nodes": []any{map[string]any{"field": "collection_id", "op": "IN", "value": []any{"network"}}}}) {
		t.Errorf("the collection filter is not named by handle: %v", query["filter"])
	}
	params := block(t, document, "rules", "nightly")["actions"].([]any)[0].(map[string]any)["params"].(map[string]any)
	if params["target_collection_id"] != "network" {
		t.Errorf("a rule step's collection is not named by handle: %v", params)
	}

	// A new queue and a new macro switched on: written through their use cases, the handles
	// back to identifiers, the rule acting as the person applying, switched on after it was
	// written; the rule in force is switched off by its own use case.
	desired := map[string]any{
		"views": map[string]any{"queue": map[string]any{"name": "Queue", "layout": "LIST_EXPANDED",
			"query": map[string]any{"scope_container_id": "desk"}}},
		"rules": map[string]any{
			"ack":     map[string]any{"name": "Acknowledge", "enabled": true, "trigger": map[string]any{"kind": "MANUAL"}, "actions": []any{map[string]any{"kind": "ADD_COMMENT", "params": map[string]any{"body": "Thanks"}}}},
			"nightly": map[string]any{"enabled": false},
		},
	}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": desired, "mode": "MERGE"})
	if plan["plan_hash"] == nil {
		t.Fatalf("plan %v", plan)
	}
	applied := invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": desired, "mode": "MERGE", "plan_hash": plan["plan_hash"], "acknowledge": true,
	})
	if applied["state"] != "APPLIED" {
		t.Fatalf("not applied: %v", applied)
	}
	created := f.world.inputs["CreateSavedView"]
	if len(created) != 1 || created[0]["key"] != "queue" || created[0]["scope_type"] != "TENANT" || created[0]["sharing"] != "SCOPE" ||
		created[0]["query"].(map[string]any)["scope_container_id"] != deskHub {
		t.Errorf("the view was written as %v", created)
	}
	rules := f.world.inputs["CreateRule"]
	if len(rules) != 1 || rules[0]["key"] != "ack" || rules[0]["run_as"] != person.AccountID.String() ||
		rules[0]["scope"].(map[string]any)["type"] != "TENANT" {
		t.Errorf("the rule was written as %v", rules)
	}
	if enabled := f.world.inputs["EnableRule"]; len(enabled) != 1 || enabled[0]["rule_id"] != createdRuleID {
		t.Errorf("the new rule was switched on as %v", enabled)
	}
	if disabled := f.world.inputs["DisableRule"]; len(disabled) != 1 || disabled[0]["rule_id"] != existingRuleID {
		t.Errorf("the rule in force was switched off as %v", disabled)
	}
	if _, updated := f.world.inputs["UpdateRule"]; updated {
		t.Errorf("switching a rule off rewrote it: %v", f.world.inputs["UpdateRule"])
	}
}

// A hub's document holds the hub's shared views and rules, naming the hub itself `@hub`, so the
// same document makes the same queues in a second hub.
func TestAHubsViewsNameTheHubByItsPlace(t *testing.T) {
	f := furnitureFixture()
	out := invoke(t, ExportConfig{f.desk}.Descriptor(), person, usecase.Input{"hub_id": deskHub})
	document := out["document"].(map[string]any)
	if views := document["views"].(map[string]any); len(views) != 1 {
		t.Fatalf("the hub's views section holds %v", views)
	}
	if anchor := block(t, document, "views", "open")["query"].(map[string]any)["scope_container_id"]; anchor != placeHub {
		t.Errorf("the hub's own anchor reads %v", anchor)
	}
	if rules := document["rules"].(map[string]any); len(rules) != 0 {
		t.Errorf("the workspace's rule is in the hub's document: %v", rules)
	}

	desired := map[string]any{"views": map[string]any{"mine": map[string]any{"name": "Mine", "layout": "LIST_EXPANDED",
		"query": map[string]any{"scope_container_id": placeHub, "filter": map[string]any{"field": "assignee_id", "op": "EQ", "value": "@me"}}}}}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": desired, "mode": "MERGE", "hub_id": deskHub})
	applied := invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": desired, "mode": "MERGE", "hub_id": deskHub, "plan_hash": plan["plan_hash"], "acknowledge": true,
	})
	if applied["state"] != "APPLIED" {
		t.Fatalf("not applied: %v", applied)
	}
	created := f.world.inputs["CreateSavedView"]
	if len(created) != 1 || created[0]["scope_type"] != "HUB" || created[0]["scope_id"] != deskHub ||
		created[0]["query"].(map[string]any)["scope_container_id"] != deskHub {
		t.Errorf("the hub's view was written as %v", created)
	}
}

// Refusal instead of corruption: a rule listing longer than the export walks is refused the
// whole document, rather than handed one whose missing rules an apply would remove.
func TestARuleListingTooLongIsRefusedNotCut(t *testing.T) {
	f := furnitureFixture()
	pages := 0
	f.desk.Catalogue = pagingCatalogue{catalogue{f.world}, &pages}
	_, err := ExportConfig{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{})
	refusal := shared.AsError(err)
	if refusal == nil || len(refusal.Fields) != 1 || refusal.Fields[0].Code != "config.section_too_large" {
		t.Fatalf("an endless listing answered %v", err)
	}
	if pages != rulePages {
		t.Errorf("walked %d pages, want the bound %d", pages, rulePages)
	}
}

type pagingCatalogue struct {
	catalogue
	pages *int
}

func (c pagingCatalogue) Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	if name != "ListRules" {
		return c.catalogue.Invoke(ctx, name, actor, in)
	}
	*c.pages++
	return usecase.Output{"data": []any{}, "page": map[string]any{"has_more": true, "next_cursor": "more"}}, nil
}
