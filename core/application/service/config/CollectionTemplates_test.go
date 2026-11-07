// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// boardPart is a collection part: a queue and a structure setting, keyed without a handle.
func boardPart(name string) map[string]any {
	return map[string]any{
		"collection_views":     map[string]any{"board": map[string]any{"name": name, "layout": "LIST_EXPANDED"}},
		"collection_structure": map[string]any{"completion": map[string]any{"value": "MANUAL"}},
	}
}

// configuration.md §6, UC-CFG-01 check 15: a template's collection part is taken by one collection
// - its blocks put under that collection's handle and written at that collection through the hub's
// document -, recorded as that collection's provenance with the part as it was, and upgraded by
// the three-way comparison against what the collection has now.
func TestACollectionTakesATemplatesCollectionPart(t *testing.T) {
	f, c := collectionsFixture()
	published := invoke(t, PublishConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{
		"key": "queue_kit", "name": "Queue kit", "document": map[string]any{}, "collection_document": boardPart("Board"),
	})
	if published["collection_document"] == nil || published["hub_document"] != nil {
		t.Fatalf("the collection part was not kept: %v", published)
	}

	plan := invoke(t, PlanConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{"key": "queue_kit", "collection_id": networkQueue})
	if plan["hub_id"] != deskHub || plan["collection_id"] != networkQueue || plan["plan_hash"] == nil {
		t.Fatalf("the collection's plan %v", plan)
	}
	if got := block(t, plan["document"].(map[string]any), "collection_views", "network.board"); got["name"] != "Board" {
		t.Fatalf("the part is not under the collection's handle: %v", plan["document"])
	}
	applied := invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": plan["document"], "plan_hash": plan["plan_hash"], "template": plan["template"],
		"mode": plan["mode"], "collection_id": networkQueue, "acknowledge": true,
	})
	if applied["state"] != "APPLIED" {
		t.Fatalf("not applied: %v", applied)
	}
	views := f.world.inputs["CreateSavedView"]
	if len(views) != 1 || views[0]["scope_type"] != "COLLECTION" || views[0]["scope_id"] != networkQueue || views[0]["key"] != "board" {
		t.Errorf("the queue was written as %v", views)
	}
	if structure := c.written["SetContainerStructure"]; len(structure) != 0 {
		t.Errorf("a setting the collection holds already was written again: %v", structure)
	}
	var recorded []shared.ID
	for _, use := range f.store.uses {
		recorded = append(recorded, use.HubID)
		if use.HubID == networkQueue && use.Document["collection_views"]["board"] == nil {
			t.Errorf("the collection's base is not the part as published: %v", use.Document)
		}
	}
	if len(recorded) != 1 || recorded[0] != networkQueue {
		t.Errorf("the provenance went to %v, not the collection", recorded)
	}

	listed := invoke(t, ListConfigTemplates{f.desk}.Descriptor(), person, usecase.Input{"collection_id": networkQueue})["data"].([]usecase.Output)
	if len(listed) != 1 || listed[0]["key"] != "queue_kit" || listed[0]["taken_version"] != 1 {
		t.Errorf("a collection's templates are %v", listed)
	}
	if hubs := invoke(t, ListConfigTemplates{f.desk}.Descriptor(), person, usecase.Input{"hub_id": deskHub})["data"].([]usecase.Output); len(hubs) > 0 {
		for _, row := range hubs {
			if row["key"] == "queue_kit" {
				t.Errorf("a template without a hub part is offered to a hub")
			}
		}
	}

	invoke(t, PublishConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{
		"key": "queue_kit", "name": "Queue kit", "document": map[string]any{}, "collection_document": boardPart("Open board"),
	})
	upgrade := invoke(t, PlanConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{"key": "queue_kit", "collection_id": networkQueue})
	changes, _ := upgrade["upgrade"].([]usecase.Output)
	if upgrade["from_version"] != 1 || len(changes) != 1 || changes[0]["path"] != "collection_views.network.board.name" || changes[0]["state"] != "TAKE" {
		t.Errorf("the upgrade is %v", upgrade["upgrade"])
	}
}

// Refusal instead of corruption: a collection part holds collection sections keyed without a
// handle; a collection of another hub, a hub's id as a collection and a template without a
// collection part are refused, never applied to whatever the identifier holds.
func TestACollectionPartIsRefusedWhereItDoesNotFit(t *testing.T) {
	f, _ := collectionsFixture()
	for name, part := range map[string]map[string]any{
		"config.section_not_for_collections": {"types": map[string]any{"incident": map[string]any{"level": "TASK", "name": "Incident"}}},
		"config.key_invalid":                 {"collection_views": map[string]any{"network.board": map[string]any{"name": "Board", "layout": "LIST_EXPANDED"}}},
	} {
		_, err := PublishConfigTemplate{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{
			"key": "bad_kit", "name": "Bad", "document": map[string]any{}, "collection_document": part,
		})
		if refusals := refusalsIn(err); len(refusals) != 1 || refusals[0].Code != name {
			t.Errorf("%s: published with %v", name, err)
		}
	}

	invoke(t, PublishConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{
		"key": "queue_kit", "name": "Queue kit", "document": map[string]any{}, "collection_document": boardPart("Board"),
	})
	_, err := PlanConfigTemplate{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{
		"key": "queue_kit", "collection_id": otherCollection, "hub_id": deskHub,
	})
	if refusals := refusalsIn(err); len(refusals) != 1 || refusals[0].Code != "config.collection_not_in_hub" {
		t.Errorf("another hub's collection was planned: %v", err)
	}
	_, err = PlanConfigTemplate{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{"key": "queue_kit", "collection_id": deskHub})
	if !isDetail(err, "config.collection_not_found") {
		t.Errorf("a hub was planned as a collection: %v", err)
	}
	_, err = PlanConfigTemplate{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{"key": "list", "collection_id": networkQueue})
	if refusals := refusalsIn(err); len(refusals) != 1 || refusals[0].Code != "config.template_not_for_collections" {
		t.Errorf("a template without a collection part was planned for a collection: %v", err)
	}
}

// D-16: an agent's proposal for a collection keeps the collection, and applying it records the
// collection's provenance.
func TestAProposalKeepsTheCollection(t *testing.T) {
	f, _ := collectionsFixture()
	invoke(t, PublishConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{
		"key": "queue_kit", "name": "Queue kit", "document": map[string]any{}, "collection_document": boardPart("Board"),
	})
	plan := invoke(t, PlanConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{"key": "queue_kit", "collection_id": networkQueue})
	proposed := invoke(t, ProposeConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": plan["document"], "plan_hash": plan["plan_hash"], "template": plan["template"],
		"mode": plan["mode"], "collection_id": networkQueue,
	})
	proposal := proposed["proposal"].(usecase.Output)
	if proposal["collection_id"] != networkQueue || proposal["hub_id"] != deskHub {
		t.Fatalf("the proposal is %v", proposal)
	}
	invoke(t, DecideConfigProposal{f.desk}.Descriptor(), person, usecase.Input{"proposal_id": proposal["id"], "decision": "APPLY", "acknowledge": true})
	if len(f.store.uses) != 1 || f.store.uses[0].HubID != networkQueue {
		t.Errorf("the provenance went to %+v", f.store.uses)
	}
}

// user-experience.md §5.6: a template sets its collection's key prefix - the network queue's NET
// becomes OPS through the rename the collection's settings call -, the hub's document shows the
// prefix in force, and a document without the block is refused rather than turning the keys back
// to the handle's.
func TestATemplateSetsItsCollectionsKeyPrefix(t *testing.T) {
	f, c := collectionsFixture()
	exported := invoke(t, ExportConfig{f.desk}.Descriptor(), person, usecase.Input{"hub_id": deskHub})["document"].(map[string]any)
	if got := block(t, exported, "collection_attributes", "network.key_prefix"); got["value"] != "NET" {
		t.Fatalf("the hub's document reads the prefix as %v", got)
	}

	invoke(t, PublishConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{
		"key": "ops_kit", "name": "Operations", "document": map[string]any{},
		"collection_document": map[string]any{"collection_attributes": map[string]any{"key_prefix": map[string]any{"value": "OPS"}}},
	})
	plan := invoke(t, PlanConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{"key": "ops_kit", "collection_id": networkQueue})
	if got := block(t, plan["document"].(map[string]any), "collection_attributes", "network.key_prefix"); got["value"] != "OPS" {
		t.Fatalf("the plan holds %v", plan["document"])
	}
	applied := invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": plan["document"], "plan_hash": plan["plan_hash"], "template": plan["template"],
		"mode": plan["mode"], "collection_id": networkQueue, "acknowledge": true,
	})
	if applied["state"] != "APPLIED" {
		t.Fatalf("not applied: %v", applied)
	}
	renamed := c.written["RenameContainer"]
	if len(renamed) != 1 || renamed[0]["container_id"] != networkQueue || renamed[0]["key_prefix"] != "OPS" {
		t.Errorf("the prefix was written as %v", renamed)
	}

	out := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{
		"hub_id": deskHub, "mode": "REPLACE", "document": map[string]any{"collection_attributes": map[string]any{}},
	})
	refusals, _ := out["refusals"].([]usecase.Output)
	if len(refusals) != 1 || refusals[0]["code"] != "config.not_removable" || out["plan_hash"] != nil {
		t.Errorf("a document without the prefix was planned: %v", out)
	}
}
