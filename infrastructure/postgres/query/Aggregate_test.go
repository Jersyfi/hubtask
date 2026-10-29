// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package query

import (
	"strings"
	"testing"

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
