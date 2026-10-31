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

// configuration.md §6: a hub's sample entry is marked once per key, listed while it is out of the
// trash, and invisible to the workspace next door (migration 0307).
func TestASampleEntryIsMarkedOnceAndStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	admin := adminPool(ctx, t)
	hub, collection, item, other := freshID(t), freshID(t), freshID(t), freshID(t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		VALUES ($1, $2, 'HUB', $3, 'a0', $4)`, hub.String(), tenantA.String(), freshName(t), authorA.String()); err != nil {
		t.Fatalf("seeding the hub: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		VALUES ($1, $2, 'COLLECTION', $3, $4, 'a0', $5)`,
		collection.String(), tenantA.String(), hub.String(), freshName(t), authorA.String()); err != nil {
		t.Fatalf("seeding the collection: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO entry_type (id, tenant_id, key, level, name, icon, color_token, created_at, updated_at)
		VALUES ($1, $2, 'incident', 'TASK', 'Incident', 'flame', 'red', now(), now())
		ON CONFLICT (tenant_id, key) DO NOTHING`, freshID(t).String(), tenantA.String()); err != nil {
		t.Fatalf("seeding the type: %v", err)
	}
	for _, id := range []shared.ID{item, other} {
		if _, err := admin.Exec(ctx, `
			INSERT INTO work_item (id, tenant_id, collection_id, type, path, depth, title, order_key,
			                       content_language, created_by, entry_type)
			VALUES ($1, $2, $3, 'TASK', $4, 1, 'Printer offline', 'a0', 'en', $5, 'incident')`,
			id.String(), tenantA.String(), collection.String(), "/"+id.String()+"/", authorA.String()); err != nil {
			t.Fatalf("seeding the entry: %v", err)
		}
	}
	samples := postgres.SampleEntryRepository{}
	at := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if marked, err := samples.Mark(ctx, hub, item, "printer", at); err != nil || !marked {
			t.Errorf("mark %v, %v", marked, err)
		}
		if marked, err := samples.Mark(ctx, hub, other, "printer", at); err != nil || marked {
			t.Errorf("a key marked twice: %v, %v", marked, err)
		}
		listed, err := samples.List(ctx, hub)
		if err != nil || len(listed) != 1 || listed[0].ItemID != item || listed[0].Key != "printer" ||
			listed[0].EntryType != "incident" || listed[0].CollectionID != collection || listed[0].Title != "Printer offline" {
			t.Errorf("listed %+v, %v", listed, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if listed, err := samples.List(ctx, hub); err != nil || len(listed) != 0 {
			t.Errorf("the workspace next door listed %+v, %v", listed, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// The mark names its entry within its own workspace: another's entry is no entry there.
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := samples.Mark(ctx, hub, other, "vpn", at)
		return err
	}); err == nil {
		t.Error("the workspace next door marked an entry of another workspace")
	}
	if _, err := admin.Exec(ctx, `UPDATE work_item SET deleted_at = now() WHERE id = $1`, item.String()); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if listed, err := samples.List(ctx, hub); err != nil || len(listed) != 0 {
			t.Errorf("a trashed sample is listed: %+v, %v", listed, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
