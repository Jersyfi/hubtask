// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package event_test

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestAnInstallationTypeIsNoWorkspaceType(t *testing.T) {
	for _, eventType := range event.InstallationTypes() {
		if eventType.Valid() {
			t.Errorf("%s is offered to a workspace's subscriptions", eventType)
		}
		if !eventType.Installation() {
			t.Errorf("%s is not an installation type", eventType)
		}
	}
	for _, eventType := range event.Types() {
		if eventType.Installation() {
			t.Errorf("%s is a workspace type and an installation type at once", eventType)
		}
	}
}

func TestAWorkspaceEventCarriesTheWholeStateAndNothingElse(t *testing.T) {
	workspace := shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	at := time.Date(2026, 10, 17, 9, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	cases := []struct {
		name      string
		state     event.WorkspaceState
		extra     map[string]any
		reference any
		plan      any
	}{
		{"bare", event.WorkspaceState{ID: workspace, Status: "ACTIVE", Version: 1}, nil, nil, nil},
		{
			"with a reference and a plan",
			event.WorkspaceState{ID: workspace, ExternalReference: "cus_4711", Status: "SUSPENDED", Plan: "familie", Version: 4},
			map[string]any{"previous_plan": "privat"}, "cus_4711", "familie",
		},
		{
			"an extra cannot replace the state",
			event.WorkspaceState{ID: workspace, Status: "ACTIVE", Version: 2},
			map[string]any{"workspace": "something else"}, nil, nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			envelope, err := event.NewWorkspaceEvent(shared.MustParseID("0192f000-0000-7000-8000-0000000000e9"),
				event.WorkspaceSuspended, c.state, event.Actor{Kind: shared.ActorSystem}, at, c.extra)
			if err != nil {
				t.Fatal(err)
			}
			state, ok := envelope.Payload["workspace"].(map[string]any)
			if !ok {
				t.Fatalf("no workspace in %v", envelope.Payload)
			}
			if state["id"] != workspace.String() || state["status"] != c.state.Status ||
				state["version"] != c.state.Version || state["external_reference"] != c.reference ||
				state["plan"] != c.plan {
				t.Errorf("state = %v", state)
			}
			if len(state) != 5 {
				t.Errorf("the state carries %d fields, want 5: %v", len(state), state)
			}
			if envelope.TenantID != workspace || envelope.Subject != "workspace/"+workspace.String() {
				t.Errorf("tenant %s, subject %s", envelope.TenantID, envelope.Subject)
			}
			if !envelope.OccurredAt.Equal(at) || envelope.OccurredAt.Location() != time.UTC {
				t.Errorf("occurred at %s", envelope.OccurredAt)
			}
		})
	}
}

func TestAWorkspaceEventRefusesWhatIsNotOne(t *testing.T) {
	id := shared.MustParseID("0192f000-0000-7000-8000-0000000000e9")
	state := event.WorkspaceState{ID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"), Status: "ACTIVE"}
	at := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		eventType event.Type
		state     event.WorkspaceState
		id        shared.ID
		at        time.Time
	}{
		{"a workspace type", event.ItemCreated, state, id, at},
		{"no status", event.WorkspaceCreated, event.WorkspaceState{ID: state.ID}, id, at},
		{"no workspace", event.WorkspaceCreated, event.WorkspaceState{Status: "ACTIVE"}, id, at},
		{"no id", event.WorkspaceCreated, state, "", at},
		{"no moment", event.WorkspaceCreated, state, id, time.Time{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := event.NewWorkspaceEvent(c.id, c.eventType, c.state, event.Actor{}, c.at, nil); err == nil {
				t.Error("built")
			}
		})
	}
}
