// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The two completion methods against a real database (B-07). Both get a cross-tenant negative, because
// the tenant boundary is row level security underneath and the only way to know it reaches a new query is
// to try it (gate SG-3).

// childrenUnder writes n children under a parent and completes the first `completed` of them, returning
// their identifiers. The completion goes through SetCompletion rather than through the insert, which is
// what makes this a fixture for the method under test as well as for the count.
func childrenUnder(
	ctx context.Context, t *testing.T, tenant, author, collection, parent shared.ID, n, completed int,
) []shared.ID {
	t.Helper()

	items := itemRepo()
	ids := make([]shared.ID, 0, n)

	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		for index := range n {
			id := freshID(t)
			child := taskIn(tenant, author, collection, id, freshName(t), "a"+string(rune('0'+index)))
			child.Type = work.ItemWorkPackage
			child.ParentID = parent
			child.Path = work.RootPath(parent) + id.String() + work.PathSeparator
			child.Depth = 2
			if err := items.Insert(ctx, child); err != nil {
				return err
			}
			if index < completed {
				done := child.Completed(author, created.Add(time.Hour))
				if err := items.SetCompletion(ctx, done, 1); err != nil {
					return err
				}
			}
			ids = append(ids, id)
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding %d children: %v", n, err)
	}
	return ids
}

// seedTask writes one task directly in a collection and returns its identifier.
func seedTask(ctx context.Context, t *testing.T, tenant, author, collection shared.ID) shared.ID {
	t.Helper()

	id := freshID(t)
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return itemRepo().
			Insert(ctx, taskIn(tenant, author, collection, id, freshName(t), "a0"))
	}); err != nil {
		t.Fatalf("seeding the task: %v", err)
	}
	return id
}

// stampColumn sets a lifecycle column the repository does not write, and insists the row was there to
// stamp: InsertWorkItem writes neither archived_at nor deleted_at - those are use cases of their own
// (B-06, B-10) - so a fixture that set the field on the struct would be silently dropped, and a test
// asserting the absence of such a row would prove nothing.
func stampColumn(ctx context.Context, t *testing.T, id shared.ID, column string) {
	t.Helper()

	// The column name is a constant of this test and never a value from anywhere else, which is what
	// keeps CLAUDE.md rule 9 intact here, where sqlc cannot express "either of two columns".
	switch column {
	case "archived_at", "deleted_at":
	default:
		t.Fatalf("stampColumn was asked for the column %q, which it does not know", column)
	}

	tag, err := adminPool(ctx, t).Exec(ctx,
		"UPDATE work_item SET "+column+" = $2 WHERE id = $1", id.String(), created)
	if err != nil {
		t.Fatalf("stamping %s: %v", column, err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("stamping %s matched %d rows, want 1", column, tag.RowsAffected())
	}
}

func childCompletionOf(ctx context.Context, t *testing.T, tenant, parent shared.ID) work.ChildCompletion {
	t.Helper()

	var summary work.ChildCompletion
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		summary, err = itemRepo().ChildCompletion(ctx, parent)
		return err
	}); err != nil {
		t.Fatalf("counting the children: %v", err)
	}
	return summary
}

func TestTheChildSummaryCountsWhatIsOpen(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	parent := seedTask(ctx, t, tenantA, authorA, collection)

	if summary := childCompletionOf(ctx, t, tenantA, parent); summary.Total != 0 {
		t.Errorf("an item with no children counted %+v", summary)
	}

	childrenUnder(ctx, t, tenantA, authorA, collection, parent, 3, 2)

	summary := childCompletionOf(ctx, t, tenantA, parent)
	if summary.Total != 3 || summary.Completed != 2 {
		t.Errorf("three children of which two are done counted %+v", summary)
	}
	if !summary.AnyOpen() || summary.AllCompleted() {
		t.Errorf("the summary reads as any_open=%v all_completed=%v", summary.AnyOpen(), summary.AllCompleted())
	}
}

// A trashed child is not counted at all: a work package whose last activity was deleted must not become
// done because of it. An archived one is counted as it stands.
func TestTheChildSummaryIgnoresTrashedChildrenAndCountsArchivedOnes(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	parent := seedTask(ctx, t, tenantA, authorA, collection)

	children := childrenUnder(ctx, t, tenantA, authorA, collection, parent, 3, 1)
	stampColumn(ctx, t, children[1], "deleted_at")
	stampColumn(ctx, t, children[2], "archived_at")

	summary := childCompletionOf(ctx, t, tenantA, parent)
	if summary.Total != 2 {
		t.Errorf("the trashed child is still counted: %+v", summary)
	}
	if summary.Completed != 1 {
		t.Errorf("the completed count is %d, want 1", summary.Completed)
	}
}

// The whole point of the summary, at the boundary that matters: with the last open child completed, the
// numbers say all done - which is what the roll-up reads.
func TestCompletingTheLastChildMakesTheSummaryComplete(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	parent := seedTask(ctx, t, tenantA, authorA, collection)

	children := childrenUnder(ctx, t, tenantA, authorA, collection, parent, 2, 1)

	if summary := childCompletionOf(ctx, t, tenantA, parent); summary.AllCompleted() {
		t.Fatalf("one of two done already reads as all completed: %+v", summary)
	}

	last := findItem(ctx, t, tenantA, children[1])
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetCompletion(ctx, last.Completed(authorA, created.Add(2*time.Hour)), last.Version)
	}); err != nil {
		t.Fatalf("completing the last child: %v", err)
	}

	if summary := childCompletionOf(ctx, t, tenantA, parent); !summary.AllCompleted() {
		t.Errorf("both children done does not read as all completed: %+v", summary)
	}
}

func TestSetCompletionWritesBothColumnsAndBumpsTheVersion(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	id := seedTask(ctx, t, tenantA, authorA, collection)

	at := created.Add(time.Hour)
	before := findItem(ctx, t, tenantA, id)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetCompletion(ctx, before.Completed(authorA, at), before.Version)
	}); err != nil {
		t.Fatalf("completing: %v", err)
	}

	done := findItem(ctx, t, tenantA, id)
	if !done.Completion.IsCompleted {
		t.Fatal("the stored item is not completed")
	}
	if done.Completion.CompletedAt == nil || !done.Completion.CompletedAt.Equal(at) {
		t.Errorf("completed_at is %v, want %v", done.Completion.CompletedAt, at)
	}
	if done.Completion.CompletedBy != authorA {
		t.Errorf("completed_by is %s", done.Completion.CompletedBy)
	}
	if done.Version != before.Version+1 {
		t.Errorf("the version is %d, want %d", done.Version, before.Version+1)
	}

	// And back again: reopening clears both columns, which the table's own CHECK insists on.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetCompletion(ctx, done.Reopened(created.Add(2*time.Hour)), done.Version)
	}); err != nil {
		t.Fatalf("reopening: %v", err)
	}

	open := findItem(ctx, t, tenantA, id)
	if open.Completion.IsCompleted || open.Completion.CompletedAt != nil || !open.Completion.CompletedBy.IsZero() {
		t.Errorf("the reopened item still carries %+v", open.Completion)
	}
}

// The lost update this exists to prevent: two callers read version 1, and the second one to write is told
// rather than silently overwriting the first (api-guidelines.md §5).
func TestSetCompletionRefusesAStaleVersion(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	id := seedTask(ctx, t, tenantA, authorA, collection)

	item := findItem(ctx, t, tenantA, id)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetCompletion(ctx, item.Completed(authorA, created.Add(time.Hour)), item.Version)
	}); err != nil {
		t.Fatalf("the first write: %v", err)
	}

	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetCompletion(ctx, item.Completed(authorA, created.Add(2*time.Hour)), item.Version)
	})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("a stale version answered %v, want a version conflict", err)
	}
}

// Gate SG-3 for ChildCompletion: another tenant's parent counts as having no children, rather than as
// having theirs.
func TestTheChildSummaryIsInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	parent := seedTask(ctx, t, tenantA, authorA, collection)
	childrenUnder(ctx, t, tenantA, authorA, collection, parent, 2, 1)

	if summary := childCompletionOf(ctx, t, tenantB, parent); summary.Total != 0 {
		t.Errorf("tenant B counted %+v under tenant A's item", summary)
	}
}

// And for SetCompletion. A row another tenant owns is out of the update's reach, and the answer is the
// same one a moved version gives - a caller must not be able to tell the two apart (multi-tenancy.md §2).
func TestSetCompletionCannotReachAnotherTenant(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	id := seedTask(ctx, t, tenantA, authorA, collection)

	item := findItem(ctx, t, tenantA, id)
	err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return itemRepo().SetCompletion(ctx, item.Completed(authorB, created.Add(time.Hour)), item.Version)
	})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("writing across the boundary answered %v", err)
	}

	// And nothing moved.
	if after := findItem(ctx, t, tenantA, id); after.Completion.IsCompleted {
		t.Error("tenant B completed tenant A's item")
	}
}

// findItem reads an item back, failing the test rather than returning an error: every caller above wants
// the item and none of them has anything to do with a failure.
func findItem(ctx context.Context, t *testing.T, tenant, id shared.ID) work.WorkItem {
	t.Helper()

	var item work.WorkItem
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		item, err = itemRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("reading item %s: %v", id, err)
	}
	return item
}
