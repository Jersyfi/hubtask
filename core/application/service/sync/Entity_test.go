// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/sync"
)

var linkMinted = shared.ID("01936f2a-7c1e-7000-8000-0000000011b1")

func pushOne(t *testing.T, f pushFixture, m Mutation) Result {
	t.Helper()
	response, err := f.push.Push(t.Context(), pusher(), PushRequest{DeviceID: device, Mutations: []Mutation{m}})
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	if len(response.Results) != 1 {
		t.Fatalf("%d results, want one", len(response.Results))
	}
	return response.Results[0]
}

func callNames(f pushFixture) []string {
	names := make([]string, 0, len(f.catalogue.calls))
	for _, call := range f.catalogue.calls {
		names = append(names, call.name)
	}
	return names
}

// An upsert of a type the server does not hold is its creation, under the device's key, through
// the use case a person would call - the change is asked first and answers "not found".
func TestAnUpsertOfAnUnknownEntityCreatesIt(t *testing.T) {
	f := pushing(t)
	f.catalogue.errs["ChangeEntryType"] = shared.ErrNotFound.WithDetail("entry_types.not_found")
	f.catalogue.outputs["CreateEntryType"] = usecase.Output{"id": "01936f2a-7c1e-7000-8000-0000000011c1", "key": "risk", "version": 1}

	result := pushOne(t, f, Mutation{
		OpID: opA, Kind: domain.EntityUpsert, Entity: domain.EntityEntryType, EntityKey: "risk",
		Payload: map[string]any{"key": "moved", "level": "TASK", "name": "Risk", "icon": "alert", "color_token": "red"},
	})
	if result.Result != domain.Applied || result.ServerState["key"] != "risk" || result.EntityID != "01936f2a-7c1e-7000-8000-0000000011c1" {
		t.Errorf("the result is %+v", result)
	}
	if names := callNames(f); len(names) != 2 || names[0] != "ChangeEntryType" || names[1] != "CreateEntryType" {
		t.Fatalf("called %v", names)
	}
	create := f.catalogue.calls[1].in
	// The key is the mutation's: the payload cannot move it. A creation keeps the level.
	if create["key"] != "risk" || create["level"] != "TASK" {
		t.Errorf("the creation's input was %v", create)
	}
	if record, found := f.ops.records[opA]; !found || record.Result != domain.Applied {
		t.Errorf("the record is %+v (found=%v)", record, found)
	}
}

// The versioned rule: an upsert of an entity the server holds changes it against the device's base
// version and leaves out what only a creation sets; the server's direction stands.
func TestAnUpsertOfAHeldEntityChangesItAgainstItsBase(t *testing.T) {
	f := pushing(t)
	f.catalogue.outputs["ChangeLinkType"] = usecase.Output{"key": "feeds", "name": "Feeds", "version": 4}
	base := 3

	result := pushOne(t, f, Mutation{
		OpID: opA, Kind: domain.EntityUpsert, Entity: domain.EntityLinkType, EntityKey: "feeds", BaseVersion: &base,
		Payload: map[string]any{"name": "Feeds", "direction": "SYMMETRIC", "cardinality": "ONE", "expected_version": 99},
	})
	if result.Result != domain.Applied || result.ServerState["version"] != 4 {
		t.Errorf("the result is %+v", result)
	}
	if names := callNames(f); len(names) != 1 || names[0] != "ChangeLinkType" {
		t.Fatalf("called %v", names)
	}
	in := f.catalogue.calls[0].in
	if in["key"] != "feeds" || in["name"] != "Feeds" || in["expected_version"] != 3 {
		t.Errorf("the change's input was %v", in)
	}
	if _, sent := in["direction"]; sent {
		t.Errorf("a creation-only field reached the change: %v", in)
	}
}

// A stale base is refused with the entity's own conflict and recorded, so that a repeat answers the
// same; the device adopts the server's copy from the log.
func TestAPatchOnAStaleBaseIsRejectedWithTheEntitysConflict(t *testing.T) {
	f := pushing(t)
	f.catalogue.errs["ChangeRole"] = shared.ErrVersionConflict.WithDetail("roles.version_conflict")
	base := 2

	m := Mutation{
		OpID: opA, Kind: domain.EntityPatch, Entity: domain.EntityCustomRole, EntityKey: "auditor", BaseVersion: &base,
		Payload: map[string]any{"name": "Auditor", "permissions": []any{"READ"}},
	}
	result := pushOne(t, f, m)
	if result.Result != domain.Rejected || result.Error == nil || result.Error.MessageCode != "roles.version_conflict" {
		t.Errorf("the result is %+v", result)
	}
	if f.catalogue.calls[0].in["expected_version"] != 2 {
		t.Errorf("the base did not reach the change: %v", f.catalogue.calls[0].in)
	}
	again := pushOne(t, f, m)
	if again.Result != domain.Rejected || len(f.catalogue.calls) != 1 {
		t.Errorf("the repeat answered %+v after %d calls", again, len(f.catalogue.calls))
	}
}

// The presence rule: a link is drawn under the device's identifier, a deletion of one already gone
// is applied, and a link is never patched.
func TestALinkIsAPresence(t *testing.T) {
	f := pushing(t)
	f.catalogue.outputs["LinkEntries"] = usecase.Output{"id": linkMinted.String(), "type": "depends_on"}
	f.catalogue.errs["UnlinkEntries"] = shared.ErrNotFound.WithDetail("links.not_found")

	drawn := pushOne(t, f, Mutation{
		OpID: opA, Kind: domain.EntityUpsert, Entity: domain.EntityEntryLink, EntityKey: linkMinted.String(),
		Payload: map[string]any{"item_id": itemX.String(), "target_id": collectionA.String(), "type": "depends_on"},
	})
	if drawn.Result != domain.Applied || drawn.EntityID != linkMinted || f.catalogue.calls[0].in["id"] != linkMinted.String() {
		t.Errorf("drawn %+v with %v", drawn, f.catalogue.calls[0].in)
	}

	gone := pushOne(t, f, Mutation{OpID: opB, Kind: domain.EntityDelete, Entity: domain.EntityEntryLink, EntityKey: linkMinted.String()})
	if gone.Result != domain.Applied || gone.EntityID != linkMinted || f.catalogue.calls[1].in["link_id"] != linkMinted.String() {
		t.Errorf("removed %+v with %v", gone, f.catalogue.calls[1].in)
	}

	patched := pushOne(t, f, Mutation{
		OpID: shared.ID("01936f2a-7c1e-7000-8000-000000000f03"), Kind: domain.EntityPatch,
		Entity: domain.EntityEntryLink, EntityKey: linkMinted.String(),
	})
	if patched.Result != domain.Rejected || patched.Error.MessageCode != "sync.entity_not_patchable" || len(f.catalogue.calls) != 2 {
		t.Errorf("a patch answered %+v", patched)
	}
}

func TestAnEntityMutationNamesAKnownEntityAndItsKey(t *testing.T) {
	cases := map[string]struct {
		m    Mutation
		code string
	}{
		"an unknown entity": {Mutation{OpID: opA, Kind: domain.EntityUpsert, Entity: "custom_field", EntityKey: "x"}, "sync.entity_unknown"},
		"no key":            {Mutation{OpID: opA, Kind: domain.EntityDelete, Entity: domain.EntityWorkflow}, "sync.entity_key_required"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := pushing(t)
			result := pushOne(t, f, c.m)
			if result.Result != domain.Rejected || result.Error.MessageCode != c.code || len(f.catalogue.calls) != 0 {
				t.Errorf("answered %+v after %d calls", result, len(f.catalogue.calls))
			}
		})
	}
}

// UC-TIM-01 check 9: a timer started, stopped and discarded offline is pushed as the caller's own
// timer - a start through StartTimer with the device's moment, a stop through StopTimer, a discard
// through DiscardTimer - and its key reaches no use case.
func TestATimerQueuedOfflineIsStartedStoppedAndDiscarded(t *testing.T) {
	f := pushing(t)
	f.catalogue.outputs["StartTimer"] = usecase.Output{"running": true, "started_at": "2026-11-02T09:00:00Z"}
	started := pushOne(t, f, Mutation{
		OpID: opA, Kind: domain.EntityUpsert, Entity: domain.EntityTimer, EntityKey: "own",
		Payload: map[string]any{"work_type": "development", "started_at": "2026-11-02T09:00:00Z"},
	})
	if started.Result != domain.Applied || started.EntityID != pusher().AccountID || started.ServerState["running"] != true {
		t.Fatalf("the start is %+v, want the timer running under the caller's account", started)
	}
	if names := callNames(f); len(names) != 1 || names[0] != "StartTimer" {
		t.Fatalf("a start called %v, want StartTimer alone", names)
	}
	if in := f.catalogue.calls[0].in; in["started_at"] != "2026-11-02T09:00:00Z" || len(in) != 2 {
		t.Errorf("the start's input was %v, want the payload and no key", in)
	}

	f = pushing(t)
	f.catalogue.outputs["StopTimer"] = usecase.Output{"id": "01936f2a-7c1e-7000-8000-0000000011d1", "minutes": 51}
	stopped := pushOne(t, f, Mutation{
		OpID: opB, Kind: domain.EntityPatch, Entity: domain.EntityTimer, EntityKey: "own",
		Payload: map[string]any{"stopped_at": "2026-11-02T09:51:00Z", "zone": "Europe/Berlin"},
	})
	if stopped.Result != domain.Applied || callNames(f)[0] != "StopTimer" || stopped.ServerState != nil || stopped.EntityID != pusher().AccountID {
		t.Fatalf("the stop is %+v, called %v", stopped, callNames(f))
	}
	if in := f.catalogue.calls[0].in; in["stopped_at"] != "2026-11-02T09:51:00Z" || in["zone"] != "Europe/Berlin" || len(in) != 2 {
		t.Errorf("the stop's input was %v", in)
	}

	f = pushing(t)
	f.catalogue.errs["DiscardTimer"] = shared.ErrNotFound.WithDetail("timers.not_running")
	discarded := pushOne(t, f, Mutation{OpID: opA, Kind: domain.EntityDelete, Entity: domain.EntityTimer, EntityKey: "own"})
	if discarded.Result != domain.Applied || callNames(f)[0] != "DiscardTimer" || len(f.catalogue.calls[0].in) != 0 {
		t.Errorf("a discard of a timer already gone is %+v with %v", discarded, f.catalogue.calls)
	}
}
