// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"slices"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func (s *snapshotStore) CIs(_ context.Context, after shared.ID, batch int) ([]cmdb.CI, error) {
	return pageAfter(s.cis, func(ci cmdb.CI) string { return ci.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) CIRelations(_ context.Context, after shared.ID, batch int) ([]cmdb.Relation, error) {
	return pageAfter(s.ciRelations, func(r cmdb.Relation) string { return r.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) CmdbCatalogue(context.Context) ([]cmdb.Class, []cmdb.RelationType, error) {
	return s.ciClasses, nil, nil
}

// The classes and relation types a device names an inventory by travel as one record under the
// workspace's identifier, the shipped catalogue with the workspace's own.
func TestTheWalkCarriesTheCmdbCatalogueOnce(t *testing.T) {
	pull, f, store := walking(t)
	f.auth.workspace = true
	store.ciClasses = []cmdb.Class{{Key: "printer", Parent: "hardware", Name: "Printer", Version: 1}}

	records, _ := wholeWalk(t, pull, 3)

	var catalogues []Record
	for _, record := range records {
		if record.Entity == "cmdb_catalogue" {
			catalogues = append(catalogues, record)
		}
	}
	if len(catalogues) != 1 || catalogues[0].EntityID != tenant || !catalogues[0].ContainerID.IsZero() {
		t.Fatalf("catalogue records %+v, want one under the workspace", catalogues)
	}
	var keys []string
	for _, c := range catalogues[0].Payload["classes"].([]usecase.Output) {
		keys = append(keys, c["key"].(string))
	}
	if !slices.Contains(keys, "printer") || !slices.Contains(keys, "server") {
		t.Errorf("the catalogue's classes %v, want the shipped ones and the workspace's printer", keys)
	}
	if len(catalogues[0].Payload["relation_types"].([]usecase.Output)) == 0 {
		t.Error("the catalogue carries no relation types")
	}
}

// A hub's inventory is read offline (UC-AST-01 check 10): the walk carries the CIs and their
// relations, each filed under its hub, and withholds those of a hub the reader may not read.
func TestTheWalkCarriesTheInventoryOfTheHubsTheReaderReads(t *testing.T) {
	pull, f, store := walking(t)
	server, app, hidden := idAt('9', 1), idAt('9', 2), idAt('9', 3)
	store.cis = []cmdb.CI{
		{ID: server, TenantID: tenant, HubID: hub, Class: "server", Name: "web1", Version: 1},
		{ID: app, TenantID: tenant, HubID: hub, Class: "application_service", Name: "Shop", Version: 1},
		{ID: hidden, TenantID: tenant, HubID: otherHub, Class: "server", Name: "vault", Version: 1},
	}
	store.ciRelations = []cmdb.Relation{
		{ID: idAt('9', 4), HubID: hub, Type: cmdb.RelRunsOn, SourceCI: app, TargetCI: server},
		{ID: idAt('9', 5), HubID: otherHub, Type: cmdb.RelDependsOn, SourceCI: hidden, TargetCI: hidden},
	}
	f.auth.allowed[otherHub] = false

	records, _ := wholeWalk(t, pull, 100)

	var cis, relations []shared.ID
	for _, record := range records {
		switch record.Entity {
		case "cmdb_ci":
			cis = append(cis, record.EntityID)
			if record.ContainerID != hub || record.Payload["hub_id"] != hub.String() {
				t.Errorf("a CI record %+v, want it filed under its hub", record)
			}
		case "cmdb_relation":
			relations = append(relations, record.EntityID)
			if record.Payload["source_ci_id"] != app.String() || record.Payload["type"] != cmdb.RelRunsOn {
				t.Errorf("a relation record %+v", record)
			}
		}
	}
	if !slices.Equal(cis, []shared.ID{server, app}) {
		t.Errorf("CIs delivered %v, want %s and %s - never %s, whose hub is out of reach", cis, server, app, hidden)
	}
	if !slices.Equal(relations, []shared.ID{idAt('9', 4)}) {
		t.Errorf("relations delivered %v, want only the one in the hub the reader reads", relations)
	}
}

func (s *snapshotStore) CIAffected(_ context.Context, after shared.ID, batch int) ([]repository.AffectedRow, error) {
	return pageAfter(s.ciAffected, func(a repository.AffectedRow) string { return a.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) CIGroups(_ context.Context, after shared.ID, batch int) ([]cmdb.Group, error) {
	return pageAfter(s.ciGroups, func(g cmdb.Group) string { return g.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) CIAssets(_ context.Context, after shared.ID, batch int) ([]cmdb.Asset, error) {
	return pageAfter(s.ciAssets, func(a cmdb.Asset) string { return a.CIID.String() }, after.String(), batch), nil
}

// An entry's affected CIs, a hub's groups and asset records are read offline (UC-AST-09 check 5,
// UC-AST-11 check 4, UC-AST-12 check 4): each is filed under its hub, the affected CI under the
// CI's, and withheld where the reader may not read the hub.
func TestTheWalkCarriesAffectedCIsGroupsAndAssetsOfTheHubsTheReaderReads(t *testing.T) {
	pull, f, store := walking(t)
	server, hidden := idAt('9', 1), idAt('9', 3)
	store.ciAffected = []repository.AffectedRow{
		{Affected: cmdb.Affected{ID: idAt('9', 6), ItemID: idAt('9', 7), CIID: server}, HubID: hub},
		{Affected: cmdb.Affected{ID: idAt('9', 8), ItemID: idAt('9', 7), CIID: hidden}, HubID: otherHub},
	}
	store.ciGroups = []cmdb.Group{{ID: idAt('9', 9), HubID: hub, Key: "web", Name: "Web", Version: 1}}
	store.ciAssets = []cmdb.Asset{
		{CIID: server, HubID: hub, State: cmdb.AssetDeployed, Version: 1},
		{CIID: hidden, HubID: otherHub, State: cmdb.AssetInStock, Version: 1},
	}
	f.auth.allowed[otherHub] = false

	records, _ := wholeWalk(t, pull, 100)

	got := map[string][]shared.ID{}
	for _, record := range records {
		switch record.Entity {
		case "cmdb_affected", "cmdb_ci_group", "cmdb_asset":
			if record.ContainerID != hub {
				t.Errorf("a record %+v, want it filed under the readable hub", record)
			}
			got[record.Entity] = append(got[record.Entity], record.EntityID)
		}
	}
	if !slices.Equal(got["cmdb_affected"], []shared.ID{idAt('9', 6)}) || !slices.Equal(got["cmdb_ci_group"], []shared.ID{idAt('9', 9)}) ||
		!slices.Equal(got["cmdb_asset"], []shared.ID{server}) {
		t.Errorf("delivered %v", got)
	}
}
