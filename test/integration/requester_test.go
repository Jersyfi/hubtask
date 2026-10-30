// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

type freshIDs struct{ t *testing.T }

func (f freshIDs) NewID() shared.ID { return freshID(f.t) }

// The record of requesters without an account (accepted.md B) against the real statements: a
// contact twice is one record and one contact, another workspace's sweep and erasure reach none of
// this one's, REQUESTER removes the record past the cutoff, and an erasure by address removes the
// record and the address's receipts.
func TestTheRequesterRecordStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	item := seedWorkItemForRules(ctx, t, tenantA)
	store := postgres.RequesterRepository{IDs: freshIDs{t}}
	long := time.Now().UTC().AddDate(-3, 0, 0)
	address := "kunde-" + freshID(t).String()[28:] + "@shop.test"
	for range 2 {
		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			return store.RecordContact(ctx, address, "Kunde", item, long)
		}); err != nil {
			t.Fatalf("recording: %v", err)
		}
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM requester_contact c JOIN requester r ON r.id = c.requester_id WHERE r.address = $1`, address); n != 1 {
		t.Fatalf("%d contacts after a mail delivered twice, want one", n)
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -730)
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if n, err := store.CountExpired(ctx, cutoff, 100); err != nil || n != 0 {
			t.Errorf("another workspace counted %d (%v)", n, err)
		}
		if n, err := store.EraseAddress(ctx, address, true); err != nil || n != 0 {
			t.Errorf("another workspace erased %d (%v)", n, err)
		}
		if n, err := store.DeleteExpired(ctx, cutoff, 100); err != nil || n != 0 {
			t.Errorf("another workspace removed %d (%v)", n, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if n, err := store.CountAddress(ctx, address); err != nil || n != 1 {
			t.Errorf("counted %d of the address (%v)", n, err)
		}
		n, err := store.DeleteExpired(ctx, cutoff, 100)
		if err != nil || n < 1 {
			t.Errorf("the workspace's sweep removed %d (%v)", n, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM requester WHERE address = $1`, address); n != 0 {
		t.Errorf("the requester outlived its period")
	}

	// A fresh contact, erased by address.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := store.RecordContact(ctx, address, "", item, time.Now().UTC()); err != nil {
			return err
		}
		n, err := store.EraseAddress(ctx, address, false)
		if err != nil || n != 1 {
			t.Errorf("erased %d (%v), want the record", n, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM requester WHERE address = $1`, address); n != 0 {
		t.Errorf("the erased requester is still there")
	}
}
