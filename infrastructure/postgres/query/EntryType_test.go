// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package query

import (
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

// The entry's type is a query field (UC-FLW-01): an entry without a type of its own answers as its
// level's system type, so a filter on `task` finds the entries nobody typed.
func TestTheEntryTypeFiltersByTheEffectiveKey(t *testing.T) {
	const effective = `coalesce(wi.entry_type, lower(wi.type::text))`

	tests := []struct {
		name   string
		filter map[string]any
		want   string
	}{
		{"equal", map[string]any{"field": "entry_type", "op": "EQ", "value": "bug"},
			effective + ` = normalize($2::text, NFC)`},
		{"not equal", map[string]any{"field": "entry_type", "op": "NEQ", "value": "task"},
			effective + ` IS DISTINCT FROM normalize($2::text, NFC)`},
		{"one of", map[string]any{"field": "entry_type", "op": "IN", "value": []any{"bug", "task"}},
			effective + ` = ANY(`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			statement := compile(t, test.filter, view.Spec{})
			if !strings.Contains(statement.SQL, test.want) {
				t.Errorf("the statement does not compare the effective key:\n%s", statement.SQL)
			}
			if !strings.Contains(statement.SQL, `wi.number, wi.entry_type`) {
				t.Errorf("the projection does not read the type:\n%s", statement.SQL)
			}
		})
	}
}

func TestTheEntryTypeGroupsAndCounts(t *testing.T) {
	grouping, err := view.ParseGroupBy(map[string]any{"field": "entry_type"}, "/group_by")
	if err != nil {
		t.Fatalf("the grouping was refused: %v", err)
	}
	search := searchOf(t, nil, view.Spec{GroupBy: grouping})

	groups, err := Groups(search, 51)
	if err != nil {
		t.Fatalf("grouping failed: %v", err)
	}
	if !strings.Contains(groups.SQL, `PARTITION BY coalesce(wi.entry_type, lower(wi.type::text))`) ||
		!strings.Contains(groups.SQL, `ORDER BY coalesce(entry_type, lower(type::text)) ASC NULLS LAST`) {
		t.Errorf("the groups are not cut by the effective key:\n%s", groups.SQL)
	}

	count, err := Count(search)
	if err != nil {
		t.Fatalf("counting failed: %v", err)
	}
	if !strings.HasPrefix(count.SQL, `SELECT coalesce(wi.entry_type, lower(wi.type::text))::text, count(*)`) {
		t.Errorf("the count is not keyed by the effective key:\n%s", count.SQL)
	}
}

// A type is a key, not an order anybody reads, so it is not a sort.
func TestTheEntryTypeIsNoSort(t *testing.T) {
	if _, err := view.ParseSort([]any{map[string]any{"field": "entry_type"}}, "/sort"); err == nil {
		t.Error("a sort by the entry type was accepted")
	}
}
