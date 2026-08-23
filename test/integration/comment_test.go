// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The discussion beside the entries, against a real database (C-03): the thread pages oldest
// first, the tombstone clears the text, and a cross-tenant negative for every method (gate SG-3).

func commentRepo() postgres.CommentRepository {
	return postgres.NewCommentRepository(pageCursors())
}

// seedComment writes one comment on the task and returns it as stored.
func seedComment(
	ctx context.Context, t *testing.T, tenant, item, author shared.ID, body string, at time.Time,
) work.Comment {
	t.Helper()

	comment := work.Comment{
		ID: freshID(t), TenantID: tenant, ItemID: item, AuthorID: author,
		Body: body, CreatedAt: at, Version: 1,
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return commentRepo().Insert(ctx, comment)
	}); err != nil {
		t.Fatalf("writing the comment: %v", err)
	}
	return comment
}

func findComment(ctx context.Context, t *testing.T, tenant, id shared.ID) work.Comment {
	t.Helper()

	var stored work.Comment
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		stored, err = commentRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("reading the comment: %v", err)
	}
	return stored
}

func TestACommentIsWrittenAndReadBack(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)

	// The diaeresis arrives decomposed (u + combining mark) and must be stored composed: the
	// database normalises to NFC, so two spellings of one word compare equal (I-W7).
	written := seedComment(ctx, t, tenantA, task, authorA, "Für Anna", created)

	stored := findComment(ctx, t, tenantA, written.ID)
	if stored.Body != "Für Anna" {
		t.Errorf("body %q, want the NFC-composed form", stored.Body)
	}
	if stored.ItemID != task || stored.AuthorID != authorA || stored.Version != 1 ||
		stored.TenantID != tenantA {
		t.Errorf("stored %+v", stored)
	}
	if stored.EditedAt != nil || stored.DeletedAt != nil {
		t.Error("a fresh comment carries stamps it has not earned")
	}
}

func TestTheThreadPagesOldestFirst(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)

	first := seedComment(ctx, t, tenantA, task, authorA, "First", created)
	second := seedComment(ctx, t, tenantA, task, authorA, "Second", created.Add(time.Minute))
	third := seedComment(ctx, t, tenantA, task, authorA, "Third", created.Add(2*time.Minute))

	var page repository.CommentPage
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		page, err = commentRepo().List(ctx, task, repository.Page{Size: 2})
		return err
	}); err != nil {
		t.Fatalf("reading the first page: %v", err)
	}
	if len(page.Comments) != 2 || page.Comments[0].ID != first.ID ||
		page.Comments[1].ID != second.ID {
		t.Fatalf("first page %+v, want the two oldest in order", page.Comments)
	}
	if !page.Info.HasMore || page.Info.NextCursor == "" {
		t.Fatalf("page info %+v, want a boundary to continue after", page.Info)
	}

	var rest repository.CommentPage
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		rest, err = commentRepo().List(ctx, task,
			repository.Page{Cursor: page.Info.NextCursor, Size: 2})
		return err
	}); err != nil {
		t.Fatalf("reading the second page: %v", err)
	}
	if len(rest.Comments) != 1 || rest.Comments[0].ID != third.ID || rest.Info.HasMore {
		t.Fatalf("second page %+v", rest.Comments)
	}
}

func TestAnEditRewritesUnderTheLock(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	comment := seedComment(ctx, t, tenantA, task, authorA, "Frist", created)

	edited, err := comment.Edited("First", changedAt)
	if err != nil {
		t.Fatal(err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return commentRepo().SetBody(ctx, edited, 7)
	}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale version was accepted: %v", err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return commentRepo().SetBody(ctx, edited, comment.Version)
	}); err != nil {
		t.Fatalf("editing: %v", err)
	}

	stored := findComment(ctx, t, tenantA, comment.ID)
	if stored.Body != "First" || stored.EditedAt == nil || stored.Version != 2 {
		t.Errorf("stored %+v, want the rewrite, its stamp and a spent version", stored)
	}
}

func TestTheTombstoneClearsTheText(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	comment := seedComment(ctx, t, tenantA, task, authorA, "Take this down", created)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return commentRepo().SetDeleted(ctx, comment.Removed(changedAt), comment.Version)
	}); err != nil {
		t.Fatalf("deleting: %v", err)
	}

	stored := findComment(ctx, t, tenantA, comment.ID)
	if stored.Body != "" || stored.DeletedAt == nil {
		t.Fatalf("stored %+v, want an empty body and the stamp", stored)
	}
	if stored.CreatedAt.IsZero() || stored.AuthorID != authorA {
		t.Error("the tombstone lost its identity")
	}

	// And the text cannot be written back: the statement never matches a tombstone.
	revived, err := work.Comment{
		ID: comment.ID, TenantID: tenantA, ItemID: task, AuthorID: authorA,
		Body: "I take it back", CreatedAt: created, Version: stored.Version,
	}.Edited("I take it back", changedAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return commentRepo().SetBody(ctx, revived, stored.Version)
	}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a tombstone accepted an edit: %v", err)
	}
}

func TestCommentsAreInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	comment := seedComment(ctx, t, tenantA, task, authorA, "Ours alone", created)

	t.Run("find", func(t *testing.T) {
		err := read(ctx, t, tenantB, func(ctx context.Context) error {
			_, err := commentRepo().Find(ctx, comment.ID)
			return err
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("tenant B read tenant A's comment: %v", err)
		}
	})

	t.Run("list", func(t *testing.T) {
		var page repository.CommentPage
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			page, err = commentRepo().List(ctx, task, repository.Page{Size: 10})
			return err
		}); err != nil {
			t.Fatalf("listing: %v", err)
		}
		if len(page.Comments) != 0 {
			t.Errorf("tenant B read tenant A's thread: %+v", page.Comments)
		}
	})

	t.Run("insert", func(t *testing.T) {
		foreign := work.Comment{
			ID: freshID(t), TenantID: tenantB, ItemID: task, AuthorID: authorB,
			Body: "Trespassing", CreatedAt: created, Version: 1,
		}
		err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return commentRepo().Insert(ctx, foreign)
		})
		// The tenant-scoped foreign key: tenant B holds no row (tenant_b, item) to reference, so
		// the insert cannot land on tenant A's entry.
		if err == nil {
			t.Fatal("tenant B commented on tenant A's entry")
		}
	})

	t.Run("set body", func(t *testing.T) {
		edited, err := comment.Edited("Defaced", changedAt)
		if err != nil {
			t.Fatal(err)
		}
		err = write(ctx, t, tenantB, func(ctx context.Context) error {
			return commentRepo().SetBody(ctx, edited, comment.Version)
		})
		if !errors.Is(err, shared.ErrVersionConflict) {
			t.Fatalf("error = %v, want the same answer as a row that moved on", err)
		}
		if stored := findComment(ctx, t, tenantA, comment.ID); stored.Body != "Ours alone" {
			t.Errorf("tenant B rewrote tenant A's comment to %q", stored.Body)
		}
	})

	t.Run("set deleted", func(t *testing.T) {
		err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return commentRepo().SetDeleted(ctx, comment.Removed(changedAt), comment.Version)
		})
		if !errors.Is(err, shared.ErrVersionConflict) {
			t.Fatalf("error = %v, want the same answer as a row that moved on", err)
		}
		if stored := findComment(ctx, t, tenantA, comment.ID); stored.DeletedAt != nil {
			t.Error("tenant B deleted tenant A's comment")
		}
	})
}
