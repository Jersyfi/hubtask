// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package query

import (
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

var (
	numberShape = view.CustomShape{Resolved: true, Defined: true, Column: view.CustomNumber}
	dayShape    = view.CustomShape{Resolved: true, Defined: true, Column: view.CustomInstant}
	textShape   = view.CustomShape{Resolved: true, Defined: true, Column: view.CustomText}
)

func shapedSearch(t *testing.T, filter any, spec view.Spec, shapes map[string]view.CustomShape) view.Spec {
	t.Helper()
	search := searchOf(t, filter, spec)
	shaped, err := search.Spec.Shaped(shapes)
	if err != nil {
		t.Fatalf("the shape refused the query: %v", err)
	}
	return shaped
}

// UC-WRK-17 check 10: a range over a custom field compares its typed value, by the column its kind
// is stored in, with the key a parameter and never text (ADR-0026).
func TestARangeOverACustomFieldReadsTheTypedValue(t *testing.T) {
	cases := []struct {
		name     string
		document string
		shape    view.CustomShape
		fragment string
		arg      any
	}{
		{"a number", `{"field":"custom_fields.budget","op":"GT","value":1000}`, numberShape,
			`efv.num > $3::numeric)`, "1000"},
		{"a date as a day", `{"field":"custom_fields.budget","op":"LT","value":"2026-10-01"}`, dayShape,
			`efv.ts < $3::timestamptz)`, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
		{"a text", `{"field":"custom_fields.budget","op":"GTE","value":"m"}`, textShape,
			`efv.text >= normalize($3::text, NFC))`, "m"},
		{"a key nothing defines", `{"field":"custom_fields.budget","op":"GT","value":1}`,
			view.CustomShape{Resolved: true}, `(false)`, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spec := shapedSearch(t, documentOf(t, c.document), view.Spec{},
				map[string]view.CustomShape{"budget": c.shape})
			search := searchOf(t, nil, view.Spec{})
			search.Spec = spec
			statement, err := Rows(search, Boundary{}, 51)
			if err != nil {
				t.Fatalf("compilation failed: %v", err)
			}
			if !strings.Contains(statement.SQL, c.fragment) {
				t.Fatalf("the statement is %s", statement.SQL)
			}
			if c.arg != nil && !containsArg(statement.Args, c.arg) {
				t.Errorf("%#v is not among %#v", c.arg, statement.Args)
			}
			if strings.Contains(statement.SQL, "budget") {
				t.Errorf("the key reached the SQL text: %s", statement.SQL)
			}
		})
	}
}

// UC-WRK-17 check 10: a custom field sorts by its typed value, nulls last, and the walk continues
// from a cursor key of the same type.
func TestASortByACustomFieldContinuesFromItsCursor(t *testing.T) {
	sort, err := view.ParseSort([]any{map[string]any{"field": "custom_fields.budget", "dir": "DESC"}}, "/sort")
	if err != nil {
		t.Fatal(err)
	}
	spec := shapedSearch(t, nil, view.Spec{Sort: sort}, map[string]view.CustomShape{"budget": numberShape})
	search := searchOf(t, nil, view.Spec{})
	search.Spec = spec

	statement, err := Rows(search, Boundary{Keys: []string{keyValue + "12.5"}, ID: task}, 51)
	if err != nil {
		t.Fatalf("compilation failed: %v", err)
	}
	for _, want := range []string{
		`(SELECT efv.num FROM entry_field_value efv JOIN custom_field_definition cfd`,
		`AND efv.ordinal = 0) DESC NULLS LAST, wi.id LIMIT`,
		`::numeric IS NULL THEN false`,
	} {
		if !strings.Contains(statement.SQL, want) {
			t.Errorf("compiled to\n  %s\nand should contain\n  %s", statement.SQL, want)
		}
	}
	if !containsArg(statement.Args, "12.5") {
		t.Errorf("the boundary is not bound: %#v", statement.Args)
	}

	item := work.WorkItem{CustomFields: map[string]any{"budget": 12.5}}
	if key := Key(spec.Sort[0], item); key != keyValue+"12.5" {
		t.Errorf("the cursor key is %q", key)
	}
	if key := Key(spec.Sort[0], work.WorkItem{}); key != keyNull {
		t.Errorf("an entry without the value keys as %q", key)
	}
}

// UC-WRK-17 check 10: a board groups by a custom field's value; the inner select carries the value
// under an alias the outer one orders by, and the count keys each group by the same spelling.
func TestAGroupingByACustomFieldIsOneWindowedQuery(t *testing.T) {
	group, err := view.ParseGroupBy(map[string]any{"field": "custom_fields.priority"}, "/group_by")
	if err != nil {
		t.Fatal(err)
	}
	spec := shapedSearch(t, nil, view.Spec{GroupBy: group}, map[string]view.CustomShape{"priority": textShape})
	search := searchOf(t, nil, view.Spec{})
	search.Spec = spec

	statement, err := Groups(search, 51)
	if err != nil {
		t.Fatalf("compilation failed: %v", err)
	}
	for _, want := range []string{
		`AND efv.ordinal = 0) AS gk, row_number() OVER (PARTITION BY (SELECT efv.text`,
		` ORDER BY gk ASC NULLS LAST, order_key COLLATE "C" ASC, id`,
	} {
		if !strings.Contains(statement.SQL, want) {
			t.Errorf("compiled to\n  %s\nand should contain\n  %s", statement.SQL, want)
		}
	}

	counted, err := Count(search)
	if err != nil {
		t.Fatalf("the count failed: %v", err)
	}
	if !strings.Contains(counted.SQL, `AND efv.ordinal = 0))::text, count(*)::bigint`) {
		t.Errorf("the count is %s", counted.SQL)
	}

	day := group
	day.Field.Custom = dayShape
	item := work.WorkItem{CustomFields: map[string]any{"priority": "2026-10-01"}}
	if key, _ := GroupKey(day.Field, item); key != "2026-10-01" {
		t.Errorf("a day groups as %q", key)
	}
	ref := group
	ref.Field.Custom = view.CustomShape{Resolved: true, Defined: true, Column: view.CustomRef}
	upper := "0192F000-0000-7000-8000-0000000000A1"
	item = work.WorkItem{CustomFields: map[string]any{"priority": upper}}
	if key, _ := GroupKey(ref.Field, item); key != shared.MustParseID(upper).String() {
		t.Errorf("a person groups as %q", key)
	}
}

// A custom field the use case did not shape is a defect, not a query.
func TestAnUnshapedCustomSortIsADefect(t *testing.T) {
	sort, err := view.ParseSort([]any{map[string]any{"field": "custom_fields.budget"}}, "/sort")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Rows(searchOf(t, nil, view.Spec{Sort: sort}), Boundary{}, 51)
	if detail := shared.AsError(err).DetailCode; detail != "query.custom_field_unshaped" {
		t.Fatalf("detail %q", detail)
	}
}
