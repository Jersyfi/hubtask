// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// A hub's own custom field against the real boundary (UC-WRK-12 check 9, gate SG-3): stored at its
// hub with the definition it was copied from, in force in the hub's collections between a
// collection's own and the workspace's, nowhere else, never in the workspace next door, and gone
// with its hub.

// hubField writes one definition of a hub and returns it as built.
func hubField(ctx context.Context, t *testing.T, tenant, hub shared.ID, key string, kind work.CustomFieldKind) work.CustomFieldDefinition {
	t.Helper()
	built, err := work.NewCustomFieldDefinition(work.NewCustomFieldInput{
		ID: freshID(t), TenantID: tenant, HubID: hub, Key: key, Kind: kind,
		AppliesTo: []work.ItemType{work.ItemTask}, Now: created, Text: text.Composing{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error { return fieldRepo().Insert(ctx, built) }); err != nil {
		t.Fatalf("defining the hub's field: %v", err)
	}
	return built
}

func TestAHubsOwnFieldIsInForceInItsCollectionsAlone(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub, collection := hubWithCollection(ctx, t, tenantA, authorA)
	_, elsewhere := hubWithCollection(ctx, t, tenantA, authorA)
	key := "region_" + shortSuffix(t)

	workspaceField := definedField(ctx, t, tenantA, shared.ID(""), key, work.CustomFieldText)
	copied, err := workspaceField.CopyAt(freshID(t), hub, created)
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error { return fieldRepo().Insert(ctx, copied) }); err != nil {
		t.Fatalf("copying the field here: %v", err)
	}
	stored := findField(ctx, t, tenantA, copied.ID)
	if stored.HubID != hub || !stored.CollectionID.IsZero() || stored.CopiedFrom != workspaceField.ID {
		t.Fatalf("the copy came back as %+v", stored)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error { return fieldRepo().Insert(ctx, copied) }); err == nil {
		t.Error("a second field under the key at the hub was taken")
	}

	inScope := func(scope shared.ID) work.CustomFieldDefinition {
		t.Helper()
		var found work.CustomFieldDefinition
		if err := read(ctx, t, tenantA, func(ctx context.Context) error {
			var err error
			found, err = fieldRepo().FindInScope(ctx, scope, key)
			return err
		}); err != nil {
			t.Fatalf("reading the field in force: %v", err)
		}
		return found
	}
	if got := inScope(collection); got.ID != copied.ID {
		t.Errorf("in the hub's collection %s is in force, want the hub's", got.ID)
	}
	if got := inScope(hub); got.ID != copied.ID {
		t.Errorf("at the hub %s is in force, want its own", got.ID)
	}
	if got := inScope(elsewhere); got.ID != workspaceField.ID {
		t.Errorf("in another hub's collection %s is in force, want the workspace's", got.ID)
	}
	own := definedField(ctx, t, tenantA, collection, key, work.CustomFieldText)
	if got := inScope(collection); got.ID != own.ID {
		t.Errorf("the collection's own did not win over the hub's: %s", got.ID)
	}

	var listed []work.CustomFieldDefinition
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		listed, err = fieldRepo().ListInScope(ctx, collection)
		return err
	}); err != nil {
		t.Fatalf("listing: %v", err)
	}
	var order []shared.ID
	for _, definition := range listed {
		if definition.Key == key {
			order = append(order, definition.ID)
		}
	}
	if len(order) != 3 || order[0] != workspaceField.ID || order[1] != copied.ID || order[2] != own.ID {
		t.Errorf("the collection lists %v, want the workspace's, the hub's and its own in that order", order)
	}

	kinds := map[string][]work.CustomFieldKind{}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		kinds, err = fieldRepo().KindsByKey(ctx, []string{key}, collection)
		return err
	}); err != nil || len(kinds[key]) != 1 {
		t.Errorf("the kinds in the collection are %v (%v)", kinds, err)
	}
}

func TestAValueUnderAHubsFieldShowsInTheHubAndIsCounted(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub, collection := hubWithCollection(ctx, t, tenantA, authorA)
	key := "site_" + shortSuffix(t)
	field := hubField(ctx, t, tenantA, hub, key, work.CustomFieldText)
	task := seedTask(ctx, t, tenantA, authorA, collection)

	item := findItem(ctx, t, tenantA, task)
	written, _ := item.WithCustomField(key, "north", changedAt)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetCustomField(ctx, written, key, field.ID, item.Version)
	}); err != nil {
		t.Fatalf("writing the value: %v", err)
	}
	if got := findItem(ctx, t, tenantA, task).CustomFields[key]; got != "north" {
		t.Fatalf("the entry shows %v under the hub's field", got)
	}

	var holding int
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		holding, err = fieldRepo().EntriesHolding(ctx, field)
		return err
	}); err != nil || holding != 1 {
		t.Errorf("the hub's field holds %d values (%v), want 1", holding, err)
	}
}

func TestAHubsFieldStaysInItsWorkspaceAndGoesWithItsHub(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub, _ := hubWithCollection(ctx, t, tenantA, authorA)
	field := hubField(ctx, t, tenantA, hub, "dock_"+shortSuffix(t), work.CustomFieldText)

	err := read(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := fieldRepo().Find(ctx, field.ID)
		return err
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the workspace next door read the hub's field: %v", err)
	}
	var listed []work.CustomFieldDefinition
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		listed, err = fieldRepo().ListInScope(ctx, hub)
		return err
	}); err != nil {
		t.Fatalf("listing next door: %v", err)
	}
	for _, definition := range listed {
		if definition.ID == field.ID {
			t.Error("the workspace next door lists the hub's field")
		}
	}

	if _, err := adminPool(ctx, t).Exec(ctx, `DELETE FROM container WHERE tenant_id = $1 AND (id = $2 OR parent_id = $2)`,
		tenantA.String(), hub.String()); err != nil {
		t.Fatalf("removing the hub: %v", err)
	}
	err = read(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := fieldRepo().Find(ctx, field.ID)
		return err
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the hub's field outlived its hub: %v", err)
	}
}

// A hub's field decides what its collections' FIELD_SUM adds up, so defining one moves the count
// of changes a kept figure of each of them is held to (UC-WRK-29): a figure kept from before would
// otherwise answer as if the field did not exist.
func TestAHubsFieldMovesTheKeptFiguresOfItsCollections(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub, collection := hubWithCollection(ctx, t, tenantA, authorA)
	key := repository.KeptFigureKey{CollectionID: collection, Kind: work.FigureOpen, BasisKey: "any"}
	before := keptState(ctx, t, tenantA, key).Changes
	hubField(ctx, t, tenantA, hub, "load_"+shortSuffix(t), work.CustomFieldNumber)
	if after := keptState(ctx, t, tenantA, key).Changes; after <= before {
		t.Errorf("the collection's changes stayed at %d after its hub defined a field", after)
	}
}
