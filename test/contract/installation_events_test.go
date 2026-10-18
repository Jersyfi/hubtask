// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/eventbus"
)

// installationSchemaDir holds the installation's event schemas, beside the workspace's rather than
// among them: the client package publishes the top level as the events a workspace can subscribe
// to, and a connector built from it must not offer what the installation tells its platform.
const installationSchemaDir = "../../api/events/installation"

func loadInstallationSchema(t *testing.T, eventType event.Type) *specification {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(installationSchemaDir, string(eventType)+".json"))
	if err != nil {
		t.Fatalf("no schema for %s: %v", eventType, err)
	}
	var root schema
	if err := yaml.Unmarshal(raw, &root); err != nil {
		t.Fatalf("the schema of %s is not readable: %v", eventType, err)
	}
	spec := &specification{}
	spec.Components.Schemas = map[string]*schema{"root": &root}
	return spec
}

func TestEveryInstallationEventTypeHasASchemaAndTheOtherWayRound(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(installationSchemaDir, "*.json"))
	if err != nil {
		t.Fatalf("reading %s: %v", installationSchemaDir, err)
	}
	declared := map[string]bool{}
	for _, file := range files {
		name := filepath.Base(file)
		declared[name[:len(name)-len(".json")]] = true
	}
	for _, eventType := range event.InstallationTypes() {
		if !declared[string(eventType)] {
			t.Errorf("%s has no schema under api/events/installation/", eventType)
		}
		delete(declared, string(eventType))
	}
	for orphan := range declared {
		t.Errorf("api/events/installation/%s.json describes an event the domain cannot emit", orphan)
	}
}

// Each installation event, as the recorder builds it, matches its schema - with and without a
// reference and a plan, since both travel as null when absent.
func TestTheInstallationEventsMatchTheirSchemas(t *testing.T) {
	at := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	extras := map[event.Type]map[string]any{
		event.WorkspacePlanChanged:       {"previous_plan": nil},
		event.WorkspaceDeletionRequested: {"purge_after": at.AddDate(0, 0, 30).Format(time.RFC3339)},
		event.WorkspaceLimitReached:      {"quota": "items", "limit": int64(5000)},
		event.WorkspaceAiBudgetSpent:     {"budget": "ai_tokens_per_day", "day": "2026-10-17"},
		event.WorkspaceExportFinished:    {"export_id": "0192f000-0000-7000-8000-0000000000f1"},
	}
	states := []event.WorkspaceState{
		{ID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"), Status: "ACTIVE", Version: 1},
		{
			ID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"), ExternalReference: "cus_4711",
			Status: "SUSPENDED", Plan: "familie", Version: 7,
		},
	}

	for _, eventType := range event.InstallationTypes() {
		spec := loadInstallationSchema(t, eventType)
		for _, state := range states {
			if eventType == event.WorkspaceDeleted {
				state.Status, state.ExternalReference = event.WorkspaceDeletedStatus, ""
			}
			envelope, err := event.NewWorkspaceEvent(shared.MustParseID("0192f000-0000-7000-8000-0000000000e9"),
				eventType, state, event.Actor{Kind: shared.ActorServiceAccount,
					ID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d")}, at, extras[eventType])
			if err != nil {
				t.Fatalf("%s: %v", eventType, err)
			}
			body, err := json.Marshal(eventbus.ToCloudEvent(envelope, "urn:hubtask:test"))
			if err != nil {
				t.Fatalf("rendering %s: %v", eventType, err)
			}
			problems, err := spec.validateAgainst("root", body)
			if err != nil {
				t.Fatalf("validating %s: %v", eventType, err)
			}
			for _, problem := range problems {
				t.Errorf("%s: %s", eventType, problem)
			}
		}
	}
}
