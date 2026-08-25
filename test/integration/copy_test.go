// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The two statements a duplication is written from, against a real database (C-11): the subtree it
// reads and the row it writes. Each gets a cross-tenant negative, because the boundary is row level
// security underneath and the only way to know it reaches a new statement is to try it (gate SG-3).

func TestASubtreeIsReadParentsBeforeChildren(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	task, pack, activity := movableSubtree(ctx, t, tenantA, authorA, collection)

	below := subtreeOf(ctx, t, tenantA, task, 10)

	if len(below) != 2 {
		t.Fatalf("the subtree has %d entries, want the work package and the activity", len(below))
	}
	// Depth first, so that one pass always meets an entry after the one it hangs from - which is
	// what lets a copy carry its mapping from old identifier to new one forwards.
	if below[0].ID != pack.ID || below[1].ID != activity.ID {
		t.Errorf("the subtree came back as %v, want the pack before the activity",
			[]shared.ID{below[0].ID, below[1].ID})
	}
	// The entry itself matches its own path prefix, and is not part of what is below it.
	for _, entry := range below {
		if entry.ID == task.ID {
			t.Error("the entry is in its own subtree")
		}
	}
}

// A trashed entry is on its way out of the system, and a copy carrying it would put back what
// somebody deleted.
func TestASubtreeLeavesTrashedEntriesOut(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	task, pack, _ := movableSubtree(ctx, t, tenantA, authorA, collection)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		stamp := created.Add(time.Hour)
		pack.DeletedAt, pack.TrashBatchID = &stamp, freshID(t)
		_, err := itemRepo().TrashSubtree(ctx, repository.ItemTrash{
			Item: pack, Prefix: pack.Path, BatchID: pack.TrashBatchID,
			ExpectedVersion: pack.Version,
		})
		return err
	}); err != nil {
		t.Fatalf("trashing the work package: %v", err)
	}

	if below := subtreeOf(ctx, t, tenantA, task, 10); len(below) != 0 {
		t.Errorf("the subtree still holds %d trashed entries", len(below))
	}
}

// The limit is a bound rather than a page, and one row beyond it comes back: that is how the caller
// tells "as large as allowed" from "larger than allowed".
func TestASubtreeReadsOneRowBeyondTheBound(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	task, _, _ := movableSubtree(ctx, t, tenantA, authorA, collection)

	if below := subtreeOf(ctx, t, tenantA, task, 1); len(below) != 2 {
		t.Errorf("a bound of one answered %d rows, want the one beyond it as well", len(below))
	}
	if below := subtreeOf(ctx, t, tenantA, task, 2); len(below) != 2 {
		t.Errorf("a bound of two answered %d rows, want the whole subtree", len(below))
	}
}

// Gate SG-3 for the subtree read.
func TestASubtreeCannotBeReadAcrossTheTenantBoundary(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	task, _, _ := movableSubtree(ctx, t, tenantA, authorA, collection)

	var below []work.WorkItem
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		below, err = itemRepo().Subtree(ctx, task, 10)
		return err
	}); err != nil {
		t.Fatalf("reading across the boundary answered %v", err)
	}
	if len(below) != 0 {
		t.Errorf("tenant B read %d of tenant A's entries", len(below))
	}
}

// What a copy carries, and what it deliberately does not.
func TestACopyCarriesTheDescriptionAndNotTheCompletion(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	field := definedField(ctx, t, tenantA, collection, "priority", work.CustomFieldText)

	archivedAt := created.Add(time.Hour)
	original := taskIn(tenantA, authorA, collection, freshID(t), freshName(t), "a0")
	original.Notes = "Semi-skimmed, two litres"
	original.AssigneeID = authorA
	original.Cover = &work.Cover{Kind: work.CoverColor, ColorToken: "blue"}
	original.CustomFields = map[string]any{field.Key: "the value"}
	original.ContentLanguage = "en"
	original.ArchivedAt = &archivedAt
	// The state a copy does not take over: somebody completed the original, at a moment, and the
	// copy was never completed by anybody.
	original.Completion = work.Completion{IsCompleted: true, CompletedAt: &archivedAt, CompletedBy: authorA}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().InsertCopy(ctx, repository.Copy{
			Item:             original,
			FieldDefinitions: map[string]shared.ID{field.Key: field.ID},
		})
	}); err != nil {
		t.Fatalf("writing the copy: %v", err)
	}

	stored := findWorkItem(ctx, t, tenantA, original.ID)
	if stored.Title != original.Title || stored.Notes != original.Notes {
		t.Errorf("the copy reads %q / %q", stored.Title, stored.Notes)
	}
	if stored.AssigneeID != authorA || stored.ContentLanguage != "en" {
		t.Errorf("the copy lost the assignee or the language: %+v", stored)
	}
	if stored.Cover == nil || stored.Cover.ColorToken != "blue" {
		t.Errorf("the copy lost the cover: %+v", stored.Cover)
	}
	if stored.CustomFields[field.Key] != "the value" {
		t.Errorf("the copy reads the custom fields as %v", stored.CustomFields)
	}
	// The archive stamp travels, so that an entry put away below the copied one is not silently
	// brought back.
	if stored.ArchivedAt == nil {
		t.Error("the copy of an archived entry came back active")
	}
	if stored.Completion.IsCompleted || !stored.Completion.CompletedBy.IsZero() {
		t.Errorf("the copy claims a completion nobody performed: %+v", stored.Completion)
	}
	if stored.Version != 1 {
		t.Errorf("the copy was born at version %d", stored.Version)
	}
}

// A value whose definition the caller did not name is refused rather than written: a value standing
// behind nothing is invisible to every read while occupying the key (C-07).
func TestACopyRefusesACustomFieldValueWithoutItsDefinition(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)

	original := taskIn(tenantA, authorA, collection, freshID(t), freshName(t), "a0")
	original.CustomFields = map[string]any{"priority": "high"}

	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().InsertCopy(ctx, repository.Copy{Item: original})
	})
	if code := shared.AsError(err); code == nil || code.DetailCode != "items.custom_field_reference_missing" {
		t.Fatalf("an unreferenced value answered %v", err)
	}
}

// Gate SG-3 for the copy.
func TestACopyCannotBeWrittenAcrossTheTenantBoundary(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)

	original := taskIn(tenantA, authorA, collection, freshID(t), freshName(t), "a0")
	err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return itemRepo().InsertCopy(ctx, repository.Copy{Item: original})
	})
	if err == nil {
		t.Fatal("tenant B wrote a copy into tenant A's collection")
	}

	var stored work.WorkItem
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = itemRepo().Find(ctx, original.ID)
		return err
	}); err == nil {
		t.Errorf("the row was written all the same: %+v", stored)
	}
}

// subtreeOf reads a subtree back, failing the test rather than returning an error: every caller
// here wants the rows and none of them has anything to do with a failure.
func subtreeOf(
	ctx context.Context, t *testing.T, tenant shared.ID, item work.WorkItem, limit int,
) []work.WorkItem {
	t.Helper()

	var below []work.WorkItem
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		below, err = itemRepo().Subtree(ctx, item, limit)
		return err
	}); err != nil {
		t.Fatalf("reading the subtree: %v", err)
	}
	return below
}
