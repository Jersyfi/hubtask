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
