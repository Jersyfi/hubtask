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

func seedCIs(ctx context.Context, t *testing.T) (hub, app, vm shared.ID) {
	t.Helper()
	seedContainerTenants(ctx, t)
	hub, app, vm = freshID(t), freshID(t), freshID(t)
	at := time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)
	repo := postgres.NewCmdbRepository()
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := containerRepo().Insert(ctx, containerIn(tenantA, authorA, hub, freshName(t), "a0")); err != nil {
			return err
		}
		for _, ci := range []cmdb.CI{
			{ID: app, HubID: hub, Class: "application_service", Name: "Shop", Stage: cmdb.StageInUse, Status: cmdb.StatusOK,
				Compliance: cmdb.Compliance{Quantity: 1, Protection: cmdb.OwnProtection{Protection: cmdb.Protection{A: cmdb.LevelHigh}}}},
			{ID: vm, HubID: hub, Class: "virtual_machine", Name: "web1", Stage: cmdb.StageInUse, Status: cmdb.StatusOK,
				Attributes: map[string]any{"hostname": "web1"}, Compliance: cmdb.Compliance{Quantity: 1}, LastSeen: &at},
		} {
			ci.CreatedBy, ci.CreatedAt, ci.UpdatedBy, ci.UpdatedAt, ci.Version = authorA, at, authorA, at, 1
			if err := repo.InsertCI(ctx, ci); err != nil {
				return err
			}
		}
		if err := repo.PutIdentifiers(ctx, cmdb.CI{ID: vm, HubID: hub}, map[string]string{"hostname": "web1", "source:netbox": "7"}); err != nil {
			return err
		}
		_, _, err := repo.PutRelation(ctx, cmdb.Relation{ID: freshID(t), HubID: hub, Type: cmdb.RelRunsOn, SourceCI: app, TargetCI: vm,
			DataSource: cmdb.ManualSource, LastSeen: at, CreatedBy: authorA})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return hub, app, vm
}

// A CI is read as written, found by its identifiers, and the impact walks its relation's way
// (migration 0236).
func TestACIIsFoundByItsIdentifiersAndItsImpact(t *testing.T) {
	ctx := context.Background()
	hub, app, vm := seedCIs(ctx, t)
	repo := postgres.NewCmdbRepository()
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		read, err := repo.FindCI(ctx, app, false)
		if err != nil || read.Compliance.Protection.A != cmdb.LevelHigh || read.Class != "application_service" {
			t.Errorf("CI %+v, %v", read, err)
		}
		rows, err := repo.FindIdentifiers(ctx, hub, map[string]string{"hostname": "web1"})
		if err != nil || len(rows) != 1 || rows[0].CI != vm || rows[0].Class != "virtual_machine" {
			t.Errorf("identifiers %+v, %v", rows, err)
		}
		down, err := repo.Walk(ctx, vm, true, []string{cmdb.RelRunsOn}, nil, 5)
		if err != nil || len(down) != 1 || down[0].CI != app || down[0].Depth != 1 {
			t.Errorf("downstream %+v, %v", down, err)
		}
		up, err := repo.Walk(ctx, app, false, []string{cmdb.RelRunsOn}, nil, 5)
		if err != nil || len(up) != 1 || up[0].CI != vm {
			t.Errorf("upstream %+v, %v", up, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Another workspace finds no CI, no identifier, no relation and walks nothing.
func TestACIIsInvisibleToTheWorkspaceNextDoor(t *testing.T) {
	ctx := context.Background()
	hub, app, vm := seedCIs(ctx, t)
	repo := postgres.NewCmdbRepository()
	at := time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := repo.FindCI(ctx, app, true); err == nil {
			t.Error("the workspace next door found the CI")
		}
		if err := repo.UpdateCI(ctx, cmdb.CI{ID: app, Name: "x", Stage: cmdb.StageInUse, Status: cmdb.StatusOK,
			Compliance: cmdb.Compliance{Quantity: 1}, UpdatedBy: authorB, UpdatedAt: at}, 1); err == nil {
			t.Error("the workspace next door wrote the CI")
		}
		if list, _ := repo.ListCIs(ctx, hub, nil, "", 10); len(list) != 0 {
			t.Error("the workspace next door listed the CIs")
		}
		if rows, _ := repo.FindIdentifiers(ctx, hub, map[string]string{"hostname": "web1"}); len(rows) != 0 {
			t.Error("the workspace next door read the identifiers")
		}
		if rels, _ := repo.RelationsOf(ctx, vm); len(rels) != 0 {
			t.Error("the workspace next door read the relations")
		}
		if down, _ := repo.Walk(ctx, vm, true, []string{cmdb.RelRunsOn}, nil, 5); len(down) != 0 {
			t.Error("the workspace next door walked the impact")
		}
		if list, _ := repo.CIsByID(ctx, []shared.ID{app, vm}); len(list) != 0 {
			t.Error("the workspace next door resolved the CIs")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
