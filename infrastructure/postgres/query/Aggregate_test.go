// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package query

import (
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

func aggregationOf(t *testing.T, raw map[string]any, shapes map[string]view.CustomShape) view.Aggregation {
	t.Helper()
	aggregation, err := view.ParseAggregation(raw, "/aggregate")
	if err != nil {
		t.Fatalf("the grammar refused the aggregation: %v", err)
	}
	aggregation, err = aggregation.Shaped(shapes)
	if err != nil {
		t.Fatalf("the shape refused the aggregation: %v", err)
	}
	return aggregation
}

func TestAnAggregationCompilesToOneGroupedStatement(t *testing.T) {
	number := view.CustomShape{Resolved: true, Defined: true, Column: view.CustomNumber}
	date := view.CustomShape{Resolved: true, Defined: true, Column: view.CustomInstant}

	tests := []struct {
		name   string
		raw    map[string]any
		shapes map[string]view.CustomShape
		want   []string
	}{
		{"a count", map[string]any{"function": "COUNT"}, nil,
			[]string{`SELECT NULL::text, NULL::text, count(*)::bigint, NULL::numeric FROM work_item wi WHERE `}},
		{"a total is not grouped", map[string]any{"function": "SUM", "field": "custom_fields.none"}, nil,
			[]string{`::numeric FROM work_item wi WHERE `}},
		{"a count per status", map[string]any{"function": "COUNT", "group_by": "status"}, nil,
			[]string{`SELECT wi.status::text, NULL::text,`, ` GROUP BY 1, 2 ORDER BY 1 ASC NULLS LAST`}},
		{"a sum per week of completion", map[string]any{"function": "SUM", "field": "custom_fields.hours",
			"bucket": map[string]any{"field": "completed_at", "period": "week"}},
			map[string]view.CustomShape{"hours": number},
			[]string{`date_trunc('week', (wi.completed_at) AT TIME ZONE $`, `sum((SELECT efv.num FROM `}},
		{"a Sunday week", map[string]any{"function": "COUNT",
			"bucket": map[string]any{"field": "created_at", "period": "week", "week_start": "sunday"}}, nil,
			[]string{`date_bin(interval '7 days', (wi.created_at) AT TIME ZONE $`,
				`::text, make_timestamp(2000, 1, 2, 0, 0, 0))::date::text`}},
		{"a lead time", map[string]any{"function": "AVG", "field": "lead_time"}, nil,
			[]string{`avg(div(extract(epoch FROM timestamptz_mi(wi.completed_at, wi.created_at)), 60))::numeric`}},
		{"a scheduled span", map[string]any{"function": "MAX", "field": "scheduled_span"}, nil,
			[]string{`max(div(extract(epoch FROM timestamptz_mi(wi.due_at, wi.start_at)), 60))::numeric`}},
		{"an undefined measure sums to nothing", map[string]any{"function": "AVG", "field": "custom_fields.none"},
			nil, []string{`avg(NULL::numeric)::numeric`}},
		{"a monthly bucket over a custom date", map[string]any{"function": "MAX", "field": "custom_fields.cost",
			"group_by": "custom_fields.team",
			"bucket":   map[string]any{"field": "custom_fields.review", "period": "month"}},
			map[string]view.CustomShape{"cost": number, "review": date,
				"team": {Resolved: true, Defined: true, Column: view.CustomText}},
			[]string{`date_trunc('month', ((SELECT efv.ts FROM `, `max((SELECT efv.num FROM `, `((SELECT efv.text FROM `}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			search := searchOf(t, map[string]any{"field": "is_completed", "op": "EQ", "value": true}, view.Spec{})
			statement, err := Aggregate(search, aggregationOf(t, test.raw, test.shapes), "Europe/Berlin", 5001)
			if err != nil {
				t.Fatalf("compilation failed: %v", err)
			}
			for _, fragment := range test.want {
				if !strings.Contains(statement.SQL, fragment) {
					t.Errorf("missing %q in\n%s", fragment, statement.SQL)
				}
			}
			grouped := strings.Contains(statement.SQL, "GROUP BY")
			if wants := test.raw["group_by"] != nil || test.raw["bucket"] != nil; grouped != wants {
				t.Errorf("grouped %v, want %v:\n%s", grouped, wants, statement.SQL)
			}
			if strings.Count(statement.SQL, "$") != len(statement.Args) {
				t.Errorf("placeholders and arguments disagree:\n%s", statement.SQL)
			}
			if strings.Contains(statement.SQL, "Europe") || strings.Contains(statement.SQL, "hours") {
				t.Errorf("a value reached the text:\n%s", statement.SQL)
			}
			if last := statement.Args[len(statement.Args)-1]; last != int64(5001) {
				t.Errorf("the limit is not the probe: %v", last)
			}
		})
	}
}

func TestAnUnshapedMeasureIsADefect(t *testing.T) {
	aggregation, err := view.ParseAggregation(map[string]any{"function": "SUM", "field": "custom_fields.k"}, "/a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Aggregate(searchOf(t, nil, view.Spec{}), aggregation, "UTC", 10); err == nil {
		t.Fatal("an unshaped measure compiled")
	}
}

func TestAWorkspaceSummaryReadsTheBoundCollections(t *testing.T) {
	aggregation := aggregationOf(t, map[string]any{"function": "COUNT"}, nil)
	search := searchOf(t, nil, view.Spec{})

	search.Anchor = repository.Anchor{Kind: repository.AnchorCollections, CollectionIDs: []shared.ID{collection}}
	statement, err := Aggregate(search, aggregation, "UTC", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(statement.SQL, `(wi.collection_id = ANY($1::uuid[]) OR wi.id = ANY($2::uuid[]))`) ||
		strings.Contains(statement.SQL, collection.String()) {
		t.Fatalf("the collections are not one bound array:\n%s", statement.SQL)
	}

	// An entry shared with the actor alone is a second bound array, never statement text.
	entry := shared.ID("0192f000-0000-7000-8000-0000000000aa")
	search.Anchor.ItemIDs = []shared.ID{entry}
	statement, err = Aggregate(search, aggregation, "UTC", 10)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(statement.SQL, entry.String()) {
		t.Fatalf("the shared entry is statement text:\n%s", statement.SQL)
	}
	search.Anchor.ItemIDs = nil

	search.Anchor.CollectionIDs = nil
	statement, err = Aggregate(search, aggregation, "UTC", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(statement.SQL, `WHERE false`) {
		t.Fatalf("no collection reads something:\n%s", statement.SQL)
	}
}

func TestTheSetAndPrefixOperatorsCompileOverACustomField(t *testing.T) {
	set := view.CustomShape{Resolved: true, Defined: true, Column: view.CustomText, Set: true}
	text := view.CustomShape{Resolved: true, Defined: true, Column: view.CustomText}
	tests := []struct {
		filter map[string]any
		shape  view.CustomShape
		want   []string
	}{
		{map[string]any{"field": "custom_fields.k", "op": "CONTAINS_ANY", "value": []any{"a", "b"}}, set,
			[]string{`jsonb_build_array(to_jsonb(normalize($`, `)) OR wi.custom_fields @> `}},
		{map[string]any{"field": "custom_fields.k", "op": "CONTAINS_ALL", "value": []any{"a", "b"}}, set,
			[]string{`)) AND wi.custom_fields @> `}},
		{map[string]any{"field": "custom_fields.k", "op": "STARTS_WITH", "value": "INV"}, text,
			[]string{`(starts_with(lower(jsonb_extract_path_text(wi.custom_fields, $`}},
	}
	for _, test := range tests {
		search := searchOf(t, test.filter, view.Spec{})
		spec, err := search.Spec.Shaped(map[string]view.CustomShape{"k": test.shape})
		if err != nil {
			t.Fatalf("refused: %v", err)
		}
		search.Spec = spec
		statement, err := Rows(search, Boundary{}, 51)
		if err != nil {
			t.Fatalf("%v: %v", test.filter, err)
		}
		for _, fragment := range append(test.want, `EXISTS (SELECT 1 FROM custom_field_definition`) {
			if !strings.Contains(statement.SQL, fragment) {
				t.Errorf("missing %q in\n%s", fragment, statement.SQL)
			}
		}
		if strings.Contains(statement.SQL, "INV") {
			t.Errorf("a value reached the text:\n%s", statement.SQL)
		}
	}
}

// A links summary counts a link only where the query keeps both its ends: the target is held to
// the same predicates, every value bound again, and the type is the one grouping of its own.
func TestALinksSummaryCountsLinksWithBothEndsKept(t *testing.T) {
	search := searchOf(t, map[string]any{"field": "is_completed", "op": "EQ", "value": false}, view.Spec{})
	search.RestrictTo = []shared.ID{shared.MustParseID("0192f000-0000-7000-8000-00000000000f")}
	aggregation := aggregationOf(t, map[string]any{"source": "links", "function": "COUNT", "group_by": "link_type"}, nil)

	statement, err := Aggregate(search, aggregation, "UTC", 5001)
	if err != nil {
		t.Fatalf("compilation failed: %v", err)
	}
	for _, fragment := range []string{
		`SELECT lk.link_type, NULL::text, count(*)::bigint, NULL::numeric FROM work_item wi JOIN entry_link lk ON lk.tenant_id = wi.tenant_id AND lk.source_id = wi.id WHERE `,
		` AND lk.target_id IN (SELECT wi.id FROM work_item wi WHERE `,
		` GROUP BY 1, 2`,
	} {
		if !strings.Contains(statement.SQL, fragment) {
			t.Errorf("missing %q in\n%s", fragment, statement.SQL)
		}
	}
	if got := strings.Count(statement.SQL, "wi.id = ANY("); got != 2 {
		t.Errorf("the restriction holds %d ends, want both:\n%s", got, statement.SQL)
	}
	if strings.Count(statement.SQL, "$") != len(statement.Args) {
		t.Errorf("placeholders and arguments disagree:\n%s", statement.SQL)
	}

	entries, err := Aggregate(search, aggregationOf(t, map[string]any{"function": "COUNT"}, nil), "UTC", 5001)
	if err != nil {
		t.Fatalf("compilation failed: %v", err)
	}
	if strings.Contains(entries.SQL, "entry_link") {
		t.Errorf("an entries summary reads links:\n%s", entries.SQL)
	}
}

// UC-GOL-04 check 11: cycle time, WIP age and time in status read the status history, with the
// started statuses and the moment bound - never statement text.
func TestAFlowQuantityReadsTheStatusHistory(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		quantity string
		want     []string
	}{
		{"cycle_time", []string{`avg(div(extract(epoch FROM timestamptz_mi(wi.completed_at, (SELECT min(sc.entered_at) FROM entry_status_change sc WHERE sc.tenant_id = wi.tenant_id AND sc.item_id = wi.id AND sc.status = ANY($`}},
		{"wip_age", []string{`CASE WHEN wi.completed_at IS NULL THEN div(extract(epoch FROM timestamptz_mi($`, `::timestamptz, (SELECT min(sc.entered_at)`}},
		{"time_in_status", []string{`(SELECT max(sc.entered_at) FROM entry_status_change sc WHERE sc.tenant_id = wi.tenant_id AND sc.item_id = wi.id AND sc.status = wi.status)`}},
	}
	for _, test := range tests {
		t.Run(test.quantity, func(t *testing.T) {
			aggregation := aggregationOf(t, map[string]any{"function": "AVG", "field": test.quantity}, nil)
			aggregation.Started, aggregation.Now = []string{"in_progress", "x'); DROP TABLE work_item; --"}, now
			statement, err := Aggregate(searchOf(t, nil, view.Spec{}), aggregation, "UTC", 10)
			if err != nil {
				t.Fatal(err)
			}
			for _, fragment := range test.want {
				if !strings.Contains(statement.SQL, fragment) {
					t.Errorf("missing %q in\n%s", fragment, statement.SQL)
				}
			}
			if strings.Contains(statement.SQL, "in_progress") || strings.Contains(statement.SQL, "DROP") {
				t.Errorf("a status reached the text:\n%s", statement.SQL)
			}
			if strings.Count(statement.SQL, "$") != len(statement.Args) {
				t.Errorf("placeholders and arguments disagree:\n%s", statement.SQL)
			}
		})
	}
}
