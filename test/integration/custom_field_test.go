// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The custom field definitions against a real database (C-07): the two scopes, the key that is
// free again once a definition is deleted, the optimistic lock, the values on the entry, and a
// cross-tenant negative for every method (gate SG-3).

func fieldRepo() postgres.CustomFieldRepository { return postgres.NewCustomFieldRepository() }

// definedField writes one definition and returns it as stored.
func definedField(
	ctx context.Context, t *testing.T, tenant, collection shared.ID,
	key string, kind work.CustomFieldKind, options ...string,
) work.CustomFieldDefinition {
	t.Helper()

	built, err := work.NewCustomFieldDefinition(work.NewCustomFieldInput{
		ID: freshID(t), TenantID: tenant, CollectionID: collection,
		Key: key, Kind: kind, Options: options,
		AppliesTo: []work.ItemType{work.ItemTask}, Now: created,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return fieldRepo().Insert(ctx, built)
	}); err != nil {
		t.Fatalf("defining the field: %v", err)
	}
	return built
}

func findField(ctx context.Context, t *testing.T, tenant, id shared.ID) work.CustomFieldDefinition {
	t.Helper()

	var stored work.CustomFieldDefinition
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		stored, err = fieldRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("reading the definition: %v", err)
	}
	return stored
}

func TestADefinitionRoundTripsWithItsOptionsAndTypes(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)

	defined := definedField(ctx, t, tenantA, collection, "priority", work.CustomFieldSelect, "high", "low")
	stored := findField(ctx, t, tenantA, defined.ID)

	if stored.Key != "priority" || stored.Kind != work.CustomFieldSelect {
		t.Fatalf("the definition is %+v", stored)
	}
	if len(stored.Options) != 2 || stored.Options[0] != "high" {
		t.Errorf("the options came back as %v", stored.Options)
	}
	if len(stored.AppliesTo) != 1 || stored.AppliesTo[0] != work.ItemTask {
		t.Errorf("applies_to came back as %v", stored.AppliesTo)
	}
	if stored.Version != 1 || stored.CollectionID != collection {
		t.Errorf("the definition is %+v", stored)
	}
}

// The two scopes, and which one wins. A collection's own definition narrows a workspace-wide one
// under the same key rather than colliding with it.
func TestACollectionsDefinitionNarrowsTheWorkspaceWideOne(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	key := "risk_" + shortSuffix(t)

	definedField(ctx, t, tenantA, shared.ID(""), key, work.CustomFieldText)
	narrow := definedField(ctx, t, tenantA, collection, key, work.CustomFieldSelect, "low")

	var inScope work.CustomFieldDefinition
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		inScope, err = fieldRepo().FindInScope(ctx, collection, key)
		return err
	}); err != nil {
		t.Fatalf("reading the definition in scope: %v", err)
	}
	if inScope.ID != narrow.ID {
		t.Errorf("the collection's own definition did not win: %+v", inScope)
	}

	// And the list carries both, the workspace-wide one first.
	var listed []work.CustomFieldDefinition
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		listed, err = fieldRepo().ListInScope(ctx, collection)
		return err
	}); err != nil {
		t.Fatalf("listing: %v", err)
	}
	if countKey(listed, key) != 2 {
		t.Errorf("the list carries %d definitions of %s", countKey(listed, key), key)
	}
}

func TestAKeyIsTakenOnlyWhileItsDefinitionLives(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	key := "phase_" + shortSuffix(t)

	first := definedField(ctx, t, tenantA, collection, key, work.CustomFieldText)

	// The same key again, while the first is live: the unique index decides, and the answer names
	// the key so a client can act on it.
	second, err := work.NewCustomFieldDefinition(work.NewCustomFieldInput{
		ID: freshID(t), TenantID: tenantA, CollectionID: collection,
		Key: key, Kind: work.CustomFieldText,
		AppliesTo: []work.ItemType{work.ItemTask}, Now: created,
	})
	if err != nil {
		t.Fatal(err)
	}
	takenErr := write(ctx, t, tenantA, func(ctx context.Context) error {
		return fieldRepo().Insert(ctx, second)
	})
	if !errors.Is(takenErr, shared.ErrConflict) ||
		shared.AsError(takenErr).DetailCode != "fields.key_taken" {
		t.Fatalf("a taken key answered %v", takenErr)
	}

	// Deleted, and the key is free again - which is what the partial unique index is for.
	deleted, _, err := first.Deleted(changedAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return fieldRepo().SetDeleted(ctx, deleted, first.Version)
	}); err != nil {
		t.Fatalf("deleting: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return fieldRepo().Insert(ctx, second)
	}); err != nil {
		t.Fatalf("the key was still taken after the deletion: %v", err)
	}

	// The deleted one is out of every scoped read, and its values are nobody's business any more.
	var listed []work.CustomFieldDefinition
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		listed, err = fieldRepo().ListInScope(ctx, collection)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, definition := range listed {
		if definition.ID == first.ID {
			t.Error("a deleted definition is still in the list")
		}
	}
}

func TestAnEditTakesTheOptimisticLock(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	defined := definedField(ctx, t, tenantA, collection, "size_"+shortSuffix(t), work.CustomFieldSelect, "s")

	options := []string{"s", "m"}
	updated, changes, err := defined.Updated(work.CustomFieldAttributes{Options: &options}, changedAt)
	if err != nil || len(changes) != 1 {
		t.Fatalf("the update answered %+v (%v)", changes, err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return fieldRepo().SetAttributes(ctx, updated, defined.Version)
	}); err != nil {
		t.Fatalf("writing the update: %v", err)
	}
	if stored := findField(ctx, t, tenantA, defined.ID); stored.Version != 2 || len(stored.Options) != 2 {
		t.Fatalf("the stored definition is %+v", stored)
	}

	// The same version again matches nothing: somebody else has moved it on.
	staleErr := write(ctx, t, tenantA, func(ctx context.Context) error {
		return fieldRepo().SetAttributes(ctx, updated, defined.Version)
	})
	if !errors.Is(staleErr, shared.ErrVersionConflict) {
		t.Errorf("a stale write answered %v", staleErr)
	}
}

// The values, on the entry rather than in the definitions table.
func TestTheValuesTravelWithTheEntry(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	// The key needs a live definition: the reads answer only what one stands behind, so a value
	// written under an undefined key would be stored and invisible - which is its own test below.
	definedField(ctx, t, tenantA, collection, "priority", work.CustomFieldText)

	item := findItem(ctx, t, tenantA, task)
	if item.CustomFields != nil {
		t.Fatalf("a fresh entry carries %v", item.CustomFields)
	}

	written, moved := item.WithCustomField("priority", "high", changedAt)
	if !moved {
		t.Fatal("writing a key reported that nothing moved")
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetCustomField(ctx, written, "priority", item.Version)
	}); err != nil {
		t.Fatalf("writing the key: %v", err)
	}

	stored := findItem(ctx, t, tenantA, task)
	if stored.CustomFields["priority"] != "high" || stored.Version != item.Version+1 {
		t.Fatalf("the entry is %+v (version %d)", stored.CustomFields, stored.Version)
	}

	// Clearing removes the key rather than storing a null, so a read cannot tell "cleared" from
	// "never set" - which is what makes them the same state.
	cleared, moved := stored.WithCustomField("priority", nil, changedAt)
	if !moved {
		t.Fatal("clearing a key reported that nothing moved")
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetCustomField(ctx, cleared, "priority", stored.Version)
	}); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	if after := findItem(ctx, t, tenantA, task); len(after.CustomFields) != 0 {
		t.Errorf("the entry still carries %v", after.CustomFields)
	}
}

// Gate SG-3: every method, asked from the wrong tenant.
func TestACustomFieldOfAnotherTenantIsOutOfReach(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	defined := definedField(ctx, t, tenantA, collection, "owner_"+shortSuffix(t), work.CustomFieldText)

	t.Run("find", func(t *testing.T) {
		err := read(ctx, t, tenantB, func(ctx context.Context) error {
			_, err := fieldRepo().Find(ctx, defined.ID)
			return err
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("tenant B read tenant A's definition: %v", err)
		}
	})

	t.Run("find in scope and list", func(t *testing.T) {
		err := read(ctx, t, tenantB, func(ctx context.Context) error {
			_, err := fieldRepo().FindInScope(ctx, collection, defined.Key)
			return err
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("tenant B resolved tenant A's key: %v", err)
		}

		var listed []work.CustomFieldDefinition
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			listed, err = fieldRepo().ListInScope(ctx, collection)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if countKey(listed, defined.Key) != 0 {
			t.Errorf("tenant B listed tenant A's definitions: %v", listed)
		}
	})

	t.Run("update and delete", func(t *testing.T) {
		required := true
		wanted, _, err := defined.Updated(work.CustomFieldAttributes{IsRequired: &required}, changedAt)
		if err != nil {
			t.Fatal(err)
		}
		updateErr := write(ctx, t, tenantB, func(ctx context.Context) error {
			return fieldRepo().SetAttributes(ctx, wanted, defined.Version)
		})
		if !errors.Is(updateErr, shared.ErrVersionConflict) {
			t.Errorf("tenant B edited tenant A's definition: %v", updateErr)
		}

		deleted, _, err := defined.Deleted(changedAt)
		if err != nil {
			t.Fatal(err)
		}
		deleteErr := write(ctx, t, tenantB, func(ctx context.Context) error {
			return fieldRepo().SetDeleted(ctx, deleted, defined.Version)
		})
		if !errors.Is(deleteErr, shared.ErrVersionConflict) {
			t.Errorf("tenant B deleted tenant A's definition: %v", deleteErr)
		}
		if stored := findField(ctx, t, tenantA, defined.ID); stored.IsDeleted() || stored.IsRequired {
			t.Errorf("tenant A's definition is %+v", stored)
		}
	})

	t.Run("the values on an entry", func(t *testing.T) {
		task := seedTask(ctx, t, tenantA, authorA, collection)
		item := findItem(ctx, t, tenantA, task)
		written, _ := item.WithCustomField(defined.Key, "high", changedAt)

		err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return itemRepo().SetCustomField(ctx, written, defined.Key, item.Version)
		})
		if !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("tenant B wrote tenant A's custom fields: %v", err)
		}
		if stored := findItem(ctx, t, tenantA, task); len(stored.CustomFields) != 0 {
			t.Errorf("tenant A's entry carries %v", stored.CustomFields)
		}
	})
}

func countKey(definitions []work.CustomFieldDefinition, key string) int {
	found := 0
	for _, definition := range definitions {
		if definition.Key == key {
			found++
		}
	}
	return found
}

// shortSuffix keeps a key unique across a package that shares one database, and keeps it a key:
// lower case letters and digits only, which is what the schema's CHECK allows.
func shortSuffix(t *testing.T) string {
	t.Helper()
	id := freshID(t)
	return "k" + id.String()[len(id.String())-6:]
}
