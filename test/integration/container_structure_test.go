// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

// The structure of hubs and collections against PostgreSQL, through every door the catalogue has:
// REST, MCP and an automation action reach the same use case and meet the same refusals
// (UC-WRK-23 checks 2, 5, 6, 8). Everything below the adapters is the production wiring.

func structureCatalogue(t *testing.T) *usecase.Registry {
	t.Helper()
	ctx := context.Background()

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
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
		Profiles: postgres.NewCapabilityProfileRepository(), Presets: postgres.NewPresetRepository(),
		Installation: postgres.NewInstallationPresetRepository(),
	}
	desk := work.PresetDesk{
		Presets: resolver.Presets, Structure: resolver, Authorizer: authoriser, Audit: sink,
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, Text: textadapter.Forms{},
	}
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
			Buckets: postgres.NewBucketRepository(), Workspaces: postgres.NewWorkspaceSettingsRepository(),
			Renderer: catalogueRenderer(t),
		}.Descriptor(),
		work.GetContainer{
			Containers: containers, Authorizer: authoriser, UnitOfWork: unitOfWork, Structure: resolver,
		}.Descriptor(),
		work.SetContainerStructure{
			Writer: writer, Structure: resolver, Epochs: postgres.NewEpochRepository(),
			Labels: labelRepo(),
		}.Descriptor(),
		work.ListPresets{
			Profiles: resolver.Profiles, Presets: resolver.Presets, Installation: resolver.Installation,
			UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.SavePreset{Desk: desk}.Descriptor(),
		work.ChangePreset{Desk: desk}.Descriptor(),
		work.WithdrawPreset{Desk: desk}.Descriptor(),
		work.SetWorkspaceStructure{
			Structure: resolver, Authorizer: authoriser, Changes: writer.Changes,
			Epochs: postgres.NewEpochRepository(), Audit: sink, UnitOfWork: unitOfWork, Clock: fixed,
			HLC: hybrid, Text: textadapter.Forms{},
		}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	return registry
}

func structureActor(tenant, account shared.ID) appshared.ActorContext {
	actor := administrator(tenant, account)
	actor.Scopes = []string{"containers:write", "containers:read"}
	return actor
}

func createIn(ctx context.Context, t *testing.T, registry *usecase.Registry, in usecase.Input) string {
	t.Helper()
	out, err := registry.Invoke(ctx, "CreateContainer", structureActor(tenantA, authorA), in)
	if err != nil {
		t.Fatalf("creating %v: %v", in, err)
	}
	return out.String("id")
}

func structureOf(ctx context.Context, t *testing.T, registry *usecase.Registry, id string) map[string]any {
	t.Helper()
	out, err := registry.Invoke(ctx, "GetContainer", structureActor(tenantA, authorA), usecase.Input{"container_id": id})
	if err != nil {
		t.Fatalf("reading %s: %v", id, err)
	}
	structure, _ := out["structure"].(map[string]any)
	return structure
}

func sourceIn(structure map[string]any, setting string) string {
	sources, _ := structure["sources"].(map[string]any)
	source, _ := sources[setting].(string)
	return source
}

// UC-WRK-23 check 2, stored and read back: a hub from a preset, one collection following it and one
// with its own preset; one setting of the hub changed on its own reaches the follower only.
func TestAHubsSettingReachesTheCollectionsThatFollowIt(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := structureCatalogue(t)

	hub := createIn(ctx, t, registry, usecase.Input{"type": "HUB", "name": freshName(t), "preset": "LIST"})
	follower := createIn(ctx, t, registry, usecase.Input{"type": "COLLECTION", "parent_id": hub, "name": freshName(t)})
	own := createIn(ctx, t, registry, usecase.Input{
		"type": "COLLECTION", "parent_id": hub, "name": freshName(t), "preset": "PROJECT",
	})

	if source := sourceIn(structureOf(ctx, t, registry, follower), "levels"); source != "HUB" {
		t.Fatalf("a collection made in a List hub reads its levels from %s", source)
	}

	_, err := registry.Invoke(ctx, "SetContainerStructure", structureActor(tenantA, authorA), usecase.Input{
		"container_id": hub,
		"settings":     map[string]any{"celebration": "ALL"},
	})
	if err != nil {
		t.Fatalf("changing the hub: %v", err)
	}

	followed := structureOf(ctx, t, registry, follower)
	settings, _ := followed["settings"].(map[string]any)
	if settings["celebration"] != "ALL" || sourceIn(followed, "celebration") != "HUB" {
		t.Errorf("the follower reads %v", followed)
	}
	kept := structureOf(ctx, t, registry, own)
	if sourceIn(kept, "celebration") != "COLLECTION" || kept["preset"] != "PROJECT" {
		t.Errorf("the collection with its own preset reads %v", kept)
	}
	// Offline clients were told each container's structure in force, the follower's again.
	if rows := countIn(ctx, t, `SELECT count(*) FROM change_log WHERE entity_id = $1 AND payload ? 'structure'`,
		follower); rows != 2 {
		t.Errorf("%d structure entries for the follower, want its creation's and the hub's change", rows)
	}
}

// restStructure changes a structure the way a request does.
func restStructure(ctx context.Context, t *testing.T, registry *usecase.Registry, actor appshared.ActorContext,
	id, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	controller := rest.NewRestController()
	controller.UseCases = registry
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, actor), http.MethodPatch,
		rest.APIBasePath+"/containers/"+id+"/structure", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

// UC-WRK-23 checks 5 and 8: the same widening is refused the same way through REST, MCP and an
// automation action, and nothing is written by any of them.
func TestAWideningIsRefusedTheSameWayOnEveryDoor(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := structureCatalogue(t)
	actor := structureActor(tenantA, authorA)
	collection := createIn(ctx, t, registry, usecase.Input{"type": "HUB", "name": freshName(t)})
	widening := map[string]any{"fields": map[string]any{
		"ACTIVITY": map[string]any{"capabilities": []any{"COMPLETION", "NOTES"}},
	}}

	document, _ := json.Marshal(map[string]any{"settings": widening})
	recorder := restStructure(ctx, t, registry, actor, collection, string(document))
	var problem map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &problem)
	if recorder.Code != http.StatusUnprocessableEntity || problem["code"] != "capability_not_supported" ||
		problem["detail_code"] != "structure.widens" {
		t.Errorf("REST answered %d %v", recorder.Code, problem)
	}

	call, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{
		"name": "set_container_structure", "arguments": map[string]any{"container_id": collection, "settings": widening},
	}})
	server := mcp.Server{Catalogue: registry, Name: "hubtask", Version: "test"}
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, actor), http.MethodPost, mcp.Path,
		bytes.NewReader(call))
	answer := httptest.NewRecorder()
	server.ServeHTTP(answer, request)
	if !strings.Contains(answer.Body.String(), `"isError":true`) || !strings.Contains(answer.Body.String(), "capability_not_supported") {
		t.Errorf("MCP answered %s", answer.Body)
	}

	rule := actor
	rule.Kind = appshared.ActorAutomation
	_, err := automation.NewActionDispatcher(registry).Dispatch(ctx, rule, automation.Action{
		Kind: "SET_CONTAINER_STRUCTURE", Params: map[string]any{"container_id": collection, "settings": widening},
	}, nil)
	if !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Errorf("the automation action answered %v", err)
	}

	if rows := countIn(ctx, t, `SELECT count(*) FROM audit_log WHERE target_id = $1 AND action = 'container.structure_updated'`,
		collection); rows != 0 {
		t.Errorf("%d structure entries after three refusals", rows)
	}
	if version := countIn(ctx, t, `SELECT version FROM container WHERE id = $1`, collection); version != 1 {
		t.Errorf("version %d after three refusals", version)
	}

	// The reset, by name, through the same door: a hub's own value goes, Hubtask's default answers.
	recorder = restStructure(ctx, t, registry, actor, collection, `{"settings":{"celebration":"SUBTLE"}}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("setting answered %d %s", recorder.Code, recorder.Body)
	}
	recorder = restStructure(ctx, t, registry, actor, collection, `{"reset":["celebration"]}`)
	if recorder.Code != http.StatusOK || sourceIn(structureOf(ctx, t, registry, collection), "celebration") != "DEFAULT" {
		t.Errorf("the reset answered %d %s", recorder.Code, recorder.Body)
	}
}

// The cross-tenant negative: another workspace's container is not found, and stays as it was.
func TestAnotherWorkspacesContainerCannotBeShaped(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := structureCatalogue(t)
	hub := createIn(ctx, t, registry, usecase.Input{"type": "HUB", "name": freshName(t), "preset": "LIST"})

	_, err := registry.Invoke(ctx, "SetContainerStructure", structureActor(tenantB, authorB), usecase.Input{
		"container_id": hub, "preset": "PROJECT",
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("tenant B's change answered %v", err)
	}
	if structureOf(ctx, t, registry, hub)["preset"] != "LIST" {
		t.Error("tenant A's hub changed")
	}
}

// UC-WRK-23 check 6: a level's name is stored, answered on the read, and reaches no log line and no
// audit entry in clear (rule 10). The log is the real JSON handler's output, captured around the
// writes and the reads.
func TestALevelNameReachesNoLogAndNoAuditEntryInClear(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := structureCatalogue(t)
	name := "Einkaufsposten " + freshName(t)
	// The done section's name is the same kind of content and is held the same way (UC-WRK-25).
	doneName := "Im Wagen " + freshName(t)

	var captured bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&captured, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	hub := createIn(ctx, t, registry, usecase.Input{"type": "HUB", "name": freshName(t)})
	document, _ := json.Marshal(map[string]any{"settings": map[string]any{
		"levels": []any{map[string]any{"type": "TASK", "name": name, "done_name": doneName}},
	}})
	if recorder := restStructure(ctx, t, registry, structureActor(tenantA, authorA), hub, string(document)); recorder.Code != http.StatusOK {
		t.Fatalf("naming answered %d %s", recorder.Code, recorder.Body)
	}
	// A refused name leaves the log as clean as an accepted one.
	tooLong, _ := json.Marshal(map[string]any{"settings": map[string]any{
		"levels": []any{map[string]any{"type": "TASK", "name": name + strings.Repeat("x", 60)}},
	}})
	if recorder := restStructure(ctx, t, registry, structureActor(tenantA, authorA), hub, string(tooLong)); recorder.Code != http.StatusUnprocessableEntity ||
		strings.Contains(recorder.Body.String(), name) {
		t.Errorf("a refused name answered %d %s", recorder.Code, recorder.Body)
	}

	settings, _ := structureOf(ctx, t, registry, hub)["settings"].(map[string]any)
	levels, _ := settings["levels"].([]any)
	if len(levels) == 0 || levels[0].(map[string]any)["name"] != name ||
		levels[0].(map[string]any)["done_name"] != doneName {
		t.Errorf("the read answers %v", settings["levels"])
	}

	var changes string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT changes::text FROM audit_log WHERE target_id = $1 AND action = 'container.structure_updated'`, hub).
		Scan(&changes); err != nil {
		t.Fatalf("reading the audit entry: %v", err)
	}
	if strings.Contains(changes, name) || strings.Contains(changes, doneName) || !strings.Contains(changes, "structure.levels") {
		t.Errorf("the audit entry is %s", changes)
	}
	if strings.Contains(captured.String(), name) || strings.Contains(captured.String(), doneName) {
		t.Errorf("a log line carries a level's name: %s", captured.String())
	}
}

// forgetWorkspaceStructure leaves no lock behind for the next test of the package to meet.
func forgetWorkspaceStructure(ctx context.Context, t *testing.T, tenant shared.ID) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = adminPool(ctx, t).Exec(context.WithoutCancel(ctx),
			`UPDATE tenant SET settings = settings - 'structure' WHERE id = $1`, tenant.String())
	})
}

// The lock writer and its refusal on every door (UC-WRK-23 checks 7 and 8): an administrator
// locks the views; a collection changing them is refused through REST as through the catalogue;
// the workspace's answer names the lock and who set it; another workspace sees none of it.
func TestAWorkspaceLockRefusesACollectionOnEveryDoor(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	forgetWorkspaceStructure(ctx, t, tenantA)
	forgetWorkspaceStructure(ctx, t, tenantB)
	registry := structureCatalogue(t)
	actor := structureActor(tenantA, authorA)
	actor.Scopes = append(actor.Scopes, "workspace:manage")
	hub := createIn(ctx, t, registry, usecase.Input{"type": "HUB", "name": freshName(t)})

	if _, err := registry.Invoke(ctx, "SetWorkspaceStructure", actor, usecase.Input{
		"locks": []any{"views"}, "default_preset": "TASKS",
	}); err != nil {
		t.Fatalf("locking: %v", err)
	}

	recorder := restStructure(ctx, t, registry, actor, hub,
		`{"settings":{"views":{"offered":["KANBAN"],"default":"KANBAN"}}}`)
	var problem map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &problem)
	params, _ := problem["params"].(map[string]any)
	if recorder.Code != http.StatusUnprocessableEntity || problem["detail_code"] != "structure.setting_locked" ||
		params["origin"] != "WORKSPACE" {
		t.Errorf("REST answered %d %v", recorder.Code, problem)
	}

	answer := work.WorkspaceStructureAnswer{
		Resolver: work.StructureResolver{
			Containers: containerRepo(), Structures: postgres.NewStructureRepository(),
			Profiles: postgres.NewCapabilityProfileRepository(),
		},
		UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)),
	}
	ours, err := answer.WorkspaceStructure(ctx, actor)
	if err != nil {
		t.Fatalf("reading the workspace's structure: %v", err)
	}
	locks, _ := ours["locks"].(map[string]any)
	dates, _ := locks["views"].(map[string]any)
	if dates["locked_by"] != authorA.String() || ours["default_preset"] != "TASKS" {
		t.Errorf("the workspace answers %v", ours)
	}
	// A new hub without a preset starts from the default.
	fresh := createIn(ctx, t, registry, usecase.Input{"type": "HUB", "name": freshName(t)})
	if structureOf(ctx, t, registry, fresh)["preset"] != "TASKS" {
		t.Error("a new hub did not start from the workspace's default")
	}

	theirs, err := answer.WorkspaceStructure(ctx, structureActor(tenantB, authorB))
	if err != nil {
		t.Fatalf("reading tenant B: %v", err)
	}
	if locks, _ := theirs["locks"].(map[string]any); len(locks) != 0 || theirs["default_preset"] != nil {
		t.Errorf("tenant B reads tenant A's structure: %v", theirs)
	}
}

// Tasks offers a step under a task where the installation's task holds one (migration 0185): the
// preset is held to the installation's topology, read from the seeded profiles.
func TestTasksOffersAStepUnderATaskInThisInstallation(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	out, err := structureCatalogue(t).Invoke(ctx, "ListPresets", structureActor(tenantB, authorB), usecase.Input{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	rows, _ := out["data"].([]usecase.Output)
	for _, row := range rows {
		if row["key"] != "TASKS" {
			continue
		}
		settings, _ := row["settings"].(map[string]any)
		levels, _ := settings["levels"].([]any)
		var types []any
		for _, level := range levels {
			types = append(types, level.(map[string]any)["type"])
		}
		if len(types) != 2 || types[0] != "TASK" || types[1] != "ACTIVITY" {
			t.Errorf("Tasks offers %v", types)
		}
		return
	}
	t.Fatal("no Tasks preset")
}
