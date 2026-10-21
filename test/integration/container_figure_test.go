// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/infrastructure/automation"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	textadapter "github.com/Jersyfi/hubtask/infrastructure/text"
	"github.com/Jersyfi/hubtask/presentation/mcp"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// What counts in a collection, through every door against PostgreSQL (UC-WRK-29 checks 1-3): the
// figure on the collection's read over REST and MCP, none on a hub or a trashed collection, and the
// refusal of progress for a list on REST, MCP and an automation action. The production wiring below
// the adapters; the clock fixed on Thursday 22 October 2026, 12:00 in Berlin.

var figureMoment = time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)

func figureCatalogue(t *testing.T) *usecase.Registry {
	t.Helper()
	ctx := context.Background()
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(figureMoment)
	ids := clockadapter.NewUUIDv7(fixed)
	hybrid, err := clockadapter.NewHybridClock(fixed, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	sink := postgres.NewAuditSink(ids)
	authoriser := access.Service{
		Memberships: postgres.NewMembershipRepository(), UnitOfWork: unitOfWork, Audit: sink, Clock: fixed,
	}
	containers := containerRepo()
	resolver := work.StructureResolver{
		Containers: containers, Structures: postgres.NewStructureRepository(),
		Profiles: postgres.NewCapabilityProfileRepository(),
	}
	figures := work.FigureReader{Figures: postgres.NewFigureRepository(), Clock: fixed}
	writer := work.ContainerWriter{
		Containers: containers, Authorizer: authoriser, Events: postgres.NewOutbox(jobQueue(t)),
		Changes: postgres.NewChangeLog(), Audit: sink, UnitOfWork: unitOfWork, Clock: fixed,
		IDs: ids, HLC: hybrid, Text: textadapter.Forms{},
	}
	registry, err := usecase.NewRegistry(nil,
		work.CreateContainer{
			Containers: containers, Authorizer: authoriser, OwnHubs: authoriser,
			Grants:      postgres.NewMembershipGrantRepository(pageCursors()),
			PrivateHubs: privateHubsRule(unitOfWork), Events: writer.Events, Changes: writer.Changes,
			Audit: sink, UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
			Text: textadapter.Forms{}, Structure: resolver,
		}.Descriptor(),
		work.GetContainer{
			Containers: containers, Authorizer: authoriser, UnitOfWork: unitOfWork,
			Structure: resolver, Figures: figures,
		}.Descriptor(),
		work.ListContainers{
			Containers: containers, Authorizer: authoriser, Reader: authoriser, UnitOfWork: unitOfWork,
			Structure: resolver, Figures: figures,
		}.Descriptor(),
		work.SetContainerStructure{
			Writer: writer, Structure: resolver, Epochs: postgres.NewEpochRepository(),
		}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	return registry
}

// figureReader is tenant B's owner. Tenant B, because another test in this package narrows tenant
// A's installation profile to a task without dates (capability_profile_test.go), and the figures
// here are about a workspace whose installation offers every field.
func figureReader() appshared.ActorContext {
	actor := structureActor(tenantB, authorB)
	actor.TimeZone, actor.WeekStart = "Europe/Berlin", "MONDAY"
	return actor
}

func createFigured(ctx context.Context, t *testing.T, registry *usecase.Registry, in usecase.Input) string {
	t.Helper()
	out, err := registry.Invoke(ctx, "CreateContainer", figureReader(), in)
	if err != nil {
		t.Fatalf("creating %v: %v", in, err)
	}
	return out.String("id")
}

func restGet(ctx context.Context, t *testing.T, registry *usecase.Registry, path string) (int, map[string]any) {
	t.Helper()
	controller := rest.NewRestController()
	controller.UseCases = registry
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, figureReader()), http.MethodGet,
		rest.APIBasePath+path, nil)
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	var body map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	return recorder.Code, body
}

func figureIn(container map[string]any) map[string]any {
	figure, _ := container["figure"].(map[string]any)
	return figure
}

func TestACollectionsReadAnswersWhatCountsOnEveryDoor(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := figureCatalogue(t)

	hub := createFigured(ctx, t, registry, usecase.Input{"type": "HUB", "name": freshName(t), "preset": "PROJECT"})
	project := createFigured(ctx, t, registry, usecase.Input{"type": "COLLECTION", "parent_id": hub, "name": freshName(t)})
	list := createFigured(ctx, t, registry, usecase.Input{
		"type": "COLLECTION", "parent_id": hub, "name": freshName(t), "preset": "LIST",
	})
	household := createFigured(ctx, t, registry, usecase.Input{
		"type": "COLLECTION", "parent_id": hub, "name": freshName(t), "preset": "TASKS",
	})
	trashed := createFigured(ctx, t, registry, usecase.Input{"type": "COLLECTION", "parent_id": hub, "name": freshName(t)})

	in := func(collection string, row entryRow) *seededEntry {
		return seedEntry(ctx, t, tenantB, shared.MustParseID(collection), row)
	}
	task := in(project, entryRow{})
	in(project, entryRow{itemType: "WORK_PACKAGE", parent: task, completedAt: "2026-10-21T08:00:00Z"})
	in(project, entryRow{itemType: "WORK_PACKAGE", parent: task})
	in(project, entryRow{completedAt: "2026-10-20T08:00:00Z", archived: true})
	in(list, entryRow{})
	in(list, entryRow{})
	in(list, entryRow{completedAt: "2026-10-21T08:00:00Z"})
	in(household, entryRow{dueAt: "2026-10-22T08:00:00Z"})
	in(household, entryRow{dueAt: "2026-10-25T08:00:00Z"})
	if _, err := adminPool(ctx, t).Exec(ctx, `UPDATE container SET deleted_at = now() WHERE id = $1`, trashed); err != nil {
		t.Fatalf("trashing: %v", err)
	}

	// The level over REST: one figure per collection, the kind its structure counts.
	code, page := restGet(ctx, t, registry, "/containers?parent_id="+hub+"&page_size=200")
	if code != http.StatusOK {
		t.Fatalf("the level answered %d %v", code, page)
	}
	rows, _ := page["data"].([]any)
	byID := map[string]map[string]any{}
	for _, row := range rows {
		container, _ := row.(map[string]any)
		id, _ := container["id"].(string)
		byID[id] = container
	}
	if got := figureIn(byID[project]); got["kind"] != "PROGRESS" || got["done"] != 2.0 || got["total"] != 3.0 || got["today"] != "2026-10-22" {
		t.Errorf("the project's figure %v", got)
	}
	if got := figureIn(byID[list]); got["kind"] != "OPEN" || got["open"] != 2.0 {
		t.Errorf("the list's figure %v", got)
	}
	if got := figureIn(byID[household]); got["kind"] != "DUE" || got["due"] != 1.0 {
		t.Errorf("the household's figure %v", got)
	}

	// The hub level: no figure on a hub.
	_, hubs := restGet(ctx, t, registry, "/containers?type=HUB&page_size=200")
	hubRows, _ := hubs["data"].([]any)
	for _, row := range hubRows {
		if container, _ := row.(map[string]any); container["figure"] != nil {
			t.Errorf("a hub answered a figure: %v", container["figure"])
		}
	}

	// A trashed collection read by its id: no figure.
	if code, body := restGet(ctx, t, registry, "/containers/"+trashed); code != http.StatusOK || body["figure"] != nil {
		t.Errorf("the trashed collection answered %d %v", code, body["figure"])
	}

	// One collection over MCP: the same figure.
	call, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{
		"name": "get_container", "arguments": map[string]any{"container_id": list},
	}})
	server := mcp.Server{Catalogue: registry, Name: "hubtask", Version: "test"}
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, figureReader()), http.MethodPost, mcp.Path,
		bytes.NewReader(call))
	answer := httptest.NewRecorder()
	server.ServeHTTP(answer, request)
	if !strings.Contains(answer.Body.String(), `\"figure\":{`) && !strings.Contains(answer.Body.String(), `"figure":{`) {
		t.Errorf("MCP answered %s", answer.Body)
	}
}

// UC-WRK-29 check 2: progress for a list is refused the same way on REST, MCP and an automation
// action, nothing is written, and the structure's change record carries no figure.
func TestProgressForAListIsRefusedOnEveryDoor(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := figureCatalogue(t)
	actor := figureReader()
	hub := createFigured(ctx, t, registry, usecase.Input{"type": "HUB", "name": freshName(t), "preset": "LIST"})
	list := createFigured(ctx, t, registry, usecase.Input{"type": "COLLECTION", "parent_id": hub, "name": freshName(t)})
	progress := map[string]any{"counts": map[string]any{"figure": "PROGRESS"}}

	document, _ := json.Marshal(map[string]any{"settings": progress})
	recorder := restStructure(ctx, t, registry, actor, list, string(document))
	var problem map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &problem)
	if recorder.Code != http.StatusUnprocessableEntity || problem["detail_code"] != "structure.figure_not_offered" {
		t.Errorf("REST answered %d %v", recorder.Code, problem)
	}

	call, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{
		"name": "set_container_structure", "arguments": map[string]any{"container_id": list, "settings": progress},
	}})
	server := mcp.Server{Catalogue: registry, Name: "hubtask", Version: "test"}
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, actor), http.MethodPost, mcp.Path,
		bytes.NewReader(call))
	answer := httptest.NewRecorder()
	server.ServeHTTP(answer, request)
	if !strings.Contains(answer.Body.String(), `"isError":true`) || !strings.Contains(answer.Body.String(), "structure.figure_not_offered") {
		t.Errorf("MCP answered %s", answer.Body)
	}

	rule := actor
	rule.Kind = appshared.ActorAutomation
	_, err := automation.NewActionDispatcher(registry).Dispatch(ctx, rule, automation.Action{
		Kind: "SET_CONTAINER_STRUCTURE", Params: map[string]any{"container_id": list, "settings": progress},
	}, nil)
	if refusal := shared.AsError(err); refusal == nil || refusal.DetailCode != "structure.figure_not_offered" ||
		!errors.Is(err, shared.ErrValidation) {
		t.Errorf("the automation action answered %v", err)
	}
	if version := countIn(ctx, t, `SELECT version FROM container WHERE id = $1`, list); version != 1 {
		t.Errorf("version %d after three refusals", version)
	}

	// Accepted with the levels that carry it; the record offline clients read has no figure.
	recorder = restStructure(ctx, t, registry, actor, list,
		`{"settings":{"counts":{"figure":"PROGRESS"},"levels":[{"type":"TASK"},{"type":"WORK_PACKAGE"}]}}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("progress with two levels answered %d %s", recorder.Code, recorder.Body)
	}
	// Narrowed back to one level: accepted, shown as open entries, and said so.
	recorder = restStructure(ctx, t, registry, actor, list, `{"settings":{"levels":[{"type":"TASK"}]}}`)
	var narrowed map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &narrowed)
	clipped, _ := narrowed["clipped_by_levels"].([]any)
	structure, _ := narrowed["structure"].(map[string]any)
	clip, _ := structure["counts_clipped"].(map[string]any)
	if recorder.Code != http.StatusOK || len(clipped) != 1 || clipped[0] != "counts" || clip["reason"] != "ONE_LEVEL" {
		t.Errorf("the narrowing answered %d %v %v", recorder.Code, narrowed["clipped_by_levels"], structure["counts_clipped"])
	}
	if code, body := restGet(ctx, t, registry, "/containers/"+list); code != http.StatusOK || figureIn(body)["kind"] != "OPEN" {
		t.Errorf("the narrowed list counts %v", body["figure"])
	}
	if records := countIn(ctx, t, `SELECT count(*) FROM change_log WHERE entity_id = $1 AND payload::text LIKE '%"figure":{%'`,
		list); records != 0 {
		t.Errorf("%d change records carry a figure", records)
	}
	if records := countIn(ctx, t, `SELECT count(*) FROM change_log WHERE entity_id = $1`, list); records == 0 {
		t.Error("the structure was not recorded for offline clients at all")
	}
}
