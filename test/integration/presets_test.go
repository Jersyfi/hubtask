// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// Saved presets through the catalogue and REST against PostgreSQL (UC-WRK-30 checks 1-4): saved for
// oneself or the workspace, offered with their origin to whom they belong, chosen on create,
// changed and withdrawn while what was made from them keeps its settings, the workspace's default.
// Tenant A; the catalogue is container_structure_test.go's.

// presetAdmin may change the workspace's shape in tenant A.
func presetAdmin() appshared.ActorContext {
	actor := structureActor(tenantA, authorA)
	actor.Scopes = append(actor.Scopes, "workspace:manage")
	return actor
}

// presetMember is a person of tenant A who may read everything and shape nothing of the workspace.
func presetMember(ctx context.Context, t *testing.T) appshared.ActorContext {
	t.Helper()
	account := seedAccount(ctx, t, tenantA)
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES ($1, $2, $3, 'TENANT', 'MEMBER')`,
		freshID(t).String(), tenantA.String(), account.String()); err != nil {
		t.Fatalf("granting MEMBER: %v", err)
	}
	actor := structureActor(tenantA, account)
	actor.Scopes = append(actor.Scopes, "workspace:manage")
	return actor
}

func savePreset(
	ctx context.Context, t *testing.T, registry *usecase.Registry, actor appshared.ActorContext,
	origin, container string,
) usecase.Output {
	t.Helper()
	out, err := registry.Invoke(ctx, "SavePreset", actor, usecase.Input{
		"name": freshName(t), "origin": origin, "container_id": container,
	})
	if err != nil {
		t.Fatalf("saving a %s preset: %v", origin, err)
	}
	return out
}

func offeredTo(ctx context.Context, t *testing.T, registry *usecase.Registry, actor appshared.ActorContext) map[string]string {
	t.Helper()
	out, err := registry.Invoke(ctx, "ListPresets", actor, usecase.Input{})
	if err != nil {
		t.Fatalf("listing the presets: %v", err)
	}
	rows, _ := out["data"].([]usecase.Output)
	origins := map[string]string{}
	for _, row := range rows {
		key := row.String("key")
		if key == "" {
			key = row.String("id")
		}
		origins[key] = row.String("origin")
	}
	return origins
}

func detailOf(err error) string {
	if err == nil {
		return ""
	}
	return shared.AsError(err).DetailCode
}

// Check 1 and 2: Save as preset stores a collection's eight settings for the person or, with the
// right to change the workspace's shape, for the workspace; each is offered with its origin, a
// person's own to them alone, and a new collection starts from one by its id.
func TestAPresetIsSavedForOneselfOrTheWorkspaceAndOfferedWithItsOrigin(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := structureCatalogue(t)
	admin, member := presetAdmin(), presetMember(ctx, t)

	hub := createIn(ctx, t, registry, usecase.Input{"type": "HUB", "name": freshName(t), "preset": "PROJECT"})
	source := createIn(ctx, t, registry, usecase.Input{
		"type": "COLLECTION", "parent_id": hub, "name": freshName(t), "preset": "FLOW",
	})
	inForce := structureOf(ctx, t, registry, source)["settings"]

	mine := savePreset(ctx, t, registry, admin, "OWN", source)
	if mine.String("origin") != "OWN" || mine.String("key") != "" || mine.Int("version") != 1 {
		t.Fatalf("an own preset answered %v", mine)
	}
	if !reflect.DeepEqual(mine["settings"], inForce) {
		t.Errorf("saved %v, want the settings in force %v", mine["settings"], inForce)
	}
	if _, err := registry.Invoke(ctx, "SavePreset", member, usecase.Input{
		"name": freshName(t), "origin": "WORKSPACE", "container_id": source,
	}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a member saving for the workspace answered %v, want forbidden", err)
	}
	theirs := savePreset(ctx, t, registry, member, "OWN", source)
	workspace := savePreset(ctx, t, registry, admin, "WORKSPACE", source)

	toAdmin, toMember := offeredTo(ctx, t, registry, admin), offeredTo(ctx, t, registry, member)
	for _, key := range []string{"LIST", "TASKS", "PROJECT", "FLOW"} {
		if toAdmin[key] != "HUBTASK" || toMember[key] != "HUBTASK" {
			t.Errorf("Hubtask's %s offered as %q and %q", key, toAdmin[key], toMember[key])
		}
	}
	if toAdmin[mine.String("id")] != "OWN" || toAdmin[workspace.String("id")] != "WORKSPACE" {
		t.Errorf("the administrator is offered %v", toAdmin)
	}
	if _, seen := toAdmin[theirs.String("id")]; seen {
		t.Errorf("the administrator is offered the member's own preset")
	}
	if _, seen := toMember[mine.String("id")]; seen || toMember[workspace.String("id")] != "WORKSPACE" {
		t.Errorf("the member is offered %v", toMember)
	}

	// Choosing: one's own and the workspace's by id; another person's own is unknown.
	made := createIn(ctx, t, registry, usecase.Input{
		"type": "COLLECTION", "parent_id": hub, "name": freshName(t), "preset_id": mine.String("id"),
	})
	structure := structureOf(ctx, t, registry, made)
	if structure["preset_id"] != mine.String("id") || structure["preset"] != nil ||
		!reflect.DeepEqual(structure["settings"], inForce) {
		t.Errorf("a collection made from one's own preset reads %v", structure)
	}
	// The member may shape this hub, so what refuses is the preset alone.
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role) VALUES ($1, $2, $3, 'HUB', $4, 'OWNER')`,
		freshID(t).String(), tenantA.String(), member.AccountID.String(), hub); err != nil {
		t.Fatalf("granting the member the hub: %v", err)
	}
	if _, err := registry.Invoke(ctx, "CreateContainer", member, usecase.Input{
		"type": "COLLECTION", "parent_id": hub, "name": freshName(t), "preset_id": mine.String("id"),
	}); detailOf(err) != "presets.unknown" {
		t.Errorf("another person's own preset chosen answered %v, want presets.unknown", err)
	}
	if _, err := registry.Invoke(ctx, "CreateContainer", admin, usecase.Input{
		"type": "COLLECTION", "parent_id": hub, "name": freshName(t), "preset": "LIST",
		"preset_id": mine.String("id"),
	}); detailOf(err) != "presets.both_named" {
		t.Errorf("two presets named answered %v, want presets.both_named", err)
	}
	// Shaping an existing collection from the workspace's preset, over REST.
	recorder := restStructure(ctx, t, registry, admin, source,
		fmt.Sprintf(`{"preset_id":%q}`, workspace.String("id")))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), workspace.String("id")) {
		t.Errorf("PATCH structure with a preset_id answered %d %s", recorder.Code, recorder.Body.String())
	}
	// The name stays out of the audit trail (rule 10).
	if rows := countIn(ctx, t, `SELECT count(*) FROM audit_log WHERE target_id = $1 AND changes::text LIKE '%' || $2 || '%'`,
		mine.String("id"), mine.String("name")); rows != 0 {
		t.Errorf("%d audit entries carry the preset's name", rows)
	}
}

// restPreset sends a request to the presets' doors the way a client does.
func restPreset(
	ctx context.Context, t *testing.T, registry *usecase.Registry, actor appshared.ActorContext,
	method, path, body, ifMatch string,
) *httptest.ResponseRecorder {
	t.Helper()
	controller := rest.NewRestController()
	controller.UseCases = registry
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, actor), method,
		rest.APIBasePath+path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if ifMatch != "" {
		request.Header.Set("If-Match", ifMatch)
	}
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

// Checks 3 and 4: a workspace preset is renamed, changed and withdrawn by whoever may change the
// workspace's shape, and what was made from it keeps its settings; an administrator makes one the
// default a new hub starts from, and the default is not withdrawn from under it.
func TestAWorkspacePresetIsChangedAndWithdrawnWhileWhatWasMadeKeepsIt(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	forgetWorkspaceStructure(ctx, t, tenantA)
	registry := structureCatalogue(t)
	admin, member := presetAdmin(), presetMember(ctx, t)

	hub := createIn(ctx, t, registry, usecase.Input{"type": "HUB", "name": freshName(t), "preset": "LIST"})
	source := createIn(ctx, t, registry, usecase.Input{
		"type": "COLLECTION", "parent_id": hub, "name": freshName(t), "preset": "TASKS",
	})
	recorder := restPreset(ctx, t, registry, admin, http.MethodPost, "/presets",
		fmt.Sprintf(`{"name":%q,"origin":"WORKSPACE","container_id":%q}`, freshName(t), source), "")
	if recorder.Code != http.StatusCreated || recorder.Header().Get("ETag") == "" {
		t.Fatalf("POST /presets answered %d %s", recorder.Code, recorder.Body.String())
	}
	var saved struct {
		ID     string `json:"id"`
		Origin string `json:"origin"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &saved); err != nil || saved.Origin != "WORKSPACE" {
		t.Fatalf("POST /presets body %s: %v", recorder.Body.String(), err)
	}
	made := createIn(ctx, t, registry, usecase.Input{
		"type": "COLLECTION", "parent_id": hub, "name": freshName(t), "preset_id": saved.ID,
	})
	before := structureOf(ctx, t, registry, made)["settings"]

	// The member may use it, not change it; another person's own preset is not even found.
	if _, err := registry.Invoke(ctx, "ChangePreset", member, usecase.Input{
		"preset_id": saved.ID, "name": freshName(t),
	}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a member renaming the workspace's preset answered %v, want forbidden", err)
	}
	own := savePreset(ctx, t, registry, admin, "OWN", source)
	if _, err := registry.Invoke(ctx, "WithdrawPreset", member, usecase.Input{
		"preset_id": own.String("id"),
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a member withdrawing another's own preset answered %v, want not found", err)
	}

	renamed := freshName(t)
	recorder = restPreset(ctx, t, registry, admin, http.MethodPatch, "/presets/"+saved.ID,
		fmt.Sprintf(`{"name":%q,"settings":{"celebration":"SUBTLE"}}`, renamed), `"1"`)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), renamed) ||
		!strings.Contains(recorder.Body.String(), `"celebration":"SUBTLE"`) {
		t.Fatalf("PATCH /presets answered %d %s", recorder.Code, recorder.Body.String())
	}
	if stale := restPreset(ctx, t, registry, admin, http.MethodPatch, "/presets/"+saved.ID,
		`{"name":"x"}`, `"1"`); stale.Code != http.StatusPreconditionFailed && stale.Code != http.StatusConflict {
		t.Errorf("a stale PATCH answered %d", stale.Code)
	}
	if after := structureOf(ctx, t, registry, made)["settings"]; !reflect.DeepEqual(after, before) {
		t.Errorf("a collection made from the preset moved with it: %v, was %v", after, before)
	}

	// The default: the workspace's preset, never a person's own.
	if _, err := registry.Invoke(ctx, "SetWorkspaceStructure", admin, usecase.Input{
		"default_preset_id": own.String("id"),
	}); detailOf(err) != "presets.unknown" {
		t.Fatalf("an own preset as the default answered %v, want presets.unknown", err)
	}
	out, err := registry.Invoke(ctx, "SetWorkspaceStructure", admin, usecase.Input{"default_preset_id": saved.ID})
	if err != nil || out["default_preset_id"] != saved.ID || out["default_preset"] != nil {
		t.Fatalf("the default set answered %v, %v", out, err)
	}
	newHub := createIn(ctx, t, registry, usecase.Input{"type": "HUB", "name": freshName(t)})
	if structure := structureOf(ctx, t, registry, newHub); structure["preset_id"] != saved.ID {
		t.Errorf("a hub created without a preset reads %v, want the default", structure)
	}
	if gone := restPreset(ctx, t, registry, admin, http.MethodDelete, "/presets/"+saved.ID, "", ""); gone.Code != http.StatusConflict ||
		!strings.Contains(gone.Body.String(), "presets.workspace_default") {
		t.Fatalf("withdrawing the default answered %d %s", gone.Code, gone.Body.String())
	}
	if _, err := registry.Invoke(ctx, "SetWorkspaceStructure", admin, usecase.Input{"clear_default_preset": true}); err != nil {
		t.Fatalf("clearing the default: %v", err)
	}
	if gone := restPreset(ctx, t, registry, admin, http.MethodDelete, "/presets/"+saved.ID, "", ""); gone.Code != http.StatusNoContent {
		t.Fatalf("withdrawing answered %d %s", gone.Code, gone.Body.String())
	}
	if _, offered := offeredTo(ctx, t, registry, admin)[saved.ID]; offered {
		t.Errorf("a withdrawn preset is still offered")
	}
	if after := structureOf(ctx, t, registry, made)["settings"]; !reflect.DeepEqual(after, before) {
		t.Errorf("a collection made from a withdrawn preset lost its settings: %v", after)
	}
}

// Another workspace neither sees, chooses, changes nor withdraws a preset (T-04).
func TestAnotherWorkspaceCannotUseOrTouchAPreset(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := structureCatalogue(t)
	hub := createIn(ctx, t, registry, usecase.Input{"type": "HUB", "name": freshName(t), "preset": "LIST"})
	preset := savePreset(ctx, t, registry, presetAdmin(), "WORKSPACE", hub).String("id")

	outsider := structureActor(tenantB, authorB)
	outsider.Scopes = append(outsider.Scopes, "workspace:manage")
	if _, seen := offeredTo(ctx, t, registry, outsider)[preset]; seen {
		t.Errorf("tenant B is offered tenant A's preset")
	}
	if _, err := registry.Invoke(ctx, "CreateContainer", outsider, usecase.Input{
		"type": "HUB", "name": freshName(t), "preset_id": preset,
	}); detailOf(err) != "presets.unknown" {
		t.Errorf("tenant B choosing tenant A's preset answered %v", err)
	}
	if _, err := registry.Invoke(ctx, "ChangePreset", outsider, usecase.Input{
		"preset_id": preset, "name": freshName(t),
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("tenant B changing tenant A's preset answered %v", err)
	}
	if _, err := registry.Invoke(ctx, "WithdrawPreset", outsider, usecase.Input{"preset_id": preset}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("tenant B withdrawing tenant A's preset answered %v", err)
	}
	if _, err := registry.Invoke(ctx, "SetWorkspaceStructure", outsider, usecase.Input{"default_preset_id": preset}); detailOf(err) != "presets.unknown" {
		t.Errorf("tenant B's default as tenant A's preset answered %v", err)
	}
	if _, err := registry.Invoke(ctx, "SavePreset", outsider, usecase.Input{
		"name": freshName(t), "origin": "OWN", "container_id": hub,
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("tenant B saving tenant A's hub answered %v", err)
	}
}
