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
		Devices:    postgres.NewDeviceRepository(),
		Ops:        postgres.NewSyncOpLog(),
		Tombstones: postgres.NewTombstoneRepository(),
		Catalogue:  itemCatalogueFor(ctx, t),
		Skew:       skew,
	}
}

func pushActor(tenant, account shared.ID) appshared.ActorContext {
	actor := itemWriter(tenant, account)
	actor.Scopes = []string{"items:read", "items:write"}
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
