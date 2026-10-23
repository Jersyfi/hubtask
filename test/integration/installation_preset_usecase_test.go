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
	"testing"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	textadapter "github.com/Jersyfi/hubtask/infrastructure/text"
)

// The installation's presets through the control plane's use cases and REST against PostgreSQL
// (UC-INS-18 checks 1, 3 and 4): created, changed and withdrawn by an operator, one made the default
// with locks, and none of it in a single installation, for a workspace's administrator, or while a
// file enforces the level.

func installationPresetCatalogue(t *testing.T, tenancy env.TenancyMode, settings postgres.InstanceSettingRepository) *usecase.Registry {
	t.Helper()
	writer := installationPresetWriter(t, tenancy, settings)
	registry, err := usecase.NewRegistry(nil,
		adminservice.ListInstallationPresets{Writer: writer}.Descriptor(),
		adminservice.CreateInstallationPreset{Writer: writer}.Descriptor(),
		adminservice.ChangeInstallationPreset{Writer: writer}.Descriptor(),
		adminservice.WithdrawInstallationPreset{Writer: writer}.Descriptor(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func installationPresetWriter(
	t *testing.T, tenancy env.TenancyMode, settings postgres.InstanceSettingRepository,
) adminservice.InstallationPresetWriter {
	t.Helper()
	ctx := context.Background()
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	writer := adminservice.InstallationPresetWriter{
		Instance: adminservice.InstanceWriter{
			Settings: settings, Journal: postgres.NewInstanceJournal(pageCursors()),
			UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)), Clock: clockadapter.System{}, IDs: ids,
		},
		Presets:  postgres.NewInstallationPresetRepository(),
		Profiles: postgres.NewCapabilityProfileRepository(), Tenancy: tenancy, Text: textadapter.Forms{},
	}
	return writer
}

func presetOperator(t *testing.T) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: freshID(t), AccountID: freshID(t), AccountName: "Olga Operator",
		Scopes: []string{"admin:tenants"},
	}
}

func TestAnOperatorKeepsTheInstallationsPresetsThroughTheAPI(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	registry := installationPresetCatalogue(t, env.TenancyMulti, postgres.NewInstanceSettingRepository())
	operator := presetOperator(t)

	// Created over REST from Hubtask's Flow, with a field of its own on top.
	name := freshName(t)
	created := restPreset(ctx, t, registry, operator, http.MethodPost, "/admin/presets",
		fmt.Sprintf(`{"name": %q, "preset": "FLOW", "settings": {"levels": [{"type": "TASK", "name": "Order"}]}}`, name), "")
	if created.Code != http.StatusCreated {
		t.Fatalf("creating answered %d: %s", created.Code, created.Body)
	}
	var preset struct {
		ID        string         `json:"id"`
		Name      string         `json:"name"`
		Version   int            `json:"version"`
		IsDefault bool           `json:"is_default"`
		Locks     []string       `json:"locks"`
		Settings  map[string]any `json:"settings"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &preset); err != nil {
		t.Fatal(err)
	}
	withdrawnAtEnd(ctx, t, shared.MustParseID(preset.ID))
	views, _ := preset.Settings["views"].(map[string]any)
	if preset.Name != name || preset.Version != 1 || preset.IsDefault || len(preset.Locks) != 0 ||
		views["default"] != "KANBAN" || created.Header().Get("ETag") != `"1"` {
		t.Fatalf("created %+v (ETag %s)", preset, created.Header().Get("ETag"))
	}

	// The same name again is refused; a lock on a preset that is not the default too.
	if again := restPreset(ctx, t, registry, operator, http.MethodPost, "/admin/presets",
		fmt.Sprintf(`{"name": %q}`, name), ""); again.Code != http.StatusConflict {
		t.Errorf("a name taken answered %d: %s", again.Code, again.Body)
	}
	if _, err := registry.Invoke(ctx, adminservice.ChangeInstallationPresetName, operator, usecase.Input{
		"preset_id": preset.ID, "locks": []any{"views"},
	}); detailOf(err) != "presets.locks_need_default" {
		t.Errorf("a lock on a preset that is not the default answered %v", err)
	}

	// Made the default with a lock; renamed on a stale version is refused.
	changed := restPreset(ctx, t, registry, operator, http.MethodPatch, "/admin/presets/"+preset.ID,
		`{"is_default": true, "locks": ["views", "fields.TASK.DUE_DATE"]}`, `"1"`)
	if changed.Code != http.StatusOK {
		t.Fatalf("making it the default answered %d: %s", changed.Code, changed.Body)
	}
	if err := json.Unmarshal(changed.Body.Bytes(), &preset); err != nil {
		t.Fatal(err)
	}
	if !preset.IsDefault || len(preset.Locks) != 2 || preset.Version != 2 {
		t.Errorf("the default reads %+v", preset)
	}
	if stale := restPreset(ctx, t, registry, operator, http.MethodPatch, "/admin/presets/"+preset.ID,
		`{"name": "Craft order"}`, `"1"`); stale.Code != http.StatusPreconditionFailed && stale.Code != http.StatusConflict {
		t.Errorf("a stale change answered %d", stale.Code)
	}

	// The default is not withdrawn from under the installation; once it is none, it is.
	if refused := restPreset(ctx, t, registry, operator, http.MethodDelete, "/admin/presets/"+preset.ID, "", ""); refused.Code != http.StatusConflict {
		t.Errorf("withdrawing the default answered %d: %s", refused.Code, refused.Body)
	}
	if _, err := registry.Invoke(ctx, adminservice.ChangeInstallationPresetName, operator, usecase.Input{
		"preset_id": preset.ID, "is_default": false,
	}); err != nil {
		t.Fatalf("making none the default: %v", err)
	}
	if withdrawn := restPreset(ctx, t, registry, operator, http.MethodDelete, "/admin/presets/"+preset.ID, "", ""); withdrawn.Code != http.StatusNoContent {
		t.Fatalf("withdrawing answered %d: %s", withdrawn.Code, withdrawn.Body)
	}
	listed := restPreset(ctx, t, registry, operator, http.MethodGet, "/admin/presets", "", "")
	if listed.Code != http.StatusOK || json.Valid(listed.Body.Bytes()) && containsID(listed.Body.Bytes(), preset.ID) {
		t.Errorf("after the withdrawal the list answered %d: %s", listed.Code, listed.Body)
	}

	// The journal holds the three writes by id, never the name.
	var journalled int
	if err := adminPool(ctx, t).QueryRow(ctx, `SELECT count(*) FROM instance_event
		WHERE details->>'preset_id' = $1 AND details::text NOT LIKE '%' || $2 || '%'`, preset.ID, name).Scan(&journalled); err != nil {
		t.Fatal(err)
	}
	if journalled < 4 {
		t.Errorf("the journal holds %d entries for the preset, want its create, two changes and the withdrawal", journalled)
	}
}

func containsID(body []byte, id string) bool {
	var answer struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &answer)
	for _, row := range answer.Data {
		if row.ID == id {
			return true
		}
	}
	return false
}

// Check 4 and the negatives: a single installation has no installation level; a workspace's
// administrator is no operator; a file that enforces the level is the one writer.
func TestNobodyButAnOperatorOfAMultiInstallationKeepsItsPresets(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	create := usecase.Input{"name": freshName(t)}

	single := installationPresetCatalogue(t, env.TenancySingle, postgres.NewInstanceSettingRepository())
	for name, in := range map[string]usecase.Input{
		adminservice.ListInstallationPresetsName: {}, adminservice.CreateInstallationPresetName: create,
	} {
		if _, err := single.Invoke(ctx, name, presetOperator(t), in); detailOf(err) != "admin.multi_mode_required" {
			t.Errorf("%s in a single installation answered %v", name, err)
		}
	}

	multi := installationPresetCatalogue(t, env.TenancyMulti, postgres.NewInstanceSettingRepository())
	administrator := structureActor(tenantA, authorA)
	administrator.Scopes = append(administrator.Scopes, "workspace:manage")
	if _, err := multi.Invoke(ctx, adminservice.CreateInstallationPresetName, administrator, create); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a workspace's administrator creating answered %v", err)
	}

	enforced := installationPresetCatalogue(t, env.TenancyMulti, postgres.FromFile("/etc/hubtask/instance.yaml", true))
	if _, err := enforced.Invoke(ctx, adminservice.CreateInstallationPresetName, presetOperator(t), create); detailOf(err) != "admin.instance_enforced_from_file" {
		t.Errorf("creating while a file enforces answered %v", err)
	}
}
