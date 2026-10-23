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
	"slices"
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
	recurrenceadapter "github.com/Jersyfi/hubtask/infrastructure/recurrence"
	textadapter "github.com/Jersyfi/hubtask/infrastructure/text"
	"github.com/Jersyfi/hubtask/presentation/mcp"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// UC-WRK-24 check 3 against PostgreSQL, through every door the catalogue has: a collection narrowed
// to List keeps everything its entries hold, every read marks what it hides, writing it is refused
// the same way on REST, MCP, an automation action and a device's push, and widening it gives every
// value back, writable. Everything below the adapters is the production wiring of the gate.

func hiddenCatalogue(t *testing.T) *usecase.Registry {
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
	assignment := work.AssignmentWriter{
		Items: itemRepo(), Containers: containers, Profiles: profiles, Gate: gate, Authorizer: authoriser,
		Visibility: authoriser, Events: outbox, Changes: changes, Audit: sink, Activity: journal,
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
	}
	registry, err := usecase.NewRegistry(nil,
		work.CreateContainer{
			Containers: containers, Authorizer: authoriser, OwnHubs: authoriser,
			Grants:      postgres.NewMembershipGrantRepository(pageCursors()),
			PrivateHubs: privateHubsRule(unitOfWork), Events: outbox, Changes: changes,
			Audit: sink, UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
			Text: textadapter.Forms{}, Structure: resolver,
		}.Descriptor(),
		work.SetContainerStructure{
			Writer: writer, Structure: resolver, Epochs: postgres.NewEpochRepository(),
		}.Descriptor(),
		work.PreviewContainerStructure{
			Set: work.SetContainerStructure{Writer: writer, Structure: resolver}, Shapes: itemRepo(),
		}.Descriptor(),
		work.CreateWorkItem{
			Items: itemRepo(), Containers: containers, Profiles: profiles, Gate: gate,
			Authorizer: authoriser, Ownership: authoriser, Events: outbox, Changes: changes, Audit: sink,
			Activity: journal, UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
			DueDates: due, AutoAssign: work.AutoAssignWorkItem{Assignment: assignment},
		}.Descriptor(),
		work.GetWorkItem{
			Items: itemRepo(), ItemLabels: itemLabelRepo(), Containers: containers,
			Authorizer: authoriser, UnitOfWork: unitOfWork, Gate: gate,
		}.Descriptor(),
		work.UpdateWorkItem{
			Items: itemRepo(), Buckets: postgres.NewBucketRepository(), Containers: containers,
			Profiles: profiles, Gate: gate, Authorizer: authoriser, Events: outbox, Changes: changes,
			Audit: sink, Activity: journal, UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
			DueDates: due, Text: textadapter.Forms{},
		}.Descriptor(),
		work.SetDueDate{Due: due}.Descriptor(),
		work.AssignWorkItem{Assignment: assignment}.Descriptor(),
		work.SetCover{Cover: work.CoverWriter{
			Items: itemRepo(), Containers: containers, Profiles: profiles, Gate: gate, Media: mediaRepo(),
			Authorizer: authoriser, Events: outbox, Changes: changes, Audit: sink, Activity: journal,
			UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
		}}.Descriptor(),
		work.AddMember{Writer: work.ItemMemberWriter{
			Items: itemRepo(), ItemMembers: itemMemberRepo(), Containers: containers, Profiles: profiles,
			Gate: gate, Authorizer: authoriser, Visibility: authoriser, Events: outbox, Changes: changes,
			Audit: sink, Activity: journal, UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
		}}.Descriptor(),
		work.AttachMedia{Writer: work.ItemAttachmentWriter{
			Items: itemRepo(), Containers: containers, Profiles: profiles, Gate: gate,
			Attachments: mediaRepo(), Media: mediaRepo(), Authorizer: authoriser, Events: outbox,
			Changes: changes, Audit: sink, Activity: journal, UnitOfWork: unitOfWork, Clock: fixed,
			IDs: ids, HLC: hybrid,
		}}.Descriptor(),
		work.AddComment{Writer: work.CommentWriter{
			Comments: commentRepo(), Items: itemRepo(), Containers: containers, Profiles: profiles,
			Gate: gate, Authorizer: authoriser, Moderation: authoriser, Events: outbox, Changes: changes,
			Audit: sink, Activity: journal, UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
			Text: textadapter.Forms{}, Holds: postgres.NewLifecycleRepository(),
		}}.Descriptor(),
		work.CreateReminder{Writer: work.ReminderWriter{
			Reminders: postgres.NewReminderRepository(), Items: itemRepo(), Containers: containers,
			Profiles: profiles, Gate: gate, Authorizer: authoriser, Visibility: authoriser,
			Changes: changes, Audit: sink, Jobs: jobQueue(t),
			UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
		}}.Descriptor(),
		work.SetRecurrence{Writer: work.RecurrenceWriter{
			Recurrences: recurrenceRepo(), Items: itemRepo(), Containers: containers, Profiles: profiles,
			Gate: gate, Authorizer: authoriser, Expander: recurrenceadapter.New(), Changes: changes,
			Audit: sink, Activity: journal, Jobs: jobQueue(t),
			UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
		}}.Descriptor(),
		work.CreateBucket{
			Buckets: postgres.NewBucketRepository(), Containers: containers, Authorizer: authoriser,
			Events: outbox, Changes: changes, Audit: sink, UnitOfWork: unitOfWork, Clock: fixed,
			IDs: ids, HLC: hybrid, Text: textadapter.Forms{},
		}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	return registry
}

func hiddenActor(tenant, account shared.ID) appshared.ActorContext {
	actor := administrator(tenant, account)
	actor.Scopes = []string{"containers:write", "containers:read", "items:write", "items:read", "comments:write", "reminders:write", "media:write", "recurrence:write"}
	return actor
}

// storedAbout is everything stored about the entries, read past the application as the
// administrator: the rows themselves and every set, thread, reminder and series that hangs off them.
func storedAbout(ctx context.Context, t *testing.T, ids ...string) string {
	t.Helper()
	var all strings.Builder
	for _, query := range []string{
		`SELECT coalesce(string_agg(to_jsonb(r)::text, '|' ORDER BY r.id), '') FROM work_item r WHERE r.id::text = ANY($1)`,
		`SELECT coalesce(string_agg(to_jsonb(r)::text, '|' ORDER BY to_jsonb(r)::text), '') FROM item_label r WHERE r.item_id::text = ANY($1)`,
		`SELECT coalesce(string_agg(to_jsonb(r)::text, '|' ORDER BY to_jsonb(r)::text), '') FROM item_member r WHERE r.item_id::text = ANY($1)`,
		`SELECT coalesce(string_agg(to_jsonb(r)::text, '|' ORDER BY to_jsonb(r)::text), '') FROM comment r WHERE r.item_id::text = ANY($1)`,
		`SELECT coalesce(string_agg(to_jsonb(r)::text, '|' ORDER BY to_jsonb(r)::text), '') FROM item_attachment r WHERE r.item_id::text = ANY($1)`,
		`SELECT coalesce(string_agg(to_jsonb(r)::text, '|' ORDER BY to_jsonb(r)::text), '') FROM reminder r WHERE r.item_id::text = ANY($1)`,
		`SELECT coalesce(string_agg(to_jsonb(r)::text, '|' ORDER BY to_jsonb(r)::text), '') FROM recurrence_rule r WHERE r.source_item_id::text = ANY($1)`,
	} {
		var rows string
		if err := adminPool(ctx, t).QueryRow(ctx, query, ids).Scan(&rows); err != nil {
			t.Fatalf("reading what is stored: %v", err)
		}
		all.WriteString(rows + "\n")
	}
	return all.String()
}

func hiddenMark(t *testing.T, out usecase.Output) (bool, []string) {
	t.Helper()
	mark, held := out["hidden"].(map[string]any)
	if !held {
		t.Fatalf("the read carries no mark: %v", out)
	}
	level, _ := mark["level"].(bool)
	capabilities, _ := mark["capabilities"].([]string)
	return level, capabilities
}

func restItem(ctx context.Context, t *testing.T, registry *usecase.Registry, actor appshared.ActorContext,
	method, path, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	controller := rest.NewRestController()
	controller.UseCases = registry
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, actor), method,
		rest.APIBasePath+path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestNarrowingAndWideningKeepEveryValue(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := hiddenCatalogue(t)
	actor := hiddenActor(tenantB, authorB)
	// Tenant B, seeded through the repository with its own ranks: tenant A carries a narrowed TASK
	// profile and tenant B's hub level a rank another test wrote, both in the shared database.
	collection := collectionFor(ctx, t, tenantB, authorB).String()
	// UC-WRK-24's story: the collection is a project first.
	if _, err := registry.Invoke(ctx, "SetContainerStructure", actor, usecase.Input{
		"container_id": collection, "preset": "PROJECT",
	}); err != nil {
		t.Fatalf("making it a project: %v", err)
	}

	task, err := registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "TASK", "collection_id": collection, "title": "Summer party", "notes": "Budget 400",
	})
	if err != nil {
		t.Fatalf("creating the task: %v", err)
	}
	taskID := task.String("id")
	pack, err := registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "WORK_PACKAGE", "collection_id": collection, "parent_id": taskID, "title": "Drinks",
	})
	if err != nil {
		t.Fatalf("creating the work package: %v", err)
	}
	step, err := registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "ACTIVITY", "collection_id": collection, "parent_id": pack.String("id"), "title": "Order ice",
	})
	if err != nil {
		t.Fatalf("creating the activity: %v", err)
	}
	if _, err := registry.Invoke(ctx, "AssignWorkItem", actor, usecase.Input{
		"item_id": taskID, "account_id": authorB.String(),
	}); err != nil {
		t.Fatalf("assigning: %v", err)
	}
	if _, err := registry.Invoke(ctx, "SetDueDate", actor, usecase.Input{
		"item_id": taskID, "due_at": "2027-07-03T10:00:00Z",
	}); err != nil {
		t.Fatalf("dating: %v", err)
	}
	// Everything else an entry can hold, each through the use case that writes it.
	column, err := registry.Invoke(ctx, "CreateBucket", actor, usecase.Input{"collection_id": collection, "name": "Doing"})
	if err != nil {
		t.Fatalf("the column: %v", err)
	}
	attachment := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx, `INSERT INTO media_object (id, tenant_id, storage_key, mime_type, byte_size, usage, status)
		VALUES ($1, $2, $3, 'application/pdf', 12, 'ATTACHMENT', 'READY')`,
		attachment.String(), tenantB.String(), "test/"+attachment.String()); err != nil {
		t.Fatalf("the file: %v", err)
	}
	for _, write := range []struct {
		name string
		in   usecase.Input
	}{
		{"UpdateWorkItem", usecase.Input{"item_id": taskID, "bucket_id": column.String("id")}},
		{"SetCover", usecase.Input{"item_id": taskID, "kind": "COLOR", "color_token": "surface.sand"}},
		{"AddMember", usecase.Input{"item_id": taskID, "account_id": authorB.String()}},
		{"AttachMedia", usecase.Input{"item_id": taskID, "media_id": attachment.String()}},
		{"AddComment", usecase.Input{"item_id": taskID, "body": "Who brings the speakers?"}},
		{"CreateReminder", usecase.Input{"item_id": taskID, "offset_spec": "REL:-PT1H"}},
		{"SetRecurrence", usecase.Input{
			"item_id": taskID, "rrule": "FREQ=YEARLY", "time_zone": "Europe/Berlin", "mode": "ON_SCHEDULE",
		}},
	} {
		if _, err := registry.Invoke(ctx, write.name, actor, write.in); err != nil {
			t.Fatalf("%s: %v", write.name, err)
		}
	}
	entries := []string{taskID, pack.String("id"), step.String("id")}
	// The shared database: other tests ask what tenant B owes next, so the date, the reminder
	// and the series go when this test ends.
	t.Cleanup(func() {
		for _, statement := range []string{
			`DELETE FROM reminder WHERE item_id::text = ANY($1)`,
			`UPDATE work_item SET due_at = NULL, due_date_only = false, due_time_zone = NULL, recurrence_rule_id = NULL WHERE id::text = ANY($1)`,
			`DELETE FROM recurrence_rule WHERE source_item_id::text = ANY($1)`,
		} {
			if _, err := adminPool(ctx, t).Exec(ctx, statement, entries); err != nil {
				t.Errorf("cleaning up: %v", err)
			}
		}
	})
	before := storedAbout(ctx, t, entries...)

	// UC-WRK-24 check 2: the preview names what List would no longer show, and writes nothing.
	preview, err := registry.Invoke(ctx, "PreviewContainerStructure", actor, usecase.Input{
		"container_id": collection, "preset": "LIST",
	})
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}
	levels, _ := json.Marshal(preview["levels"])
	fields, _ := json.Marshal(preview["fields"])
	if preview["narrows"] != true ||
		string(levels) != `[{"entries":1,"type":"WORK_PACKAGE"},{"entries":1,"type":"ACTIVITY"}]` {
		t.Errorf("the preview named levels %s", levels)
	}
	for _, capability := range []string{"NOTES", "DUE_DATE", "ASSIGNMENT", "BUCKET", "COVER", "MEMBERS",
		"ATTACHMENTS", "COMMENTS", "REMINDER", "RECURRENCE"} {
		if !strings.Contains(string(fields), `{"capability":"`+capability+`","entries":1,"type":"TASK"}`) {
			t.Errorf("the preview did not name %s: %s", capability, fields)
		}
	}
	if previewed := storedAbout(ctx, t, entries...); previewed != before {
		t.Fatalf("the preview changed what is stored")
	}
	// Another workspace previews nothing of it.
	if _, err := registry.Invoke(ctx, "PreviewContainerStructure", hiddenActor(tenantA, authorA), usecase.Input{
		"container_id": collection, "preset": "LIST",
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("tenant A previewed tenant B's collection: %v", err)
	}

	// Narrow to List: nothing stored moves.
	if _, err := registry.Invoke(ctx, "SetContainerStructure", actor, usecase.Input{
		"container_id": collection, "preset": "LIST",
	}); err != nil {
		t.Fatalf("narrowing: %v", err)
	}
	if narrowed := storedAbout(ctx, t, entries...); narrowed != before {
		t.Fatalf("narrowing changed what is stored:\nbefore %s\nafter  %s", before, narrowed)
	}

	// Every read marks what List hides, and answers it as stored.
	read, err := registry.Invoke(ctx, "GetWorkItem", actor, usecase.Input{"item_id": taskID})
	if err != nil {
		t.Fatalf("reading the task: %v", err)
	}
	level, capabilities := hiddenMark(t, read)
	if level || !slices.Contains(capabilities, "NOTES") || !slices.Contains(capabilities, "ASSIGNMENT") ||
		!slices.Contains(capabilities, "DUE_DATE") {
		t.Errorf("the task is marked %v %v", level, capabilities)
	}
	// UC-WRK-24 check 4: the task says what List hides under it.
	if below := read["hidden"].(map[string]any)["below"]; below != 2 {
		t.Errorf("the task says %v entries hidden below it, want 2", below)
	}
	if read["notes"] != "Budget 400" || read["assignee_id"] != authorB.String() || read["due_at"] == nil {
		t.Errorf("the read lost a hidden value: %v", read)
	}
	for _, id := range entries[1:] {
		below, err := registry.Invoke(ctx, "GetWorkItem", actor, usecase.Input{"item_id": id})
		if err != nil {
			t.Fatalf("reading %s: %v", id, err)
		}
		if level, _ := hiddenMark(t, below); !level {
			t.Errorf("%s is shown in List", below["type"])
		}
	}

	// The REST answer carries the mark as the contract spells it.
	fetched := restItem(ctx, t, registry, actor, http.MethodGet, "/items/"+taskID, "")
	if fetched.Code != http.StatusOK || !strings.Contains(fetched.Body.String(), `"hidden":{`) ||
		!strings.Contains(fetched.Body.String(), `"NOTES"`) || !strings.Contains(fetched.Body.String(), `"below":2`) {
		t.Errorf("REST read answered %d %s", fetched.Code, fetched.Body)
	}

	// Writing what is hidden is refused the same way on every door, and nothing is written.
	recorder := restItem(ctx, t, registry, actor, http.MethodPatch, "/items/"+taskID, `{"notes":"Budget 500"}`)
	var problem map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &problem)
	if recorder.Code != http.StatusUnprocessableEntity || problem["code"] != "capability_not_supported" ||
		problem["detail_code"] != "items.capability_hidden" {
		t.Errorf("REST answered %d %v", recorder.Code, problem)
	}

	call, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{
		"name": "update_work_item", "arguments": map[string]any{"item_id": taskID, "notes": "Budget 500"},
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
	_, err = automation.NewActionDispatcher(registry).Dispatch(ctx, rule, automation.Action{
		Kind: "UPDATE_WORK_ITEM", Params: map[string]any{"item_id": taskID, "notes": "Budget 500"},
	}, nil)
	if !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Errorf("the automation action answered %v", err)
	}

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
		Fields: map[string]syncservice.FieldChange{"notes": {Value: "Budget 500", HLC: reading.String()}},
	}}})
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	if pushed.Results[0].Result != syncdomain.Rejected || pushed.Results[0].Error == nil ||
		pushed.Results[0].Error.Code != "capability_not_supported" {
		t.Errorf("the push answered %+v", pushed.Results[0])
	}

	// A new entry at a hidden level is refused as such.
	if _, err := registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "WORK_PACKAGE", "collection_id": collection, "parent_id": taskID, "title": "Food",
	}); !errors.Is(err, shared.ErrCapabilityNotSupported) || shared.AsError(err).DetailCode != "items.level_hidden" {
		t.Errorf("a work package in List answered %v", err)
	}

	if refused := storedAbout(ctx, t, entries...); refused != before {
		t.Fatalf("a refused write changed what is stored:\nbefore %s\nafter  %s", before, refused)
	}

	// Widen: everything is back where it was, and writable.
	if _, err := registry.Invoke(ctx, "SetContainerStructure", actor, usecase.Input{
		"container_id": collection, "preset": "PROJECT",
	}); err != nil {
		t.Fatalf("widening: %v", err)
	}
	if widened := storedAbout(ctx, t, entries...); widened != before {
		t.Fatalf("widening changed what is stored:\nbefore %s\nafter  %s", before, widened)
	}
	read, err = registry.Invoke(ctx, "GetWorkItem", actor, usecase.Input{"item_id": entries[2]})
	if err != nil {
		t.Fatalf("reading the activity: %v", err)
	}
	if level, capabilities := hiddenMark(t, read); level || len(capabilities) != 0 {
		t.Errorf("widened, the activity is marked %v %v", level, capabilities)
	}
	recorder = restItem(ctx, t, registry, actor, http.MethodPatch, "/items/"+taskID, `{"notes":"Budget 500"}`)
	if recorder.Code != http.StatusOK {
		t.Errorf("widened, REST answered %d %s", recorder.Code, recorder.Body)
	}

	// UC-WRK-24 check 8: every switch is in the trail with who, when and from what to what.
	rows, err := adminPool(ctx, t).Query(ctx, `SELECT actor_id::text, occurred_at IS NOT NULL, changes::text
		FROM audit_log WHERE target_id::text = $1 AND action = 'container.structure_updated' ORDER BY seq`, collection)
	if err != nil {
		t.Fatalf("reading the trail: %v", err)
	}
	switches := []string{}
	for rows.Next() {
		var who, changes string
		var when bool
		if err := rows.Scan(&who, &when, &changes); err != nil {
			t.Fatalf("reading the trail: %v", err)
		}
		if who != authorB.String() || !when {
			t.Errorf("a switch was recorded as %s at %v", who, when)
		}
		switches = append(switches, changes)
	}
	rows.Close()
	if len(switches) != 3 || !strings.Contains(switches[1], `"PROJECT"`) || !strings.Contains(switches[1], `"LIST"`) ||
		!strings.Contains(switches[2], `"PROJECT"`) {
		t.Errorf("the trail holds %d switches: %v", len(switches), switches)
	}
}

// The cross-tenant negative: another workspace reads none of it, mark or not.
func TestAnotherWorkspaceReadsNoHiddenEntry(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := hiddenCatalogue(t)
	actor := hiddenActor(tenantB, authorB)
	collection := collectionFor(ctx, t, tenantB, authorB).String()
	if _, err := registry.Invoke(ctx, "SetContainerStructure", actor, usecase.Input{
		"container_id": collection, "preset": "LIST",
	}); err != nil {
		t.Fatalf("narrowing: %v", err)
	}
	task, err := registry.Invoke(ctx, "CreateWorkItem", actor, usecase.Input{
		"type": "TASK", "collection_id": collection, "title": "Ours",
	})
	if err != nil {
		t.Fatalf("creating: %v", err)
	}

	_, err = registry.Invoke(ctx, "GetWorkItem", hiddenActor(tenantA, authorA), usecase.Input{"item_id": task.String("id")})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("tenant B read tenant A's entry: %v", err)
	}
}
