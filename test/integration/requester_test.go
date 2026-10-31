// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"slices"
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
	// Its own workspaces, so the test holds alone and not only after one that seeds them.
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name, default_locale, default_time_zone)
		VALUES ($1, 'tenant-a', 'A', 'de', 'Europe/Berlin'), ($2, 'tenant-b', 'B', 'en', 'UTC')
		ON CONFLICT (id) DO NOTHING`, tenantA.String(), tenantB.String()); err != nil {
		t.Fatalf("seeding the workspaces: %v", err)
	}
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
	// Who a reply is mailed to (UC-SVC-12): the entry's requesters here, nobody next door.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		got, err := store.AddressesOn(ctx, item)
		if err != nil || len(got) != 1 || got[0].Address != address || got[0].Name != "Kunde" {
			t.Errorf("the entry's requesters are %+v (%v)", got, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if got, err := store.AddressesOn(ctx, item); err != nil || len(got) != 0 {
			t.Errorf("another workspace read the entry's requesters %+v (%v)", got, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
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

// Third answers 2: the intake's "From:" lines of an address are found in this workspace's notes and
// comments, never in another's, and the expiring batch's addresses are this workspace's.
func TestTheIntakeLinesOfAnAddressStayInTheirWorkspace(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name, default_locale, default_time_zone)
		VALUES ($1, 'tenant-a', 'A', 'de', 'Europe/Berlin'), ($2, 'tenant-b', 'B', 'en', 'UTC')
		ON CONFLICT (id) DO NOTHING`, tenantA.String(), tenantB.String()); err != nil {
		t.Fatalf("seeding the workspaces: %v", err)
	}
	address := "line-" + freshID(t).String()[28:] + "@shop.test"
	items := map[shared.ID]shared.ID{}
	for _, tenant := range []shared.ID{tenantA, tenantB} {
		item := seedWorkItemForRules(ctx, t, tenant)
		items[tenant] = item
		if _, err := admin.Exec(ctx, `UPDATE work_item SET notes = $1 WHERE id = $2`,
			"From: Erika <"+address+">\n\nThe printer", item.String()); err != nil {
			t.Fatalf("writing the notes: %v", err)
		}
		if _, err := admin.Exec(ctx, `
			INSERT INTO comment (id, tenant_id, item_id, author_id, body)
			SELECT $1, w.tenant_id, w.id, w.created_by, $2 FROM work_item w WHERE w.id = $3`,
			freshID(t).String(), "From: "+address+"\n\nStill broken", item.String()); err != nil {
			t.Fatalf("writing the comment: %v", err)
		}
		if _, err := admin.Exec(ctx, `
			INSERT INTO requester (id, tenant_id, address, first_contact_at, last_contact_at)
			VALUES ($1, $2, $3, now() - interval '3 years', now() - interval '3 years')`,
			freshID(t).String(), tenant.String(), address); err != nil {
			t.Fatalf("seeding the requester: %v", err)
		}
	}
	store := postgres.RequesterRepository{IDs: freshIDs{t}}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		lines, err := store.IntakeLinesFrom(ctx, []string{address})
		if err != nil {
			return err
		}
		if len(lines) != 2 {
			t.Fatalf("found %d lines, want this workspace's notes and comment: %+v", len(lines), lines)
		}
		for _, line := range lines {
			if line.ItemID != items[tenantA] || line.WrittenBy.IsZero() {
				t.Errorf("a line of another entry or without its filer: %+v", line)
			}
		}
		if other, err := store.IntakeLinesFrom(ctx, []string{"nobody@shop.test"}); err != nil || len(other) != 0 {
			t.Errorf("another address found %d lines (%v)", len(other), err)
		}
		due, err := store.ExpiredAddresses(ctx, time.Now().UTC().AddDate(0, 0, -730), 100)
		if err != nil {
			return err
		}
		if !slices.Contains(due, address) {
			t.Errorf("the expiring addresses are %v", due)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
