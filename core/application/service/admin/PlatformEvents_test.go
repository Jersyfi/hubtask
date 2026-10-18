// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"strings"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/application/service/platform"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// platformLog keeps the installation events a use case wrote, as the table would.
type platformLog struct {
	events    []event.Envelope
	forgotten int
	keys      map[string]bool
}

func (l *platformLog) Append(_ context.Context, envelope event.Envelope, key string) (bool, error) {
	if key != "" {
		if l.keys == nil {
			l.keys = map[string]bool{}
		}
		if l.keys[key] {
			return false, nil
		}
		l.keys[key] = true
	}
	l.events = append(l.events, envelope)
	return true, nil
}

func (l *platformLog) After(context.Context, int64, int) ([]adminrepo.PlatformEvent, error) {
	return nil, nil
}

func (l *platformLog) ForgetReference(context.Context) error {
	l.forgotten++
	return nil
}

func (l *platformLog) Sweep(context.Context, time.Time, int) (int, error) { return 0, nil }

// recorderOn writes into a fresh log, reading the workspace from the fake's row.
func recorderOn(tenants *tenantsStore) (platform.Recorder, *platformLog) {
	log := &platformLog{}
	return platform.Recorder{
		Events: log, Workspaces: tenants, IDs: &sequentialIDs{}, Tenancy: env.TenancyMulti,
	}, log
}

// state reads the workspace an event carries.
func stateOf(t *testing.T, envelope event.Envelope) map[string]any {
	t.Helper()
	state, ok := envelope.Payload["workspace"].(map[string]any)
	if !ok {
		t.Fatalf("%s carries no workspace: %v", envelope.Type, envelope.Payload)
	}
	return state
}

// onlyEvent is the one event a use case wrote, of the type expected.
func onlyEvent(t *testing.T, log *platformLog, want event.Type) event.Envelope {
	t.Helper()
	if len(log.events) != 1 || log.events[0].Type != want {
		types := make([]string, 0, len(log.events))
		for _, e := range log.events {
			types = append(types, string(e.Type))
		}
		t.Fatalf("wrote [%s], want one %s", strings.Join(types, ", "), want)
	}
	return log.events[0]
}

// No event carries a name a person gave: neither the workspace's display name nor its short name,
// which is often a family's (rule 10, UC-INS-14 check 3).
func carriesNoName(t *testing.T, envelope event.Envelope, names ...string) {
	t.Helper()
	var walk func(any)
	walk = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			for _, inner := range v {
				walk(inner)
			}
		case []any:
			for _, inner := range v {
				walk(inner)
			}
		case string:
			for _, name := range names {
				if name != "" && strings.Contains(v, name) {
					t.Errorf("%s carries %q", envelope.Type, name)
				}
			}
		}
	}
	walk(envelope.Payload)
}

func TestAWorkspaceProvisionedIsTold(t *testing.T) {
	f := newProvisionFixture()
	recorder, log := recorderOn(f.tenants)
	f.handler.Platform = recorder

	if _, err := f.handler.Execute(t.Context(), operator(), provisionCommand()); err != nil {
		t.Fatalf("provisioning: %v", err)
	}
	created := onlyEvent(t, log, event.WorkspaceCreated)
	state := stateOf(t, created)
	if state["id"] != f.tenants.record.ID.String() || state["status"] != "ACTIVE" ||
		state["version"] != int64(f.tenants.record.Version) {
		t.Errorf("created with %v; the row is %+v", state, f.tenants.record)
	}
	carriesNoName(t, created, f.tenants.record.Slug, f.tenants.record.DisplayName)
}

func TestAWorkspaceMovedIsToldWithThePlanItLeft(t *testing.T) {
	f := newMoveFixture(t)
	recorder, log := recorderOn(f.tenants)
	f.registry = moveRegistry(t, f, recorder)

	if _, err := f.registry.Invoke(t.Context(), MoveTenantPlanName, operator(), moveTo("family", 4)); err != nil {
		t.Fatalf("moving: %v", err)
	}
	moved := onlyEvent(t, log, event.WorkspacePlanChanged)
	if state := stateOf(t, moved); state["plan"] != "family" || state["version"] != int64(5) {
		t.Errorf("moved with %v", state)
	}
	if from, present := moved.Payload["previous_plan"]; !present || from != nil {
		t.Errorf("previous plan %v (present %v), want null", from, present)
	}
	carriesNoName(t, moved, "maria")

	// To the plan it is on: nothing changed, nothing is told.
	if _, err := f.registry.Invoke(t.Context(), MoveTenantPlanName, operator(), moveTo("family")); err != nil {
		t.Fatalf("moving again: %v", err)
	}
	if len(log.events) != 1 {
		t.Errorf("a move to the plan it is on wrote %d events", len(log.events)-1)
	}
}

func TestASuspensionAndAResumptionAreTold(t *testing.T) {
	f := newShiftFixture(domain.TenantActive)
	f.tenants.record.Version = 3
	recorder, log := recorderOn(f.tenants)
	f.shift.Platform = recorder

	if err := (SuspendTenant{f.shift}).Execute(t.Context(), operator(), lifecycleTenant); err != nil {
		t.Fatalf("suspending: %v", err)
	}
	suspended := onlyEvent(t, log, event.WorkspaceSuspended)
	if state := stateOf(t, suspended); state["status"] != "SUSPENDED" || state["version"] != int64(4) {
		t.Errorf("suspended with %v", state)
	}
	if suspended.Actor.ID != operator().AccountID {
		t.Errorf("told as %+v", suspended.Actor)
	}
	carriesNoName(t, suspended, "acme")

	// Suspending again: the state already stands, and nothing is told.
	if err := (SuspendTenant{f.shift}).Execute(t.Context(), operator(), lifecycleTenant); err != nil {
		t.Fatal(err)
	}
	if len(log.events) != 1 {
		t.Fatalf("a repeated suspension wrote %d events", len(log.events)-1)
	}

	if err := (ResumeTenant{f.shift}).Execute(t.Context(), operator(), lifecycleTenant); err != nil {
		t.Fatalf("resuming: %v", err)
	}
	if len(log.events) != 2 || log.events[1].Type != event.WorkspaceResumed {
		t.Fatalf("resuming wrote %v", log.events)
	}
	if state := stateOf(t, log.events[1]); state["status"] != "ACTIVE" || state["version"] != int64(5) {
		t.Errorf("resumed with %v", state)
	}
}

func TestADeletionRequestedAndCancelledIsTold(t *testing.T) {
	f := newDeletionFixture(domain.TenantActive)
	f.tenants.record.Version = 2
	recorder, log := recorderOn(f.tenants)
	f.handler.Platform = recorder

	scheduled, err := f.handler.Execute(t.Context(), operator(), deletionCommand())
	if err != nil {
		t.Fatalf("requesting: %v", err)
	}
	requested := onlyEvent(t, log, event.WorkspaceDeletionRequested)
	if state := stateOf(t, requested); state["status"] != "PENDING_DELETION" || state["version"] != int64(3) {
		t.Errorf("requested with %v", state)
	}
	if requested.Payload["purge_after"] != scheduled.PurgeAfter.Format(time.RFC3339) {
		t.Errorf("purge after %v, want %s", requested.Payload["purge_after"], scheduled.PurgeAfter)
	}
	carriesNoName(t, requested, "acme", "Acme GmbH")

	cancel := newCancelFixture(domain.TenantActive)
	cancel.tenants.record = f.tenants.record
	cancelRecorder, cancelLog := recorderOn(cancel.tenants)
	cancel.handler.Platform = cancelRecorder
	if _, err := cancel.cancel(t, operator()); err != nil {
		t.Fatalf("cancelling: %v", err)
	}
	cancelled := onlyEvent(t, cancelLog, event.WorkspaceDeletionCancelled)
	if state := stateOf(t, cancelled); state["status"] != "ACTIVE" || state["version"] != int64(4) {
		t.Errorf("cancelled with %v", state)
	}

	// Nothing to call off: nothing is told.
	if _, err := cancel.cancel(t, operator()); err != nil {
		t.Fatal(err)
	}
	if len(cancelLog.events) != 1 {
		t.Errorf("a cancel without a deletion wrote %d events", len(cancelLog.events)-1)
	}
}

// The purge tells the platform last, with the workspace's id alone: DELETED, the version one past
// the last, no reference - which leaves the workspace's kept events in the same transaction.
func TestThePurgeIsTold(t *testing.T) {
	f := newHardDeleteFixture(domain.TenantPendingDeletion, now.Add(-time.Hour))
	f.tenants.record.Version, f.tenants.record.ExternalReference = 6, "cus_4711"
	recorder, log := recorderOn(f.tenants)
	f.handler.Platform = recorder

	if _, err := f.handler.Execute(t.Context(), lifecycleTenant, purgeJobID); err != nil {
		t.Fatalf("deleting: %v", err)
	}
	deleted := onlyEvent(t, log, event.WorkspaceDeleted)
	state := stateOf(t, deleted)
	if state["status"] != event.WorkspaceDeletedStatus || state["version"] != int64(7) ||
		state["external_reference"] != nil || state["id"] != lifecycleTenant.String() {
		t.Errorf("deleted with %v", state)
	}
	if log.forgotten != 1 {
		t.Errorf("the reference was removed from the kept events %d times", log.forgotten)
	}
	if deleted.Actor.Kind != shared.ActorSystem {
		t.Errorf("told as %+v", deleted.Actor)
	}
}

func TestSingleModeTellsNobody(t *testing.T) {
	f := newProvisionFixture()
	recorder, log := recorderOn(f.tenants)
	recorder.Tenancy = env.TenancySingle
	if recorder.On() {
		t.Error("single mode writes installation events")
	}
	if err := recorder.Record(t.Context(), event.WorkspaceSuspended, event.Actor{}, now, nil); err != nil {
		t.Fatal(err)
	}
	if len(log.events) != 0 {
		t.Errorf("single mode wrote %d events", len(log.events))
	}
}
