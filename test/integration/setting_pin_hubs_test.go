// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"slices"
	"testing"
	"time"

	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	domain "github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The keys are the test's own: the hub level is shared by every test of the package, and a pin
// under a module's key would switch other tests' hubs.
const (
	pinnedKey  domain.Key = "module.pin_hubs_test"
	needingKey domain.Key = "module.pin_hubs_test_needing"
)

func hubPlace(hub shared.ID) settingrepo.Place {
	return settingrepo.Place{Level: domain.LevelHub, ScopeID: hub}
}

func storeFlag(ctx context.Context, t *testing.T, tenant, hub shared.ID, key domain.Key, on bool) {
	t.Helper()
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return postgres.SettingRepository{}.Put(ctx, freshID(t), hubPlace(hub), key,
			domain.Entry{Value: on, Hold: domain.HoldSet}, created)
	}); err != nil {
		t.Fatalf("storing a hub's flag: %v", err)
	}
}

// storedFlags reads what each hub stores under the pinned key, as the tenant.
func storedFlags(ctx context.Context, t *testing.T, tenant shared.ID, hubs ...shared.ID) map[shared.ID]any {
	t.Helper()
	places := make([]settingrepo.Place, 0, len(hubs))
	for _, hub := range hubs {
		places = append(places, hubPlace(hub))
	}
	flags := map[shared.ID]any{}
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		stored, err := postgres.SettingRepository{}.At(ctx, places, string(pinnedKey))
		for _, entry := range stored {
			if entry.Key == pinnedKey {
				flags[entry.Place.ScopeID] = entry.Entry.Value
			}
		}
		return err
	}); err != nil {
		t.Fatalf("reading the hubs' flags: %v", err)
	}
	return flags
}

// hubsWithout reads, as the tenant, the hubs that store nothing under the pinned key.
func hubsWithout(ctx context.Context, t *testing.T, tenant shared.ID) []shared.ID {
	t.Helper()
	var hubs []shared.ID
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		hubs, err = postgres.SettingRepository{}.HubsWithout(ctx, pinnedKey)
		return err
	}); err != nil {
		t.Fatalf("reading the hubs that store nothing: %v", err)
	}
	return hubs
}

// pinHubs pins the hubs named, as the tenant, and answers the hubs written and each hub's row id.
func pinHubs(
	ctx context.Context, t *testing.T, tenant shared.ID, pin settingrepo.HubPin, hubs ...shared.ID,
) ([]shared.ID, map[shared.ID]shared.ID) {
	t.Helper()
	rows := map[shared.ID]shared.ID{}
	for _, hub := range hubs {
		rows[hub] = freshID(t)
		pin.Hubs = append(pin.Hubs, settingrepo.PinnedHub{RowID: rows[hub], HubID: hub})
	}
	var written []shared.ID
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		written, err = postgres.SettingRepository{}.PinHubs(ctx, pin, created.Add(time.Hour))
		return err
	}); err != nil {
		t.Fatalf("pinning the hubs: %v", err)
	}
	return written, rows
}

// HubsWithout and PinHubs against a real database (UC-INS-21 check 13), with their cross-tenant
// negative test (gate SG-3): every hub of A that stores nothing under the key is read - an archived
// and a trashed hub too - and gets the flag under the row id handed in; a hub that stores a value
// keeps it, a hub storing on under a key named in OnWhere gets on, and B's hubs, with a row under
// the same key or without, are neither read nor - named to the write all the same - written.
func TestPinHubsWritesOnlyTheCallersHubsThatStoreNothing(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	admin := adminPool(ctx, t)
	clean := func() {
		if _, err := admin.Exec(ctx, `DELETE FROM setting_value WHERE key = ANY($1)`,
			[]string{string(pinnedKey), string(needingKey)}); err != nil {
			t.Errorf("removing the test's flags: %v", err)
		}
	}
	clean()
	t.Cleanup(clean)

	plain, _ := hubWithCollection(ctx, t, tenantA, authorA)
	archived, _ := hubWithCollection(ctx, t, tenantA, authorA)
	trashed, _ := hubWithCollection(ctx, t, tenantA, authorA)
	decided, _ := hubWithCollection(ctx, t, tenantA, authorA)
	needing, _ := hubWithCollection(ctx, t, tenantA, authorA)
	foreign, _ := hubWithCollection(ctx, t, tenantB, authorB)
	foreignDecided, _ := hubWithCollection(ctx, t, tenantB, authorB)
	if _, err := admin.Exec(ctx, `UPDATE container SET archived_at = now() WHERE id = $1`, archived.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `UPDATE container SET deleted_at = now() WHERE id = $1`, trashed.String()); err != nil {
		t.Fatal(err)
	}
	storeFlag(ctx, t, tenantA, decided, pinnedKey, true)
	storeFlag(ctx, t, tenantA, needing, needingKey, true)
	storeFlag(ctx, t, tenantB, foreignDecided, pinnedKey, true)

	without := hubsWithout(ctx, t, tenantA)
	for _, hub := range []shared.ID{plain, archived, trashed, needing} {
		if !slices.Contains(without, hub) {
			t.Errorf("the hub %s is not read as storing nothing", hub)
		}
	}
	for _, hub := range []shared.ID{decided, foreign, foreignDecided} {
		if slices.Contains(without, hub) {
			t.Errorf("the hub %s is read as one of A's that stores nothing", hub)
		}
	}

	// B's hubs and the hub that decided are named to the write as well: the statement, not the
	// caller's list, keeps the write to A's hubs that store nothing.
	pin := settingrepo.HubPin{Key: pinnedKey, On: false, OnWhere: []domain.Key{needingKey}}
	named := append(slices.Clone(without), decided, foreign, foreignDecided)
	written, rows := pinHubs(ctx, t, tenantA, pin, named...)
	for _, hub := range []shared.ID{plain, archived, trashed, needing} {
		if !slices.Contains(written, hub) {
			t.Errorf("the hub %s was not written", hub)
		}
		var stored string
		if err := admin.QueryRow(ctx, `SELECT id::text FROM setting_value WHERE scope_id = $1 AND key = $2`,
			hub.String(), string(pinnedKey)).Scan(&stored); err != nil {
			t.Fatalf("reading the row of the hub %s: %v", hub, err)
		}
		if stored != rows[hub].String() {
			t.Errorf("the hub %s is stored under the row id %s, want the id handed in, %s", hub, stored, rows[hub])
		}
	}
	for _, hub := range []shared.ID{decided, foreign, foreignDecided} {
		if slices.Contains(written, hub) {
			t.Errorf("the hub %s was written", hub)
		}
	}

	own := storedFlags(ctx, t, tenantA, plain, archived, trashed, decided, needing)
	want := map[shared.ID]any{plain: false, archived: false, trashed: false, decided: true, needing: true}
	for hub, flag := range want {
		if own[hub] != flag {
			t.Errorf("the hub %s stores %v, want %v", hub, own[hub], flag)
		}
	}

	// A second pin finds a value everywhere and writes nothing over it.
	left := hubsWithout(ctx, t, tenantA)
	for hub := range want {
		if slices.Contains(left, hub) {
			t.Errorf("after the pin the hub %s still reads as storing nothing", hub)
		}
	}
	again, _ := pinHubs(ctx, t, tenantA, settingrepo.HubPin{Key: pinnedKey, On: true}, named...)
	for hub := range want {
		if slices.Contains(again, hub) {
			t.Errorf("the hub %s was written a second time", hub)
		}
	}
	if own := storedFlags(ctx, t, tenantA, plain); own[plain] != false {
		t.Errorf("the second pin changed the stored value to %v", own[plain])
	}

	// B's hubs are as they were, and B reads none of A's rows.
	theirs := storedFlags(ctx, t, tenantB, foreign, foreignDecided, plain)
	if _, stored := theirs[foreign]; stored || theirs[foreignDecided] != true {
		t.Errorf("A's pin reached B's hubs: %v", theirs)
	}
	if _, visible := theirs[plain]; visible {
		t.Error("B reads the flag A's hub stores")
	}
}
