// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A hub's own type against the real boundary (UC-FLW-01 check 10, gate SG-3): it is stored with
// its hub and the key it was copied from, an entry of the hub carrying it is found under the
// entry and in the collection, the workspace next door reads none of it, and the hub takes its
// types with it when it goes.

var (
	ehTenantA  = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fd01")
	ehTenantB  = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fd02")
	ehAccount  = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fd03")
	ehHub      = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fd04")
	ehOtherHub = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fd05")
	ehList     = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fd06")
	ehItem     = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fd07")
)

func TestAHubsOwnTypeStaysWithItsHub(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)
	a, account := ehTenantA.String(), ehAccount.String()
	for _, statement := range []string{
		`INSERT INTO tenant (id, slug, display_name) VALUES ('` + a + `', 'eh-a', 'Hub types A') ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO tenant (id, slug, display_name) VALUES ('` + ehTenantB.String() + `', 'eh-b', 'Hub types B') ON CONFLICT (id) DO NOTHING`,
		`DELETE FROM work_item WHERE tenant_id = '` + a + `'`,
		`DELETE FROM entry_type WHERE tenant_id = '` + a + `'`,
		`DELETE FROM container WHERE tenant_id = '` + a + `'`,
		`INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		 VALUES ('` + account + `', '` + a + `', 'USER', 'hubtypes@example.org', 'Hub types', 'ACTIVE')
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		 VALUES ('` + ehHub.String() + `', '` + a + `', 'HUB', 'Support', 'a0', '` + account + `'),
		        ('` + ehOtherHub.String() + `', '` + a + `', 'HUB', 'Sales', 'a1', '` + account + `')`,
		`INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		 VALUES ('` + ehList.String() + `', '` + a + `', 'COLLECTION', '` + ehHub.String() + `', 'Desk', 'a0', '` + account + `')`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}

	types := postgres.NewEntryTypeRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC().Truncate(time.Microsecond)
	common, err := domain.NewEntryType(domain.NewEntryTypeInput{
		ID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fd11"), TenantID: ehTenantA,
		Key: "bug", Level: domain.ItemTask, Name: "Bug", Icon: "bug", ColorToken: "red",
		Behaviours: domain.EntryBehaviours{Completable: true}, Now: now, Text: text.Composing{},
	})
	if err != nil {
		t.Fatal(err)
	}
	copied, err := common.CopyAt(domain.EntryTypeCopy{
		ID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fd12"), HubID: ehHub, Key: "support_bug",
		Now: now, Text: text.Composing{},
	})
	if err != nil {
		t.Fatal(err)
	}
	inTenant(t, uow, ehTenantA, func(ctx context.Context) error {
		for _, entryType := range []domain.EntryType{common, copied} {
			if err := types.Insert(ctx, entryType); err != nil {
				t.Fatalf("storing %s: %v", entryType.Key, err)
			}
		}
		read, err := types.Find(ctx, "support_bug")
		if err != nil || read.HubID != ehHub || read.CopiedFrom != "bug" {
			t.Errorf("read back %+v, %v", read, err)
		}
		return nil
	})
	if _, err := admin.Exec(ctx, `INSERT INTO work_item (id, tenant_id, collection_id, type, path, depth, title, order_key, created_by, entry_type)
		 VALUES ('`+ehItem.String()+`', '`+a+`', '`+ehList.String()+`', 'TASK', '/`+ehItem.String()+`/', 1, 'Printer jam', 'a0', '`+account+`', 'support_bug')`); err != nil {
		t.Fatalf("seeding the entry: %v", err)
	}
	inTenant(t, uow, ehTenantA, func(ctx context.Context) error {
		if bound, err := types.OfHubUnder(ctx, "/"+ehItem.String()+"/", ehHub); err != nil || !bound {
			t.Errorf("under the entry: %v, %v", bound, err)
		}
		if bound, err := types.OfHubInCollection(ctx, ehList, ehHub); err != nil || !bound {
			t.Errorf("in the collection: %v, %v", bound, err)
		}
		if bound, err := types.OfHubInCollection(ctx, ehList, ehOtherHub); err != nil || bound {
			t.Errorf("another hub's types in the collection: %v, %v", bound, err)
		}
		return nil
	})

	// The workspace next door sees neither the type nor the entry carrying it.
	inTenant(t, uow, ehTenantB, func(ctx context.Context) error {
		if _, err := types.Find(ctx, "support_bug"); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("B found A's hub type: %v", err)
		}
		if bound, err := types.OfHubUnder(ctx, "/"+ehItem.String()+"/", ehHub); err != nil || bound {
			t.Errorf("B read A's entry: %v, %v", bound, err)
		}
		if bound, err := types.OfHubInCollection(ctx, ehList, ehHub); err != nil || bound {
			t.Errorf("B read A's collection: %v, %v", bound, err)
		}
		return nil
	})

	// A purge takes the hub's entries and collections first; the hub then takes its own types
	// with it, and the shared one stays.
	for _, statement := range []string{
		`DELETE FROM work_item WHERE tenant_id = '` + a + `'`,
		`DELETE FROM container WHERE tenant_id = '` + a + `' AND id = '` + ehList.String() + `'`,
		`DELETE FROM container WHERE tenant_id = '` + a + `' AND id = '` + ehHub.String() + `'`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("purging the hub: %v", err)
		}
	}
	inTenant(t, uow, ehTenantA, func(ctx context.Context) error {
		if _, err := types.Find(ctx, "support_bug"); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the hub's type outlived its hub: %v", err)
		}
		if _, err := types.Find(ctx, "bug"); err != nil {
			t.Errorf("the shared type went with the hub: %v", err)
		}
		return nil
	})
}
