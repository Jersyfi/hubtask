// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

func seedBoard(ctx context.Context, t *testing.T) (hub, board, entry shared.ID) {
	t.Helper()
	seedContainerTenants(ctx, t)
	hub, board, entry = freshID(t), freshID(t), freshID(t)
	at := time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := containerRepo().Insert(ctx, containerIn(tenantA, authorA, hub, freshName(t), "a0")); err != nil {
			return err
		}
		return postgres.NewWhiteboardRepository().Insert(ctx, whiteboard.Board{
			ID: board, HubID: hub, Title: "Retro", CreatedBy: authorA, CreatedAt: at, UpdatedBy: authorA, UpdatedAt: at, Version: 1,
			Scene: whiteboard.Scene{Elements: []whiteboard.Element{
				{ID: "s1", Kind: whiteboard.KindSticky, Width: 160, Height: 160, Text: "Keep it", Colour: shared.LabelTokenAmber},
				{ID: "c1", Kind: whiteboard.KindCard, X: 300, Width: 240, Height: 120,
					Target: reference.Reference{Kind: reference.KindEntry, ID: entry}},
				{ID: "l1", Kind: whiteboard.KindConnector, From: "s1", To: "c1", Route: whiteboard.RouteElbow,
					Start: whiteboard.HeadNone, End: whiteboard.HeadArrow},
			}},
		})
	}); err != nil {
		t.Fatal(err)
	}
	return hub, board, entry
}

// A board is read as it was written, its cards are where it points, and the trash takes them
// away (migration 0233).
func TestABoardIsReadAsItWasWritten(t *testing.T) {
	ctx := context.Background()
	hub, board, entry := seedBoard(ctx, t)
	repo := postgres.NewWhiteboardRepository()
	at := time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		read, err := repo.Find(ctx, board)
		if err != nil || len(read.Scene.Elements) != 3 || read.Scene.Elements[1].Target.ID != entry ||
			read.Scene.Elements[0].Colour != shared.LabelTokenAmber || read.Scene.Elements[2].Route != whiteboard.RouteElbow {
			t.Errorf("board %+v, %v", read, err)
		}
		list, err := repo.List(ctx, hub, false, 10)
		if err != nil || len(list) != 1 || list[0].Elements != 3 {
			t.Errorf("list %+v, %v", list, err)
		}
		pointing, err := repo.Referencing(ctx, "ENTRY", entry, 10)
		if err != nil || len(pointing) != 1 || pointing[0].ID != board || pointing[0].Strength != "CARD" {
			t.Errorf("pointing %+v, %v", pointing, err)
		}
		trashed := at
		read.DeletedAt, read.UpdatedBy, read.UpdatedAt = &trashed, authorA, at
		if err := repo.Update(ctx, read, 1); err != nil {
			t.Errorf("trash: %v", err)
		}
		if pointing, _ := repo.Referencing(ctx, "ENTRY", entry, 10); len(pointing) != 0 {
			t.Errorf("a trashed board still points at the entry: %+v", pointing)
		}
		if err := repo.Update(ctx, read, 1); err == nil {
			t.Error("a stale version wrote the board")
		}
		if n, _ := repo.Count(ctx, hub); n != 0 {
			t.Errorf("a trashed board is counted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Another workspace finds no board and changes nothing.
func TestABoardIsInvisibleToTheWorkspaceNextDoor(t *testing.T) {
	ctx := context.Background()
	hub, board, entry := seedBoard(ctx, t)
	repo := postgres.NewWhiteboardRepository()
	at := time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := repo.Find(ctx, board); err == nil {
			t.Error("the workspace next door found the board")
		}
		if _, err := repo.FindForUpdate(ctx, board); err == nil {
			t.Error("the workspace next door locked the board")
		}
		if err := repo.Update(ctx, whiteboard.Board{ID: board, Title: "x", UpdatedBy: authorB, UpdatedAt: at}, 1); err == nil {
			t.Error("the workspace next door wrote the board")
		}
		if list, _ := repo.List(ctx, hub, false, 10); len(list) != 0 {
			t.Error("the workspace next door listed the boards")
		}
		if n, _ := repo.Count(ctx, hub); n != 0 {
			t.Error("the workspace next door counted the boards")
		}
		if pointing, _ := repo.Referencing(ctx, "ENTRY", entry, 10); len(pointing) != 0 {
			t.Error("the workspace next door read where the board points")
		}
		if summaries, _ := repo.Summaries(ctx, []shared.ID{board}); len(summaries) != 0 {
			t.Error("the workspace next door resolved the board")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// A board's history keeps its versions, folds a session's into one, names one, and another
// workspace reads, folds, names and adds none (migration 0234).
func TestABoardsHistoryStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	_, board, _ := seedBoard(ctx, t)
	repo := postgres.NewWhiteboardRepository()
	at := time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)
	scene := whiteboard.Scene{Elements: []whiteboard.Element{
		{ID: "p1", Kind: whiteboard.KindStroke, Pen: whiteboard.PenPen, Width: 10, Height: 10, Points: []whiteboard.Point{{X: 1, Y: 2}, {X: 9, Y: 9}}},
		{ID: "w1", Kind: whiteboard.KindLink, Width: 240, Height: 72, URL: "https://example.org", Group: "g1"},
	}}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := repo.InsertVersion(ctx, whiteboard.Version{BoardID: board, Seq: 1, Title: "Retro", Scene: scene, AuthorID: authorA, CreatedAt: at}); err != nil {
			return err
		}
		if err := repo.FoldVersion(ctx, whiteboard.Version{BoardID: board, Seq: 1, Title: "Retro 2", Scene: scene, AuthorID: authorA, CreatedAt: at.Add(time.Minute)}); err != nil {
			return err
		}
		if err := repo.NameVersion(ctx, board, 1, "Before"); err != nil {
			return err
		}
		last, found, err := repo.LastVersion(ctx, board)
		if err != nil || !found || last.Title != "Retro 2" || last.Name != "Before" || !last.Scene.Equal(scene) {
			t.Errorf("last %+v, %v, %v", last, found, err)
		}
		rows, err := repo.Versions(ctx, board, 0, 10)
		if err != nil || len(rows) != 1 || rows[0].Elements != 2 || rows[0].AuthorID != authorA {
			t.Errorf("versions %+v, %v", rows, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if _, found, _ := repo.LastVersion(ctx, board); found {
			t.Error("the workspace next door read the board's last version")
		}
		if _, err := repo.Version(ctx, board, 1); err == nil {
			t.Error("the workspace next door read a version")
		}
		if rows, _ := repo.Versions(ctx, board, 0, 10); len(rows) != 0 {
			t.Error("the workspace next door listed the history")
		}
		if err := repo.NameVersion(ctx, board, 1, "x"); err == nil {
			t.Error("the workspace next door named a version")
		}
		if err := repo.InsertVersion(ctx, whiteboard.Version{BoardID: board, Seq: 2, Title: "x", CreatedAt: at}); err == nil {
			t.Error("the workspace next door added a version")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// A picture placed on a board is held by it and read through it, and the workspace next door
// neither places nor finds it (migration 0235, UC-CNV-08 check 4).
func TestABoardsPicturesStayInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	hub, board, _ := seedBoard(ctx, t)
	picture := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx, `INSERT INTO media_object (id, tenant_id, storage_key, mime_type, byte_size, usage, status)
		VALUES ($1, $2, $3, 'image/png', 12, 'ATTACHMENT', 'READY')`,
		picture.String(), tenantA.String(), "test/"+picture.String()); err != nil {
		t.Fatalf("the picture: %v", err)
	}
	repo := postgres.NewWhiteboardRepository()
	media := postgres.NewMediaRepository(pageCursors())
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for range 2 {
			if err := repo.AttachMedia(ctx, board, picture); err != nil {
				return err
			}
		}
		if n, err := repo.CountMedia(ctx, board); err != nil || n != 1 {
			t.Errorf("held %d, %v", n, err)
		}
		hubs, err := media.ReferencingBoards(ctx, picture)
		if err != nil || len(hubs) != 1 || hubs[0] != hub {
			t.Errorf("boards %v, %v", hubs, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if err := repo.AttachMedia(ctx, board, picture); err == nil {
			t.Error("the workspace next door placed a picture on the board")
		}
		if hubs, _ := media.ReferencingBoards(ctx, picture); len(hubs) != 0 {
			t.Error("the workspace next door found the board holding the picture")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// A board's comments are read as they were written, pinned where they were put, and the workspace
// next door neither finds nor writes them (migration 0266, UC-CNV-09).
func TestABoardsCommentsStayInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	_, board, _ := seedBoard(ctx, t)
	repo := postgres.NewWhiteboardRepository()
	at := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	thread := whiteboard.Comment{
		ID: freshID(t), BoardID: board, AuthorID: authorA, Body: "Why here?", Element: "s1", X: 12.5, Y: -4,
		CreatedAt: at, Version: 1,
	}
	reply := whiteboard.Comment{ID: freshID(t), BoardID: board, ParentID: thread.ID, AuthorID: authorA, Body: "Because", CreatedAt: at.Add(time.Minute), Version: 1}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := repo.InsertComment(ctx, thread); err != nil {
			return err
		}
		if err := repo.InsertComment(ctx, reply); err != nil {
			return err
		}
		resolved := thread
		resolved.ResolvedAt, resolved.ResolvedBy = &at, authorA
		if err := repo.UpdateComment(ctx, resolved, 1); err != nil {
			return err
		}
		if err := repo.UpdateComment(ctx, resolved, 1); err == nil {
			t.Error("a stale version was written")
		}
		comments, err := repo.Comments(ctx, board, 10)
		if err != nil || len(comments) != 2 {
			t.Fatalf("comments %v, %v", comments, err)
		}
		got := comments[0]
		if got.Element != "s1" || got.X != 12.5 || got.Y != -4 || got.ResolvedAt == nil || got.Version != 2 || comments[1].ParentID != thread.ID {
			t.Errorf("read back %+v", comments)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := repo.FindComment(ctx, thread.ID); err == nil {
			t.Error("the workspace next door found the comment")
		}
		if comments, _ := repo.Comments(ctx, board, 10); len(comments) != 0 {
			t.Error("the workspace next door listed the board's comments")
		}
		if err := repo.UpdateComment(ctx, thread, 2); err == nil {
			t.Error("the workspace next door changed the comment")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// The initial sync's board walk (offline reading, UC-CNV-01 check 9) answers a workspace's own
// boards with their scenes, and none of the workspace next door's.
func TestTheBoardWalkStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	_, board, _ := seedBoard(ctx, t)
	snapshot := postgres.NewSnapshotRepository()
	seen := func(tenant shared.ID) map[shared.ID]int {
		found := map[shared.ID]int{}
		if err := write(ctx, t, tenant, func(ctx context.Context) error {
			boards, err := snapshot.Whiteboards(ctx, "", 5000)
			for _, each := range boards {
				found[each.ID] = len(each.Scene.Elements)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return found
	}
	if own := seen(tenantA); own[board] != 3 {
		t.Errorf("the workspace's walk holds %v, want the board with its three objects", own)
	}
	if _, found := seen(tenantB)[board]; found {
		t.Error("the workspace next door walked the board")
	}
}
