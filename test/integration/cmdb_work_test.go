// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A hub's open work per CI reads the open, living entries naming its CIs - never a completed or an
// archived one - and nothing of the workspace next door.
func TestAHubsOpenWorkOnItsCIsStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	hub, app, vm := seedCIs(ctx, t)
	open, done, archived := seedWorkItemForRules(ctx, t, tenantA), seedWorkItemForRules(ctx, t, tenantA), seedWorkItemForRules(ctx, t, tenantA)
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `UPDATE work_item SET is_completed = true, completed_at = now() WHERE id = $1`, done.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `UPDATE work_item SET archived_at = now() WHERE id = $1`, archived.String()); err != nil {
		t.Fatal(err)
	}
	repo := postgres.NewCmdbAssuranceRepository()
	at := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, item := range []cmdb.Affected{
			{ItemID: open, CIID: vm}, {ItemID: open, CIID: app}, {ItemID: done, CIID: vm}, {ItemID: archived, CIID: vm},
		} {
			item.ID, item.AddedBy, item.AddedAt = freshID(t), authorA, at
			if _, err := repo.AddAffected(ctx, item); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		rows, err := repo.OpenWorkOfHub(ctx, hub, 100)
		if err != nil {
			return err
		}
		if len(rows) != 2 {
			t.Fatalf("the open namings: %+v", rows)
		}
		for _, r := range rows {
			if r.ItemID != open || r.CollectionID.IsZero() || r.HubID.IsZero() || r.CIID != vm && r.CIID != app {
				t.Errorf("an open naming: %+v", r)
			}
		}
		if rows, _ := repo.OpenWorkOfHub(ctx, hub, 1); len(rows) != 1 {
			t.Errorf("the limit: %+v", rows)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if rows, _ := repo.OpenWorkOfHub(ctx, hub, 100); len(rows) != 0 {
			t.Errorf("the workspace next door read the open work: %+v", rows)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// A group keeps the inventory line's owners, locations and tags with their nones (UC-AST-11 check
// 1), and the workspace next door finds none of it.
func TestAGroupKeepsItsOwnerLocationAndTagConditions(t *testing.T) {
	ctx := context.Background()
	hub, app, _ := seedCIs(ctx, t)
	repo := postgres.NewCmdbAssuranceRepository()
	group := freshID(t)
	f := cmdb.Filter{Owners: []shared.ID{authorA}, NoOwner: true, Locations: []shared.ID{app}, NoLocation: true, Tags: []string{"inv-7"}, NoTag: true}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.PutGroup(ctx, cmdb.Group{ID: group, HubID: hub, Key: "line", Name: "Line", Filter: f, UpdatedBy: authorA, UpdatedAt: time.Now()}, 0)
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		g, err := repo.FindGroup(ctx, group)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(g.Filter, f) {
			t.Errorf("the filter read back: %+v", g.Filter)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := repo.FindGroup(ctx, group); err == nil {
			t.Error("the workspace next door found the group")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// An asset keeps each step of its life (migration 0430, UC-AST-12 checks 1 and 2), oldest first;
// the steps go with the asset, and the workspace next door neither reads nor adds one.
func TestAnAssetsStepsStayInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	hub, app, vm := seedCIs(ctx, t)
	repo := postgres.NewCmdbAssuranceRepository()
	ordered := time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := repo.PutAsset(ctx, cmdb.Asset{CIID: vm, HubID: hub, State: cmdb.AssetDeployed, AssignedTo: authorA, UpdatedBy: authorA, UpdatedAt: ordered}, 0); err != nil {
			return err
		}
		for i, step := range []cmdb.AssetMove{
			{State: cmdb.AssetDeployed, At: ordered.AddDate(0, 0, 7), AssignedTo: authorA},
			{State: cmdb.AssetOrdered, At: ordered},
			{State: cmdb.AssetInStock, At: ordered.AddDate(0, 0, 6), StockroomID: app},
		} {
			step.ID, step.HubID, step.CIID, step.By = freshID(t), hub, vm, authorA
			if err := repo.AddAssetMove(ctx, step); err != nil {
				t.Fatalf("step %d: %v", i, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		steps, err := repo.AssetMoves(ctx, vm, 10)
		if err != nil {
			return err
		}
		if len(steps) != 3 || steps[0].State != cmdb.AssetOrdered || steps[1].StockroomID != app || steps[2].AssignedTo != authorA ||
			!steps[0].At.Equal(ordered) || steps[2].By != authorA {
			t.Errorf("the steps, oldest first: %+v", steps)
		}
		if steps, _ := repo.AssetMoves(ctx, vm, 2); len(steps) != 2 {
			t.Errorf("the limit: %+v", steps)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if steps, _ := repo.AssetMoves(ctx, vm, 10); len(steps) != 0 {
			t.Errorf("the workspace next door read the steps: %+v", steps)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return repo.AddAssetMove(ctx, cmdb.AssetMove{ID: freshID(t), HubID: hub, CIID: vm, State: cmdb.AssetRetired, At: ordered, By: authorB})
	}); err == nil {
		t.Error("the workspace next door added a step to the asset")
	}
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `DELETE FROM cmdb_asset WHERE ci_id = $1`, vm.String()); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM cmdb_asset_move WHERE ci_id = $1`, vm.String()).Scan(&left); err != nil || left != 0 {
		t.Errorf("the steps outlived their asset: %d %v", left, err)
	}
}
