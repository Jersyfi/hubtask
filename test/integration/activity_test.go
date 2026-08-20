// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/activity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The item history against the real database (B-11): the order a page comes back in, the walk over
// several pages, the cross-tenant negative for both halves of the port (gate SG-3), and the cascade
// that is the deletion path the data catalogue declares.

func historyRepo() postgres.ActivityRepository {
	return postgres.NewActivityRepository(pageCursors())
}

// stepOn builds one step of one entry's history, at the moment given.
func stepOn(
	t *testing.T, tenant, author, collection, item shared.ID, verb domain.Verb, at time.Time,
) domain.Entry {
	t.Helper()

	return domain.Entry{
		ID: freshID(t), TenantID: tenant, ItemID: item, CollectionID: collection,
		Actor:      domain.Actor{Kind: shared.ActorUser, ID: author},
		Verb:       verb,
		ChangeSet:  map[string]any{},
		OccurredAt: at,
	}
}

// recordSteps writes a history through the repository, oldest first.
func recordSteps(ctx context.Context, t *testing.T, tenant shared.ID, entries ...domain.Entry) {
	t.Helper()

	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		for _, entry := range entries {
			if err := historyRepo().Record(ctx, entry); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("writing the history: %v", err)
	}
}

func historyOf(
	ctx context.Context, t *testing.T, tenant, item shared.ID, page repository.Page,
) repository.EntryPage {
	t.Helper()

	var history repository.EntryPage
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		history, err = historyRepo().List(ctx, item, page)
		return err
	}); err != nil {
		t.Fatalf("reading the history: %v", err)
	}
	return history
}

func verbsOf(page repository.EntryPage) []domain.Verb {
	verbs := make([]domain.Verb, 0, len(page.Entries))
	for _, entry := range page.Entries {
		verbs = append(verbs, entry.Verb)
	}
	return verbs
}

// The acceptance criterion's second half: the entries appear in order, newest first, and the change
// set survives the round trip through jsonb.
func TestTheHistoryComesBackNewestFirst(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	item := freshID(t)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, taskIn(tenantA, authorA, collection, item, "Weekly shop", "a0"))
	}); err != nil {
		t.Fatalf("seeding the entry: %v", err)
	}

	renamed := stepOn(t, tenantA, authorA, collection, item, domain.ItemUpdated, created.Add(time.Hour))
	renamed.ChangeSet = domain.ChangeSet(domain.Full,
		domain.Field{Name: "title", Detail: domain.WithValues, From: "Milk", To: "Oat milk"},
		domain.Field{Name: "notes", Detail: domain.NameOnly, From: "a page", To: "another page"})

	recordSteps(ctx, t, tenantA,
		stepOn(t, tenantA, authorA, collection, item, domain.ItemCreated, created),
		renamed,
		stepOn(t, tenantA, authorA, collection, item, domain.ItemCompleted, created.Add(2*time.Hour)),
	)

	page := historyOf(ctx, t, tenantA, item, repository.Page{Size: 50})

	want := []domain.Verb{domain.ItemCompleted, domain.ItemUpdated, domain.ItemCreated}
	if got := verbsOf(page); len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("the history reads %v, want %v", got, want)
	}
	if page.Info.HasMore {
		t.Errorf("a full history reports another page: %+v", page.Info)
	}

	step := page.Entries[1]
	if step.Actor.Kind != shared.ActorUser || step.Actor.ID != authorA {
		t.Errorf("the actor reads %+v", step.Actor)
	}
	if step.CollectionID != collection {
		t.Errorf("the step is filed under %s rather than the collection", step.CollectionID)
	}
	title, _ := step.ChangeSet["title"].(map[string]any)
	if title["from"] != "Milk" || title["to"] != "Oat milk" {
		t.Errorf("the rename came back as %v", title)
	}
	// The note's text never reached the database, so it cannot come back out of it.
	notes, _ := step.ChangeSet["notes"].(map[string]any)
	if notes["changed"] != true || len(notes) != 1 {
		t.Errorf("the note came back as %v", notes)
	}
}

// The boundary is the pair (occurred_at, id), because two steps of one act share a timestamp: a
// cursor on the moment alone would either skip the second or return the first forever.
func TestAHistoryIsWalkedPageByPageWithoutLosingASimultaneousStep(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	item := freshID(t)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, taskIn(tenantA, authorA, collection, item, "Weekly shop", "a0"))
	}); err != nil {
		t.Fatalf("seeding the entry: %v", err)
	}

	// Four steps, two of them at the same moment - the shape a roll-up produces.
	moment := created.Add(time.Hour)
	recordSteps(ctx, t, tenantA,
		stepOn(t, tenantA, authorA, collection, item, domain.ItemCreated, created),
		stepOn(t, tenantA, authorA, collection, item, domain.ItemCompleted, moment),
		stepOn(t, tenantA, authorA, collection, item, domain.ItemArchived, moment),
		stepOn(t, tenantA, authorA, collection, item, domain.ItemTrashed, created.Add(2*time.Hour)),
	)

	seen := map[shared.ID]bool{}
	cursor := ""
	for pages := 0; ; pages++ {
		if pages > 4 {
			t.Fatal("the walk did not end")
		}
		page := historyOf(ctx, t, tenantA, item, repository.Page{Cursor: cursor, Size: 2})
		for _, entry := range page.Entries {
			if seen[entry.ID] {
				t.Errorf("%s came back twice", entry.Verb)
			}
			seen[entry.ID] = true
		}
		if !page.Info.HasMore {
			break
		}
		cursor = page.Info.NextCursor
	}

	if len(seen) != 4 {
		t.Errorf("the walk saw %d steps, want all four", len(seen))
	}
}

// Gate SG-3, for both halves of the port: another tenant's history is not readable, and a step
// written for one tenant does not appear in the other's.
func TestAHistoryIsNotReadableFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	item := freshID(t)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, taskIn(tenantA, authorA, collection, item, "Weekly shop", "a0"))
	}); err != nil {
		t.Fatalf("seeding the entry: %v", err)
	}
	recordSteps(ctx, t, tenantA,
		stepOn(t, tenantA, authorA, collection, item, domain.ItemCreated, created))

	if page := historyOf(ctx, t, tenantB, item, repository.Page{Size: 50}); len(page.Entries) != 0 {
		t.Errorf("tenant B read %v of tenant A's history", verbsOf(page))
	}

	// And the writing side: a step whose entry belongs to another tenant is refused rather than
	// written under the wrong one. Row level security would refuse the row anyway - the foreign key
	// has nothing to point at inside tenant B - and the repository refuses before that, so the
	// answer is about the entry rather than about a constraint.
	stray := stepOn(t, tenantA, authorA, collection, item, domain.ItemUpdated, created.Add(time.Hour))
	err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return historyRepo().Record(ctx, stray)
	})
	if err == nil {
		t.Fatal("a step for another tenant's entry was accepted")
	}
	if page := historyOf(ctx, t, tenantA, item, repository.Page{Size: 50}); len(page.Entries) != 1 {
		t.Errorf("tenant A's history now reads %v", verbsOf(page))
	}
}

// The deletion path the data catalogue declares for `activity_entry`: CASCADE, "with the item". A
// history that outlived the entry it describes would be a copy of somebody's business content
// surviving the deletion meant to reach it.
func TestPurgingAnEntryTakesItsHistoryWithIt(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	item, neighbour := freshID(t), freshID(t)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := itemRepo().Insert(
			ctx, taskIn(tenantA, authorA, collection, item, "Weekly shop", "a0")); err != nil {
			return err
		}
		return itemRepo().Insert(
			ctx, taskIn(tenantA, authorA, collection, neighbour, "Next week", "a1"))
	}); err != nil {
		t.Fatalf("seeding the entries: %v", err)
	}

	recordSteps(ctx, t, tenantA,
		stepOn(t, tenantA, authorA, collection, item, domain.ItemCreated, created),
		stepOn(t, tenantA, authorA, collection, neighbour, domain.ItemCreated, created))

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := trashRepo().PurgeItems(ctx, []shared.ID{item})
		return err
	}); err != nil {
		t.Fatalf("purging the entry: %v", err)
	}

	if page := historyOf(ctx, t, tenantA, item, repository.Page{Size: 50}); len(page.Entries) != 0 {
		t.Errorf("the purged entry still has a history: %v", verbsOf(page))
	}
	// And only that entry's. A cascade that reached further would be a deletion nobody asked for.
	if page := historyOf(ctx, t, tenantA, neighbour, repository.Page{Size: 50}); len(page.Entries) != 1 {
		t.Errorf("the neighbour's history reads %v, want its one step", verbsOf(page))
	}
}
