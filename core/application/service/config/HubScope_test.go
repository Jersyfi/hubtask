// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const supportHub = "0190a8f0-0000-7000-8000-0000000000f1"

func hubFixture() fixture {
	f := newFixture()
	f.world.hub = supportHub
	f.world.hubStructure = map[string]any{"completion": "MANUAL"}
	f.world.workflows = map[string]map[string]any{"service": {"key": "service", "name": "Service", "published": serviceShape("todo")}}
	f.world.scheme = []any{
		map[string]any{"container_id": supportHub, "entry_type": "incident", "workflow": "service"},
		map[string]any{"container_id": "0190a8f0-0000-7000-8000-0000000000f2", "entry_type": nil, "workflow": "service"},
	}
	return f
}

// configuration.md §1: a hub's own document holds what the hub decides itself - its structure
// settings and which workflow it runs for which type - by keys the same in every hub, and none of
// the workspace's blocks.
func TestAHubsDocumentHoldsWhatTheHubDecides(t *testing.T) {
	f := hubFixture()
	out := invoke(t, ExportConfig{f.desk}.Descriptor(), person, usecase.Input{"hub_id": supportHub})
	document := out["document"].(map[string]any)
	if document["structure"].(map[string]any)["completion"].(map[string]any)["value"] != "MANUAL" {
		t.Errorf("the hub's own structure setting is not a block: %v", document["structure"])
	}
	scheme := document["scheme"].(map[string]any)
	if len(scheme) != 1 || scheme["incident"].(map[string]any)["workflow"] != "service" {
		t.Errorf("the scheme holds another place's row or misses the hub's: %v", scheme)
	}
	for _, name := range []string{"types", "roles", "settings"} {
		if _, held := document[name]; held {
			t.Errorf("a hub's document holds the workspace's %s", name)
		}
	}
	if out["hub_id"] != supportHub {
		t.Errorf("the export does not name its hub: %v", out["hub_id"])
	}

	_, err := ExportConfig{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{"hub_id": "0190a8f0-0000-7000-8000-0000000000f2"})
	if !isDetail(err, "config.hub_unknown") {
		t.Errorf("a collection was read as a hub: %v", err)
	}
}

// A hub's document is planned and applied through the hub's own use cases, its version names the
// hub, and the workspace's history and rollback keep to the workspace's document.
func TestAHubsDocumentIsAppliedThroughItsUseCasesAndVersionedApart(t *testing.T) {
	f := hubFixture()
	document := map[string]any{
		"structure": map[string]any{"completion": nil, "capture": map[string]any{"value": "INBOX"}},
		"scheme":    map[string]any{"every_type": map[string]any{"workflow": "service"}},
		"types":     map[string]any{"incident": map[string]any{"level": "TASK", "name": "Incident"}},
	}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document, "hub_id": supportHub})
	refusals := plan["refusals"].([]usecase.Output)
	if len(refusals) != 1 || refusals[0]["path"] != "/types" || refusals[0]["code"] != "config.section_unknown" {
		t.Fatalf("a workspace section in a hub's document was not refused: %v", refusals)
	}
	delete(document, "types")
	f.world.preview = usecase.Output{"entries": 4, "moves": []usecase.Output{}, "removed": []usecase.Output{}}
	plan = invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document, "hub_id": supportHub})
	if plan["plan_hash"] == nil {
		t.Fatalf("plan %v", plan)
	}
	reached := map[string]usecase.Output{}
	for _, row := range plan["reach"].([]usecase.Output) {
		reached[row["path"].(string)] = row
	}
	if len(reached) != 2 || reached["scheme.every_type"]["entries"] != 4 {
		t.Errorf("the scheme's reach is not the preview's: %v", reached)
	}
	if hidden := reached["structure"]; hidden["hidden_entries"] != 3 || hidden["hidden_values"] != 5 {
		t.Errorf("the structure's reach is not its preview's: %v", hidden)
	}
	if asked := f.world.inputs["PreviewContainerStructure"]; len(asked) != 1 || asked[0]["container_id"] != supportHub {
		t.Errorf("the structure preview was asked %v", asked)
	}
	out := invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": document, "hub_id": supportHub, "plan_hash": plan["plan_hash"], "acknowledge": true,
	})
	if out["state"] != "APPLIED" {
		t.Fatalf("not applied: %v", out)
	}
	for _, call := range f.world.inputs["SetContainerStructure"] {
		if call["container_id"] != supportHub {
			t.Errorf("the structure was written elsewhere: %v", call)
		}
	}
	if f.world.hubStructure["capture"] != "INBOX" || f.world.hubStructure["completion"] != nil {
		t.Errorf("hub structure %v", f.world.hubStructure)
	}
	schemes := f.world.inputs["SetWorkflowScheme"]
	if len(schemes) != 1 || schemes[0].String("entry_type") != "" || schemes[0]["workflow"] != "service" || schemes[0]["container_id"] != supportHub {
		t.Errorf("scheme writes %v", schemes)
	}
	if len(f.store.revisions) != 1 || f.store.revisions[0].HubID != shared.ID(supportHub) {
		t.Fatalf("the version does not name its hub: %+v", f.store.revisions)
	}

	workspace := invoke(t, ListConfigVersions{f.desk}.Descriptor(), person, usecase.Input{})
	if len(workspace["data"].([]usecase.Output)) != 0 {
		t.Errorf("a hub's version is in the workspace's history: %v", workspace["data"])
	}
	hub := invoke(t, ListConfigVersions{f.desk}.Descriptor(), person, usecase.Input{"hub_id": supportHub})
	if len(hub["data"].([]usecase.Output)) != 1 {
		t.Errorf("the hub's history %v", hub["data"])
	}
}

// A version restores the document it was of: rolling the workspace back to a hub's version is
// refused rather than stating the hub's blocks as the workspace's.
func TestAHubsVersionIsNotRestoredAsTheWorkspaces(t *testing.T) {
	f := hubFixture()
	document := map[string]any{"structure": map[string]any{"capture": map[string]any{"value": "INBOX"}}}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document, "hub_id": supportHub})
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": document, "hub_id": supportHub, "plan_hash": plan["plan_hash"], "acknowledge": true,
	})
	_, err := PlanConfig{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{"to_version": 1})
	if refusals := refusalsIn(err); len(refusals) != 1 || refusals[0].Code != "config.version_other_scope" {
		t.Errorf("a hub's version was planned as the workspace's: %v", err)
	}
	back := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"to_version": 1, "hub_id": supportHub})
	if back["plan_hash"] == nil {
		t.Errorf("the hub's own version does not plan back: %v", back)
	}
}

// UC-SVC-01 check 5: a hub's own versions of the workspace's calendars are blocks of the hub's
// document under the workspace's keys, applied through the hub's own calendar use cases, each
// naming the hub.
func TestAHubsCalendarsAreBlocksOfItsDocument(t *testing.T) {
	f := hubFixture()
	f.world.hubCalendars = map[string]map[string]any{
		"office": {"key": "office", "hub_id": supportHub, "name": "Support office", "zone": "Europe/Berlin",
			"hours": []any{map[string]any{"day": "MONDAY", "from": "08:00", "to": "16:00"}}, "holidays": []any{}},
		"weekend": {"key": "weekend", "hub_id": supportHub, "name": "Weekend", "zone": "Europe/Berlin", "hours": []any{}, "holidays": []any{}},
	}
	out := invoke(t, ExportConfig{f.desk}.Descriptor(), person, usecase.Input{"hub_id": supportHub})
	calendars := out["document"].(map[string]any)[sectionCalendars].(map[string]any)
	office, _ := calendars["office"].(map[string]any)
	if len(calendars) != 2 || office["name"] != "Support office" || office["zone"] != "Europe/Berlin" {
		t.Fatalf("the hub's calendars are not its document's blocks: %v", calendars)
	}
	if _, held := office["hub_id"]; held {
		t.Errorf("a calendar block names its hub: %v", office)
	}

	document := map[string]any{sectionCalendars: map[string]any{
		"office":  map[string]any{"name": "Support office", "zone": "Europe/Vienna", "hours": office["hours"], "holidays": []any{"2026-12-24"}},
		"weekend": nil,
		"night":   map[string]any{"name": "Night", "zone": "UTC", "hours": []any{}},
	}}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document, "hub_id": supportHub, "mode": "MERGE"})
	if plan["plan_hash"] == nil {
		t.Fatalf("plan %v", plan)
	}
	applied := invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": document, "hub_id": supportHub, "mode": "MERGE", "plan_hash": plan["plan_hash"], "acknowledge": true,
	})
	if applied["state"] != "APPLIED" {
		t.Fatalf("not applied: %v", applied)
	}
	defined := map[string]usecase.Input{}
	for _, in := range f.world.inputs["DefineHubCalendar"] {
		defined[in.String("key")] = in
	}
	if len(defined) != 2 || defined["office"]["zone"] != "Europe/Vienna" || defined["night"]["name"] != "Night" ||
		defined["office"]["hub_id"] != supportHub || defined["night"]["hub_id"] != supportHub {
		t.Errorf("the calendars were defined as %v", f.world.inputs["DefineHubCalendar"])
	}
	removed := f.world.inputs["DeleteHubCalendar"]
	if len(removed) != 1 || removed[0]["key"] != "weekend" || removed[0]["hub_id"] != supportHub {
		t.Errorf("the removed calendar ran %v", removed)
	}
}

// recorded counts the permissions a test's authoriser was asked.
type recorded struct{ asked []access.Request }

func (r *recorded) Authorize(_ context.Context, _ appshared.ActorContext, request access.Request) error {
	r.asked = append(r.asked, request)
	return nil
}

// A hub the reader does not reach answers as a missing one does - before any permission is asked
// of it - on the export and on a plan alike (T-04; UC-CFG-01 check 2).
func TestAHubTheReaderDoesNotReachIsNotFound(t *testing.T) {
	f := hubFixture()
	const private = "0190a8f0-0000-7000-8000-0000000000f9"
	f.world.unreached = private
	guard := &recorded{}
	f.desk.Authorizer = guard

	for name, descriptor := range map[string]usecase.Descriptor{
		"export": ExportConfig{f.desk}.Descriptor(), "plan": PlanConfig{f.desk}.Descriptor(),
	} {
		in := usecase.Input{"hub_id": private}
		if name == "plan" {
			in["document"] = map[string]any{"structure": map[string]any{}}
		}
		_, err := descriptor.Handler.Invoke(t.Context(), person, in)
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the %s of an unreached hub answered %v, want not found", name, err)
		}
	}
	if len(guard.asked) != 0 {
		t.Errorf("a permission was asked of a hub the reader does not reach: %+v", guard.asked)
	}
}
