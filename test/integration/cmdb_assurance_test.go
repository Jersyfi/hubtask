// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The CMDB's assurance (migration 0267) answers its own workspace and nobody next door: an
// entry's affected CIs, a campaign and its attestations, a group, an asset and an alias.
func TestTheInventorysAssuranceStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	hub, app, vm := seedCIs(ctx, t)
	item := seedWorkItemForRules(ctx, t, tenantA)
	repo := postgres.NewCmdbAssuranceRepository()
	at := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	campaign, group, alias := freshID(t), freshID(t), freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if added, err := repo.AddAffected(ctx, cmdb.Affected{ID: freshID(t), ItemID: item, CIID: vm, AddedBy: authorA, AddedAt: at}); err != nil || !added {
			t.Fatalf("naming the CI: %v %v", added, err)
		}
		if again, _ := repo.AddAffected(ctx, cmdb.Affected{ID: freshID(t), ItemID: item, CIID: vm, AddedBy: authorA, AddedAt: at}); again {
			t.Error("the CI was named twice")
		}
		c := cmdb.Campaign{ID: campaign, HubID: hub, Name: "Annual", DueOn: at.AddDate(0, 0, 14), ReviewMonths: 12, StartedBy: authorA, StartedAt: at}
		if err := repo.InsertCampaign(ctx, c, []cmdb.Attestation{{CIID: app, Owner: authorA}, {CIID: vm}}); err != nil {
			return err
		}
		g := cmdb.Group{ID: group, HubID: hub, Key: "web", Name: "Web", Filter: cmdb.Filter{NameHas: "web"}, ServiceID: app, UpdatedBy: authorA, UpdatedAt: at}
		if err := repo.PutGroup(ctx, g, 0); err != nil {
			return err
		}
		if err := repo.PutAsset(ctx, cmdb.Asset{CIID: vm, HubID: hub, State: cmdb.AssetInStock, Currency: "EUR", CostMinor: 100, UpdatedBy: authorA, UpdatedAt: at}, 0); err != nil {
			return err
		}
		return repo.PutAlias(ctx, cmdb.SoftwareAlias{ID: alias, MatchProduct: "code", Product: "Visual Studio Code", UpdatedBy: authorA, UpdatedAt: at}, 0)
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		if near, _ := repo.AffectingAny(ctx, []shared.ID{vm}, 10); len(near) != 1 || near[0].ItemID != item {
			t.Errorf("the entries on the CI: %+v", near)
		}
		if as, _ := repo.Attestations(ctx, campaign); len(as) != 2 || as[0].Verdict != cmdb.VerdictPending {
			t.Errorf("the attestations: %+v", as)
		}
		if g, err := repo.FindGroup(ctx, group); err != nil || g.Filter.NameHas != "web" || g.ServiceID != app {
			t.Errorf("the group: %+v %v", g, err)
		}
		if a, err := repo.FindAsset(ctx, vm); err != nil || a.State != cmdb.AssetInStock || a.Version != 1 {
			t.Errorf("the asset: %+v %v", a, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if list, _ := repo.AffectedOf(ctx, item); len(list) != 0 {
			t.Error("the workspace next door read the affected CIs")
		}
		if _, err := repo.RemoveAffected(ctx, item, vm); err == nil {
			t.Error("the workspace next door removed an affected CI")
		}
		if _, err := repo.FindCampaign(ctx, campaign); err == nil {
			t.Error("the workspace next door found the campaign")
		}
		if as, _ := repo.Attestations(ctx, campaign); len(as) != 0 {
			t.Error("the workspace next door read the attestations")
		}
		if _, err := repo.FindGroup(ctx, group); err == nil {
			t.Error("the workspace next door found the group")
		}
		if _, err := repo.FindAsset(ctx, vm); err == nil {
			t.Error("the workspace next door found the asset")
		}
		if err := repo.PutAsset(ctx, cmdb.Asset{CIID: vm, HubID: hub, State: cmdb.AssetRetired, UpdatedBy: authorB, UpdatedAt: at}, 1); err == nil {
			t.Error("the workspace next door wrote the asset")
		}
		if list, _ := repo.Aliases(ctx); len(list) != 0 {
			t.Error("the workspace next door read the aliases")
		}
		if err := repo.RemoveAlias(ctx, alias); err == nil {
			t.Error("the workspace next door removed the alias")
		}
		snap := postgres.NewSnapshotRepository()
		if rows, _ := snap.CIAffected(ctx, "", 100); len(rows) != 0 {
			t.Error("the workspace next door walked the affected CIs")
		}
		if rows, _ := snap.CIGroups(ctx, "", 100); len(rows) != 0 {
			t.Error("the workspace next door walked the groups")
		}
		if rows, _ := snap.CIAssets(ctx, "", 100); len(rows) != 0 {
			t.Error("the workspace next door walked the assets")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
