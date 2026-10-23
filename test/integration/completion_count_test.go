// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The completion count as the row keeps it (UC-WRK-25 check 4, migration 0189): the completing
// statement counts a move from open to done and nothing else, whatever the caller's copy said.

func setCompletion(ctx context.Context, t *testing.T, id work.WorkItem, done bool) {
	t.Helper()
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		current, err := itemRepo().Find(ctx, id.ID)
		if err != nil {
			return err
		}
		next := current.Reopened(created)
		if done {
			next = current
			next.Completion = work.Completion{IsCompleted: true, CompletedAt: &created, CompletedBy: authorA}
		}
		return itemRepo().SetCompletion(ctx, next, current.Version)
	}); err != nil {
		t.Fatalf("setting the completion to %v: %v", done, err)
	}
}

func TestTheRowCountsCompletionsAndNothingElse(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	task := taskIn(tenantA, authorA, collection, freshID(t), "milk", "a0")
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, task)
	}); err != nil {
		t.Fatal(err)
	}
	if got := findItem(ctx, t, tenantA, task.ID).CompletionCount; got != 0 {
		t.Fatalf("a new entry counts %d", got)
	}

	setCompletion(ctx, t, task, true)
	setCompletion(ctx, t, task, true) // a repeat: done stays done
	if got := findItem(ctx, t, tenantA, task.ID).CompletionCount; got != 1 {
		t.Errorf("after a completion and a repeat the row counts %d, want 1", got)
	}
	setCompletion(ctx, t, task, false)
	if got := findItem(ctx, t, tenantA, task.ID).CompletionCount; got != 1 {
		t.Errorf("a reopen changed the count to %d", got)
	}
	setCompletion(ctx, t, task, true)
	if got := findItem(ctx, t, tenantA, task.ID).CompletionCount; got != 2 {
		t.Errorf("after the second completion the row counts %d, want 2", got)
	}

	// A copy was never completed: its count starts at 0 like its completion, its amount comes along.
	copied := findItem(ctx, t, tenantA, task.ID)
	copied.ID = freshID(t)
	copied.Path = work.RootPath(copied.ID)
	copied.OrderKey = "a1"
	copied.Amount = "2"
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().InsertCopy(ctx, repository.Copy{Item: copied})
	}); err != nil {
		t.Fatalf("copying: %v", err)
	}
	if stored := findItem(ctx, t, tenantA, copied.ID); stored.CompletionCount != 0 || stored.Amount != "2" {
		t.Errorf("the copy counts %d and carries amount %q", stored.CompletionCount, stored.Amount)
	}

	// An archive written before migration 0189 carries no count, and restores to 0.
	row := exportedShape(ctx, t, tenantA, task.ID)
	if row["completion_count"] != float64(2) {
		t.Fatalf("the export carries count %#v", row["completion_count"])
	}
	delete(row, "completion_count")
	if _, err := adminPool(ctx, t).Exec(ctx,
		`DELETE FROM work_item WHERE tenant_id = $1 AND id = $2`, tenantA.String(), task.ID.String()); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := importRepo().Write(ctx, "work_item", row, false)
		return err
	}); err != nil {
		t.Fatalf("restoring the older row: %v", err)
	}
	if got := findItem(ctx, t, tenantA, task.ID).CompletionCount; got != 0 {
		t.Errorf("an archive without the count restored %d", got)
	}
}
