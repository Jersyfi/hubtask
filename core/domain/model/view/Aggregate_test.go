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
