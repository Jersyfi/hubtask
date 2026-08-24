// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The acceptance runs C-07 names, against a real database. Each of them is a sentence in the task
// that would otherwise be a claim - and the first is where the compiled jsonb SQL is actually
// executed rather than only proved parameter-clean.

// setKey writes one custom field on an entry as SetCustomField's write path does: through the
// domain's one-key application and the repository's one-key statement.
func setKey(
	ctx context.Context, t *testing.T, tenant shared.ID, itemID shared.ID, key string, value any,
) work.WorkItem {
	t.Helper()

	item := findItem(ctx, t, tenant, itemID)
	// The definition in force, resolved the way the use case resolves it: its identity travels
	// with the value, which is what the recreated-key acceptance below rests on.
	var definition work.CustomFieldDefinition
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		definition, err = fieldRepo().FindInScope(ctx, item.CollectionID, key)
		return err
	}); err != nil {
		t.Fatalf("resolving %s: %v", key, err)
	}

	wanted, moved := item.WithCustomField(key, value, changedAt)
	if !moved {
		t.Fatalf("writing %s moved nothing", key)
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return itemRepo().SetCustomField(ctx, wanted, key, definition.ID, item.Version)
	}); err != nil {
		t.Fatalf("writing %s: %v", key, err)
	}
	return findItem(ctx, t, tenant, itemID)
}

// TestACustomFieldFilterAnswersAgainstTheDatabase is the acceptance sentence about the query
// language: `custom_fields.<key>` filters answer as api-guidelines.md §3 shows, and the key is a
// parameter - which FuzzCompile proves about the text, and only PostgreSQL can prove about the
// statement being valid and answering the question.
func TestACustomFieldFilterAnswersAgainstTheDatabase(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)

	prioKey := "prio_" + shortSuffix(t)
	budgetKey := "budget_" + shortSuffix(t)
	tagsKey := "tags_" + shortSuffix(t)
	definedField(ctx, t, tenantA, collection, prioKey, work.CustomFieldSelect, "high", "low")
	definedField(ctx, t, tenantA, collection, budgetKey, work.CustomFieldNumber)
	definedField(ctx, t, tenantA, collection, tagsKey, work.CustomFieldMultiSelect, "urgent", "later")

	urgent := seedTask(ctx, t, tenantA, authorA, collection)
	relaxed := seedTask(ctx, t, tenantA, authorA, collection)
	blank := seedTask(ctx, t, tenantA, authorA, collection)
	setKey(ctx, t, tenantA, urgent, prioKey, "high")
	setKey(ctx, t, tenantA, urgent, budgetKey, float64(1500))
	setKey(ctx, t, tenantA, urgent, tagsKey, []any{"urgent", "later"})
	setKey(ctx, t, tenantA, relaxed, prioKey, "low")
	setKey(ctx, t, tenantA, relaxed, budgetKey, float64(200))

	cases := []struct {
		name   string
		filter map[string]any
		want   []shared.ID
	}{
		{
			name:   "EQ on a SELECT, through containment and the GIN index",
			filter: map[string]any{"field": "custom_fields." + prioKey, "op": "EQ", "value": "high"},
			want:   []shared.ID{urgent},
		},
		{
			name:   "EQ on a NUMBER matches the number, not its spelling",
			filter: map[string]any{"field": "custom_fields." + budgetKey, "op": "EQ", "value": 1500},
			want:   []shared.ID{urgent},
		},
		{
			// An entry that holds nothing under the key counts as "not that one".
			name:   "NEQ counts the entry with no value",
			filter: map[string]any{"field": "custom_fields." + prioKey, "op": "NEQ", "value": "high"},
			want:   []shared.ID{relaxed, blank},
		},
		{
			name:   "IN over the text form",
			filter: map[string]any{"field": "custom_fields." + prioKey, "op": "IN", "value": []any{"high", "low"}},
			want:   []shared.ID{urgent, relaxed},
		},
		{
			name:   "IS_NULL is the key never set",
			filter: map[string]any{"field": "custom_fields." + prioKey, "op": "IS_NULL"},
			want:   []shared.ID{blank},
		},
		{
			name:   "CONTAINS reaches one element of a MULTI_SELECT",
			filter: map[string]any{"field": "custom_fields." + tagsKey, "op": "CONTAINS", "value": "urgent"},
			want:   []shared.ID{urgent},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := queried(ctx, t, tenantA, searchIn(t, collection, c.filter, view.Spec{}))
			assertSameIDs(t, result.Items, c.want)
		})
	}
}

// TestARecreatedKeyExposesNothingOfTheOldValue is the acceptance sentence about the soft delete: a
// definition deleted and recreated under the same key is a new definition, and no read - the find,
// the list, the filter - shows what the old one held.
func TestARecreatedKeyExposesNothingOfTheOldValue(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	key := "phase_" + shortSuffix(t)

	first := definedField(ctx, t, tenantA, collection, key, work.CustomFieldText)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	setKey(ctx, t, tenantA, task, key, "confidential planning note")

	// Deleted: the value stays in the row and stops being visible.
	deleted, _, err := first.Deleted(changedAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return fieldRepo().SetDeleted(ctx, deleted, first.Version)
	}); err != nil {
		t.Fatalf("deleting the definition: %v", err)
	}
	if hidden := findItem(ctx, t, tenantA, task); len(hidden.CustomFields) != 0 {
		t.Fatalf("a read still answers %v", hidden.CustomFields)
	}

	// Recreated under the same key: still nothing, until somebody writes the field again.
	definedField(ctx, t, tenantA, collection, key, work.CustomFieldText)
	recreated := findItem(ctx, t, tenantA, task)
	if len(recreated.CustomFields) != 0 {
		t.Fatalf("the recreated key resurrected %v", recreated.CustomFields)
	}

	// A filter is not a way around it: the old value matches nothing, and IS_NULL says "not set".
	filter := map[string]any{"field": "custom_fields." + key, "op": "EQ", "value": "confidential planning note"}
	if result := queried(ctx, t, tenantA, searchIn(t, collection, filter, view.Spec{})); len(result.Items) != 0 {
		t.Errorf("a filter found the hidden value: %v", titlesOf(result.Items))
	}
	isNull := map[string]any{"field": "custom_fields." + key, "op": "IS_NULL"}
	if result := queried(ctx, t, tenantA, searchIn(t, collection, isNull, view.Spec{})); len(result.Items) != 1 {
		t.Errorf("IS_NULL answers %d entries, want the one whose value is hidden", len(result.Items))
	}

	// A new value under the recreated key works, and shows only itself.
	after := setKey(ctx, t, tenantA, task, key, "fresh start")
	if after.CustomFields[key] != "fresh start" || len(after.CustomFields) != 1 {
		t.Errorf("the entry answers %v", after.CustomFields)
	}
}

// TestTwoDevicesSettingTwoKeysConvergeToBoth is the acceptance sentence about the merge rule, at
// the level this milestone owns: the per-key write plus the version predicate. Device B wrote
// against the version it had read, is told the row moved, re-reads and lands its own key - and
// nothing of device A's is lost on the way.
func TestTwoDevicesSettingTwoKeysConvergeToBoth(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	keyA := "device_a_" + shortSuffix(t)
	keyB := "device_b_" + shortSuffix(t)
	defA := definedField(ctx, t, tenantA, collection, keyA, work.CustomFieldText)
	defB := definedField(ctx, t, tenantA, collection, keyB, work.CustomFieldText)
	task := seedTask(ctx, t, tenantA, authorA, collection)

	// Both devices read the same state.
	seen := findItem(ctx, t, tenantA, task)

	// Device A lands first.
	wantedA, _ := seen.WithCustomField(keyA, "from A", changedAt)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetCustomField(ctx, wantedA, keyA, defA.ID, seen.Version)
	}); err != nil {
		t.Fatalf("device A: %v", err)
	}

	// Device B wrote against the state it read, and the version is what catches it.
	wantedB, _ := seen.WithCustomField(keyB, "from B", changedAt)
	staleErr := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetCustomField(ctx, wantedB, keyB, defB.ID, seen.Version)
	})
	if !errors.Is(staleErr, shared.ErrVersionConflict) {
		t.Fatalf("the concurrent write answered %v, want a version conflict", staleErr)
	}

	// B re-reads and retries - which is the whole client protocol - and both keys are there.
	current := findItem(ctx, t, tenantA, task)
	retryB, _ := current.WithCustomField(keyB, "from B", changedAt)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetCustomField(ctx, retryB, keyB, defB.ID, current.Version)
	}); err != nil {
		t.Fatalf("device B's retry: %v", err)
	}

	converged := findItem(ctx, t, tenantA, task)
	if converged.CustomFields[keyA] != "from A" || converged.CustomFields[keyB] != "from B" {
		t.Fatalf("the entry converged to %v", converged.CustomFields)
	}
}

// assertSameIDs compares a result against the entries it should hold, order-free: what these
// acceptance runs assert is membership, and the ordering has its own tests.
func assertSameIDs(t *testing.T, items []work.WorkItem, want []shared.ID) {
	t.Helper()

	if len(items) != len(want) {
		t.Fatalf("%d entries answered, want %d: %v", len(items), len(want), titlesOf(items))
	}
	held := map[shared.ID]bool{}
	for _, item := range items {
		held[item.ID] = true
	}
	for _, id := range want {
		if !held[id] {
			t.Errorf("the answer is missing %s", id)
		}
	}
}
