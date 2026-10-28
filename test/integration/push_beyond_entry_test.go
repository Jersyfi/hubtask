// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	syncdomain "github.com/Jersyfi/hubtask/core/domain/model/sync"
	workdomain "github.com/Jersyfi/hubtask/core/domain/model/work"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	recurrenceadapter "github.com/Jersyfi/hubtask/infrastructure/recurrence"
	textadapter "github.com/Jersyfi/hubtask/infrastructure/text"
)

// The kinds beyond the entry's against a real database and the routes' own use cases
// (offline-sync.md §4.2): a reminder, a series, a container, a label and a bucket created,
// changed field by field and deleted, and a template applied - each through `:push`, a replayed
// push applied once, and another workspace's label refused (UC-SYN-01 check 6).

func beyondEntryCatalogue(ctx context.Context, t *testing.T) *usecase.Registry {
	t.Helper()
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	hybrid, err := clockadapter.NewHybridClock(fixed, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	sink := postgres.NewAuditSink(ids)
	profiles := postgres.NewCapabilityProfileRepository()
	journal := work.ActivityJournal{Entries: historyRepo(), IDs: ids}
	outbox := postgres.NewOutbox(jobQueue(t))
	changes := postgres.NewChangeLog()
	containers := containerRepo()
	authorizer := access.Service{
		Memberships: postgres.NewMembershipRepository(), UnitOfWork: unitOfWork, Audit: sink, Clock: fixed,
	}
	due := work.DueDateWriter{
		Items: itemRepo(), Containers: containers, Profiles: profiles, Authorizer: authorizer,
		Reminders: postgres.NewReminderRepository(), Jobs: jobQueue(t),
		Events: outbox, Changes: changes, Audit: sink, Activity: journal,
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
	}
	reminders := work.ReminderWriter{
		Reminders: postgres.NewReminderRepository(), Items: itemRepo(), Containers: containers,
		Profiles: profiles, Authorizer: authorizer, Visibility: authorizer,
		Changes: changes, Audit: sink, Jobs: jobQueue(t),
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
	}
	series := work.RecurrenceWriter{
		Recurrences: recurrenceRepo(), Items: itemRepo(), Containers: containers, Profiles: profiles,
		Authorizer: authorizer, Expander: recurrenceadapter.New(), Changes: changes,
		Audit: sink, Activity: journal, Jobs: jobQueue(t),
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
	}
	containerWriter := work.ContainerWriter{
		Containers: containers, Authorizer: authorizer, Events: outbox, Changes: changes, Audit: sink,
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid, Text: textadapter.Forms{},
	}
	labels := work.LabelWriter{
		Labels: labelRepo(), Containers: containers, Authorizer: authorizer, Events: outbox, Changes: changes,
		Audit: sink, UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid, Text: textadapter.Forms{},
	}
	buckets := work.BucketWriter{
		Buckets: bucketRepo(), Containers: containers, Authorizer: authorizer, Events: outbox, Changes: changes,
		Audit: sink, UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid, Text: textadapter.Forms{},
	}
	templates := work.TemplateWriter{
		Templates: templateRepo(), Containers: containers, Profiles: profiles,
		Authorizer: authorizer, Changes: changes, Audit: sink,
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
		Holds: postgres.NewLifecycleRepository(), Removals: postgres.NewLifecycleRepository(),
		TombstoneWindow: 90 * 24 * time.Hour,
	}
	registry, err := usecase.NewRegistry(nil,
		work.CreateWorkItem{
			Items: itemRepo(), Containers: containers, Profiles: profiles,
			Authorizer: authorizer, Ownership: authorizer, Events: outbox, Changes: changes,
			Audit: sink, Activity: journal, UnitOfWork: unitOfWork,
			Clock: fixed, IDs: ids, HLC: hybrid, DueDates: due,
		}.Descriptor(),
		work.CreateReminder{Writer: reminders}.Descriptor(),
		work.UpdateReminder{Writer: reminders}.Descriptor(),
		work.DeleteReminder{Writer: reminders}.Descriptor(),
		work.ListReminders{
			Reminders: postgres.NewReminderRepository(), Items: itemRepo(), Containers: containers,
			Authorizer: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.SetRecurrence{Writer: series}.Descriptor(),
		work.RemoveRecurrence{Writer: series}.Descriptor(),
		work.GetRecurrence{
			Recurrences: recurrenceRepo(), Items: itemRepo(), Containers: containers,
			Authorizer: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.CreateContainer{
			Containers: containers, Authorizer: authorizer, OwnHubs: authorizer,
			Grants:      postgres.NewMembershipGrantRepository(pageCursors()),
			PrivateHubs: privateHubsRule(unitOfWork), Events: outbox, Changes: changes,
			Audit: sink, UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
			Text: textadapter.Forms{},
		}.Descriptor(),
		work.RenameContainer{Writer: containerWriter}.Descriptor(),
		work.TrashContainer{Writer: containerWriter}.Descriptor(),
		work.GetContainer{Containers: containers, Authorizer: authorizer, UnitOfWork: unitOfWork}.Descriptor(),
		work.CreateLabel{
			Labels: labelRepo(), Containers: containers, Authorizer: authorizer, Events: outbox,
			Changes: changes, Audit: sink, UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
			Text: textadapter.Forms{},
		}.Descriptor(),
		work.UpdateLabel{Writer: labels}.Descriptor(),
		work.DeleteLabel{Writer: labels}.Descriptor(),
		work.ListLabels{Labels: labelRepo(), Containers: containers, Authorizer: authorizer, UnitOfWork: unitOfWork}.Descriptor(),
		work.CreateBucket{
			Buckets: bucketRepo(), Containers: containers, Authorizer: authorizer,
			Events: outbox, Changes: changes, Audit: sink, UnitOfWork: unitOfWork, Clock: fixed,
			IDs: ids, HLC: hybrid, Text: textadapter.Forms{},
		}.Descriptor(),
		work.UpdateBucket{Writer: buckets}.Descriptor(),
		work.DeleteBucket{Writer: buckets}.Descriptor(),
		work.ListBuckets{Buckets: bucketRepo(), Containers: containers, Authorizer: authorizer, UnitOfWork: unitOfWork}.Descriptor(),
		work.CreateTemplate{Writer: templates}.Descriptor(),
		work.InstantiateTemplate{
			Writer: templates, Items: postgres.NewItemRepository(pageCursors()), ItemMembers: itemMemberRepo(),
			Visibility: authorizer, Events: outbox, Activity: journal,
		}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	return registry
}

func beyondEntryActor(tenant, account shared.ID) appshared.ActorContext {
	actor := itemWriter(tenant, account)
	actor.Scopes = []string{"items:read", "items:write", "reminders:write", "recurrence:write",
		"containers:read", "containers:write", "labels:read", "labels:write", "buckets:read", "buckets:write",
		"templates:read", "templates:write"}
	return actor
}

// readingAt is a device's reading, later than anything the fixed server clock stamps.
func readingAt(t *testing.T, minutes int) string {
	t.Helper()
	reading, err := shared.NewHLC(created.Add(time.Duration(minutes)*time.Minute), 1, "dev-train")
	if err != nil {
		t.Fatalf("minting a reading: %v", err)
	}
	return reading.String()
}

func TestEveryKindBeyondTheEntryLandsThroughThePushAndOnlyOnce(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	registry := beyondEntryCatalogue(ctx, t)
	actor := beyondEntryActor(tenantB, authorB)
	push := pushFor(ctx, t, 365*24*time.Hour)
	push.Catalogue = registry
	push.Clocks = postgres.NewChangeLog()

	template, err := registry.Invoke(ctx, "CreateTemplate", actor, usecase.Input{
		"scope_type": string(workdomain.TemplateScopeCollection), "scope_id": collection.String(),
		"name": "Move house " + freshName(t), "root_type": "TASK",
		"nodes": []any{map[string]any{"type": "TASK", "title": "Move house"}},
	})
	if err != nil {
		t.Fatalf("creating the template: %v", err)
	}

	item, reminder, rule, hub, label, bucket := freshID(t), freshID(t), freshID(t), freshID(t), freshID(t), freshID(t)
	op := func() shared.ID { return freshID(t) }
	mutations := []syncservice.Mutation{
		{OpID: op(), Kind: syncdomain.ItemCreate, ItemID: item, HLC: readingAt(t, 1), Payload: map[string]any{
			"type": "TASK", "collection_id": collection.String(), "title": "Water the plants", "due_at": "2027-07-03T10:00:00Z"}},
		{OpID: op(), Kind: syncdomain.ReminderCreate, ItemID: item, EntityID: reminder, HLC: readingAt(t, 2),
			Payload: map[string]any{"offset_spec": "REL:-PT1H"}},
		{OpID: op(), Kind: syncdomain.ReminderPatch, ItemID: item, EntityID: reminder, Fields: map[string]syncservice.FieldChange{
			"offset_spec": {Value: "REL:-PT2H", HLC: readingAt(t, 3)}}},
		{OpID: op(), Kind: syncdomain.RecurrenceCreate, ItemID: item, EntityID: rule, HLC: readingAt(t, 4),
			Payload: map[string]any{"rrule": "FREQ=YEARLY", "time_zone": "Europe/Berlin", "mode": "ON_SCHEDULE"}},
		{OpID: op(), Kind: syncdomain.RecurrencePatch, ItemID: item, EntityID: rule, Fields: map[string]syncservice.FieldChange{
			"time_zone": {Value: "Europe/Vienna", HLC: readingAt(t, 5)}}},
		{OpID: op(), Kind: syncdomain.ContainerCreate, EntityID: hub, HLC: readingAt(t, 6),
			Payload: map[string]any{"type": "HUB", "name": "Allotment " + freshName(t)}},
		{OpID: op(), Kind: syncdomain.ContainerPatch, EntityID: hub, Fields: map[string]syncservice.FieldChange{
			"description": {Value: "The garden by the river", HLC: readingAt(t, 7)}}},
		{OpID: op(), Kind: syncdomain.LabelCreate, EntityID: label, HLC: readingAt(t, 8),
			Payload: map[string]any{"collection_id": collection.String(), "name": "Outdoors", "color_token": "red"}},
		{OpID: op(), Kind: syncdomain.LabelPatch, EntityID: label, ContainerID: collection, Fields: map[string]syncservice.FieldChange{
			"name": {Value: "Garden", HLC: readingAt(t, 9)}}},
		{OpID: op(), Kind: syncdomain.BucketCreate, EntityID: bucket, HLC: readingAt(t, 10),
			Payload: map[string]any{"collection_id": collection.String(), "name": "Waiting"}},
		{OpID: op(), Kind: syncdomain.BucketPatch, EntityID: bucket, ContainerID: collection, Fields: map[string]syncservice.FieldChange{
			"wip_limit": {Value: 3, HLC: readingAt(t, 11)}}},
		{OpID: op(), Kind: syncdomain.TemplateApply, HLC: readingAt(t, 12), Payload: map[string]any{
			"template_id": template.String("id"), "collection_id": collection.String(), "anchor_date": "2027-07-01"}},
		{OpID: op(), Kind: syncdomain.ReminderDelete, ItemID: item, EntityID: reminder, HLC: readingAt(t, 13)},
		{OpID: op(), Kind: syncdomain.RecurrenceDelete, ItemID: item, EntityID: rule, HLC: readingAt(t, 14)},
		{OpID: op(), Kind: syncdomain.LabelDelete, EntityID: label, ContainerID: collection, HLC: readingAt(t, 15)},
		{OpID: op(), Kind: syncdomain.BucketDelete, EntityID: bucket, ContainerID: collection, HLC: readingAt(t, 16)},
		{OpID: op(), Kind: syncdomain.ContainerDelete, EntityID: hub, HLC: readingAt(t, 17)},
	}
	request := syncservice.PushRequest{DeviceID: freshID(t), Platform: "hubctl", Mutations: mutations}

	first, err := push.Push(ctx, actor, request)
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	for i, result := range first.Results {
		if result.Result != syncdomain.Applied {
			t.Errorf("%s answered %s %+v", mutations[i].Kind, result.Result, result.Error)
		}
	}
	// What each step left, read past the application: the device's identifiers are final.
	checks := []struct {
		what  string
		query string
		args  []any
	}{
		{"the reminder was created under the device's identifier, changed, and deleted",
			`SELECT count(*) FROM change_log WHERE entity = 'reminder' AND entity_id = $1 AND op = 'DELETE'`, []any{reminder.String()}},
		{"the reminder's offset was written under the device's reading",
			`SELECT count(*) FROM field_clock WHERE entity = 'reminder' AND entity_id = $1 AND field = 'offset_spec' AND hlc = $2`,
			[]any{reminder.String(), readingAt(t, 3)}},
		{"the series was created under the device's identifier and its time zone written under the reading",
			`SELECT count(*) FROM field_clock WHERE entity = 'recurrence_rule' AND entity_id = $1 AND field = 'time_zone' AND hlc = $2`,
			[]any{rule.String(), readingAt(t, 5)}},
		{"the hub carries the description and is in the trash",
			`SELECT count(*) FROM container WHERE id = $1 AND description = 'The garden by the river' AND deleted_at IS NOT NULL`, []any{hub.String()}},
		{"the label was renamed and deleted",
			`SELECT count(*) FROM label WHERE id = $1 AND name = 'Garden' AND deleted_at IS NOT NULL`, []any{label.String()}},
		{"the bucket took the limit and was deleted",
			`SELECT count(*) FROM bucket WHERE id = $1 AND wip_limit = 3 AND deleted_at IS NOT NULL`, []any{bucket.String()}},
		{"the template's entry arrived as an ordinary creation",
			`SELECT count(*) FROM work_item WHERE collection_id = $1 AND title = 'Move house'`, []any{collection.String()}},
	}
	for _, check := range checks {
		if rows := countIn(ctx, t, check.query, check.args...); rows != 1 {
			t.Errorf("%s: %d rows, want one", check.what, rows)
		}
	}

	// The whole queue again, as a device whose connection died before the answer does: every
	// answer the same, nothing applied twice.
	second, err := push.Push(ctx, actor, request)
	if err != nil {
		t.Fatalf("pushing again: %v", err)
	}
	for i := range second.Results {
		if second.Results[i].Result != first.Results[i].Result || second.Results[i].EntityID != first.Results[i].EntityID {
			t.Errorf("%s answered %+v the second time, %+v the first", mutations[i].Kind, second.Results[i], first.Results[i])
		}
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM work_item WHERE collection_id = $1 AND title = 'Move house'`,
		collection.String()); rows != 1 {
		t.Errorf("the template was applied %d times", rows)
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM change_log WHERE entity = 'label' AND entity_id = $1 AND op = 'UPSERT' AND payload ? 'collection_id'`,
		label.String()); rows != 1 {
		t.Errorf("the label was created %d times", rows)
	}
}

// A device of another workspace names a label it cannot see: refused, recorded, and the label
// untouched - row level security holds under the push as under the route.
func TestAnotherWorkspacesLabelIsRefusedToAPush(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	registry := beyondEntryCatalogue(ctx, t)
	label, err := registry.Invoke(ctx, "CreateLabel", beyondEntryActor(tenantB, authorB), usecase.Input{
		"collection_id": collection.String(), "name": "Private", "color_token": "red",
	})
	if err != nil {
		t.Fatalf("creating the label: %v", err)
	}
	push := pushFor(ctx, t, 365*24*time.Hour)
	push.Catalogue = registry
	push.Clocks = postgres.NewChangeLog()

	for _, m := range []syncservice.Mutation{
		{OpID: freshID(t), Kind: syncdomain.LabelPatch, EntityID: shared.ID(label.String("id")), ContainerID: collection,
			Fields: map[string]syncservice.FieldChange{"name": {Value: "Mine now", HLC: readingAt(t, 30)}}},
		{OpID: freshID(t), Kind: syncdomain.LabelDelete, EntityID: shared.ID(label.String("id")), ContainerID: collection,
			HLC: readingAt(t, 31)},
	} {
		response, err := push.Push(ctx, beyondEntryActor(tenantA, authorA), syncservice.PushRequest{
			DeviceID: freshID(t), Mutations: []syncservice.Mutation{m},
		})
		if err != nil {
			t.Fatalf("pushing: %v", err)
		}
		if response.Results[0].Result != syncdomain.Rejected {
			t.Errorf("%s across workspaces answered %+v", m.Kind, response.Results[0])
		}
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM label WHERE id = $1 AND name = 'Private' AND deleted_at IS NULL`,
		label.String("id")); rows != 1 {
		t.Errorf("another workspace's push changed the label")
	}
}
