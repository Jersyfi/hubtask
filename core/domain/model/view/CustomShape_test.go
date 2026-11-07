// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package view_test

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

func TestTheKindsDecideTheColumn(t *testing.T) {
	cases := []struct {
		name  string
		kinds []work.CustomFieldKind
		want  view.CustomShape
	}{
		{"nothing defines it", nil, view.CustomShape{Resolved: true}},
		{"a number", []work.CustomFieldKind{work.CustomFieldNumber},
			view.CustomShape{Resolved: true, Defined: true, Column: view.CustomNumber}},
		{"money and a duration agree", []work.CustomFieldKind{work.CustomFieldMoney, work.CustomFieldDuration},
			view.CustomShape{Resolved: true, Defined: true, Column: view.CustomNumber}},
		{"a calculated number", []work.CustomFieldKind{work.CustomFieldCalculated},
			view.CustomShape{Resolved: true, Defined: true, Column: view.CustomNumber}},
		{"a date", []work.CustomFieldKind{work.CustomFieldDate},
			view.CustomShape{Resolved: true, Defined: true, Column: view.CustomInstant}},
		{"several choices", []work.CustomFieldKind{work.CustomFieldMultiSelect},
			view.CustomShape{Resolved: true, Defined: true, Column: view.CustomText, Set: true}},
		{"people", []work.CustomFieldKind{work.CustomFieldUserSet},
			view.CustomShape{Resolved: true, Defined: true, Column: view.CustomRef, Set: true}},
		{"a number and a text disagree", []work.CustomFieldKind{work.CustomFieldNumber, work.CustomFieldText},
			view.CustomShape{Resolved: true, Defined: true, Ambiguous: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := view.ShapeOf(c.kinds); got != c.want {
				t.Errorf("the shape is %+v, want %+v", got, c.want)
			}
		})
	}
}

// UC-WRK-17 check 10: what has no order is refused by name before anything compiles - a range over
// people or prose, a sort or a grouping by several values, and any of them over a key whose kinds
// disagree; a range over a date reads its bounds as days.
func TestTheShapeHoldsARangeASortAndAGrouping(t *testing.T) {
	people := view.ShapeOf([]work.CustomFieldKind{work.CustomFieldUserSet})
	prose := view.ShapeOf([]work.CustomFieldKind{work.CustomFieldRichText})
	day := view.ShapeOf([]work.CustomFieldKind{work.CustomFieldDate})
	ambiguous := view.ShapeOf([]work.CustomFieldKind{work.CustomFieldNumber, work.CustomFieldDate})

	cases := []struct {
		name   string
		filter any
		sort   any
		group  any
		shape  view.CustomShape
		detail string
	}{
		{"a range over people", map[string]any{"field": "custom_fields.f", "op": "GT", "value": "x"}, nil, nil,
			people, "query.operator_unsupported"},
		{"a range over prose", map[string]any{"field": "custom_fields.f", "op": "LT", "value": "x"}, nil, nil,
			prose, "query.operator_unsupported"},
		{"a range over a disagreeing key", map[string]any{"field": "custom_fields.f", "op": "LT", "value": 1}, nil, nil,
			ambiguous, "query.operator_unsupported"},
		{"a date that is no day", map[string]any{"field": "custom_fields.f", "op": "LT", "value": "soon"}, nil, nil,
			day, "query.value_type_invalid"},
		{"a sort by people", nil, []any{map[string]any{"field": "custom_fields.f"}}, nil,
			people, "query.field_not_sortable"},
		{"a grouping by prose", nil, nil, map[string]any{"field": "custom_fields.f"},
			prose, "query.field_not_groupable"},
		{"a grouping by people", nil, nil, map[string]any{"field": "custom_fields.f"},
			people, "query.field_not_groupable"},
		{"equality over people stays", map[string]any{"field": "custom_fields.f", "op": "IS_NULL"}, nil, nil,
			people, ""},
		{"a range over a day", map[string]any{"field": "custom_fields.f", "op": "BETWEEN",
			"value": []any{"2026-01-01", "2026-12-31"}}, nil, nil, day, ""},
		{"a sort by a key nothing defines", nil, []any{map[string]any{"field": "custom_fields.f"}}, nil,
			view.ShapeOf(nil), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var spec view.Spec
			var err error
			if c.filter != nil {
				if spec.Filter, err = view.ParseFilter(c.filter, "/filter"); err != nil {
					t.Fatalf("the grammar refused the filter: %v", err)
				}
			}
			if spec.Sort, err = view.ParseSort(c.sort, "/sort"); err != nil {
				t.Fatalf("the grammar refused the sort: %v", err)
			}
			if spec.GroupBy, err = view.ParseGroupBy(c.group, "/group_by"); err != nil {
				t.Fatalf("the grammar refused the grouping: %v", err)
			}
			if keys := spec.CustomKeys(); len(keys) != 1 || keys[0] != "f" {
				t.Fatalf("the keys are %v", keys)
			}

			shaped, err := spec.Shaped(map[string]view.CustomShape{"f": c.shape})
			if c.detail != "" {
				if detail := shared.AsError(err).DetailCode; detail != c.detail {
					t.Fatalf("detail %q, want %s", detail, c.detail)
				}
				return
			}
			if err != nil {
				t.Fatalf("refused: %v", err)
			}
			if c.shape == day && shaped.Filter.Values[0].Kind != view.KindTimestamp {
				t.Errorf("the bound is %+v", shaped.Filter.Values[0])
			}
		})
	}
}

// UC-WRK-17 check 10: a moment ranges and sorts by its instant, never beside a day under one key; an
// address ranges and sorts by the address and compares in its canonical spelling; a group and a
// file are identifiers with no order; labels are a set; a two-level choice and a state are text.
func TestTheRegisterKindsHaveTheirShape(t *testing.T) {
	moment := view.ShapeOf([]work.CustomFieldKind{work.CustomFieldDateTime})
	address := view.ShapeOf([]work.CustomFieldKind{work.CustomFieldIPAddress})
	if moment != (view.CustomShape{Resolved: true, Defined: true, Column: view.CustomInstant, Moment: true}) {
		t.Errorf("a moment is %+v", moment)
	}
	if address != (view.CustomShape{Resolved: true, Defined: true, Column: view.CustomAddress}) {
		t.Errorf("an address is %+v", address)
	}
	if got := view.ShapeOf([]work.CustomFieldKind{work.CustomFieldDate, work.CustomFieldDateTime}); !got.Ambiguous {
		t.Errorf("a day and a moment under one key are %+v", got)
	}
	for kind, want := range map[work.CustomFieldKind]view.CustomShape{
		work.CustomFieldGroup:           {Resolved: true, Defined: true, Column: view.CustomRef},
		work.CustomFieldFile:            {Resolved: true, Defined: true, Column: view.CustomRef},
		work.CustomFieldLabel:           {Resolved: true, Defined: true, Column: view.CustomText, Set: true},
		work.CustomFieldCascadingSelect: {Resolved: true, Defined: true, Column: view.CustomText},
		work.CustomFieldStatus:          {Resolved: true, Defined: true, Column: view.CustomText},
	} {
		if got := view.ShapeOf([]work.CustomFieldKind{kind}); got != want {
			t.Errorf("%s is %+v, want %+v", kind, got, want)
		}
	}

	shape := func(t *testing.T, filter any, sort any, with view.CustomShape) (view.Spec, error) {
		t.Helper()
		var spec view.Spec
		var err error
		if filter != nil {
			if spec.Filter, err = view.ParseFilter(filter, "/filter"); err != nil {
				t.Fatalf("the grammar refused the filter: %v", err)
			}
		}
		if spec.Sort, err = view.ParseSort(sort, "/sort"); err != nil {
			t.Fatalf("the grammar refused the sort: %v", err)
		}
		return spec.Shaped(map[string]view.CustomShape{"f": with})
	}

	t.Run("a moment's bound is an instant", func(t *testing.T) {
		shaped, err := shape(t, map[string]any{"field": "custom_fields.f", "op": "GTE",
			"value": "2026-10-31T10:00:00+01:00"}, nil, moment)
		if err != nil {
			t.Fatal(err)
		}
		if got := shaped.Filter.Values[0]; got.Kind != view.KindTimestamp || got.Time.UTC().Hour() != 9 {
			t.Errorf("the bound is %+v", got)
		}
	})
	t.Run("a day is no moment's bound", func(t *testing.T) {
		_, err := shape(t, map[string]any{"field": "custom_fields.f", "op": "LT", "value": "2026-10-31"}, nil, moment)
		if detail := shared.AsError(err).DetailCode; detail != "query.value_type_invalid" {
			t.Fatalf("detail %q", detail)
		}
	})
	t.Run("an address ranges and sorts", func(t *testing.T) {
		if _, err := shape(t, map[string]any{"field": "custom_fields.f", "op": "BETWEEN",
			"value": []any{"10.0.0.0", "10.0.0.255"}}, []any{map[string]any{"field": "custom_fields.f"}}, address); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("a host name is no address's bound", func(t *testing.T) {
		_, err := shape(t, map[string]any{"field": "custom_fields.f", "op": "GT", "value": "intranet"}, nil, address)
		if detail := shared.AsError(err).DetailCode; detail != "query.value_type_invalid" {
			t.Fatalf("detail %q", detail)
		}
	})
	t.Run("an address compares in its canonical spelling", func(t *testing.T) {
		shaped, err := shape(t, map[string]any{"field": "custom_fields.f", "op": "IN",
			"value": []any{"2001:DB8:0:0:0:0:0:1", "10.0.0.1"}}, nil, address)
		if err != nil {
			t.Fatal(err)
		}
		if got := shaped.Filter.Values[0].Text; got != "2001:db8::1" {
			t.Errorf("the value compared is %q", got)
		}
	})
	t.Run("an address takes a prefix", func(t *testing.T) {
		if _, err := shape(t, map[string]any{"field": "custom_fields.f", "op": "STARTS_WITH", "value": "10.0."}, nil, address); err != nil {
			t.Fatal(err)
		}
	})
}
