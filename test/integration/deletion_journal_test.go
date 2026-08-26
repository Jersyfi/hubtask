// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The deletion journal, read (E-06, backup-restore.md §7). It has been written since B-10 with a
// comment saying nothing reads it in production; these are the tests of the reader that changes
// that - the window it reads, the pages it reads in, and the boundary it may not cross (gate SG-3).

func journalRepo(batch int) postgres.DeletionJournalRepository {
	return postgres.NewDeletionJournalRepository(batch)
}

// journal writes one entry straight into the table, which is what the lifecycle side does when it
// removes a row for good.
func journal(ctx context.Context, t *testing.T, tenant shared.ID, entity string, id shared.ID, at time.Time) {
	t.Helper()
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO deletion_journal (tenant_id, entity, entity_id, deleted_at, reason)
		VALUES ($1, $2, $3, $4, 'USER')`,
		tenant.String(), entity, id.String(), at); err != nil {
		t.Fatalf("journalling: %v", err)
	}
}

func deletedSince(
	ctx context.Context, t *testing.T, tenant shared.ID, batch int, since time.Time,
) []repository.Deletion {
	t.Helper()

	var entries []repository.Deletion
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		return journalRepo(batch).DeletedSince(ctx, since, func(entry repository.Deletion) error {
			entries = append(entries, entry)
			return nil
		})
	}); err != nil {
		t.Fatalf("reading the journal: %v", err)
	}
	return entries
}

// The window is what makes the read bounded: an object deleted before the archive was taken is not
// in the archive, so nothing has to be kept out on its account.
func TestTheJournalAnswersOnlyWhatWasDeletedAfterTheArchive(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	archiveAt := time.Now().UTC().Truncate(time.Millisecond)
	before, after := freshID(t), freshID(t)
	journal(ctx, t, tenantA, "work_item", before, archiveAt.Add(-time.Hour))
	journal(ctx, t, tenantA, "work_item", after, archiveAt.Add(time.Hour))

	seen := map[shared.ID]bool{}
	for _, entry := range deletedSince(ctx, t, tenantA, 100, archiveAt) {
		seen[entry.EntityID] = true
	}

	if !seen[after] {
		t.Error("a deletion after the archive was not answered, so a restore would bring it back")
	}
	if seen[before] {
		t.Error("a deletion from before the archive was answered")
	}
}

// A page smaller than the journal: every entry comes back exactly once, whatever the page is.
func TestTheJournalPagesWithoutRepeatingOrSkippingAnEntry(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	archiveAt := time.Now().UTC().Truncate(time.Millisecond)
	written := map[shared.ID]bool{}
	for i := range 7 {
		id := freshID(t)
		written[id] = true
		// Three of them share an instant, which is what the three-part cursor is for: a page that
		// compared the timestamp alone would repeat them for ever.
		journal(ctx, t, tenantA, "work_item", id, archiveAt.Add(time.Duration(i/3)*time.Second+time.Second))
	}

	seen := map[shared.ID]int{}
	for _, entry := range deletedSince(ctx, t, tenantA, 2, archiveAt) {
		seen[entry.EntityID]++
	}

	for id := range written {
		switch seen[id] {
		case 1:
		case 0:
			t.Errorf("%s was never answered", id)
		default:
			t.Errorf("%s was answered %d times", id, seen[id])
		}
	}
}

// Gate SG-3. A journal entry read across the boundary would not be a wrong answer but somebody
// else's erasure applied to this tenant's restore - or, the other way round, not applied to
// theirs.
func TestTheJournalIsInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	archiveAt := time.Now().UTC().Truncate(time.Millisecond)
	theirs := freshID(t)
	journal(ctx, t, tenantB, "work_item", theirs, archiveAt.Add(time.Hour))

	for _, entry := range deletedSince(ctx, t, tenantA, 100, archiveAt) {
		if entry.EntityID == theirs {
			t.Fatalf("tenant A read tenant B's deletion journal")
		}
	}
}
