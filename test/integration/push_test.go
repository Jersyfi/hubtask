// SPDX-License-Identifier: BUSL-1.1
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
	"github.com/Jersyfi/hubtask/core/port/text"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The push against a real database and the real catalogue (N-04): a creation lands through
// CreateWorkItem exactly as an online one does, a duplicate push takes effect exactly once
// (SY-7), and a device three hours out is bounded (SY-2).

func pushFor(ctx context.Context, t *testing.T, skew time.Duration) syncservice.PushChanges {
	t.Helper()
	return syncservice.PushChanges{
		Stream:     streamFor(ctx, t),
		IDs:        clockadapter.NewUUIDv7(portclock.Fixed(created)),
		Devices:    postgres.NewDeviceRepository(),
		Ops:        postgres.NewSyncOpLog(),
		Tombstones: postgres.NewTombstoneRepository(),
		Catalogue:  itemCatalogueFor(ctx, t),
		Skew:       skew,
	}
}

func pushActor(tenant, account shared.ID) appshared.ActorContext {
	actor := itemWriter(tenant, account)
	actor.Scopes = []string{"items:read", "items:write", "comments:write"}
	return actor
}

// SY-7: a duplicate push of the same mutations takes effect exactly once.
func TestADuplicatePushCreatesTheEntryExactlyOnce(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	push := pushFor(ctx, t, 5*time.Minute)
	deviceID, opID, itemID := freshID(t), freshID(t), freshID(t)

	request := syncservice.PushRequest{
		DeviceID: deviceID, Platform: "hubctl",
		Mutations: []syncservice.Mutation{{
			OpID: opID, Kind: syncdomain.ItemCreate, ItemID: itemID,
			Payload: map[string]any{"type": "TASK", "collection_id": collection.String(), "title": "Pushed from the train"},
		}},
	}
	first, err := push.Push(ctx, pushActor(tenantB, authorB), request)
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	if first.Results[0].Result != syncdomain.Applied || first.Results[0].EntityID != itemID {
		t.Fatalf("the first push answered %+v", first.Results[0])
	}
	if first.Results[0].ServerState["id"] != itemID.String() || first.Results[0].ServerState["title"] != "Pushed from the train" {
		t.Errorf("the server state is %v", first.Results[0].ServerState)
	}

	second, err := push.Push(ctx, pushActor(tenantB, authorB), request)
	if err != nil {
		t.Fatalf("pushing again: %v", err)
	}
	if second.Results[0].Result != syncdomain.Applied || second.Results[0].EntityID != itemID {
		t.Errorf("the repeat answered %+v", second.Results[0])
	}

	// Exactly one entry, one change log entry for the creation, and it names the device.
	if rows := countIn(ctx, t, `SELECT count(*) FROM work_item WHERE id = $1`, itemID.String()); rows != 1 {
		t.Errorf("%d entries, want one", rows)
	}
	var deviceOnChange *string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT device_id::text FROM change_log WHERE entity_id = $1 AND op = 'UPSERT'`, itemID.String(),
	).Scan(&deviceOnChange); err != nil {
		t.Fatalf("reading the change: %v", err)
	}
	if deviceOnChange == nil || *deviceOnChange != deviceID.String() {
		t.Errorf("the change names device %v, want %s", deviceOnChange, deviceID)
	}
	// The device is registered, and the cursor after the push is where the log stands.
	if rows := countIn(ctx, t, `SELECT count(*) FROM sync_device WHERE id = $1 AND account_id = $2`,
		deviceID.String(), authorB.String()); rows != 1 {
		t.Errorf("the device was not registered")
	}
	if first.Cursor.Seq < 1 {
		t.Errorf("the push answered no cursor")
	}
}

// A refusal is a result, recorded, and does not block the next; the repeat answers the same.
func TestARefusalIsRecordedAndTheQueueGoesOn(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	push := pushFor(ctx, t, 5*time.Minute)
	deviceID := freshID(t)
	refusedOp, appliedOp := freshID(t), freshID(t)

	request := syncservice.PushRequest{DeviceID: deviceID, Mutations: []syncservice.Mutation{
		{OpID: refusedOp, Kind: syncdomain.ItemCreate, ItemID: freshID(t),
			Payload: map[string]any{"type": "TASK", "collection_id": collection.String(), "title": ""}},
		{OpID: appliedOp, Kind: syncdomain.ItemCreate, ItemID: freshID(t),
			Payload: map[string]any{"type": "TASK", "collection_id": collection.String(), "title": "After the refusal"}},
	}}
	response, err := push.Push(ctx, pushActor(tenantB, authorB), request)
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	if response.Results[0].Result != syncdomain.Rejected || response.Results[0].Error == nil {
		t.Errorf("the empty title was answered %+v", response.Results[0])
	}
	if response.Results[1].Result != syncdomain.Applied {
		t.Errorf("the mutation after the refusal was answered %+v", response.Results[1])
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM sync_op_log WHERE op_id IN ($1, $2)`,
		refusedOp.String(), appliedOp.String()); rows != 2 {
		t.Errorf("%d operations recorded, want both", rows)
	}
}

// SY-2: a device three hours out synchronises, and its readings do not outvote everybody - the
// change log carries a server reading under the device's identifier, and nothing else changes.
func TestADeviceThreeHoursOutIsBoundedAndStillLands(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	push := pushFor(ctx, t, 5*time.Minute)
	deviceID, itemID := freshID(t), freshID(t)
	threeHoursOut, err := shared.NewHLC(created.Add(3*time.Hour), 1, deviceID.String())
	if err != nil {
		t.Fatalf("stamping: %v", err)
	}

	response, err := push.Push(ctx, pushActor(tenantB, authorB), syncservice.PushRequest{
		DeviceID: deviceID, Mutations: []syncservice.Mutation{{
			OpID: freshID(t), Kind: syncdomain.ItemCreate, ItemID: itemID, HLC: threeHoursOut.String(),
			Payload: map[string]any{"type": "TASK", "collection_id": collection.String(), "title": "Clock is wrong"},
		}},
	})
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	if response.Results[0].Result != syncdomain.Applied {
		t.Errorf("a device with a wrong clock was answered %+v", response.Results[0])
	}
	var hlc string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT hlc FROM change_log WHERE entity_id = $1 AND op = 'UPSERT'`, itemID.String()).Scan(&hlc); err != nil {
		t.Fatalf("reading the change: %v", err)
	}
	recorded, err := shared.ParseHLC(hlc)
	if err != nil {
		t.Fatalf("parsing the recorded reading: %v", err)
	}
	if recorded.Physical.After(created.Add(5 * time.Minute)) {
		t.Errorf("the recorded reading %s is three hours out; the device's clock outvoted the server", hlc)
	}
}

// patchCatalogueFor is the catalogue an ITEM_PATCH performs through: the read, the update and
// the due trio, over the real adapters.
func patchCatalogueFor(ctx context.Context, t *testing.T) *usecase.Registry {
	t.Helper()

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	ids := clockadapter.NewUUIDv7(fixed)
	hybrid, err := clockadapter.NewHybridClock(fixed, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	sink := postgres.NewAuditSink(ids)
	profiles := postgres.NewCapabilityProfileRepository()
	journal := work.ActivityJournal{Entries: historyRepo(), IDs: ids}
	outbox := postgres.NewOutbox(jobQueue(t))
	changes := postgres.NewChangeLog()
	authorizer := access.Service{
		Memberships: postgres.NewMembershipRepository(), UnitOfWork: unitOfWork, Audit: sink, Clock: fixed,
	}
	dueDates := work.DueDateWriter{
		Items: itemRepo(), Containers: containerRepo(), Profiles: profiles, Authorizer: authorizer,
		Events: outbox, Changes: changes, Audit: sink, Activity: journal,
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
	}

	registry, err := usecase.NewRegistry(nil,
		work.GetWorkItem{
			Items: itemRepo(), ItemLabels: itemLabelRepo(), Containers: containerRepo(),
			Authorizer: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.UpdateWorkItem{
			Items: itemRepo(), Buckets: bucketRepo(), Containers: containerRepo(), Profiles: profiles,
			Authorizer: authorizer, Events: outbox, Changes: changes, Audit: sink, Activity: journal,
			UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid, DueDates: dueDates,
			Text: text.Composing{},
		}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	return registry
}

// SY-1: two devices change different fields of the same entry offline, and both changes survive.
// Device A renames it, device B writes notes; each pushes its own field under its own reading, and
// the entry ends with A's title and B's notes - with every winning field stamped under the reading
// that decided it.
func TestTwoDevicesChangingDifferentFieldsBothSurvive(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	task := seedTask(ctx, t, tenantB, authorB, collection)

	push := pushFor(ctx, t, 5*time.Minute)
	push.Catalogue = patchCatalogueFor(ctx, t)
	push.Clocks = postgres.NewChangeLog()
	actor := pushActor(tenantB, authorB)
	// The readings stand near the server's own clock - the stream's is the system's - so that
	// the skew leaves them alone; the fixture's `created` is a month back and would be bounded.
	wall := time.Now()
	deviceA, deviceB := freshID(t), freshID(t)
	readingA, _ := shared.NewHLC(wall.Add(time.Minute), 1, deviceA.String())
	readingB, _ := shared.NewHLC(wall.Add(2*time.Minute), 1, deviceB.String())

	fromA, err := push.Push(ctx, actor, syncservice.PushRequest{DeviceID: deviceA, Mutations: []syncservice.Mutation{{
		OpID: freshID(t), Kind: syncdomain.ItemPatch, ItemID: task,
		Fields: map[string]syncservice.FieldChange{"title": {Value: "Renamed on the train", HLC: readingA.String()}},
	}}})
	if err != nil {
		t.Fatalf("device A: %v", err)
	}
	fromB, err := push.Push(ctx, actor, syncservice.PushRequest{DeviceID: deviceB, Mutations: []syncservice.Mutation{{
		OpID: freshID(t), Kind: syncdomain.ItemPatch, ItemID: task,
		Fields: map[string]syncservice.FieldChange{"notes": {Value: "Written at the airport", HLC: readingB.String()}},
	}}})
	if err != nil {
		t.Fatalf("device B: %v", err)
	}
	if fromA.Results[0].Result != syncdomain.Applied || fromB.Results[0].Result != syncdomain.Applied {
		t.Errorf("A answered %s, B answered %s, want both APPLIED", fromA.Results[0].Result, fromB.Results[0].Result)
	}

	item := findWorkItem(ctx, t, tenantB, task)
	if item.Title != "Renamed on the train" || item.Notes != "Written at the airport" {
		t.Errorf("the entry ended as %q / %q; one device's change was lost", item.Title, item.Notes)
	}
	// The clocks are the devices' readings, not fresh server readings.
	var clocks map[string]shared.HLC
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		clocks, err = postgres.NewChangeLog().Of(ctx, "item", task)
		return err
	}); err != nil {
		t.Fatalf("reading the clocks: %v", err)
	}
	if clocks["title"].Compare(readingA) != 0 || clocks["notes"].Compare(readingB) != 0 {
		t.Errorf("the clocks are %v, want the devices' readings", clocks)
	}

	// SY-2's other half: a third device, its clock three hours ahead, is bounded to server time -
	// which sorts before the reading device A wrote a minute ahead - and does not outvote A.
	deviceC := freshID(t)
	hoursAhead, _ := shared.NewHLC(wall.Add(3*time.Hour), 1, deviceC.String())
	fromC, err := push.Push(ctx, actor, syncservice.PushRequest{DeviceID: deviceC, Mutations: []syncservice.Mutation{{
		OpID: freshID(t), Kind: syncdomain.ItemPatch, ItemID: task,
		Fields: map[string]syncservice.FieldChange{"title": {Value: "Outvoted", HLC: hoursAhead.String()}},
	}}})
	if err != nil {
		t.Fatalf("device C: %v", err)
	}
	if fromC.Results[0].Result != syncdomain.Merged {
		t.Errorf("a device three hours ahead was answered %s, want MERGED", fromC.Results[0].Result)
	}
	if findWorkItem(ctx, t, tenantB, task).Title != "Renamed on the train" {
		t.Errorf("a device with a clock three hours ahead outvoted device A")
	}
}

// mergeCatalogueFor is the catalogue N-06's kinds perform through: the read, the update, the
// completion pair and the move, over the real adapters - and the writers the push files a
// displaced version and a step of the history through.
func mergeCatalogueFor(ctx context.Context, t *testing.T) (*usecase.Registry, work.AddComment, work.ActivityJournal) {
	t.Helper()

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	ids := clockadapter.NewUUIDv7(fixed)
	hybrid, err := clockadapter.NewHybridClock(fixed, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	sink := postgres.NewAuditSink(ids)
	profiles := postgres.NewCapabilityProfileRepository()
	journal := work.ActivityJournal{Entries: historyRepo(), IDs: ids}
	outbox := postgres.NewOutbox(jobQueue(t))
	changes := postgres.NewChangeLog()
	authorizer := access.Service{
		Memberships: postgres.NewMembershipRepository(), UnitOfWork: unitOfWork, Audit: sink, Clock: fixed,
	}
	dueDates := work.DueDateWriter{
		Items: itemRepo(), Containers: containerRepo(), Profiles: profiles, Authorizer: authorizer,
		Events: outbox, Changes: changes, Audit: sink, Activity: journal,
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
	}
	completion := work.CompletionWriter{
		Items: itemRepo(), Containers: containerRepo(), Profiles: profiles, Authorizer: authorizer,
		Events: outbox, Changes: changes, Audit: sink, Activity: journal, Jobs: jobQueue(t),
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
	}
	placement := work.PlacementWriter{
		Items: itemRepo(), Buckets: bucketRepo(), Containers: containerRepo(), Profiles: profiles,
		Authorizer: authorizer, Events: outbox, Changes: changes, Audit: sink, Activity: journal,
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
	}
	comments := work.AddComment{Writer: work.CommentWriter{
		Comments: commentRepo(), Items: itemRepo(), Containers: containerRepo(), Profiles: profiles,
		Authorizer: authorizer, Moderation: authorizer, Events: outbox, Changes: changes, Audit: sink,
		Activity: journal, UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid, Text: text.Composing{},
	}}

	registry, err := usecase.NewRegistry(nil,
		work.GetWorkItem{
			Items: itemRepo(), ItemLabels: itemLabelRepo(), Containers: containerRepo(),
			Authorizer: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.UpdateWorkItem{
			Items: itemRepo(), Buckets: bucketRepo(), Containers: containerRepo(), Profiles: profiles,
			Authorizer: authorizer, Events: outbox, Changes: changes, Audit: sink, Activity: journal,
			UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid, DueDates: dueDates,
			Text: text.Composing{},
		}.Descriptor(),
		work.CompleteWorkItem{Completion: completion}.Descriptor(),
		work.ReopenWorkItem{Completion: completion}.Descriptor(),
		work.MoveWorkItem{Placement: placement}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	return registry, comments, journal
}

func mergingPushFor(ctx context.Context, t *testing.T) syncservice.PushChanges {
	t.Helper()
	push := pushFor(ctx, t, 5*time.Minute)
	registry, comments, journal := mergeCatalogueFor(ctx, t)
	push.Catalogue = registry
	push.Clocks = postgres.NewChangeLog()
	push.Activity = journal
	push.Displaced = comments
	return push
}

func patchMutation(t *testing.T, item shared.ID, field string, value any, reading shared.HLC) syncservice.Mutation {
	t.Helper()
	return syncservice.Mutation{
		OpID: freshID(t), Kind: syncdomain.ItemPatch, ItemID: item,
		Fields: map[string]syncservice.FieldChange{field: {Value: value, HLC: reading.String()}},
	}
}

// SY-10: a displaced free-text version is findable again after the merge - as a system comment
// on the entry, in the pushing person's name, with the merge marked in the history.
func TestADisplacedNotesVersionIsFindableAgainAfterTheMerge(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	task := seedTask(ctx, t, tenantB, authorB, collection)
	push := mergingPushFor(ctx, t)
	actor := pushActor(tenantB, authorB)
	wall := time.Now()
	deviceA, deviceB := freshID(t), freshID(t)
	later, _ := shared.NewHLC(wall.Add(2*time.Minute), 1, deviceA.String())
	earlier, _ := shared.NewHLC(wall.Add(time.Minute), 1, deviceB.String())

	if _, err := push.Push(ctx, actor, syncservice.PushRequest{DeviceID: deviceA,
		Mutations: []syncservice.Mutation{patchMutation(t, task, "notes", "Kept: the later version", later)}}); err != nil {
		t.Fatalf("device A: %v", err)
	}
	response, err := push.Push(ctx, actor, syncservice.PushRequest{DeviceID: deviceB,
		Mutations: []syncservice.Mutation{patchMutation(t, task, "notes", "Lost: the earlier version", earlier)}})
	if err != nil {
		t.Fatalf("device B: %v", err)
	}
	result := response.Results[0]
	if result.Result != syncdomain.Conflict || result.Conflict == nil || result.Conflict.PreservedCommentID.IsZero() {
		t.Fatalf("device B was answered %+v, want CONFLICT with the preserved comment", result)
	}
	if result.Conflict.Mine != "Lost: the earlier version" || result.Conflict.Theirs != "Kept: the later version" {
		t.Errorf("the conflict carries %+v", result.Conflict)
	}

	// The entry keeps the later version, and the displaced one is a system comment in B's name.
	if item := findWorkItem(ctx, t, tenantB, task); item.Notes != "Kept: the later version" {
		t.Errorf("the entry's notes are %q", item.Notes)
	}
	comment := findComment(ctx, t, tenantB, result.Conflict.PreservedCommentID)
	if comment.Kind != workdomain.CommentBySystem || comment.SystemCode != workdomain.DisplacedVersionCode ||
		comment.Body != "Lost: the earlier version" || comment.AuthorID != authorB ||
		comment.SystemParams["device"] != deviceB.String() || comment.SystemParams["field"] != "notes" {
		t.Errorf("the preserved comment is %+v", comment)
	}
	// And the history marks the merge.
	var merged int
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT count(*) FROM activity_entry WHERE item_id = $1 AND verb = 'item.merged'`, task.String(),
	).Scan(&merged); err != nil {
		t.Fatalf("reading the history: %v", err)
	}
	if merged != 1 {
		t.Errorf("%d merge steps in the history, want one", merged)
	}
}

// SY-12: a move that would close a cycle is detected and rejected, in the contract's word, and
// the tree is as it was.
func TestAMoveThatWouldCloseACycleIsRejected(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	collection := collectionFor(ctx, t, tenantB, authorB)
	parent := seedRootTask(ctx, t, tenantB, authorB, collection)
	registry := itemCatalogueFor(ctx, t)
	child, err := registry.Invoke(ctx, "CreateWorkItem", itemWriter(tenantB, authorB), usecase.Input{
		"type": "WORK_PACKAGE", "parent_id": parent.String(), "title": "Under the parent",
	})
	if err != nil {
		t.Fatalf("the child: %v", err)
	}
	push := mergingPushFor(ctx, t)
	reading, _ := shared.NewHLC(time.Now(), 1, "dev-a")

	response, err := push.Push(ctx, pushActor(tenantB, authorB), syncservice.PushRequest{
		DeviceID: freshID(t), Mutations: []syncservice.Mutation{{
			OpID: freshID(t), Kind: syncdomain.Move, ItemID: parent, HLC: reading.String(),
			Payload: map[string]any{"parent_id": child.String("id")},
		}},
	})
	if err != nil {
		t.Fatalf("pushing: %v", err)
	}
	result := response.Results[0]
	if result.Result != syncdomain.Rejected || result.Error == nil || result.Error.MessageCode != "sync.cycle_detected" {
		t.Errorf("the cycle was answered %+v", result)
	}
	if item := findWorkItem(ctx, t, tenantB, parent); !item.ParentID.IsZero() {
		t.Errorf("the parent moved under its own child")
	}
}

// SY-8: a recurrence completed offline produces exactly one follow-up, even on a double
// completion - two devices complete the same occurrence, the second is idempotent in the use
// case, and the materialisation owes one occurrence.
func TestADoubleCompletionFromTwoDevicesProducesOneFollowUp(t *testing.T) {
	ctx := context.Background()
	// A workspace of its own, the materialisation tests' habit: a due date in tenant B would be
	// an announcement tenant B owes, which another test asserts it does not.
	tenant, author := seedMaterialisingTenant(ctx, t)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES ($1, $2, $3, 'TENANT', 'OWNER')`,
		freshID(t).String(), tenant.String(), author.String()); err != nil {
		t.Fatalf("seeding the membership: %v", err)
	}
	_, collection := hubWithCollection(ctx, t, tenant, author)
	template := seedTask(ctx, t, tenant, author, collection)
	due := created.Add(24 * time.Hour)
	dueDate, err := workdomain.NewDueDate(&due, false, "Europe/Berlin")
	if err != nil {
		t.Fatalf("the due date was refused: %v", err)
	}
	item := findWorkItem(ctx, t, tenant, template)
	item.Due, item.UpdatedAt = dueDate, due
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return itemRepo().SetDueDate(ctx, item, item.Version)
	}); err != nil {
		t.Fatalf("setting the due date: %v", err)
	}
	rule, err := workdomain.NewRecurrenceRule(workdomain.NewRecurrenceRuleInput{
		ID: freshID(t), TenantID: tenant, ItemID: template,
		Spec: workdomain.RecurrenceSpec{RRULE: "FREQ=DAILY", TimeZone: "Europe/Berlin",
			Mode: string(workdomain.RecurrenceOnCompletion), HorizonDays: 30},
		Due: dueDate, Now: due,
	})
	if err != nil {
		t.Fatalf("the series was refused: %v", err)
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return recurrenceRepo().Insert(ctx, rule)
	}); err != nil {
		t.Fatalf("writing the series: %v", err)
	}

	push := mergingPushFor(ctx, t)
	wall := time.Now()
	for i, device := range []shared.ID{freshID(t), freshID(t)} {
		reading, _ := shared.NewHLC(wall.Add(time.Duration(i+1)*time.Minute), 1, device.String())
		response, err := push.Push(ctx, pushActor(tenant, author), syncservice.PushRequest{
			DeviceID: device, Mutations: []syncservice.Mutation{
				patchMutation(t, template, "completion", map[string]any{"is_completed": true}, reading),
			},
		})
		if err != nil {
			t.Fatalf("device %d: %v", i, err)
		}
		if response.Results[0].Result == syncdomain.Rejected {
			t.Fatalf("device %d was rejected: %+v", i, response.Results[0])
		}
	}

	at := due.Add(time.Hour)
	for range 2 {
		if err := write(ctx, t, tenant, func(ctx context.Context) error {
			_, err := newMaterialisation(ctx, t, at).Execute(ctx, materialisingActor(tenant))
			return err
		}); err != nil {
			t.Fatalf("the pass failed: %v", err)
		}
	}
	if occurrences := occurrencesOf(ctx, t, tenant, collection, template); len(occurrences) != 1 {
		t.Errorf("two completions produced %d follow-ups, want exactly one", len(occurrences))
	}
}
