// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	syncdomain "github.com/Jersyfi/hubtask/core/domain/model/sync"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The operation log and the tombstone lookup behind a push (N-04): a record round-trips with its
// response, a repeat that raced the first is left standing, a purged entity is held - and a
// cross-tenant negative for every method (gate SG-3).

func opLog() postgres.SyncOpLog { return postgres.NewSyncOpLog() }

func tombstones() postgres.TombstoneRepository { return postgres.NewTombstoneRepository() }

func TestAnOperationRecordRoundTripsAndTheFirstAnswerStands(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	opID, deviceID, entityID := freshID(t), freshID(t), freshID(t)

	first := repository.OpRecord{
		OpID: opID, DeviceID: deviceID, Result: syncdomain.Applied, EntityID: entityID,
		AppliedAt: created, Response: map[string]any{"op_id": opID.String(), "result": "APPLIED"},
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return opLog().Record(ctx, first)
	}); err != nil {
		t.Fatalf("recording: %v", err)
	}
	// A repeat with a different answer changes nothing: the first answer is the answer.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return opLog().Record(ctx, repository.OpRecord{OpID: opID, Result: syncdomain.Rejected, AppliedAt: created})
	}); err != nil {
		t.Fatalf("recording again: %v", err)
	}

	var stored repository.OpRecord
	var found bool
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, found, err = opLog().Find(ctx, opID)
		return err
	}); err != nil || !found {
		t.Fatalf("finding: %v (found=%v)", err, found)
	}
	if stored.Result != syncdomain.Applied || stored.DeviceID != deviceID || stored.EntityID != entityID ||
		!stored.AppliedAt.Equal(created) || stored.Response["result"] != "APPLIED" {
		t.Errorf("the record came back as %+v", stored)
	}

	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		_, found, err := opLog().Find(ctx, freshID(t))
		if found {
			t.Errorf("an operation nobody recorded was found")
		}
		return err
	}); err != nil {
		t.Fatalf("finding an unknown operation: %v", err)
	}
}

func TestAPurgedEntityIsHeldByItsTombstone(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	purged, live := freshID(t), freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return lifecycleRepo().Record(ctx, []lifecycle.Removal{
			{Entity: "work_item", EntityID: purged, Reason: lifecycle.DeletedByRetention},
		}, created, created.Add(90*24*time.Hour))
	}); err != nil {
		t.Fatalf("recording the removal: %v", err)
	}

	for id, want := range map[shared.ID]bool{purged: true, live: false} {
		var held bool
		if err := read(ctx, t, tenantA, func(ctx context.Context) error {
			var err error
			held, err = tombstones().Holds(ctx, "work_item", id)
			return err
		}); err != nil {
			t.Fatalf("asking: %v", err)
		}
		if held != want {
			t.Errorf("%s held=%v, want %v", id, held, want)
		}
	}
}

// Gate SG-3: one negative per port method.
func TestTheOperationLogAndTheTombstonesAreInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	opID, purged := freshID(t), freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := opLog().Record(ctx, repository.OpRecord{OpID: opID, Result: syncdomain.Applied, AppliedAt: created}); err != nil {
			return err
		}
		return lifecycleRepo().Record(ctx, []lifecycle.Removal{
			{Entity: "work_item", EntityID: purged, Reason: lifecycle.DeletedByRetention},
		}, created, created.Add(90*24*time.Hour))
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	t.Run("find", func(t *testing.T) {
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			_, found, err := opLog().Find(ctx, opID)
			if found {
				t.Errorf("tenant B found tenant A's operation")
			}
			return err
		}); err != nil {
			t.Fatalf("finding: %v", err)
		}
	})
	t.Run("record", func(t *testing.T) {
		// The same op_id in tenant B is tenant B's own row, not a repeat of tenant A's.
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return opLog().Record(ctx, repository.OpRecord{OpID: opID, Result: syncdomain.Rejected, AppliedAt: created})
		}); err != nil {
			t.Fatalf("recording in tenant B: %v", err)
		}
		var stored repository.OpRecord
		if err := read(ctx, t, tenantA, func(ctx context.Context) error {
			var err error
			stored, _, err = opLog().Find(ctx, opID)
			return err
		}); err != nil || stored.Result != syncdomain.Applied {
			t.Errorf("tenant B's record reached tenant A: %+v (%v)", stored, err)
		}
	})
	t.Run("holds", func(t *testing.T) {
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			held, err := tombstones().Holds(ctx, "work_item", purged)
			if held {
				t.Errorf("tenant B saw tenant A's tombstone")
			}
			return err
		}); err != nil {
			t.Fatalf("asking: %v", err)
		}
	})
}
