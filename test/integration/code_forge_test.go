// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/forge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// seedCodeEntry writes a hub, a collection and an entry of the tenant's own.
func seedCodeEntry(ctx context.Context, t *testing.T, tenant shared.ID) (hub, item shared.ID) {
	t.Helper()
	seedContainerTenants(ctx, t)
	admin := adminPool(ctx, t)
	hub, collection, item := freshID(t), freshID(t), freshID(t)
	if _, err := admin.Exec(ctx, `INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		VALUES ($1, $2, 'HUB', $3, 'a0', $4)`, hub.String(), tenant.String(), freshName(t), authorA.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		VALUES ($1, $2, 'COLLECTION', $3, $4, 'a0', $5)`,
		collection.String(), tenant.String(), hub.String(), freshName(t), authorA.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO work_item (id, tenant_id, collection_id, type, path, depth, title, order_key,
		content_language, created_by) VALUES ($1, $2, $3, 'TASK', $4, 1, 'Login', 'a0', 'en', $5)`,
		item.String(), tenant.String(), collection.String(), "/"+item.String()+"/", authorA.String()); err != nil {
		t.Fatal(err)
	}
	return hub, item
}

// A hook and its links are read as written, a delivery is seen once, and the workspace next door
// finds none of them (migration 0275, UC-INT-13).
func TestACodeLinkStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	hub, item := seedCodeEntry(ctx, t, tenantA)
	repo := postgres.NewCodeForgeRepository()
	at := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	hook := forge.Hook{ID: freshID(t), HubID: hub, Kind: forge.GitHub, Name: "web", Enabled: true,
		Salt: "0123456789abcdef0123", RunAs: authorA, CreatedAt: at, UpdatedAt: at}
	link := forge.Link{ID: freshID(t), ItemID: item, HubID: hub, HookID: hook.ID, Kind: forge.PullRequest,
		Repository: "team/web", Ref: "7", Title: "WEB-42 login", URL: "https://example.org/pr/7", Author: "ada",
		State: forge.Open, HappenedAt: at, UpdatedAt: at}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := repo.PutHook(ctx, hook); err != nil {
			return err
		}
		if err := repo.PutLink(ctx, link); err != nil {
			return err
		}
		link.State = forge.Merged
		if err := repo.PutLink(ctx, link); err != nil {
			return err
		}
		fresh, err := repo.SeeDelivery(ctx, hook.ID, []byte("d1"), at, at.Add(-time.Hour))
		if err != nil || !fresh {
			t.Errorf("first delivery fresh = %v, %v", fresh, err)
		}
		again, err := repo.SeeDelivery(ctx, hook.ID, []byte("d1"), at, at.Add(-time.Hour))
		if err != nil || again {
			t.Errorf("repeated delivery fresh = %v, %v", again, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		hooks, err := repo.ListHooks(ctx, hub)
		if err != nil || len(hooks) != 1 || hooks[0].Salt != hook.Salt || hooks[0].Kind != forge.GitHub {
			t.Errorf("hooks %+v, %v", hooks, err)
		}
		found, err := repo.FindLink(ctx, item, hook.ID, forge.PullRequest, "team/web", "7")
		if err != nil || found.State != forge.Merged || found.Title != "WEB-42 login" {
			t.Errorf("link %+v, %v", found, err)
		}
		links, err := repo.ListLinks(ctx, item, 10)
		if err != nil || len(links) != 1 {
			t.Errorf("links %+v, %v", links, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := repo.FindHook(ctx, hook.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the workspace next door read the hook: %v", err)
		}
		links, err := repo.ListLinks(ctx, item, 10)
		if err != nil || len(links) != 0 {
			t.Errorf("the workspace next door read links: %+v, %v", links, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
