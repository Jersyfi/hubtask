// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/search"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-WRK-18 check 9: the files and comments on pages and boards are found by their words, in their
// hub, on the page or board they are on; a comment taken back, and what is on a trashed page or
// board, are not; the workspace next door finds none of them.
func TestWhatIsOnPagesAndBoardsIsFoundInItsWorkspaceAlone(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	word := strings.ToLower("zo" + freshName(t)[:6])
	hub, _ := seedEveryKind(ctx, t, tenantA, strings.ToLower("zs"+freshName(t)[:6]))
	page, trashedPage, board, trashedBoard := freshID(t), freshID(t), freshID(t), freshID(t)
	tenant, author := tenantA.String(), authorA.String()
	pageSQL := `INSERT INTO page (id, tenant_id, hub_id, title, order_key, created_by, created_at, updated_by, updated_at, deleted_at)
	            VALUES ($1, $2, $3, $4, $5, $6, now(), $6, now(), $7)`
	seedSQL(ctx, t, pageSQL, page.String(), tenant, hub.String(), "Travel policy", "a", author, nil)
	seedSQL(ctx, t, pageSQL, trashedPage.String(), tenant, hub.String(), "Old policy", "b", author, "2026-11-01T00:00:00Z")
	boardSQL := `INSERT INTO whiteboard (id, tenant_id, hub_id, title, created_by, created_at, updated_by, updated_at, deleted_at)
	             VALUES ($1, $2, $3, $4, $5, now(), $5, now(), $6)`
	seedSQL(ctx, t, boardSQL, board.String(), tenant, hub.String(), "Launch plan", author, nil)
	seedSQL(ctx, t, boardSQL, trashedBoard.String(), tenant, hub.String(), "Old plan", author, "2026-11-01T00:00:00Z")
	media := func(on string, link string) {
		id := freshID(t)
		seedSQL(ctx, t, `INSERT INTO media_object (id, tenant_id, storage_key, mime_type, byte_size, usage, created_by, file_name, status)
		  VALUES ($1, $2, $3, 'application/pdf', 10, 'ATTACHMENT', $4, $5, 'READY')`, id.String(), tenant, "key-"+id.String(), author, word+"-sheet.pdf")
		seedSQL(ctx, t, link, tenant, on, id.String())
	}
	media(page.String(), `INSERT INTO page_media (tenant_id, page_id, media_id) VALUES ($1, $2, $3)`)
	media(trashedPage.String(), `INSERT INTO page_media (tenant_id, page_id, media_id) VALUES ($1, $2, $3)`)
	media(board.String(), `INSERT INTO whiteboard_media (tenant_id, whiteboard_id, media_id) VALUES ($1, $2, $3)`)
	media(trashedBoard.String(), `INSERT INTO whiteboard_media (tenant_id, whiteboard_id, media_id) VALUES ($1, $2, $3)`)
	comment := func(table, column, on string, deleted any) {
		seedSQL(ctx, t, `INSERT INTO `+table+` (id, tenant_id, `+column+`, author_id, body, created_at, deleted_at)
		  VALUES ($1, $2, $3, $4, $5, now(), $6)`, freshID(t).String(), tenant, on, author, "see the "+word+" notes", deleted)
	}
	comment("page_comment", "page_id", page.String(), nil)
	comment("page_comment", "page_id", page.String(), "2026-11-01T00:00:00Z")
	comment("page_comment", "page_id", trashedPage.String(), nil)
	comment("whiteboard_comment", "whiteboard_id", board.String(), nil)
	comment("whiteboard_comment", "whiteboard_id", board.String(), "2026-11-01T00:00:00Z")
	comment("whiteboard_comment", "whiteboard_id", trashedBoard.String(), nil)

	repo := postgres.NewSearchNamesRepository()
	kinds := []struct {
		name  string
		find  func(context.Context, repository.Ask) ([]repository.Row, error)
		on    string
		title string
	}{
		{"page files", repo.PageFiles, page.String(), word + "-sheet.pdf"},
		{"page comments", repo.PageComments, page.String(), "Travel policy"},
		{"board files", repo.WhiteboardFiles, board.String(), word + "-sheet.pdf"},
		{"board comments", repo.WhiteboardComments, board.String(), "Launch plan"},
	}
	ask := repository.Ask{Terms: []string{word}, Batch: 10}
	for _, k := range kinds {
		t.Run(k.name, func(t *testing.T) {
			if err := read(ctx, t, tenantA, func(ctx context.Context) error {
				rows, err := k.find(ctx, ask)
				if err != nil {
					return err
				}
				if len(rows) != 1 || rows[0].OnID.String() != k.on || rows[0].HubID != hub || rows[0].Title != k.title {
					t.Fatalf("found %+v, want one on %s", rows, k.on)
				}
				if inHub, err := k.find(ctx, repository.Ask{Terms: ask.Terms, HubID: hub, Batch: 10}); err != nil || len(inHub) != 1 {
					t.Errorf("narrowed to its hub it found %+v, %v", inHub, err)
				}
				if both, err := k.find(ctx, repository.Ask{Terms: []string{word, "nowhere-held"}, Batch: 10}); err != nil || len(both) != 0 {
					t.Errorf("a word not held still found %+v, %v", both, err)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if err := read(ctx, t, tenantB, func(ctx context.Context) error {
				rows, err := k.find(ctx, ask)
				if err == nil && len(rows) != 0 {
					t.Errorf("the workspace next door found %+v", rows)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
