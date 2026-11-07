// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestAnAggregationIsReadFromTheClosedVocabulary(t *testing.T) {
	tests := []struct {
		name string
		raw  any
		code string
	}{
		{"a count", map[string]any{"function": "COUNT"}, ""},
		{"a count per status", map[string]any{"function": "COUNT", "group_by": "status"}, ""},
		{"a sum of a custom number", map[string]any{"function": "SUM", "field": "custom_fields.budget"}, ""},
		{"a sum per week", map[string]any{"function": "SUM", "field": "custom_fields.hours",
			"bucket": map[string]any{"field": "completed_at", "period": "week"}}, ""},
		{"a bucket over a custom date", map[string]any{"function": "COUNT",
			"bucket": map[string]any{"field": "custom_fields.review", "period": "month"}}, ""},
		{"a lead time per month", map[string]any{"function": "AVG", "field": "lead_time",
			"bucket": map[string]any{"field": "completed_at", "period": "month"}}, ""},
		{"a scheduled span", map[string]any{"function": "SUM", "field": "scheduled_span"}, ""},
		// UC-GOL-04 check 12: the percentiles beside the mean.
		{"a cycle time's 85th percentile", map[string]any{"function": "P85", "field": "cycle_time"}, ""},
		{"a percentile of nothing", map[string]any{"function": "P50"}, "query.field_required"},
		{"a week from Sunday", map[string]any{"function": "COUNT",
			"bucket": map[string]any{"field": "created_at", "period": "week", "week_start": "sunday"}}, ""},
		{"a week start on a month", map[string]any{"function": "COUNT",
			"bucket": map[string]any{"field": "created_at", "period": "month", "week_start": "sunday"}},
			"query.week_start_unexpected"},
		{"an unknown week start", map[string]any{"function": "COUNT",
			"bucket": map[string]any{"field": "created_at", "period": "week", "week_start": "friday"}},
			"query.week_start_unknown"},
		{"a count of a quantity", map[string]any{"function": "COUNT", "field": "lead_time"},
			"query.aggregate_field_unexpected"},
		{"not a document", "COUNT", "query.aggregate_malformed"},
		{"an unknown function", map[string]any{"function": "MEDIAN"}, "query.aggregate_function_unknown"},
		{"a function in lower case", map[string]any{"function": "sum", "field": "custom_fields.x"},
			"query.aggregate_function_unknown"},
		{"a count of a field", map[string]any{"function": "COUNT", "field": "custom_fields.x"},
			"query.aggregate_field_unexpected"},
		{"a sum without a field", map[string]any{"function": "SUM"}, "query.field_required"},
		{"a sum of a column", map[string]any{"function": "SUM", "field": "depth"}, "query.field_not_aggregatable"},
		{"a sum of an unknown field", map[string]any{"function": "SUM", "field": "price"}, "query.field_unknown"},
		{"a grouping by a title", map[string]any{"function": "COUNT", "group_by": "title"},
			"query.field_not_groupable"},
		{"an unknown period", map[string]any{"function": "COUNT",
			"bucket": map[string]any{"field": "created_at", "period": "fortnight"}}, "query.bucket_period_unknown"},
		{"a bucket over a non-time field", map[string]any{"function": "COUNT",
			"bucket": map[string]any{"field": "depth", "period": "day"}}, "query.field_not_bucketable"},
		{"a malformed bucket", map[string]any{"function": "COUNT", "bucket": "day"}, "query.aggregate_malformed"},
		{"a count of links", map[string]any{"source": "links", "function": "COUNT"}, ""},
		{"links per type", map[string]any{"source": "links", "function": "COUNT", "group_by": "link_type"}, ""},
		{"links per week of their source", map[string]any{"source": "links", "function": "COUNT",
			"group_by": "status", "bucket": map[string]any{"field": "created_at", "period": "week"}}, ""},
		{"entries named as the source", map[string]any{"source": "entries", "function": "COUNT"}, ""},
		{"an unknown source", map[string]any{"source": "records", "function": "COUNT"},
			"query.aggregate_source_unknown"},
		{"pages per verification and month", map[string]any{"source": "pages", "function": "COUNT",
			"group_by": "verification", "bucket": map[string]any{"field": "updated_at", "period": "month"}}, ""},
		{"pages per owner", map[string]any{"source": "pages", "function": "COUNT", "group_by": "owner"}, ""},
		{"a sum of pages", map[string]any{"source": "pages", "function": "SUM", "field": "lead_time"},
			"query.aggregate_pages_count_only"},
		{"pages per status", map[string]any{"source": "pages", "function": "COUNT", "group_by": "status"},
			"query.field_not_groupable"},
		{"pages per due date", map[string]any{"source": "pages", "function": "COUNT",
			"bucket": map[string]any{"field": "due_at", "period": "day"}}, "query.field_not_bucketable"},
		{"entries per verification", map[string]any{"function": "COUNT", "group_by": "verification"},
			"query.field_unknown"},
		{"a sum of links", map[string]any{"source": "links", "function": "SUM", "field": "lead_time"},
			"query.aggregate_links_count_only"},
		{"entries per link type", map[string]any{"function": "COUNT", "group_by": "link_type"},
			"query.field_unknown"},
		{"the minutes recorded", map[string]any{"source": "time", "function": "SUM", "field": "minutes"}, ""},
		{"the records per person and week of their day", map[string]any{"source": "time", "function": "COUNT",
			"group_by": "person", "bucket": map[string]any{"field": "day", "period": "week"}}, ""},
		{"the minutes per kind of work", map[string]any{"source": "time", "function": "AVG", "field": "minutes",
			"group_by": "work_type"}, ""},
		{"the minutes per status of their entry", map[string]any{"source": "time", "function": "SUM",
			"field": "minutes", "group_by": "status"}, ""},
		{"an entry's measure over time", map[string]any{"source": "time", "function": "SUM", "field": "lead_time"},
			"query.aggregate_time_minutes_only"},
		{"minutes of entries", map[string]any{"function": "SUM", "field": "minutes"}, "query.field_unknown"},
		{"entries per person", map[string]any{"function": "COUNT", "group_by": "person"}, "query.field_unknown"},
		{"entries per record day", map[string]any{"function": "COUNT",
			"bucket": map[string]any{"field": "day", "period": "day"}}, "query.field_unknown"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseAggregation(test.raw, "/aggregate")
			if test.code == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			var refusal *shared.Error
			if !errors.As(err, &refusal) || refusal.DetailCode != test.code {
				t.Fatalf("got %v, want %s", err, test.code)
			}
		})
	}
}

func TestAnAggregationIsShapedByItsDefinitions(t *testing.T) {
	number := CustomShape{Resolved: true, Defined: true, Column: CustomNumber}
	date := CustomShape{Resolved: true, Defined: true, Column: CustomInstant}
	text := CustomShape{Resolved: true, Defined: true, Column: CustomText}
	set := CustomShape{Resolved: true, Defined: true, Column: CustomNumber, Set: true}

	tests := []struct {
		name   string
		raw    map[string]any
		shapes map[string]CustomShape
		code   string
	}{
		{"a number is summed", map[string]any{"function": "SUM", "field": "custom_fields.k"},
			map[string]CustomShape{"k": number}, ""},
		{"an undefined key sums to nothing", map[string]any{"function": "SUM", "field": "custom_fields.k"},
			nil, ""},
		{"text is not summed", map[string]any{"function": "SUM", "field": "custom_fields.k"},
			map[string]CustomShape{"k": text}, "query.field_not_aggregatable"},
		{"a set is not summed", map[string]any{"function": "AVG", "field": "custom_fields.k"},
			map[string]CustomShape{"k": set}, "query.field_not_aggregatable"},
		{"a date buckets", map[string]any{"function": "COUNT",
			"bucket": map[string]any{"field": "custom_fields.k", "period": "day"}},
			map[string]CustomShape{"k": date}, ""},
		{"a number does not bucket", map[string]any{"function": "COUNT",
			"bucket": map[string]any{"field": "custom_fields.k", "period": "day"}},
			map[string]CustomShape{"k": number}, "query.field_not_bucketable"},
		{"a set does not group", map[string]any{"function": "COUNT", "group_by": "custom_fields.k"},
			map[string]CustomShape{"k": set}, "query.field_not_groupable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			aggregation, err := ParseAggregation(test.raw, "/aggregate")
			if err != nil {
				t.Fatalf("refused: %v", err)
			}
			shaped, err := aggregation.Shaped(test.shapes)
			if test.code == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				for _, field := range []Field{shaped.Measure, shaped.GroupBy, shaped.BucketField} {
					if _, custom := CustomField(field.Name); custom && !field.Custom.Resolved {
						t.Errorf("%s left unshaped", field.Name)
					}
				}
				return
			}
			var refusal *shared.Error
			if !errors.As(err, &refusal) || refusal.DetailCode != test.code {
				t.Fatalf("got %v, want %s", err, test.code)
			}
		})
	}
}

func TestAWeekStartsOnMondayUnlessSaidOtherwise(t *testing.T) {
	tests := []struct {
		bucket map[string]any
		want   WeekStart
	}{
		{map[string]any{"field": "created_at", "period": "week"}, WeekMonday},
		{map[string]any{"field": "created_at", "period": "week", "week_start": "sunday"}, WeekSunday},
		{map[string]any{"field": "created_at", "period": "day"}, ""},
	}
	for _, test := range tests {
		aggregation, err := ParseAggregation(map[string]any{"function": "COUNT", "bucket": test.bucket}, "/a")
		if err != nil {
			t.Fatalf("refused: %v", err)
		}
		if aggregation.WeekStart != test.want {
			t.Errorf("%v: week starts %q, want %q", test.bucket, aggregation.WeekStart, test.want)
		}
	}
}

func TestABuiltInQuantityNeedsNoDefinition(t *testing.T) {
	aggregation, err := ParseAggregation(map[string]any{"function": "AVG", "field": "lead_time"}, "/a")
	if err != nil {
		t.Fatal(err)
	}
	if aggregation.Quantity != QuantityLeadTime || aggregation.Measure.Name != "" {
		t.Fatalf("got %+v", aggregation)
	}
	if keys := aggregation.CustomKeys(); len(keys) != 0 {
		t.Fatalf("a quantity asked for definitions: %v", keys)
	}
	if _, err := aggregation.Shaped(nil); err != nil {
		t.Fatalf("a quantity was refused by the shapes: %v", err)
	}
}

func TestASummaryScopeNamesTheWorkspaceExplicitly(t *testing.T) {
	collection := shared.MustParseID("0192f000-0000-7000-8000-000000000001")
	tests := []struct {
		name      string
		container shared.ID
		workspace bool
		code      string
	}{
		{"the workspace", "", true, ""},
		{"a collection", collection, false, ""},
		{"neither", "", false, "query.scope_required"},
		{"both", collection, true, "query.scope_ambiguous"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scope, err := ParseSummaryScope(test.container, "", true, test.workspace, "/scope")
			if test.code == "" {
				if err != nil || scope.Workspace != test.workspace {
					t.Fatalf("got %+v, %v", scope, err)
				}
				return
			}
			var refusal *shared.Error
			if !errors.As(err, &refusal) || refusal.DetailCode != test.code {
				t.Fatalf("got %v, want %s", err, test.code)
			}
		})
	}
}

func TestTheSetAndPrefixOperatorsAreHeldToTheShape(t *testing.T) {
	set := CustomShape{Resolved: true, Defined: true, Column: CustomText, Set: true}
	text := CustomShape{Resolved: true, Defined: true, Column: CustomText}
	number := CustomShape{Resolved: true, Defined: true, Column: CustomNumber}

	tests := []struct {
		name  string
		op    string
		value any
		shape *CustomShape
		code  string
	}{
		{"any of a set", "CONTAINS_ANY", []any{"red", "blue"}, &set, ""},
		{"all of a set", "CONTAINS_ALL", []any{"red", "blue"}, &set, ""},
		{"any of one value", "CONTAINS_ANY", []any{"red"}, &text, "query.operator_unsupported"},
		{"a prefix of text", "STARTS_WITH", "INV-", &text, ""},
		{"a prefix of a set", "STARTS_WITH", "INV-", &set, "query.operator_unsupported"},
		{"a prefix of a number", "STARTS_WITH", "1", &number, "query.operator_unsupported"},
		{"a number as a prefix", "STARTS_WITH", 12, &text, "query.value_type_invalid"},
		{"an undefined key", "CONTAINS_ALL", []any{"x"}, nil, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node, err := ParseFilter(map[string]any{
				"field": "custom_fields.k", "op": test.op, "value": test.value,
			}, "/filter")
			if err != nil {
				t.Fatalf("the grammar refused it: %v", err)
			}
			shapes := map[string]CustomShape{}
			if test.shape != nil {
				shapes["k"] = *test.shape
			}
			_, err = Spec{Filter: node}.Shaped(shapes)
			if test.code == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			var refusal *shared.Error
			if !errors.As(err, &refusal) || refusal.DetailCode != test.code {
				t.Fatalf("got %v, want %s", err, test.code)
			}
		})
	}
}

// UC-GOL-04 check 11: cycle time, WIP age and time in status are built-in quantities read from the
// status history; a count of one is refused like any quantity's.
func TestAFlowQuantityIsReadFromTheStatusHistory(t *testing.T) {
	for _, q := range []Quantity{QuantityCycleTime, QuantityWipAge, QuantityTimeInStatus} {
		aggregation, err := ParseAggregation(map[string]any{"function": "AVG", "field": string(q)}, "/a")
		if err != nil || aggregation.Quantity != q || !q.ReadsFlow() {
			t.Fatalf("%s: %+v %v", q, aggregation, err)
		}
		if _, err := ParseAggregation(map[string]any{"function": "COUNT", "field": string(q)}, "/a"); err == nil {
			t.Fatalf("%s: a count of a quantity was kept", q)
		}
	}
	if QuantityLeadTime.ReadsFlow() {
		t.Fatal("lead time is the entry's own stamps, not the history")
	}
}
