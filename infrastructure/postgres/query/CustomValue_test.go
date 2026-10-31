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
		`AND efv.ordinal = 0) AS gk FROM work_item wi LEFT JOIN work_item_retention rt ON rt.tenant_id = wi.tenant_id AND rt.item_id = wi.id WHERE wi.id = ANY (`,
		`SELECT wi.id, row_number() OVER (PARTITION BY (SELECT efv.text`,
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
	person := "0192f000-0000-7000-8000-0000000000a1"
	item = work.WorkItem{CustomFields: map[string]any{"priority": person}}
	if key, _ := GroupKey(ref.Field, item); key != shared.MustParseID(person).String() {
		t.Errorf("a person groups as %q", key)
	}
	// An identifier is lower case on the wire (shared.ParseID); anything else groups nowhere.
	item = work.WorkItem{CustomFields: map[string]any{"priority": strings.ToUpper(person)}}
	if key, grouped := GroupKey(ref.Field, item); grouped {
		t.Errorf("an upper-case identifier groups as %q", key)
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

var (
	momentShape  = view.CustomShape{Resolved: true, Defined: true, Column: view.CustomInstant, Moment: true}
	addressShape = view.CustomShape{Resolved: true, Defined: true, Column: view.CustomAddress}
)

// UC-WRK-17 check 10: a moment ranges over its instant, an address over `addr` as an address, and
// both sort from a cursor key of their own type.
func TestAMomentAndAnAddressRangeAndSortByTheirValue(t *testing.T) {
	cases := []struct {
		name     string
		document string
		shape    view.CustomShape
		fragment string
		arg      any
	}{
		{"a moment", `{"field":"custom_fields.seen","op":"GTE","value":"2026-10-31T10:00:00+01:00"}`, momentShape,
			`efv.ts >= $3::timestamptz)`, time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)},
		{"an address", `{"field":"custom_fields.seen","op":"BETWEEN","value":["10.0.0.0","10.0.0.255"]}`, addressShape,
			`efv.addr BETWEEN $3::inet AND $4::inet)`, "10.0.0.255"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spec := shapedSearch(t, documentOf(t, c.document), view.Spec{}, map[string]view.CustomShape{"seen": c.shape})
			search := searchOf(t, nil, view.Spec{})
			search.Spec = spec
			statement, err := Rows(search, Boundary{}, 51)
			if err != nil {
				t.Fatalf("compilation failed: %v", err)
			}
			if !strings.Contains(statement.SQL, c.fragment) {
				t.Fatalf("the statement is %s", statement.SQL)
			}
			if !containsArg(statement.Args, c.arg) {
				t.Errorf("%#v is not among %#v", c.arg, statement.Args)
			}
		})
	}

	sort, err := view.ParseSort([]any{map[string]any{"field": "custom_fields.seen"}}, "/sort")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		shape  view.CustomShape
		value  string
		key    string
		cursor string
	}{
		{momentShape, "2026-10-31T09:00:00Z", "2026-10-31T09:00:00Z", `::timestamptz IS NULL THEN false`},
		{addressShape, "10.0.0.7", "10.0.0.7", `::inet IS NULL THEN false`},
	} {
		spec := shapedSearch(t, nil, view.Spec{Sort: sort}, map[string]view.CustomShape{"seen": c.shape})
		item := work.WorkItem{CustomFields: map[string]any{"seen": c.value}}
		if key := Key(spec.Sort[0], item); key != keyValue+c.key {
			t.Errorf("the cursor key of %s is %q", c.value, key)
		}
		search := searchOf(t, nil, view.Spec{})
		search.Spec = spec
		statement, err := Rows(search, Boundary{Keys: []string{keyValue + c.key}, ID: task}, 51)
		if err != nil {
			t.Fatalf("compilation failed: %v", err)
		}
		if !strings.Contains(statement.SQL, c.cursor) {
			t.Errorf("compiled to\n  %s\nand should contain\n  %s", statement.SQL, c.cursor)
		}
	}

	group, err := view.ParseGroupBy(map[string]any{"field": "custom_fields.seen"}, "/group_by")
	if err != nil {
		t.Fatal(err)
	}
	spec := shapedSearch(t, nil, view.Spec{GroupBy: group}, map[string]view.CustomShape{"seen": addressShape})
	if key, _ := GroupKey(spec.GroupBy.Field, work.WorkItem{CustomFields: map[string]any{"seen": "10.0.0.7"}}); key != "10.0.0.7" {
		t.Errorf("an address groups as %q", key)
	}
	search := searchOf(t, nil, view.Spec{})
	search.Spec = spec
	counted, err := Count(search)
	if err != nil {
		t.Fatalf("the count failed: %v", err)
	}
	if !strings.Contains(counted.SQL, `host((SELECT efv.addr`) {
		t.Errorf("an address is counted as %s", counted.SQL)
	}
}
