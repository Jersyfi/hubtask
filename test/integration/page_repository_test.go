// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

func seedSpace(ctx context.Context, t *testing.T) (hub, root, child shared.ID) {
	t.Helper()
	seedContainerTenants(ctx, t)
	hub, root, child = freshID(t), freshID(t), freshID(t)
	repo := postgres.NewPageRepository()
	at := time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := containerRepo().Insert(ctx, containerIn(tenantA, authorA, hub, freshName(t), "a0")); err != nil {
			return err
		}
		for _, page := range []knowledge.Page{
			{ID: root, HubID: hub, Title: "Handbook", Body: "Our budget plan, see [x](hubtask:page/" + child.String() + ")",
				OrderKey: "a0", CreatedBy: authorA, CreatedAt: at, UpdatedBy: authorA, UpdatedAt: at, Version: 1},
			{ID: child, HubID: hub, ParentID: root, Title: "Onboarding", Body: "Day one",
				OrderKey: "a0", CreatedBy: authorA, CreatedAt: at, UpdatedBy: authorA, UpdatedAt: at, Version: 1},
		} {
			if err := repo.Insert(ctx, page); err != nil {
				return err
			}
		}
		return repo.InsertVersion(ctx, knowledge.Version{PageID: root, Seq: 1, Title: "Handbook", Body: "x", AuthorID: authorA, CreatedAt: at})
	}); err != nil {
		t.Fatal(err)
	}
	return hub, root, child
}

// A space's tree, a page's ancestors and subtree, search, backlinks, trash and restore read what
// was written (migration 0231).
func TestASpaceIsReadAsItWasWritten(t *testing.T) {
	ctx := context.Background()
	hub, root, child := seedSpace(ctx, t)
	repo := postgres.NewPageRepository()
	at := time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		tree, err := repo.List(ctx, hub, false, false, 100)
		if err != nil || len(tree) != 2 || tree[0].ID != root || tree[0].Body != "" {
			t.Errorf("tree %+v, %v", tree, err)
		}
		ancestors, err := repo.Ancestors(ctx, child)
		if err != nil || len(ancestors) != 1 || ancestors[0].ID != root {
			t.Errorf("ancestors %+v, %v", ancestors, err)
		}
		hits, err := repo.Search(ctx, "budget", []shared.ID{hub}, 10)
		if err != nil || len(hits) != 1 || hits[0].ID != root {
			t.Errorf("hits %+v, %v", hits, err)
		}
		backlinks, err := repo.Backlinks(ctx, "PAGE", child, 10)
		if err != nil || len(backlinks) != 1 || backlinks[0].SourceID != root || backlinks[0].HubID != hub {
			t.Errorf("backlinks %+v, %v", backlinks, err)
		}
		if n, err := repo.Trash(ctx, root, authorA, at); err != nil || n != 2 {
			t.Errorf("trashed %d, %v; want the page and the one under it", n, err)
		}
		if tree, _ := repo.List(ctx, hub, true, false, 100); len(tree) != 0 {
			t.Errorf("a trashed page stayed in the tree: %+v", tree)
		}
		if n, err := repo.Restore(ctx, root, authorA, at, at.Add(time.Minute)); err != nil || n != 2 {
			t.Errorf("restored %d, %v", n, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Another workspace finds no page, no version, no comment and no hit, and changes nothing.
func TestAPageIsInvisibleToTheWorkspaceNextDoor(t *testing.T) {
	ctx := context.Background()
	hub, root, child := seedSpace(ctx, t)
	repo := postgres.NewPageRepository()
	at := time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := repo.Find(ctx, root); err == nil {
			t.Error("the workspace next door found the page")
		}
		if _, err := repo.FindForUpdate(ctx, root); err == nil {
			t.Error("the workspace next door locked the page")
		}
		if err := repo.Update(ctx, knowledge.Page{ID: root, Title: "x", OrderKey: "a0", UpdatedBy: authorB, UpdatedAt: at}, 1); err == nil {
			t.Error("the workspace next door wrote the page")
		}
		if tree, _ := repo.List(ctx, hub, true, false, 100); len(tree) != 0 {
			t.Error("the workspace next door read the tree")
		}
		if n, _ := repo.Count(ctx, hub); n != 0 {
			t.Error("the workspace next door counted the pages")
		}
		if trashed, _ := repo.Trashed(ctx, hub, 10); len(trashed) != 0 {
			t.Error("the workspace next door read the trash")
		}
		if up, _ := repo.Ancestors(ctx, child); len(up) != 0 {
			t.Error("the workspace next door read the ancestors")
		}
		if down, _ := repo.Subtree(ctx, root, true, 10); len(down) != 0 {
			t.Error("the workspace next door read the subtree")
		}
		if key, _ := repo.LastOrderKey(ctx, hub, ""); key != "" {
			t.Error("the workspace next door read the order")
		}
		if n, _ := repo.Trash(ctx, root, authorB, at); n != 0 {
			t.Error("the workspace next door trashed the page")
		}
		if n, _ := repo.Restore(ctx, root, authorB, at, at); n != 0 {
			t.Error("the workspace next door restored the page")
		}
		if hits, _ := repo.Search(ctx, "budget", []shared.ID{hub}, 10); len(hits) != 0 {
			t.Error("the workspace next door searched the page")
		}
		if summaries, _ := repo.Summaries(ctx, []shared.ID{root}); len(summaries) != 0 {
			t.Error("the workspace next door resolved the page")
		}
		if backlinks, _ := repo.Backlinks(ctx, "PAGE", child, 10); len(backlinks) != 0 {
			t.Error("the workspace next door read the backlinks")
		}
		if _, found, _ := repo.LastVersion(ctx, root); found {
			t.Error("the workspace next door read the history")
		}
		if versions, _ := repo.Versions(ctx, root, 0, 10); len(versions) != 0 {
			t.Error("the workspace next door listed the history")
		}
		if _, err := repo.Version(ctx, root, 1); err == nil {
			t.Error("the workspace next door read a version")
		}
		if err := repo.NameVersion(ctx, root, 1, "x"); err == nil {
			t.Error("the workspace next door named a version")
		}
		if comments, _ := repo.Comments(ctx, root, 10); len(comments) != 0 {
			t.Error("the workspace next door read the comments")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// A comment on another workspace's page meets no page to belong to.
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return repo.InsertComment(ctx, knowledge.Comment{ID: freshID(t), PageID: root, AuthorID: authorB, Body: "x", CreatedAt: at})
	}); err == nil {
		t.Error("the workspace next door commented on the page")
	}
}
