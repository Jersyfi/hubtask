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

// A text's `hubtask:ci/<id>` is a reference the database keeps, as reference.Extract reads it
// (migration 0237), and a reference table takes the kind.
func TestACIAddressIsAReferenceOfTheText(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)
	ci := freshID(t)
	var kind, strength string
	if err := admin.QueryRow(ctx,
		`SELECT target_kind, strength FROM entry_references_of($1) WHERE target_id = $2`,
		"runs on ![](hubtask:ci/"+ci.String()+")", ci.String(),
	).Scan(&kind, &strength); err != nil || kind != "CI" || strength != "CARD" {
		t.Fatalf("the address reads as %q %q, %v", kind, strength, err)
	}
	var allowed bool
	if err := admin.QueryRow(ctx,
		`SELECT pg_get_constraintdef(oid) LIKE '%''CI''%' FROM pg_constraint WHERE conname = 'whiteboard_reference_target_kind_check'`,
	).Scan(&allowed); err != nil || !allowed {
		t.Fatalf("a whiteboard reference takes the kind CI: %v, %v", allowed, err)
	}
}

// The initial sync's walk of the inventory (offline reading) answers a workspace's own CIs and
// relations, and none of the workspace next door's.
func TestTheInventoryWalkStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	_, app, vm := seedCIs(ctx, t)
	snapshot := postgres.NewSnapshotRepository()
	seen := func(tenant shared.ID) (map[shared.ID]bool, int) {
		found, relations := map[shared.ID]bool{}, 0
		if err := write(ctx, t, tenant, func(ctx context.Context) error {
			cis, err := snapshot.CIs(ctx, "", 5000)
			if err != nil {
				return err
			}
			for _, ci := range cis {
				found[ci.ID] = true
			}
			rels, err := snapshot.CIRelations(ctx, "", 5000)
			for _, r := range rels {
				if r.SourceCI == app && r.TargetCI == vm {
					relations++
				}
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return found, relations
	}
	own, ownRelations := seen(tenantA)
	if !own[app] || !own[vm] || ownRelations != 1 {
		t.Errorf("the workspace's walk holds %v and %d relations, want both CIs and their relation", own, ownRelations)
	}
	next, nextRelations := seen(tenantB)
	if next[app] || next[vm] || nextRelations != 0 {
		t.Errorf("the workspace next door walked %v and %d relations", next, nextRelations)
	}
}

// A connector, its sealed credential, its run record and its alerts are kept in its workspace
// (migration 0238); the workspace next door finds none of them and cannot write them.
func TestAConnectorAndItsAlertsStayInTheirWorkspace(t *testing.T) {
	ctx := context.Background()
	hub, _, vm := seedCIs(ctx, t)
	repo := postgres.NewCmdbConnectorRepository()
	at := time.Date(2026, 10, 29, 11, 0, 0, 0, time.UTC)
	conn := cmdb.Connector{ID: freshID(t), HubID: hub, Key: "am_" + freshName(t)[:8], Kind: cmdb.ConnectorAlertmanager,
		Source: "alerts", Settings: map[string]string{}, Enabled: true, RunAs: authorA, CreatedAt: at, UpdatedAt: at, Version: 1}
	alert := cmdb.Alert{ID: freshID(t), HubID: hub, Connector: conn.ID, Key: "disk", Status: cmdb.AlertFiring,
		Title: "disk full", CI: vm, FirstAt: at, LastAt: at, Count: 1}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := repo.PutConnector(ctx, conn, 0); err != nil {
			return err
		}
		if err := repo.SetCredential(ctx, conn.ID, []byte("sealed"), "k1", at); err != nil {
			return err
		}
		if err := repo.PutAlert(ctx, alert); err != nil {
			return err
		}
		seen, err := repo.SeeDigest(ctx, conn.ID, []byte("d1"), at, at.Add(-10*time.Minute))
		if err != nil || !seen {
			t.Errorf("a first delivery is new: %v %v", seen, err)
		}
		if again, _ := repo.SeeDigest(ctx, conn.ID, []byte("d1"), at, at.Add(-10*time.Minute)); again {
			t.Error("a replayed delivery was taken as new")
		}
		read, err := repo.FindConnector(ctx, conn.ID, true)
		if err != nil || !read.HasCredential || read.Version != 2 {
			t.Errorf("connector %+v, %v", read, err)
		}
		found, err := repo.FindAlert(ctx, conn.ID, "disk")
		if err != nil || found.CI != vm || found.Title != "disk full" {
			t.Errorf("alert %+v, %v", found, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := repo.FindConnector(ctx, conn.ID, false); err == nil {
			t.Error("the workspace next door found the connector")
		}
		if _, err := repo.Credential(ctx, conn.ID); err == nil {
			t.Error("the workspace next door read the sealed credential")
		}
		if err := repo.SetCredential(ctx, conn.ID, []byte("x"), "k2", at); err == nil {
			t.Error("the workspace next door replaced the credential")
		}
		if list, _ := repo.ListConnectors(ctx, ""); len(list) != 0 {
			t.Error("the workspace next door listed the connectors")
		}
		if list, _ := repo.ListAlerts(ctx, hub, "", 10); len(list) != 0 {
			t.Error("the workspace next door listed the alerts")
		}
		if _, err := repo.FindAlert(ctx, conn.ID, "disk"); err == nil {
			t.Error("the workspace next door found the alert")
		}
		if sealed, _ := repo.SealedNotUnder(ctx, "k9"); len(sealed) != 0 {
			t.Error("the workspace next door saw the sealed credentials")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
