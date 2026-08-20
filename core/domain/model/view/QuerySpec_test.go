// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	collection = shared.MustParseID("0192f000-0000-7000-8000-00000000000c")
	entry      = shared.MustParseID("0192f000-0000-7000-8000-00000000000e")
)

func TestParseScope(t *testing.T) {
	t.Run("a collection", func(t *testing.T) {
		scope, err := ParseScope(collection, "", true, "/scope")
		if err != nil {
			t.Fatalf("unexpected refusal: %v", err)
		}
		if scope.ContainerID != collection || !scope.IncludeDescendants {
			t.Errorf("read as %+v", scope)
		}
	})

	t.Run("no anchor at all", func(t *testing.T) {
		_, err := ParseScope("", "", true, "/scope")
		if code := detailOf(t, err); code != "query.scope_required" {
			t.Errorf("detail code = %q", code)
		}
	})

	t.Run("two anchors", func(t *testing.T) {
		_, err := ParseScope(collection, entry, true, "/scope")
		if code := detailOf(t, err); code != "query.scope_ambiguous" {
			t.Errorf("detail code = %q", code)
		}
	})
}

func TestParseSort(t *testing.T) {
	t.Run("none is the manual order", func(t *testing.T) {
		for _, raw := range []any{nil, []any{}} {
			sort, err := ParseSort(raw, "/sort")
			if err != nil {
				t.Fatalf("unexpected refusal: %v", err)
			}
			if len(sort) != 1 || sort[0].Field.Name != FieldOrderKey || sort[0].Descending {
				t.Errorf("read as %+v", sort)
			}
		}
	})

	t.Run("directions and null placement", func(t *testing.T) {
		sort, err := ParseSort([]any{
			map[string]any{"field": "completed_at", "dir": "DESC", "nulls": "FIRST"},
			map[string]any{"field": "title"},
		}, "/sort")
		if err != nil {
			t.Fatalf("unexpected refusal: %v", err)
		}
		if !sort[0].Descending || !sort[0].NullsFirst {
			t.Errorf("first term read as %+v", sort[0])
		}
		if sort[1].Descending || sort[1].NullsFirst {
			t.Errorf("the defaults are ascending and nulls last, got %+v", sort[1])
		}
	})

	tests := []struct {
		name string
		raw  any
		code string
	}{
		{"not a list", "title ASC", "query.sort_malformed"},
		{"a term that is not an object", []any{"title"}, "query.sort_malformed"},
		{
			"a field that cannot be sorted by",
			[]any{map[string]any{"field": "labels"}}, "query.field_not_sortable",
		},
		{
			"a field nobody serves",
			[]any{map[string]any{"field": "due_at"}}, "query.field_unknown",
		},
		{
			"a direction that is not one",
			[]any{map[string]any{"field": "title", "dir": "RANDOM"}}, "query.sort_direction_unknown",
		},
		{
			"a null placement that is not one",
			[]any{map[string]any{"field": "completed_at", "nulls": "MIDDLE"}}, "query.sort_nulls_unknown",
		},
		{
			"the same field twice",
			[]any{map[string]any{"field": "title"}, map[string]any{"field": "title", "dir": "DESC"}},
			"query.sort_duplicated",
		},
		{
			"more keys than an ordering needs",
			[]any{
				map[string]any{"field": "title"}, map[string]any{"field": "depth"},
				map[string]any{"field": "created_at"}, map[string]any{"field": "updated_at"},
				map[string]any{"field": "completed_at"},
			},
			"query.sort_too_long",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if code := detailOf(t, func() error { _, err := ParseSort(test.raw, "/sort"); return err }()); code != test.code {
				t.Errorf("detail code = %q, want %q", code, test.code)
			}
		})
	}
}

func TestParseGroupBy(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		group, err := ParseGroupBy(nil, "/group_by")
		if err != nil || !group.IsZero() {
			t.Fatalf("%+v, %v", group, err)
		}
	})

	t.Run("the board", func(t *testing.T) {
		group, err := ParseGroupBy(map[string]any{"field": "bucket_id"}, "/group_by")
		if err != nil {
			t.Fatalf("unexpected refusal: %v", err)
		}
		if group.Field.Name != FieldBucketID || group.LimitPerGroup != DefaultGroupSize {
			t.Errorf("read as %+v", group)
		}
	})

	t.Run("a column size beyond the ceiling is clamped, not refused", func(t *testing.T) {
		group, err := ParseGroupBy(
			map[string]any{"field": "bucket_id", "limit_per_group": 5000.0}, "/group_by")
		if err != nil {
			t.Fatalf("unexpected refusal: %v", err)
		}
		if group.LimitPerGroup != MaxGroupSize {
			t.Errorf("limit read as %d", group.LimitPerGroup)
		}
	})

	tests := []struct {
		name string
		raw  any
		code string
	}{
		{"not an object", "bucket_id", "query.group_by_malformed"},
		{"no field", map[string]any{}, "query.field_required"},
		{
			"a field that cannot be grouped by",
			map[string]any{"field": "title"}, "query.field_not_groupable",
		},
		{
			"a column size that is not a number",
			map[string]any{"field": "bucket_id", "limit_per_group": "many"}, "query.value_type_invalid",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseGroupBy(test.raw, "/group_by")
			if code := detailOf(t, err); code != test.code {
				t.Errorf("detail code = %q, want %q", code, test.code)
			}
		})
	}
}

func TestParseCount(t *testing.T) {
	for raw, want := range map[string]CountMode{"": CountNone, "none": CountNone, "exact": CountExact} {
		got, err := ParseCount(raw, "/count")
		if err != nil || got != want {
			t.Errorf("%q read as %q, %v", raw, got, err)
		}
	}

	// Named refusals rather than a silently absent total: a client that asked how large the result
	// is and received nothing cannot tell that from a result of no rows.
	for raw, code := range map[string]string{
		"estimated": "query.count_not_supported", "approximate": "query.count_unknown",
	} {
		_, err := ParseCount(raw, "/count")
		if got := detailOf(t, err); got != code {
			t.Errorf("%q refused with %q, want %q", raw, got, code)
		}
	}
}

func TestSpecValidate(t *testing.T) {
	board, err := ParseGroupBy(map[string]any{"field": "bucket_id"}, "/group_by")
	if err != nil {
		t.Fatalf("unexpected refusal: %v", err)
	}

	spec := Spec{Sort: defaultSort(), GroupBy: board, Cursor: "opaque"}
	if code := detailOf(t, spec.Validate("")); code != "query.cursor_not_grouped" {
		t.Errorf("detail code = %q", code)
	}

	spec.Cursor = ""
	if err := spec.Validate(""); err != nil {
		t.Errorf("a grouped query without a cursor is ordinary: %v", err)
	}
}
