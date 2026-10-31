// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// estimated writes one value under a defined key, through the statement every writer uses.
func estimated(
	ctx context.Context, t *testing.T, tenant, task shared.ID,
	definition work.CustomFieldDefinition, value any,
) {
	t.Helper()
	item := findItem(ctx, t, tenant, task)
	written, _ := item.WithCustomField(definition.Key, value, changedAt)
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return itemRepo().SetCustomField(ctx, written, definition.Key, definition.ID, item.Version)
	}); err != nil {
		t.Fatalf("writing the value: %v", err)
	}
}

func typedQuery(
	ctx context.Context, t *testing.T, tenant, collection shared.ID, filter, sort, group any,
) repository.ItemQueryResult {
	t.Helper()
	node, err := view.ParseFilter(filter, "/filter")
	if err != nil {
		t.Fatal(err)
	}
	terms, err := view.ParseSort(sort, "/sort")
	if err != nil {
		t.Fatal(err)
	}
	grouping, err := view.ParseGroupBy(group, "/group_by")
	if err != nil {
		t.Fatal(err)
	}
	spec := view.Spec{Filter: node, Sort: terms, GroupBy: grouping, Size: 50, Count: view.CountExact}

	var result repository.ItemQueryResult
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		kinds, err := fieldRepo().KindsByKey(ctx, spec.CustomKeys(), collection)
		if err != nil {
			return err
		}
		shapes := map[string]view.CustomShape{}
		for _, key := range spec.CustomKeys() {
			shapes[key] = view.ShapeOf(kinds[key])
		}
		if spec, err = spec.Shaped(shapes); err != nil {
			return err
		}
		result, err = itemRepo().Query(ctx, repository.ItemSearch{
			Anchor: repository.Anchor{
				Kind: repository.AnchorCollection, CollectionID: collection, IncludeDescendants: true,
			},
			Spec: spec,
		})
		return err
	}); err != nil {
		t.Fatalf("the query failed: %v", err)
	}
	return result
}

// UC-WRK-17 check 10 against the database: the trigger keeps the typed values in the transaction
// that writes the document, and a range, a sort and a grouping read them.
func TestACustomNumberRangesSortsAndGroups(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	estimate := definedField(ctx, t, tenantA, collection, "estimate", work.CustomFieldNumber)

	small := seedTask(ctx, t, tenantA, authorA, collection)
	large := seedTask(ctx, t, tenantA, authorA, collection)
	middle := seedTask(ctx, t, tenantA, authorA, collection)
	none := seedTask(ctx, t, tenantA, authorA, collection)
	estimated(ctx, t, tenantA, small, estimate, 1.0)
	estimated(ctx, t, tenantA, large, estimate, 8.0)
	estimated(ctx, t, tenantA, middle, estimate, 3.0)
	// A value changed afterwards ranges by the new one only.
	estimated(ctx, t, tenantA, middle, estimate, 2.5)

	result := typedQuery(ctx, t, tenantA, collection,
		map[string]any{"field": "custom_fields.estimate", "op": "GT", "value": 2},
		[]any{map[string]any{"field": "custom_fields.estimate", "dir": "DESC"}}, nil)
	if len(result.Items) != 2 || result.Items[0].ID != large || result.Items[1].ID != middle {
		t.Fatalf("the range answered %v", entryIDs(result.Items))
	}

	sorted := typedQuery(ctx, t, tenantA, collection, nil,
		[]any{map[string]any{"field": "custom_fields.estimate"}}, nil)
	if len(sorted.Items) != 4 || sorted.Items[0].ID != small || sorted.Items[3].ID != none {
		t.Fatalf("the sort answered %v", entryIDs(sorted.Items))
	}

	grouped := typedQuery(ctx, t, tenantA, collection, nil, nil,
		map[string]any{"field": "custom_fields.estimate"})
	if len(grouped.Groups) != 4 || grouped.Groups[0].Key != "1" || !grouped.Groups[3].Absent {
		t.Fatalf("the groups are %+v", grouped.Groups)
	}
	if grouped.Groups[0].Total != 1 {
		t.Errorf("the first group counts %d", grouped.Groups[0].Total)
	}

	// A deleted definition's values stop ranging, as they stop being read.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		gone, _, _ := estimate.Deleted(changedAt)
		return fieldRepo().SetDeleted(ctx, gone, estimate.Version)
	}); err != nil {
		t.Fatalf("deleting the definition: %v", err)
	}
	after := typedQuery(ctx, t, tenantA, collection,
		map[string]any{"field": "custom_fields.estimate", "op": "GT", "value": 0}, nil, nil)
	if len(after.Items) != 0 {
		t.Errorf("a deleted field still ranges: %v", entryIDs(after.Items))
	}
}

// A date ranges by its day.
func TestACustomDateRangesByItsDay(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	review := definedField(ctx, t, tenantA, collection, "review_on", work.CustomFieldDate)
	early := seedTask(ctx, t, tenantA, authorA, collection)
	late := seedTask(ctx, t, tenantA, authorA, collection)
	estimated(ctx, t, tenantA, early, review, "2026-03-01")
	estimated(ctx, t, tenantA, late, review, "2026-11-30")

	result := typedQuery(ctx, t, tenantA, collection, map[string]any{
		"field": "custom_fields.review_on", "op": "BETWEEN", "value": []any{"2026-11-01", "2026-12-31"},
	}, nil, nil)
	if len(result.Items) != 1 || result.Items[0].ID != late {
		t.Fatalf("the range answered %v", entryIDs(result.Items))
	}
}

// UC-WRK-17 check 10 against the database: a moment ranges by its instant, an address ranges and
// sorts as an address in `addr` (10.0.0.9 before 10.0.0.10, which text would not give), labels are
// a set, and a state and a two-level choice compare as text (migration 0298).
func TestTheRegisterKindsRangeSortAndMatchInTheDatabase(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	seen := definedField(ctx, t, tenantA, collection, "seen_at", work.CustomFieldDateTime)
	address := definedField(ctx, t, tenantA, collection, "address", work.CustomFieldIPAddress)
	labels := definedField(ctx, t, tenantA, collection, "tags", work.CustomFieldLabel)
	state := definedField(ctx, t, tenantA, collection, "state", work.CustomFieldStatus, "Planned", "In use")
	kind := definedField(ctx, t, tenantA, collection, "category", work.CustomFieldCascadingSelect, "Hardware", "Hardware/Laptop")

	nine := seedTask(ctx, t, tenantA, authorA, collection)
	ten := seedTask(ctx, t, tenantA, authorA, collection)
	estimated(ctx, t, tenantA, nine, seen, "2026-10-31T08:00:00Z")
	estimated(ctx, t, tenantA, ten, seen, "2026-10-31T12:00:00Z")
	estimated(ctx, t, tenantA, nine, address, "10.0.0.9")
	estimated(ctx, t, tenantA, ten, address, "10.0.0.10")
	estimated(ctx, t, tenantA, nine, labels, []any{"backend", "ops"})
	estimated(ctx, t, tenantA, ten, state, "In use")
	estimated(ctx, t, tenantA, ten, kind, "Hardware/Laptop")

	if got := typedQuery(ctx, t, tenantA, collection, map[string]any{
		"field": "custom_fields.seen_at", "op": "GT", "value": "2026-10-31T11:00:00+01:00",
	}, nil, nil); len(got.Items) != 1 || got.Items[0].ID != ten {
		t.Errorf("the moment's range answered %v", entryIDs(got.Items))
	}
	if got := typedQuery(ctx, t, tenantA, collection, map[string]any{
		"field": "custom_fields.address", "op": "BETWEEN", "value": []any{"10.0.0.0", "10.0.0.9"},
	}, nil, nil); len(got.Items) != 1 || got.Items[0].ID != nine {
		t.Errorf("the address's range answered %v", entryIDs(got.Items))
	}
	if got := typedQuery(ctx, t, tenantA, collection, nil,
		[]any{map[string]any{"field": "custom_fields.address"}}, nil); len(got.Items) != 2 || got.Items[0].ID != nine {
		t.Errorf("the address's sort answered %v", entryIDs(got.Items))
	}
	if got := typedQuery(ctx, t, tenantA, collection, map[string]any{
		"field": "custom_fields.tags", "op": "CONTAINS_ANY", "value": []any{"ops"},
	}, nil, nil); len(got.Items) != 1 || got.Items[0].ID != nine {
		t.Errorf("the labels answered %v", entryIDs(got.Items))
	}
	if got := typedQuery(ctx, t, tenantA, collection, map[string]any{
		"field": "custom_fields.category", "op": "STARTS_WITH", "value": "Hardware",
	}, nil, nil); len(got.Items) != 1 || got.Items[0].ID != ten {
		t.Errorf("the two-level choice answered %v", entryIDs(got.Items))
	}
	if got := typedQuery(ctx, t, tenantA, collection, nil, nil,
		map[string]any{"field": "custom_fields.state"}); len(got.Groups) != 2 || got.Groups[0].Key != "In use" {
		t.Errorf("the state's groups are %+v", got.Groups)
	}

	// Gate SG-3: another tenant reads none of the typed values, an address's included.
	addresses := func(tenant shared.ID) int {
		var n int
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			tx, err := postgres.FromContext(ctx)
			if err != nil {
				return err
			}
			return tx.QueryRow(ctx, `SELECT count(*) FROM entry_field_value WHERE addr IS NOT NULL`).Scan(&n)
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if own, foreign := addresses(tenantA), addresses(tenantB); own != 2 || foreign != 0 {
		t.Errorf("the own tenant reads %d addresses, another %d", own, foreign)
	}
}

// Gate SG-3 for KindsByKey: another tenant's definitions are no kinds at all.
func TestTheKindsOfAnotherTenantAreOutOfReach(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	definedField(ctx, t, tenantA, collection, "estimate", work.CustomFieldNumber)

	var own, foreign map[string][]work.CustomFieldKind
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		own, err = fieldRepo().KindsByKey(ctx, []string{"estimate"}, collection)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		var workspace shared.ID
		foreign, err = fieldRepo().KindsByKey(ctx, []string{"estimate"}, workspace)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(own["estimate"]) != 1 || own["estimate"][0] != work.CustomFieldNumber {
		t.Errorf("the own tenant reads %v", own)
	}
	if len(foreign) != 0 {
		t.Errorf("another tenant reads %v", foreign)
	}
}

func entryIDs(items []work.WorkItem) []shared.ID {
	out := make([]shared.ID, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}
