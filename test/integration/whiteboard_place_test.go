// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-CNV-10 checks 1, 2 and 4: a board keeps its parent page and its key; the tree lists the boards
// by parent and place, a board without a key after the keyed ones; the siblings of a parent merge
// pages and boards in one key order; the boards under pages are found for lifting; and the
// workspace next door sees none of it (migration 0390, a cross-tenant negative per method).
func TestABoardsPlaceIsKeptAmongThePagesAndStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	hub, board, _ := seedBoard(ctx, t)
	boards := postgres.NewWhiteboardRepository()
	at := time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)
	team, loose := freshID(t), freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, page := range []knowledge.Page{
			{ID: team, HubID: hub, Title: "Team", OrderKey: "a0"},
		} {
			page.CreatedBy, page.CreatedAt, page.UpdatedBy, page.UpdatedAt, page.Version = authorA, at, authorA, at, 1
			if err := postgres.NewPageRepository().Insert(ctx, page); err != nil {
				return err
			}
		}
		current, err := boards.Find(ctx, board)
		if err != nil {
			return err
		}
		current.ParentPageID, current.OrderKey = team, "a0V"
		if err := boards.Update(ctx, current, current.Version); err != nil {
			return err
		}
		return boards.Insert(ctx, whiteboard.Board{
			ID: loose, HubID: hub, Title: "Loose", OrderKey: "a1", CreatedBy: authorA, CreatedAt: at,
			UpdatedBy: authorA, UpdatedAt: at, Version: 1,
		})
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		read, err := boards.Find(ctx, board)
		if err != nil || read.ParentPageID != team || read.OrderKey != "a0V" {
			t.Errorf("read %+v, %v; want it under Team at a0V", read, err)
		}
		tree, err := boards.Tree(ctx, hub, 10)
		if err != nil || len(tree) != 2 || tree[0].Board.ID != loose || tree[1].Board.ID != board {
			t.Errorf("tree %+v, %v; want the top-level board first", tree, err)
		}
		top, err := boards.Siblings(ctx, hub, "")
		if err != nil || len(top) != 2 || top[0].ID != team || top[0].IsBoard || top[1].ID != loose || !top[1].IsBoard {
			t.Errorf("top siblings %+v, %v; want Team then the loose board", top, err)
		}
		under, err := boards.Under(ctx, []shared.ID{team})
		if err != nil || len(under) != 1 || under[0] != board {
			t.Errorf("under Team %v, %v", under, err)
		}
		parentHub, err := boards.ParentHub(ctx, team)
		if err != nil || parentHub != hub {
			t.Errorf("Team's hub %s, %v", parentHub, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if tree, err := boards.Tree(ctx, hub, 10); err != nil || len(tree) != 0 {
			t.Errorf("the workspace next door listed %+v, %v", tree, err)
		}
		if top, err := boards.Siblings(ctx, hub, ""); err != nil || len(top) != 0 {
			t.Errorf("the workspace next door's siblings %+v, %v", top, err)
		}
		if under, err := boards.Under(ctx, []shared.ID{team}); err != nil || len(under) != 0 {
			t.Errorf("the workspace next door found %v, %v", under, err)
		}
		if _, err := boards.ParentHub(ctx, team); shared.AsError(err).Code != "not_found" {
			t.Errorf("the workspace next door read Team's hub: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// UC-CNV-10 check 5: the backfill of migration 0390 gives a board that had no place a key after
// its hub's last top-level page, A-Z by title, at the top level.
func TestTheBackfillFilesOldBoardsAfterThePagesByTitle(t *testing.T) {
	ctx := context.Background()
	hub, board, _ := seedBoard(ctx, t)
	at := time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)
	older := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := postgres.NewPageRepository().Insert(ctx, knowledge.Page{ID: freshID(t), HubID: hub, Title: "Home", OrderKey: "a3",
			CreatedBy: authorA, CreatedAt: at, UpdatedBy: authorA, UpdatedAt: at, Version: 1}); err != nil {
			return err
		}
		return postgres.NewWhiteboardRepository().Insert(ctx, whiteboard.Board{
			ID: older, HubID: hub, Title: "Archive map", CreatedBy: authorA, CreatedAt: at, UpdatedBy: authorA, UpdatedAt: at, Version: 1,
		})
	}); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../db/migrations/0390_whiteboard_place.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(migration)
	backfill := text[strings.Index(text, "WITH last_page AS"):strings.Index(text, "-- +goose Down")]
	pool := adminPool(ctx, t)
	if _, err := pool.Exec(ctx, "UPDATE whiteboard SET order_key = NULL WHERE id = ANY($1::uuid[])", []string{board.String(), older.String()}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, backfill); err != nil {
		t.Fatalf("the backfill: %v", err)
	}
	var first, second string
	if err := pool.QueryRow(ctx, "SELECT order_key FROM whiteboard WHERE id = $1", older.String()).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT order_key FROM whiteboard WHERE id = $1", board.String()).Scan(&second); err != nil {
		t.Fatal(err)
	}
	if first <= "a3" || second <= first || !strings.HasPrefix(first, "a3") {
		t.Errorf("keys %q (Archive map), %q (Retro); want both after the page a3, Archive map first", first, second)
	}
}
