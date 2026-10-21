// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"slices"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

func presetReader() appshared.ActorContext {
	reader := actor()
	reader.Scopes = []string{containersRead}
	return reader
}

// UC-WRK-23 check 2, through the catalogue: four presets, each with all eight settings, held to
// this installation's profiles.
func TestThePresetsAnswerEightSettingsEach(t *testing.T) {
	handler := ListPresets{Profiles: &profiles{rows: systemProfiles()}, UnitOfWork: &unitOfWork{}}
	descriptor := handler.Descriptor()
	if !descriptor.ReadOnly || descriptor.Destructive || descriptor.MCPTool() != "list_presets" {
		t.Errorf("descriptor %+v", descriptor)
	}
	registry, err := usecase.NewRegistry(nil, descriptor)
	if err != nil {
		t.Fatalf("the catalogue refused the entry: %v", err)
	}

	out, err := registry.Invoke(context.Background(), ListPresetsName, presetReader(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing failed: %v", err)
	}
	rows, _ := out["data"].([]usecase.Output)
	var keys []string
	for _, row := range rows {
		keys = append(keys, row["key"].(string))
		settings, _ := row["settings"].(map[string]any)
		if len(settings) != 8 {
			t.Errorf("%s answers %d settings: %v", row["key"], len(settings), settings)
		}
	}
	if !slices.Equal(keys, []string{"LIST", "TASKS", "PROJECT", "FLOW"}) {
		t.Errorf("keys %v", keys)
	}

	// The installation's own profile bounds them: the fixture's task carries no due date, so no
	// preset offers one.
	project := rows[2]["settings"].(map[string]any)
	fields := project["fields"].(map[string]any)
	capabilities := fields[string(domain.ItemTask)].(map[string]any)["capabilities"].([]any)
	if slices.Contains(capabilities, any(string(domain.CapabilityDueDate))) {
		t.Errorf("a preset offers what the installation does not: %v", capabilities)
	}
}

func TestThePresetsAskForTheReadScope(t *testing.T) {
	handler := ListPresets{Profiles: &profiles{rows: systemProfiles()}, UnitOfWork: &unitOfWork{}}
	writer := actor()
	writer.Scopes = []string{"items:write"}

	_, err := handler.Execute(context.Background(), writer)
	if refusal := shared.AsError(err); refusal == nil || refusal.DetailCode != "access.insufficient_scope" {
		t.Errorf("refused as %v", err)
	}
}

func TestAFailedProfileReadAnswersNoPresets(t *testing.T) {
	handler := ListPresets{
		Profiles: &profiles{err: shared.ErrUnavailable.WithDetail("postgres.query_failed")}, UnitOfWork: &unitOfWork{},
	}
	if _, err := handler.Execute(context.Background(), presetReader()); err == nil {
		t.Error("presets were answered without the installation's profiles")
	}
}
