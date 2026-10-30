// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/filelink"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A file link is read as written, an address is named once per entry, and the workspace next door
// finds none (migration 0276, UC-INT-14).
func TestAFileLinkStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	_, item := seedCodeEntry(ctx, t, tenantA)
	repo := postgres.NewFileLinkRepository()
	at := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	link := filelink.Link{ID: freshID(t), ItemID: item, Provider: filelink.Nextcloud,
		Address: "https://cloud.example.org/s/abc", Name: "Login flow.pdf", AddedBy: authorA, AddedAt: at}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		added, err := repo.Add(ctx, link)
		if err != nil || !added {
			t.Errorf("first add = %v, %v", added, err)
		}
		again := link
		again.ID = freshID(t)
		added, err = repo.Add(ctx, again)
		if err != nil || added {
			t.Errorf("the same address again = %v, %v", added, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := repo.Find(ctx, link.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the workspace next door read the link: %v", err)
		}
		links, err := repo.List(ctx, item)
		if err != nil || len(links) != 0 {
			t.Errorf("the workspace next door read links: %+v, %v", links, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		links, err := repo.List(ctx, item)
		if err != nil || len(links) != 1 || links[0].Name != "Login flow.pdf" || links[0].Provider != filelink.Nextcloud {
			t.Errorf("links %+v, %v", links, err)
		}
		n, err := repo.Count(ctx, item)
		if err != nil || n != 1 {
			t.Errorf("count %d, %v", n, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := repo.Remove(ctx, link.ID); err != nil {
			t.Errorf("remove: %v", err)
		}
		if err := repo.Remove(ctx, link.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("removed twice: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
