// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
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

// The initial sync's page walk (offline reading) answers a workspace's own living pages with their
// bodies, and none of the workspace next door's.
func TestThePageWalkStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	_, root, child := seedSpace(ctx, t)
	snapshot := postgres.NewSnapshotRepository()
	seen := func(tenant shared.ID) map[shared.ID]string {
		found := map[shared.ID]string{}
		if err := write(ctx, t, tenant, func(ctx context.Context) error {
			pages, err := snapshot.Pages(ctx, "", 5000)
			for _, page := range pages {
				found[page.ID] = page.Body
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return found
	}
	own := seen(tenantA)
	if own[child] != "Day one" || own[root] == "" {
		t.Errorf("the workspace's walk holds %v, want both pages with their bodies", own)
	}
	next := seen(tenantB)
	if _, found := next[root]; found {
		t.Errorf("the workspace next door walked the page: %v", next)
	}
	if _, found := next[child]; found {
		t.Errorf("the workspace next door walked the page: %v", next)
	}
}

// A restriction narrows down the tree and keeps the page off the walk; a share and a personal
// space are read back; the workspace next door sees none of them (migration 0261).
func TestAPagesAccessIsReadAsWrittenAndStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	hub, root, child := seedSpace(ctx, t)
	repo := postgres.NewPageRepository()
	at := time.Date(2026, 10, 29, 11, 0, 0, 0, time.UTC)
	day := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		page, err := repo.Find(ctx, root)
		if err != nil {
			return err
		}
		page.Restricted, page.UpdatedBy, page.UpdatedAt = true, authorA, at
		if err := repo.Restrict(ctx, page, []shared.ID{authorA}, page.Version); err != nil {
			return err
		}
		readers, err := repo.Readers(ctx, []shared.ID{root, child})
		if err != nil || !readers[child].Restricted || !readers[child].Admits(authorA) || readers[child].Admits(authorB) {
			t.Errorf("readers %+v, %v", readers, err)
		}
		if walked, _ := postgres.NewSnapshotRepository().Pages(ctx, "", 100); len(walked) != 0 {
			t.Errorf("the walk carried a restricted page: %+v", walked)
		}
		if added, err := repo.InsertShare(ctx, knowledge.Share{PageID: child, AccountID: authorA, GrantedBy: authorA, GrantedAt: at}); err != nil || !added {
			t.Errorf("share %v, %v", added, err)
		}
		if held, _ := repo.SharedWith(ctx, child, authorA); !held {
			t.Error("the share was not read back")
		}
		if claimed, err := repo.ClaimPersonalSpace(ctx, authorA, hub, at); err != nil || claimed != hub {
			t.Errorf("claimed %v, %v", claimed, err)
		}
		note := knowledge.Page{ID: freshID(t), HubID: hub, Title: "2026-10-30", OrderKey: "b0", DailyFor: &day,
			CreatedBy: authorA, CreatedAt: at, UpdatedBy: authorA, UpdatedAt: at, Version: 1}
		if err := repo.Insert(ctx, note); err != nil {
			return err
		}
		if id, found, err := repo.DailyNote(ctx, hub, day); err != nil || !found || id != note.ID {
			t.Errorf("daily note %v %v, %v", id, found, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if readers, _ := repo.Readers(ctx, []shared.ID{child}); readers[child].Restricted {
			t.Error("the workspace next door read the restriction")
		}
		if to, _ := repo.RestrictedTo(ctx, root); len(to) != 0 {
			t.Error("the workspace next door read who the page is restricted to")
		}
		if held, _ := repo.SharedWith(ctx, child, authorA); held {
			t.Error("the workspace next door read the share")
		}
		if shares, _ := repo.Shares(ctx, child, 10); len(shares) != 0 {
			t.Error("the workspace next door listed the shares")
		}
		if _, found, _ := repo.PersonalSpace(ctx, authorA); found {
			t.Error("the workspace next door read the personal space")
		}
		if _, found, _ := repo.DailyNote(ctx, hub, day); found {
			t.Error("the workspace next door read the daily note")
		}
		if removed, _ := repo.DeleteShare(ctx, child, authorA); removed {
			t.Error("the workspace next door removed the share")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// UC-DOC-08 check 4: a public link is found by its token in its own workspace alone, a new one
// replaces the old, and taking it away leaves nothing to find (migration 0262).
func TestAPagesPublicLinkIsFoundByItsTokenInItsWorkspaceAlone(t *testing.T) {
	ctx := context.Background()
	_, root, _ := seedSpace(ctx, t)
	links := postgres.NewPagePublicLinkRepository(security.NewPageLinkHasher(secret.New(feedSecret)))
	at := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	mint := func(tenant shared.ID, fill byte) integration.PageLinkToken {
		token, err := integration.NewPageLinkToken(tenant, bytes.Repeat([]byte{fill}, integration.PageLinkTokenSecretBytes))
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	first, second := mint(tenantA, 1), mint(tenantA, 2)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		link := knowledge.PublicLink{PageID: root, CreatedBy: authorA, CreatedAt: at}
		if err := links.Put(ctx, link, first); err != nil {
			return err
		}
		if found, err := links.FindByToken(ctx, first); err != nil || found.PageID != root || found.CreatedBy != authorA {
			t.Errorf("by token %+v, %v", found, err)
		}
		if err := links.Put(ctx, link, second); err != nil {
			return err
		}
		if _, err := links.FindByToken(ctx, first); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the replaced token still answers: %v", err)
		}
		if _, found, err := links.Find(ctx, root); err != nil || !found {
			t.Errorf("find %v, %v", found, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := links.FindByToken(ctx, second); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the workspace next door found the link: %v", err)
		}
		if removed, _ := links.Delete(ctx, root); removed {
			t.Error("the workspace next door removed the link")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if removed, err := links.Delete(ctx, root); err != nil || !removed {
			t.Errorf("delete %v, %v", removed, err)
		}
		if _, err := links.FindByToken(ctx, second); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("a removed link answers: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// UC-DOC-13: an article is published to one service, found by its words there, counted and
// rated, and read in its own workspace alone (migration 0264).
func TestAnArticleIsFoundInItsServiceAndStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	_, root, child := seedSpace(ctx, t)
	collection := collectionFor(ctx, t, tenantA, authorA)
	articles := postgres.NewPageArticleRepository()
	at := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, page := range []shared.ID{root, child} {
			if added, err := articles.Publish(ctx, knowledge.Article{PageID: page, CollectionID: collection, PublishedBy: authorA, PublishedAt: at}); err != nil || !added {
				t.Errorf("publish %v, %v", added, err)
			}
		}
		hits, err := articles.Search(ctx, collection, "budget", 10)
		if err != nil || len(hits) != 1 || hits[0].PageID != root {
			t.Errorf("search %+v, %v", hits, err)
		}
		if err := articles.CountView(ctx, child, collection); err != nil {
			return err
		}
		if err := articles.Rate(ctx, child, collection, authorA, true, at); err != nil {
			return err
		}
		if err := articles.Rate(ctx, child, collection, authorA, false, at); err != nil {
			return err
		}
		of, err := articles.Of(ctx, child)
		if err != nil || len(of) != 1 || of[0].Views != 1 || of[0].Helpful != 0 || of[0].Unhelpful != 1 {
			t.Errorf("of %+v, %v", of, err)
		}
		if top, _ := articles.Search(ctx, collection, "", 10); len(top) != 2 || top[0].PageID != child {
			t.Errorf("the most read does not come first: %+v", top)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if hits, _ := articles.Search(ctx, collection, "", 10); len(hits) != 0 {
			t.Error("the workspace next door found the articles")
		}
		if held, _ := articles.Published(ctx, root, collection); held {
			t.Error("the workspace next door read the publication")
		}
		if removed, _ := articles.Unpublish(ctx, root, collection); removed {
			t.Error("the workspace next door unpublished the article")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// A picture a page holds is counted once and found through the page, a trashed page serves it no
// more, and the workspace next door neither attaches nor finds it (migration 0265, UC-DOC-14
// check 4).
func TestAPagesPicturesStayInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	_, root, _ := seedSpace(ctx, t)
	picture := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx, `INSERT INTO media_object (id, tenant_id, storage_key, mime_type, byte_size, usage, status)
		VALUES ($1, $2, $3, 'image/png', 12, 'ATTACHMENT', 'READY')`,
		picture.String(), tenantA.String(), "test/"+picture.String()); err != nil {
		t.Fatalf("the picture: %v", err)
	}
	repo := postgres.NewPageRepository()
	media := postgres.NewMediaRepository(pageCursors())
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for range 2 {
			if err := repo.AttachMedia(ctx, root, picture); err != nil {
				return err
			}
		}
		if n, err := repo.CountMedia(ctx, root); err != nil || n != 1 {
			t.Errorf("held %d, %v", n, err)
		}
		pages, err := media.ReferencingPages(ctx, picture)
		if err != nil || len(pages) != 1 || pages[0] != root {
			t.Errorf("pages %v, %v", pages, err)
		}
		if _, err := repo.Trash(ctx, root, authorA, time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)); err != nil {
			return err
		}
		if pages, _ := media.ReferencingPages(ctx, picture); len(pages) != 0 {
			t.Errorf("a trashed page served the picture: %v", pages)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// The refused attach aborts its transaction, so it is the transaction's answer.
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return repo.AttachMedia(ctx, root, picture)
	}); err == nil {
		t.Error("the workspace next door attached a picture to the page")
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if pages, _ := media.ReferencingPages(ctx, picture); len(pages) != 0 {
			t.Error("the workspace next door found the page holding the picture")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
