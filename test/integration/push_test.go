// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	syncdomain "github.com/Jersyfi/hubtask/core/domain/model/sync"
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
