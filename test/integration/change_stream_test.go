// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The read half of the change log (C-10): the walk the stream and `:pull` share, and a cross-tenant
// negative for each of its methods (gate SG-3).

// recordChange writes one entry for the tenant and returns the container it names.
func recordChange(
	ctx context.Context, t *testing.T, tenant, actor, container shared.ID, entity string,
) shared.ID {
	t.Helper()

	reading, err := shared.HLC{}.Tick(created, "server-1")
	if err != nil {
		t.Fatalf("stamping: %v", err)
	}
	entityID := freshID(t)
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return postgres.NewChangeLog().Record(ctx, changelog.Change{
			TenantID: tenant, Entity: entity, EntityID: entityID,
			Op: changelog.Upsert, ContainerID: container, ActorID: actor,
			HLC: reading, Payload: map[string]any{"title": "Review the quote"},
		})
	}); err != nil {
		t.Fatalf("recording the change: %v", err)
	}
	return entityID
}

func readChanges(
	ctx context.Context, t *testing.T, tenant shared.ID, after int64, batch int,
) []changelog.Recorded {
	t.Helper()

	var entries []changelog.Recorded
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		entries, err = postgres.NewChangeLog().After(ctx, after, batch)
		return err
	}); err != nil {
		t.Fatalf("reading the change log: %v", err)
	}
	return entries
}

func latestSeq(ctx context.Context, t *testing.T, tenant shared.ID) int64 {
	t.Helper()

	var latest int64
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		latest, err = postgres.NewChangeLog().Latest(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading the head of the change log: %v", err)
	}
	return latest
}

func TestTheChangeLogIsWalkedInCursorOrder(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	start := latestSeq(ctx, t, tenantA)
	container := freshID(t)
	first := recordChange(ctx, t, tenantA, authorA, container, "work_item")
	second := recordChange(ctx, t, tenantA, authorA, container, "work_item")

	entries := readChanges(ctx, t, tenantA, start, 10)
	if len(entries) != 2 {
		t.Fatalf("%d entries after the cursor, want the two just written", len(entries))
	}
	if entries[0].EntityID != first || entries[1].EntityID != second {
		t.Errorf("the walk is out of order: %v, %v", entries[0].EntityID, entries[1].EntityID)
	}
	if entries[0].Seq >= entries[1].Seq {
		t.Errorf("the sequence did not advance: %d then %d", entries[0].Seq, entries[1].Seq)
	}
	if entries[0].Seq <= start {
		t.Errorf("an entry at %d came back for a cursor of %d", entries[0].Seq, start)
	}

	// Everything the writer recorded comes back out, parsed rather than as text.
	entry := entries[0]
	if entry.Entity != "work_item" || entry.Op != changelog.Upsert {
		t.Errorf("entry %+v", entry)
	}
	if entry.ContainerID != container || entry.ActorID != authorA {
		t.Errorf("the references did not survive: %+v", entry)
	}
	if entry.HLC.IsZero() {
		t.Error("the clock did not come back, and every merge needs it")
	}
	if entry.Payload["title"] != "Review the quote" {
		t.Errorf("payload %v", entry.Payload)
	}
	if entry.OccurredAt.IsZero() {
		t.Error("the entry has no time")
	}

	// Resuming from the last cursor returns nothing: no gap, and no duplicate.
	if rest := readChanges(ctx, t, tenantA, entries[1].Seq, 10); len(rest) != 0 {
		t.Errorf("%d entries after the last cursor, want none", len(rest))
	}
}

// A batch is a batch, and the cursor of its last entry is what continues the walk. There is no
// `has_more` to compute: a short page is the end of the log.
func TestTheWalkIsBatchedAndResumesWithoutAGap(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	start := latestSeq(ctx, t, tenantA)
	container := freshID(t)
	written := make([]shared.ID, 0, 5)
	for range 5 {
		written = append(written, recordChange(ctx, t, tenantA, authorA, container, "work_item"))
	}

	var seen []shared.ID
	cursor := start
	for range 10 {
		batch := readChanges(ctx, t, tenantA, cursor, 2)
		if len(batch) == 0 {
			break
		}
		for _, entry := range batch {
			seen = append(seen, entry.EntityID)
		}
		cursor = batch[len(batch)-1].Seq
	}

	if len(seen) != len(written) {
		t.Fatalf("the walk saw %d entries, want %d", len(seen), len(written))
	}
	for i, id := range written {
		if seen[i] != id {
			t.Errorf("entry %d is %s, want %s", i, seen[i], id)
		}
	}
}

// A deletion carries no payload by design, and it comes back as nothing rather than as an empty
// change set - which is a different statement.
func TestADeletionComesBackWithoutAPayload(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	start := latestSeq(ctx, t, tenantA)
	reading, err := shared.HLC{}.Tick(created, "server-1")
	if err != nil {
		t.Fatalf("stamping: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.NewChangeLog().Record(ctx, changelog.Change{
			TenantID: tenantA, Entity: "work_item", EntityID: freshID(t),
			Op: changelog.Delete, ContainerID: freshID(t), ActorID: authorA, HLC: reading,
		})
	}); err != nil {
		t.Fatalf("recording the deletion: %v", err)
	}

	entries := readChanges(ctx, t, tenantA, start, 10)
	if len(entries) != 1 {
		t.Fatalf("%d entries, want the deletion", len(entries))
	}
	if entries[0].Op != changelog.Delete {
		t.Errorf("op %q", entries[0].Op)
	}
	if entries[0].Payload != nil {
		t.Errorf("the deletion carries a payload: %v", entries[0].Payload)
	}
}

// Gate SG-3, and the acceptance criterion's first half: a client sees only its own tenant's
// records. Row level security narrows the walk rather than failing it, which is the stronger
// property - a query that errored would still have had to be trusted not to match.
func TestTheChangeWalkSeesOnlyItsOwnTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	startA := latestSeq(ctx, t, tenantA)
	startB := latestSeq(ctx, t, tenantB)

	inA := recordChange(ctx, t, tenantA, authorA, freshID(t), "work_item")
	inB := recordChange(ctx, t, tenantB, authorB, freshID(t), "work_item")

	for _, tc := range []struct {
		name    string
		tenant  shared.ID
		from    int64
		wanted  shared.ID
		refused shared.ID
	}{
		{"tenant A", tenantA, startA, inA, inB},
		{"tenant B", tenantB, startB, inB, inA},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, entry := range readChanges(ctx, t, tc.tenant, tc.from, 100) {
				if entry.EntityID == tc.refused {
					t.Errorf("the walk returned another tenant's entry %s", entry.EntityID)
				}
			}

			// And its own is there, so the test is about the boundary rather than about an empty
			// answer.
			var found bool
			for _, entry := range readChanges(ctx, t, tc.tenant, tc.from, 100) {
				if entry.EntityID == tc.wanted {
					found = true
				}
			}
			if !found {
				t.Errorf("the walk did not return this tenant's own entry %s", tc.wanted)
			}
		})
	}

	// Latest is the head of *this* tenant's log. A head that counted everybody's writes would hand
	// a new client a cursor that skips its own first change.
	if latest := latestSeq(ctx, t, tenantA); latest <= startA {
		t.Errorf("the head of tenant A's log did not move: %d", latest)
	}
	headB := latestSeq(ctx, t, tenantB)
	entriesB := readChanges(ctx, t, tenantB, startB, 100)
	if len(entriesB) == 0 || entriesB[len(entriesB)-1].Seq != headB {
		t.Errorf("the head %d is not the last entry tenant B can see", headB)
	}
}

// A workspace nothing has happened in has a head of zero rather than an error, which is what lets a
// client open a stream on a new workspace and be told about its first change.
func TestAnUntouchedWorkspaceHasAHeadOfZero(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	fresh := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'Fresh')`,
		fresh.String(), "tenant-"+fresh.String()[:8]); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}

	if latest := latestSeq(ctx, t, fresh); latest != 0 {
		t.Errorf("the head of an untouched workspace is %d, want 0", latest)
	}
	if entries := readChanges(ctx, t, fresh, 0, 10); len(entries) != 0 {
		t.Errorf("%d entries in an untouched workspace", len(entries))
	}
}
