// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"encoding/json"
	"maps"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
)

const (
	otherHub        = "0190a8f0-0000-7000-8000-0000000000a9"
	otherCollection = "0190a8f0-0000-7000-8000-0000000000c9"
	secondHub       = "0190a8f0-0000-7000-8000-0000000000b1"
	secondNetwork   = "0190a8f0-0000-7000-8000-0000000000b2"
)

// placesCatalogue answers what a collection decides itself - its structure and its tolerance -
// and the hub's tolerance, and records what is written; the rest is the shared fake's.
type placesCatalogue struct {
	catalogue
	structure map[string]map[string]any
	tolerance map[string]map[string]any
	written   map[string][]usecase.Input
}

func (c placesCatalogue) Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	switch name {
	case "GetContainer":
		own, held := c.structure[in.String("container_id")]
		if !held {
			return c.catalogue.Invoke(ctx, name, actor, in)
		}
		sources := map[string]any{}
		for name := range own {
			sources[name] = "COLLECTION"
		}
		return usecase.Output{"type": "COLLECTION", "structure": map[string]any{"settings": maps.Clone(own), "sources": sources}}, nil
	case "ReadPlanTolerance":
		if in["tolerance_only"] != true {
			panic("the document read the plan's standing")
		}
		if row, held := c.tolerance[in.String("plan_id")]; held {
			return usecase.Output(row), nil
		}
		return usecase.Output{"source": "WORKSPACE", "time_tolerance_days": nil, "locked_by": nil}, nil
	case "SetPlanTolerance", "SetContainerStructure":
		c.written[name] = append(c.written[name], maps.Clone(in))
		return usecase.Output{}, nil
	}
	return c.catalogue.Invoke(ctx, name, actor, in)
}

// collectionsFixture is the desk hub with its network queue, which decides its own structure,
// tolerance, queue and rule; another hub's collection beside it decides its own too.
func collectionsFixture() (fixture, placesCatalogue) {
	f := furnitureFixture()
	f.world.containers = append(f.world.containers,
		map[string]any{"id": otherHub, "type": "HUB", "handle": "field"},
		map[string]any{"id": otherCollection, "type": "COLLECTION", "parent_id": otherHub, "handle": "visits"})
	f.world.views["network_open"] = map[string]any{"id": "0190a8f0-0000-7000-8000-0000000000e7", "key": "open", "scope_type": "COLLECTION",
		"scope_id": networkQueue, "sharing": "SCOPE", "name": "Open network", "layout": "LIST_EXPANDED",
		"query": map[string]any{"scope_container_id": networkQueue}}
	f.world.views["visits_open"] = map[string]any{"id": "0190a8f0-0000-7000-8000-0000000000e8", "key": "open", "scope_type": "COLLECTION",
		"scope_id": otherCollection, "sharing": "SCOPE", "name": "Open visits", "layout": "LIST_EXPANDED", "query": map[string]any{}}
	f.world.rules["triage"] = map[string]any{"id": "0190a8f0-0000-7000-8000-0000000000d7", "key": "triage",
		"scope": map[string]any{"type": "COLLECTION", "id": networkQueue}, "name": "Triage", "enabled": true,
		"trigger": map[string]any{"kind": "MANUAL"}, "conditions": []any{}, "on_error": "STOP",
		"actions": []any{map[string]any{"kind": "ADD_COMMENT", "params": map[string]any{"body": "Seen"}}}}
	c := placesCatalogue{
		catalogue: catalogue{f.world},
		structure: map[string]map[string]any{networkQueue: {"completion": "MANUAL"}, otherCollection: {"completion": "AUTO"}},
		tolerance: map[string]map[string]any{
			deskHub:      {"source": "HUB", "time_tolerance_days": 5, "locked_by": "HUB"},
			networkQueue: {"source": "COLLECTION", "time_tolerance_days": 2, "locked_by": nil},
		},
		written: map[string][]usecase.Input{},
	}
	f.desk.Catalogue = c
	return f, c
}

// config-document.md §1, UC-CFG-01: a hub's document holds its collections' own configuration -
// structure, tolerance, queue and rule - by the collection's handle, and the hub's own tolerance;
// another hub's collection is in none of it.
func TestAHubsDocumentHoldsItsCollectionsOwnBlocks(t *testing.T) {
	f, _ := collectionsFixture()
	out := invoke(t, ExportConfig{f.desk}.Descriptor(), person, usecase.Input{"hub_id": deskHub})
	document := out["document"].(map[string]any)
	if got := block(t, document, "tolerance", "time_tolerance_days"); got["value"] != float64(5) || got["lock"] != true {
		t.Errorf("the hub's tolerance reads %v", got)
	}
	if got := block(t, document, "collection_structure", "network.completion"); got["value"] != "MANUAL" {
		t.Errorf("the collection's structure reads %v", got)
	}
	if got := block(t, document, "collection_tolerance", "network.time_tolerance_days"); got["value"] != float64(2) || got["lock"] != nil {
		t.Errorf("the collection's tolerance reads %v", got)
	}
	queue := block(t, document, "collection_views", "network.open")
	if queue["name"] != "Open network" || queue["query"].(map[string]any)["scope_container_id"] != "network" {
		t.Errorf("the collection's queue reads %v", queue)
	}
	if got := block(t, document, "collection_rules", "network.triage"); got["name"] != "Triage" {
		t.Errorf("the collection's rule reads %v", got)
	}
	for _, name := range []string{"collection_structure", "collection_views"} {
		if section := document[name].(map[string]any); len(section) != 1 {
			t.Errorf("%s holds another hub's collection: %v", name, section)
		}
	}
}

// UC-CFG-01: a collection's queue, rule, structure and tolerance survive an export applied to a
// second hub - a second workspace's, whose collection has the same handle - and are written there
// through their use cases at that collection.
func TestACollectionsQueueSurvivesExportAppliedToASecondHub(t *testing.T) {
	f, _ := collectionsFixture()
	exported := invoke(t, ExportConfig{f.desk}.Descriptor(), person, usecase.Input{"hub_id": deskHub})["document"].(map[string]any)
	raw, err := json.Marshal(exported)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}

	second, c := collectionsFixture()
	second.world.hub = secondHub
	second.world.containers = []any{
		map[string]any{"id": secondHub, "type": "HUB", "handle": "desk"},
		map[string]any{"id": secondNetwork, "type": "COLLECTION", "parent_id": secondHub, "handle": "network"},
	}
	second.world.views = map[string]map[string]any{}
	second.world.rules = map[string]map[string]any{}
	c.structure, c.tolerance = map[string]map[string]any{}, map[string]map[string]any{}
	second.desk.Catalogue = c

	in := usecase.Input{"document": document, "mode": "MERGE", "hub_id": secondHub}
	plan := invoke(t, PlanConfig{second.desk}.Descriptor(), person, in)
	if plan["plan_hash"] == nil {
		t.Fatalf("plan %v", plan)
	}
	in["plan_hash"], in["acknowledge"] = plan["plan_hash"], true
	if applied := invoke(t, ApplyConfig{second.desk}.Descriptor(), person, in); applied["state"] != "APPLIED" {
		t.Fatalf("not applied: %v", applied)
	}
	var views []usecase.Input
	for _, view := range second.world.inputs["CreateSavedView"] {
		if view["scope_type"] == "COLLECTION" {
			views = append(views, view)
		}
	}
	if len(views) != 1 || views[0]["key"] != "open" || views[0]["scope_type"] != "COLLECTION" || views[0]["scope_id"] != secondNetwork ||
		views[0]["query"].(map[string]any)["scope_container_id"] != secondNetwork {
		t.Errorf("the queue was written as %v", views)
	}
	rules := second.world.inputs["CreateRule"]
	if len(rules) != 1 || rules[0]["key"] != "triage" || rules[0]["scope"].(map[string]any)["id"] != secondNetwork {
		t.Errorf("the rule was written as %v", rules)
	}
	structure := c.written["SetContainerStructure"]
	if len(structure) != 1 || structure[0]["container_id"] != secondNetwork || structure[0]["settings"].(map[string]any)["completion"] != "MANUAL" {
		t.Errorf("the collection's structure was written as %v", structure)
	}
	tolerances := map[string]any{}
	for _, write := range c.written["SetPlanTolerance"] {
		tolerances[write.String("plan_id")] = write["time_tolerance_days"]
	}
	if len(tolerances) != 2 || tolerances[secondHub] != float64(5) || tolerances[secondNetwork] != float64(2) {
		t.Errorf("the tolerances were written as %v", c.written["SetPlanTolerance"])
	}
}

// Refusal instead of corruption, T-04: a handle that names no collection of the hub - another
// hub's, another workspace's, or none - is refused, never written to whatever holds it.
func TestACollectionBlockOfAnotherPlaceIsRefused(t *testing.T) {
	f, _ := collectionsFixture()
	for _, key := range []string{"visits.open", "nowhere.open", "network", "network.Open"} {
		plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"hub_id": deskHub, "mode": "MERGE",
			"document": map[string]any{"collection_views": map[string]any{key: map[string]any{"name": "Theirs", "layout": "LIST_EXPANDED"}}}})
		refusals, _ := plan["refusals"].([]usecase.Output)
		if plan["plan_hash"] != nil || len(refusals) != 1 {
			t.Errorf("%s planned as %v", key, plan)
			continue
		}
		want := "config.collection_unknown"
		if key == "network" || key == "network.Open" {
			want = "config.key_invalid"
		}
		if refusals[0]["code"] != want {
			t.Errorf("%s was refused with %v, want %s", key, refusals[0], want)
		}
	}
}
