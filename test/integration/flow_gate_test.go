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
	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	syncdomain "github.com/Jersyfi/hubtask/core/domain/model/sync"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/infrastructure/automation"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	textadapter "github.com/Jersyfi/hubtask/infrastructure/text"
	"github.com/Jersyfi/hubtask/presentation/mcp"
)

// UC-WRK-31 check 2 against PostgreSQL, through every door: in a collection run as a Flow, an entry
// enters a column after the first only with its required fields - refused by name on REST, MCP,
// an automation action, a device's push and a move, and let through once the field is set. The
// gate is the production wiring of cmd/server: one FlowGate over the bucket and field repositories.

func flowCatalogue(t *testing.T) *usecase.Registry {
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
	profiles := postgres.NewCapabilityProfileRepository()
	resolver := work.StructureResolver{
		Containers: containers, Structures: postgres.NewStructureRepository(), Profiles: profiles,
	}
	gate := work.EntryGate{Structure: resolver, Types: itemRepo()}
	flow := work.FlowGate{Buckets: bucketRepo(), Fields: postgres.NewCustomFieldRepository()}
	outbox := postgres.NewOutbox(jobQueue(t))
	changes := postgres.NewChangeLog()
	journal := work.ActivityJournal{Entries: historyRepo(), IDs: ids}
	writer := work.ContainerWriter{
		Containers: containers, Authorizer: authoriser, Events: outbox, Changes: changes, Audit: sink,
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid, Text: textadapter.Forms{},
	}
	due := work.DueDateWriter{
		Items: itemRepo(), Containers: containers, Profiles: profiles, Gate: gate, Authorizer: authoriser,
		Reminders: postgres.NewReminderRepository(), Jobs: jobQueue(t),
		Events: outbox, Changes: changes, Audit: sink, Activity: journal,
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
	}
	placement := work.PlacementWriter{
		Items: itemRepo(), Buckets: bucketRepo(), Containers: containers, Profiles: profiles, Gate: gate,
		Flow: flow, Authorizer: authoriser, Events: outbox, Changes: changes, Audit: sink, Activity: journal,
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
	}
	registry, err := usecase.NewRegistry(nil,
		work.SetContainerStructure{
			Writer: writer, Structure: resolver, Epochs: postgres.NewEpochRepository(),
		}.Descriptor(),
		work.CreateWorkItem{
			Items: itemRepo(), Buckets: bucketRepo(), Containers: containers, Profiles: profiles, Gate: gate,
			Flow: flow, Authorizer: authoriser, Ownership: authoriser, Events: outbox, Changes: changes,
			Audit: sink, Activity: journal, UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
			DueDates: due, Structure: resolver, Text: textadapter.Forms{},
		}.Descriptor(),
		work.UpdateWorkItem{
			Items: itemRepo(), Buckets: bucketRepo(), Containers: containers, Profiles: profiles, Gate: gate,
			Flow: flow, Authorizer: authoriser, Events: outbox, Changes: changes, Audit: sink,
			Activity: journal, UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
			DueDates: due, Text: textadapter.Forms{},
		}.Descriptor(),
		work.GetWorkItem{
			Items: itemRepo(), ItemLabels: itemLabelRepo(), Containers: containers,
			Authorizer: authoriser, UnitOfWork: unitOfWork, Gate: gate,
		}.Descriptor(),
		work.MoveWorkItem{Placement: placement}.Descriptor(),
		work.CreateBucket{
			Buckets: bucketRepo(), Containers: containers, Authorizer: authoriser,
			Events: outbox, Changes: changes, Audit: sink, UnitOfWork: unitOfWork, Clock: fixed,
			IDs: ids, HLC: hybrid, Text: textadapter.Forms{},
		}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	return registry
}

// refusedByName is whether an error is the gate's refusal naming the field.
func refusedByName(err error) bool {
	refusal := shared.AsError(err)
	return errors.Is(err, shared.ErrValidation) && refusal != nil &&
		refusal.DetailCode == "items.required_fields_missing" && refusal.Params["keys"] == "customer"
}

func TestAFlowEntryLeavesItsFirstColumnOnlyWithItsRequiredFields(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := flowCatalogue(t)
	actor := hiddenActor(tenantB, authorB)
	collection := collectionFor(ctx, t, tenantB, authorB).String()

	if _, err := registry.Invoke(ctx, "SetContainerStructure", actor, usecase.Input{
		"container_id": collection, "preset": "FLOW",
	}); err != nil {
		t.Fatalf("choosing Flow: %v", err)
	}
	column := func(name string) string {
		t.Helper()
		out, err := registry.Invoke(ctx, "CreateBucket", actor, usecase.Input{"collection_id": collection, "name": name})
		if err != nil {
			t.Fatalf("the column %s: %v", name, err)
		}
		return out.String("id")
	}
	first, doing := column("New"), column("In progress")
	field := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO custom_field_definition (id, tenant_id, collection_id, key, kind, is_required, applies_to)
		VALUES ($1, $2, $3, 'customer', 'TEXT', true, '{TASK}')`,
		field.String(), tenantB.String(), collection); err != nil {
		t.Fatalf("the required field: %v", err)
	}

	// Arriving in the first column needs nothing.
	arrived, err := registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "TASK", "collection_id": collection, "title": "Printer jams", "bucket_id": first,
	})
	if err != nil {
		t.Fatalf("creating in the first column: %v", err)
	}
	taskID := arrived.String("id")
	stored := func() string {
		t.Helper()
		var bucket string
		if err := adminPool(ctx, t).QueryRow(ctx, `SELECT coalesce(bucket_id::text, '') FROM work_item WHERE id = $1`,
			taskID).Scan(&bucket); err != nil {
			t.Fatalf("reading the column: %v", err)
		}
		return bucket
	}

	// REST.
	recorder := restItem(ctx, t, registry, actor, http.MethodPatch, "/items/"+taskID, `{"bucket_id":"`+doing+`"}`)
	var problem map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &problem)
	if recorder.Code != http.StatusUnprocessableEntity || problem["detail_code"] != "items.required_fields_missing" ||
		!strings.Contains(recorder.Body.String(), "/custom_fields/customer") {
		t.Errorf("REST answered %d %s", recorder.Code, recorder.Body)
	}

	// MCP.
	call, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{
		"name": "update_work_item", "arguments": map[string]any{"item_id": taskID, "bucket_id": doing},
	}})
	server := mcp.Server{Catalogue: registry, Name: "hubtask", Version: "test"}
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, actor), http.MethodPost, mcp.Path,
		bytes.NewReader(call))
	answer := httptest.NewRecorder()
	server.ServeHTTP(answer, request)
	if !strings.Contains(answer.Body.String(), `"isError":true`) ||
		!strings.Contains(answer.Body.String(), "items.required_fields_missing") {
		t.Errorf("MCP answered %s", answer.Body)
	}

	// An automation action.
	rule := actor
	rule.Kind = appshared.ActorAutomation
	if _, err := automation.NewActionDispatcher(registry).Dispatch(ctx, rule, automation.Action{
		Kind: "UPDATE_WORK_ITEM", Params: map[string]any{"item_id": taskID, "bucket_id": doing},
	}, nil); !refusedByName(err) {
		t.Errorf("the automation action answered %v", err)
	}

	// A device's push.
	push := syncservice.PushChanges{
		Stream: streamFor(ctx, t), IDs: clockadapter.NewUUIDv7(portclock.Fixed(created)),
		Devices: postgres.NewDeviceRepository(), Ops: postgres.NewSyncOpLog(),
		Tombstones: postgres.NewTombstoneRepository(), Catalogue: registry, Skew: 5 * time.Minute,
		Clocks: postgres.NewChangeLog(),
	}
	device := freshID(t)
	reading, _ := shared.NewHLC(time.Now().Add(time.Minute), 1, device.String())
	pushed, err := push.Push(ctx, actor, syncservice.PushRequest{DeviceID: device, Mutations: []syncservice.Mutation{{
		OpID: freshID(t), Kind: syncdomain.ItemPatch, ItemID: shared.ID(taskID),
		Fields: map[string]syncservice.FieldChange{"bucket_id": {Value: doing, HLC: reading.String()}},
	}}})
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	if pushed.Results[0].Result != syncdomain.Rejected || pushed.Results[0].Error == nil ||
		pushed.Results[0].Error.Code != "validation_failed" {
		t.Errorf("the push answered %+v", pushed.Results[0])
	}

	// A move, and a creation straight into the later column.
	if _, err := registry.Invoke(ctx, "MoveWorkItem", actor, usecase.Input{
		"item_id": taskID, "target_bucket_id": doing,
	}); !refusedByName(err) {
		t.Errorf("the move answered %v", err)
	}
	if _, err := registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "TASK", "collection_id": collection, "title": "Toner low", "bucket_id": doing,
	}); !refusedByName(err) {
		t.Errorf("a creation into the later column answered %v", err)
	}
	if got := stored(); got != first {
		t.Fatalf("a refused move moved the entry to %s", got)
	}

	// With the field set, the same move passes.
	if _, err := adminPool(ctx, t).Exec(ctx, `
		UPDATE work_item SET custom_fields = jsonb_build_object('customer', 'ACME'),
		       custom_field_refs = jsonb_build_object('customer', $2::text)
		 WHERE id = $1`, taskID, field.String()); err != nil {
		t.Fatalf("setting the field: %v", err)
	}
	recorder = restItem(ctx, t, registry, actor, http.MethodPatch, "/items/"+taskID, `{"bucket_id":"`+doing+`"}`)
	if recorder.Code != http.StatusOK || stored() != doing {
		t.Errorf("with the field, REST answered %d %s", recorder.Code, recorder.Body)
	}
}

// UC-WRK-31 check 4: the moments the figure measures by are recorded on every way an entry changes
// column - its creation, a move, a device's push - and the first column's stays when it comes back.
func TestAFlowEntryRecordsWhenItEntersTheFirstAndTheDoneColumn(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := flowCatalogue(t)
	actor := hiddenActor(tenantB, authorB)
	collection := collectionFor(ctx, t, tenantB, authorB).String()
	column := func(name string, done bool) string {
		t.Helper()
		out, err := registry.Invoke(ctx, "CreateBucket", actor, usecase.Input{
			"collection_id": collection, "name": name, "is_done_bucket": done,
		})
		if err != nil {
			t.Fatalf("the column %s: %v", name, err)
		}
		return out.String("id")
	}
	first, doing, done := column("New", false), column("In progress", false), column("Done", true)

	arrived, err := registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "TASK", "collection_id": collection, "title": "Printer jams", "bucket_id": first,
	})
	if err != nil {
		t.Fatalf("creating in the first column: %v", err)
	}
	taskID := arrived.String("id")
	type moments struct{ created, started, finished *time.Time }
	read := func() moments {
		t.Helper()
		var m moments
		if err := adminPool(ctx, t).QueryRow(ctx,
			`SELECT created_at, flow_started_at, flow_finished_at FROM work_item WHERE id = $1`, taskID,
		).Scan(&m.created, &m.started, &m.finished); err != nil {
			t.Fatalf("reading the moments: %v", err)
		}
		return m
	}
	move := func(bucket string) {
		t.Helper()
		if _, err := registry.Invoke(ctx, "MoveWorkItem", actor, usecase.Input{
			"item_id": taskID, "target_bucket_id": bucket,
		}); err != nil {
			t.Fatalf("moving: %v", err)
		}
	}

	created := read()
	if created.started == nil || !created.started.Equal(*created.created) || created.finished != nil {
		t.Fatalf("created into the first column, recorded %+v", created)
	}

	// Through a move and back: the first column's moment stays.
	move(doing)
	move(first)
	if back := read(); back.started == nil || !back.started.Equal(*created.started) || back.finished != nil {
		t.Errorf("moved back, recorded %+v", back)
	}

	// A device's push into the done column.
	push := syncservice.PushChanges{
		Stream: streamFor(ctx, t), IDs: clockadapter.NewUUIDv7(portclock.Fixed(time.Now())),
		Devices: postgres.NewDeviceRepository(), Ops: postgres.NewSyncOpLog(),
		Tombstones: postgres.NewTombstoneRepository(), Catalogue: registry, Skew: 5 * time.Minute,
		Clocks: postgres.NewChangeLog(),
	}
	device := freshID(t)
	reading, _ := shared.NewHLC(time.Now().Add(time.Minute), 1, device.String())
	pushed, err := push.Push(ctx, actor, syncservice.PushRequest{DeviceID: device, Mutations: []syncservice.Mutation{{
		OpID: freshID(t), Kind: syncdomain.ItemPatch, ItemID: shared.ID(taskID),
		Fields: map[string]syncservice.FieldChange{"bucket_id": {Value: done, HLC: reading.String()}},
	}}})
	if err != nil || pushed.Results[0].Result == syncdomain.Rejected {
		t.Fatalf("pushing into the done column: %v %+v", err, pushed)
	}
	finished := read()
	if finished.finished == nil || finished.finished.Before(*created.started) ||
		!finished.started.Equal(*created.started) {
		t.Fatalf("pushed into the done column, recorded %+v", finished)
	}

	// Out of the done column and in again: the last entry counts.
	move(doing)
	move(done)
	if again := read(); again.finished == nil || again.finished.Before(*finished.finished) ||
		!again.started.Equal(*created.started) {
		t.Errorf("moved into the done column again, recorded %+v", again)
	}
}
