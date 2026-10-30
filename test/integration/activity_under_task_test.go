// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/meta"
	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	suggestiondomain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	syncdomain "github.com/Jersyfi/hubtask/core/domain/model/sync"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/infrastructure/automation"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/presentation/mcp"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// A task holds activities directly, beside its work packages (domain-model.md §2, UC-WRK-03 check
// 1), and every door that creates or moves an entry reaches that placement through the same
// catalogue against the migrated profile: REST, MCP, an automation action, the bulk, an offline
// push, a template, a duplicate and an accepted breakdown. Each test works in tenant B or a tenant
// of its own - tenant A carries the narrowed TASK row capability_profile_test.go seeds.

// levelCatalogueFor wires every use case this file drives to the real adapters, so that each door
// meets the same rows the server reads.
func levelCatalogueFor(ctx context.Context, t *testing.T) *usecase.Registry {
	t.Helper()

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	hybrid, err := clockadapter.NewHybridClock(fixed, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	sink := postgres.NewAuditSink(ids)
	profiles := postgres.NewCapabilityProfileRepository()
	journal := work.ActivityJournal{Entries: historyRepo(), IDs: ids}
	outbox := postgres.NewOutbox(jobQueue(t))
	changes := postgres.NewChangeLog()
	authorizer := access.Service{
		Memberships: postgres.NewMembershipRepository(), UnitOfWork: unitOfWork, Audit: sink, Clock: fixed,
	}
	placement := work.PlacementWriter{
		Items: itemRepo(), Buckets: bucketRepo(), Containers: containerRepo(), Profiles: profiles,
		Authorizer: authorizer, Events: outbox, Changes: changes, Audit: sink, Activity: journal,
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
	}
	templates := work.TemplateWriter{
		Templates: templateRepo(), Containers: containerRepo(), Profiles: profiles,
		Authorizer: authorizer, Changes: changes, Audit: sink,
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
		Holds: postgres.NewLifecycleRepository(), Removals: postgres.NewLifecycleRepository(),
		TombstoneWindow: 90 * 24 * time.Hour,
	}
	catalogue := &deferredRegistry{}

	registry, err := usecase.NewRegistry(nil,
		work.CreateWorkItem{
			Items: itemRepo(), Containers: containerRepo(), Profiles: profiles,
			Authorizer: authorizer, Ownership: authorizer, Events: outbox, Changes: changes,
			Audit: sink, Activity: journal, UnitOfWork: unitOfWork,
			Clock: fixed, IDs: ids, HLC: hybrid,
		}.Descriptor(),
		work.GetWorkItem{
			Items: itemRepo(), ItemLabels: itemLabelRepo(), Containers: containerRepo(),
			Authorizer: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.UpdateWorkItem{
			Items: itemRepo(), Buckets: bucketRepo(), Containers: containerRepo(), Profiles: profiles,
			Authorizer: authorizer, Events: outbox, Changes: changes, Audit: sink, Activity: journal,
			UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid, Text: text.Composing{},
		}.Descriptor(),
		work.MoveWorkItem{Placement: placement}.Descriptor(),
		work.BulkUpdateWorkItems{
			Catalogue: catalogue, Audit: sink, UnitOfWork: unitOfWork, Clock: fixed,
		}.Descriptor(),
		work.DuplicateWorkItem{
			Items: itemRepo(), ItemLabels: itemLabelRepo(), ItemMembers: itemMemberRepo(),
			Labels: labelRepo(), Buckets: bucketRepo(), Fields: fieldRepo(),
			Containers: containerRepo(), Attachments: mediaRepo(), Media: mediaRepo(),
			Profiles: profiles, Authorizer: authorizer, Ownership: authorizer, Visibility: authorizer,
			Events: outbox, Changes: changes, Audit: sink, Activity: journal,
			UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
		}.Descriptor(),
		work.CreateTemplate{Writer: templates}.Descriptor(),
		work.InstantiateTemplate{
			Writer: templates, Items: postgres.NewItemRepository(pageCursors()), ItemMembers: itemMemberRepo(),
			Visibility: authorizer, Events: outbox, Activity: journal,
		}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	catalogue.catalogue = registry
	return registry
}

// levelWriter may read and write entries and answer suggestions, as a person signed in may.
func levelWriter(tenant, account shared.ID) appshared.ActorContext {
	actor := itemWriter(tenant, account)
	actor.Scopes = []string{"items:read", "items:write", "suggestions:write", "templates:read", "templates:write"}
	return actor
}

// levelTask creates a task with one work package and one activity under the work package, the
// shape every move below starts from.
func levelTask(ctx context.Context, t *testing.T, registry *usecase.Registry, collection shared.ID) (task, pack, leaf string) {
	t.Helper()
	actor := levelWriter(tenantB, authorB)
	create := func(in usecase.Input) string {
		t.Helper()
		out, err := registry.Invoke(ctx, "CreateWorkItem", actor, in)
		if err != nil {
			t.Fatalf("creating %v: %v", in["type"], err)
		}
		return out.String("id")
	}
	task = create(usecase.Input{"type": "TASK", "collection_id": collection.String(), "title": freshName(t)})
	pack = create(usecase.Input{"type": "WORK_PACKAGE", "parent_id": task, "title": freshName(t)})
	leaf = create(usecase.Input{"type": "ACTIVITY", "parent_id": pack, "title": freshName(t)})
	return task, pack, leaf
}

// placed reads where an entry sits now, from the database.
func placed(ctx context.Context, t *testing.T, id string) domain.WorkItem {
	t.Helper()
	return findItem(ctx, t, tenantB, shared.MustParseID(id))
}

// overREST sends one request through the router as tenant B's writer.
func overREST(ctx context.Context, t *testing.T, registry *usecase.Registry, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	controller := rest.NewRestController()
	controller.UseCases = registry
	controller.Meta = &rest.MetaDoor{Capabilities: meta.GetCapabilities{
		Profiles: postgres.NewCapabilityProfileRepository(), Languages: postgres.NewTextLanguageRepository(),
		UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)),
	}}
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, levelWriter(tenantB, authorB)),
		method, rest.APIBasePath+path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

// toolCall calls one tool over JSON-RPC as tenant B's writer, and answers the structured result.
func toolCall(ctx context.Context, t *testing.T, registry *usecase.Registry, tool string, arguments map[string]any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": arguments},
	})
	if err != nil {
		t.Fatalf("encoding the call: %v", err)
	}
	server := mcp.Server{Catalogue: registry, Name: "hubtask", Version: "test"}
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, levelWriter(tenantB, authorB)),
		http.MethodPost, mcp.Path, strings.NewReader(string(encoded)))
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	var answer struct {
		Result struct {
			IsError    bool           `json:"isError"`
			Structured map[string]any `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil || answer.Result.IsError {
		t.Fatalf("the %s call failed: %v %s", tool, err, recorder.Body)
	}
	return answer.Result.Structured
}

func problemOf(t *testing.T, recorder *httptest.ResponseRecorder) rest.Problem {
	t.Helper()
	var problem rest.Problem
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("the refusal is not a problem document: %v %s", err, recorder.Body)
	}
	return problem
}

// UC-WRK-03 check 1 at the server: the manifest a client reads offers both kinds under a task, and
// an activity is created directly under one through REST, MCP and an automation action alike.
func TestAnActivityIsCreatedUnderATaskThroughEveryDoor(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	registry := levelCatalogueFor(ctx, t)
	task, _, _ := levelTask(ctx, t, registry, collection)

	manifest := overREST(ctx, t, registry, http.MethodGet, "/meta/capabilities", "")
	if manifest.Code != http.StatusOK {
		t.Fatalf("the manifest answered %d: %s", manifest.Code, manifest.Body)
	}
	var published struct {
		ItemTypes []struct {
			Type     string   `json:"type"`
			Children []string `json:"allowed_child_types"`
		} `json:"item_types"`
	}
	if err := json.Unmarshal(manifest.Body.Bytes(), &published); err != nil {
		t.Fatalf("the manifest is not JSON: %v", err)
	}
	for _, entry := range published.ItemTypes {
		if entry.Type == "TASK" && strings.Join(entry.Children, ",") != "WORK_PACKAGE,ACTIVITY" {
			t.Errorf("the manifest offers %v under a task", entry.Children)
		}
	}

	viaREST := overREST(ctx, t, registry, http.MethodPost, "/items",
		`{"type":"ACTIVITY","parent_id":"`+task+`","title":"Call the plumber"}`)
	if viaREST.Code != http.StatusCreated {
		t.Fatalf("REST answered %d: %s", viaREST.Code, viaREST.Body)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(viaREST.Body.Bytes(), &created); err != nil {
		t.Fatalf("the REST answer: %v", err)
	}

	viaMCP := toolCall(ctx, t, registry, "create_work_item", map[string]any{
		"type": "ACTIVITY", "parent_id": task, "title": "Order the tiles",
	})

	rule := levelWriter(tenantB, authorB)
	rule.Kind = appshared.ActorAutomation
	viaRule, err := automation.NewActionDispatcher(registry).Dispatch(ctx, rule, automation.Action{
		Kind:   "CREATE_WORK_ITEM",
		Params: map[string]any{"type": "ACTIVITY", "parent_id": task, "title": "Remind the landlord"},
	}, nil)
	if err != nil {
		t.Fatalf("the automation action failed: %v", err)
	}

	for door, id := range map[string]string{
		"REST": created.ID, "MCP": viaMCP["id"].(string), "automation": viaRule.String("id"),
	} {
		item := placed(ctx, t, id)
		if item.Type != domain.ItemActivity || item.ParentID.String() != task || item.Depth != 2 {
			t.Errorf("through %s the activity sits as %s under %s at depth %d", door, item.Type, item.ParentID, item.Depth)
		}
	}
}

// The refusals at the REST door, with the status each answers: a note on the task's own activity,
// and moving that activity out to the collection, where an activity cannot stand.
func TestWhatATasksActivityCannotDoIsRefusedWithItsStatus(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	registry := levelCatalogueFor(ctx, t)
	task, _, _ := levelTask(ctx, t, registry, collection)

	noted := overREST(ctx, t, registry, http.MethodPost, "/items",
		`{"type":"ACTIVITY","parent_id":"`+task+`","title":"Milk","notes":"Semi-skimmed"}`)
	if noted.Code != http.StatusUnprocessableEntity || problemOf(t, noted).DetailCode != "items.capability_not_supported" {
		t.Errorf("a note on the task's activity answered %d: %s", noted.Code, noted.Body)
	}

	direct := toolCall(ctx, t, registry, "create_work_item", map[string]any{
		"type": "ACTIVITY", "parent_id": task, "title": "Call the plumber",
	})["id"].(string)
	out := overREST(ctx, t, registry, http.MethodPost, "/items/"+direct+":move",
		`{"target_parent_id":null,"target_collection_id":"`+collection.String()+`"}`)
	if out.Code != http.StatusUnprocessableEntity || problemOf(t, out).DetailCode != "items.parent_item_required" {
		t.Errorf("moving the task's activity to the collection answered %d: %s", out.Code, out.Body)
	}
	if item := placed(ctx, t, direct); item.ParentID.String() != task {
		t.Error("the refused move moved the activity")
	}
}

// Moves in both directions: an activity out of its work package to the task (REST), and the task's
// own activity into a work package (MCP).
func TestAnActivityMovesBetweenItsTaskAndAWorkPackage(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	registry := levelCatalogueFor(ctx, t)
	task, pack, leaf := levelTask(ctx, t, registry, collection)

	up := overREST(ctx, t, registry, http.MethodPost, "/items/"+leaf+":move", `{"target_parent_id":"`+task+`"}`)
	if up.Code != http.StatusOK {
		t.Fatalf("moving the activity out of its work package answered %d: %s", up.Code, up.Body)
	}
	if item := placed(ctx, t, leaf); item.ParentID.String() != task || item.Depth != 2 {
		t.Errorf("after the move the activity sits under %s at depth %d", item.ParentID, item.Depth)
	}

	toolCall(ctx, t, registry, "move_work_item", map[string]any{"item_id": leaf, "target_parent_id": pack})
	if item := placed(ctx, t, leaf); item.ParentID.String() != pack || item.Depth != 3 {
		t.Errorf("after the move back the activity sits under %s at depth %d", item.ParentID, item.Depth)
	}
}

// The bulk performs the same two operations through the catalogue: a creation directly under the
// task and a move out of the work package.
func TestTheBulkCreatesAndMovesATasksActivity(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	registry := levelCatalogueFor(ctx, t)
	task, _, leaf := levelTask(ctx, t, registry, collection)

	out, err := registry.Invoke(ctx, "BulkUpdateWorkItems", levelWriter(tenantB, authorB), usecase.Input{
		"atomic": true,
		"operations": []any{
			map[string]any{"op": "CREATE_ITEM", "payload": map[string]any{
				"type": "ACTIVITY", "parent_id": task, "title": "Bulk activity",
			}},
			map[string]any{"op": "MOVE_ITEM", "item_id": leaf, "payload": map[string]any{
				"target_parent_id": task,
			}},
		},
	})
	if err != nil {
		t.Fatalf("the bulk was refused: %v", err)
	}
	if out.Int("applied") != 2 || out.Int("failed") != 0 {
		t.Fatalf("%d applied, %d failed: %v", out.Int("applied"), out.Int("failed"), out["results"])
	}
	if item := placed(ctx, t, leaf); item.ParentID.String() != task {
		t.Errorf("the bulk left the activity under %s", item.ParentID)
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM work_item
		WHERE tenant_id = $1 AND parent_id = $2 AND type = 'ACTIVITY'`, tenantB.String(), task); rows != 2 {
		t.Errorf("%d activities sit directly under the task, want 2", rows)
	}
}

// An offline device pushes the same two things: a creation directly under the task, and a move.
func TestAPushCreatesAndMovesATasksActivity(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	registry := levelCatalogueFor(ctx, t)
	task, _, leaf := levelTask(ctx, t, registry, collection)

	push := pushFor(ctx, t, 5*time.Minute)
	push.Catalogue = registry
	push.Clocks = postgres.NewChangeLog()
	reading, _ := shared.NewHLC(time.Now(), 1, "dev-a")
	pushedID := freshID(t)

	response, err := push.Push(ctx, pushActor(tenantB, authorB), syncservice.PushRequest{
		DeviceID: freshID(t), Platform: "hubctl",
		Mutations: []syncservice.Mutation{
			{
				OpID: freshID(t), Kind: syncdomain.ItemCreate, ItemID: pushedID,
				Payload: map[string]any{"type": "ACTIVITY", "parent_id": task, "title": "Pushed from the train"},
			},
			{
				OpID: freshID(t), Kind: syncdomain.Move, ItemID: shared.MustParseID(leaf), HLC: reading.String(),
				Payload: map[string]any{"parent_id": task},
			},
		},
	})
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	for i, result := range response.Results {
		if result.Result != syncdomain.Applied {
			t.Errorf("mutation %d answered %+v", i, result)
		}
	}
	if item := placed(ctx, t, pushedID.String()); item.ParentID.String() != task || item.Type != domain.ItemActivity {
		t.Errorf("the pushed activity sits as %s under %s", item.Type, item.ParentID)
	}
	if item := placed(ctx, t, leaf); item.ParentID.String() != task {
		t.Errorf("the pushed move left the activity under %s", item.ParentID)
	}
}

// A template whose task holds a work package and an activity side by side is defined, stamped out,
// and the result duplicated - each against the migrated profile, through the catalogue.
func TestAMixedTaskIsDefinedStampedAndDuplicated(t *testing.T) {
	ctx := context.Background()
	tenant, author, collection := seedTemplateTenant(ctx, t)
	registry := levelCatalogueFor(ctx, t)
	actor := templateActor(tenant, author, "Europe/Berlin")

	template, err := registry.Invoke(ctx, "CreateTemplate", actor, usecase.Input{
		"scope_type": string(domain.TemplateScopeCollection), "scope_id": collection.String(),
		"name": "Renovate " + freshName(t), "root_type": "TASK",
		"nodes": []any{map[string]any{
			"type": "TASK", "title": "Renovate the bathroom",
			"children": []any{
				map[string]any{"type": "WORK_PACKAGE", "title": "Tiles", "children": []any{
					map[string]any{"type": "ACTIVITY", "title": "Buy grout"},
				}},
				map[string]any{"type": "ACTIVITY", "title": "Call the plumber"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("a mixed template was refused at the definition: %v", err)
	}

	stamped, err := registry.Invoke(ctx, "InstantiateTemplate", actor, usecase.Input{
		"template_id": template.String("id"), "collection_id": collection.String(), "anchor_date": "2026-09-07",
	})
	if err != nil {
		t.Fatalf("stamping the mixed template failed: %v", err)
	}
	if stamped.Int("created") != 4 {
		t.Fatalf("the stamp created %v entries, want 4", stamped["created"])
	}
	root := rootOf(t, stamped)
	if kinds := kindsUnder(ctx, t, tenant, root); kinds != "ACTIVITY,WORK_PACKAGE" {
		t.Errorf("the stamped task holds %s", kinds)
	}

	copied, err := registry.Invoke(ctx, "DuplicateWorkItem", actor, usecase.Input{
		"item_id": root.String(), "include_subtree": true,
	})
	if err != nil {
		t.Fatalf("duplicating the mixed task failed: %v", err)
	}
	copyRoot := shared.MustParseID(itemOf(t, copied).String("id"))
	if kinds := kindsUnder(ctx, t, tenant, copyRoot); kinds != "ACTIVITY,WORK_PACKAGE" {
		t.Errorf("the copy holds %s", kinds)
	}
}

// A breakdown whose first piece is an activity directly under the task, accepted against the
// database: before a task could hold one, the first node's refusal was the whole answer.
func TestAnAcceptedBreakdownPlantsAnActivityUnderTheTask(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	registry := levelCatalogueFor(ctx, t)
	actor := levelWriter(tenantB, authorB)

	task, err := registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "TASK", "collection_id": collection.String(), "title": "Renovate the bathroom",
	})
	if err != nil {
		t.Fatalf("the task: %v", err)
	}
	taskID := shared.MustParseID(task.String("id"))

	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	suggestions := postgres.NewSuggestionRepository(pageCursors())
	proposal, err := suggestiondomain.New(suggestiondomain.NewInput{
		ID: ids.NewID(), TenantID: tenantB, TargetType: suggestiondomain.TargetWorkItem, TargetID: taskID,
		Kind: suggestiondomain.KindDecomposition,
		Payload: map[string]any{"children": []any{
			map[string]any{"type": "ACTIVITY", "title": "Call the plumber"},
			map[string]any{"type": "WORK_PACKAGE", "title": "Tiles", "children": []any{
				map[string]any{"type": "ACTIVITY", "title": "Buy grout"},
			}},
		}},
		Provenance: suggestiondomain.Provenance{
			Model: "stub", PromptID: "decompose", PromptVersion: "1", ProducedAt: time.Now().UTC(),
		},
		InputDigest: suggestiondomain.Digest("Renovate the bathroom", ""),
		Now:         time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("building the proposal: %v", err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return suggestions.Record(ctx, proposal)
	}); err != nil {
		t.Fatalf("storing the proposal: %v", err)
	}

	cases := suggestion.Cases{
		Suggestions: suggestions, Targets: suggestion.EntryTargets{Catalogue: registry},
		Authorizer: realAuthoriser(ctx, t), Catalogue: registry,
		Audit: postgres.NewAuditSink(ids), UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)),
		Clock: clockadapter.System{},
	}
	accepting, err := usecase.NewRegistry(nil, suggestion.AcceptSuggestion{Cases: cases}.Descriptor())
	if err != nil {
		t.Fatalf("building the acceptance: %v", err)
	}
	if _, err := accepting.Invoke(ctx, "AcceptSuggestion", actor, usecase.Input{
		"suggestion_id": proposal.ID.String(),
	}); err != nil {
		t.Fatalf("accepting the breakdown failed: %v", err)
	}

	if kinds := kindsUnder(ctx, t, tenantB, taskID); kinds != "ACTIVITY,WORK_PACKAGE" {
		t.Errorf("the accepted breakdown left %s under the task", kinds)
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM work_item WHERE tenant_id = $1 AND path LIKE $2 || '%'`,
		tenantB.String(), task.String("path")); rows != 4 {
		t.Errorf("%d entries in the task's subtree, want 4", rows)
	}
}

// rootOf reads the stamped tree's top entry off the instantiation's answer.
func rootOf(t *testing.T, stamped usecase.Output) shared.ID {
	t.Helper()
	id, err := shared.ParseID(stamped.String("root_item_id"))
	if err != nil {
		t.Fatalf("the stamp answered no root: %v", stamped)
	}
	return id
}

// itemOf reads the entry a duplicate answers, whichever key it carries it under.
func itemOf(t *testing.T, out usecase.Output) usecase.Output {
	t.Helper()
	if item, nested := out["item"].(usecase.Output); nested {
		return item
	}
	return out
}

// kindsUnder lists the kinds directly under an entry, sorted and without repeats.
func kindsUnder(ctx context.Context, t *testing.T, tenant, parent shared.ID) string {
	t.Helper()
	var kinds string
	if err := adminPool(ctx, t).QueryRow(ctx, `SELECT coalesce(string_agg(DISTINCT type::text, ',' ORDER BY type::text), '')
		FROM work_item WHERE tenant_id = $1 AND parent_id = $2`, tenant.String(), parent.String()).Scan(&kinds); err != nil {
		t.Fatalf("reading the kinds under %s: %v", parent, err)
	}
	return kinds
}
