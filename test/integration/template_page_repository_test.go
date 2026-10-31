// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// configuration.md §6: a page a template brought to a hub is marked once per key, listed while it
// is out of the trash, and invisible to the workspace next door (migration 0309).
func TestATemplatePageIsMarkedOnceAndStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	admin := adminPool(ctx, t)
	hub, guide, other := freshID(t), freshID(t), freshID(t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		VALUES ($1, $2, 'HUB', $3, 'a0', $4)`, hub.String(), tenantA.String(), freshName(t), authorA.String()); err != nil {
		t.Fatalf("seeding the hub: %v", err)
	}
	for i, id := range []shared.ID{guide, other} {
		if _, err := admin.Exec(ctx, `INSERT INTO page (id, tenant_id, hub_id, title, icon, order_key, created_by,
			created_at, updated_by, updated_at) VALUES ($1, $2, $3, 'How we work', '📘', $4, $5, now(), $5, now())`,
			id.String(), tenantA.String(), hub.String(), string(rune('a'+i)), authorA.String()); err != nil {
			t.Fatalf("seeding the page: %v", err)
		}
	}
	pages := postgres.TemplatePageRepository{}
	at := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if marked, err := pages.Mark(ctx, hub, guide, "how_we_work", at); err != nil || !marked {
			t.Errorf("mark %v, %v", marked, err)
		}
		if marked, err := pages.Mark(ctx, hub, other, "how_we_work", at); err != nil || marked {
			t.Errorf("a key marked twice: %v, %v", marked, err)
		}
		listed, err := pages.List(ctx, hub)
		if err != nil || len(listed) != 1 || listed[0].PageID != guide || listed[0].Key != "how_we_work" ||
			listed[0].Title != "How we work" || listed[0].Icon != "📘" {
			t.Errorf("listed %+v, %v", listed, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if listed, err := pages.List(ctx, hub); err != nil || len(listed) != 0 {
			t.Errorf("the workspace next door listed %+v, %v", listed, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// The mark names its page within its own workspace: another's page is no page there.
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := pages.Mark(ctx, hub, other, "guide", at)
		return err
	}); err == nil {
		t.Error("the workspace next door marked a page of another workspace")
	}
	if _, err := admin.Exec(ctx, `UPDATE page SET deleted_at = now() WHERE id = $1`, guide.String()); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if listed, err := pages.List(ctx, hub); err != nil || len(listed) != 0 {
			t.Errorf("a trashed page is listed: %+v, %v", listed, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
