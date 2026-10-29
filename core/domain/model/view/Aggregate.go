// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"slices"
)

// AggregateFunction is what an aggregation computes over the entries a query keeps. A closed
// vocabulary like the operators (ADR-0026): the compiler writes one constant per function.
type AggregateFunction string

const (
	AggregateCount AggregateFunction = "COUNT"
	AggregateSum   AggregateFunction = "SUM"
	AggregateAvg   AggregateFunction = "AVG"
	AggregateMin   AggregateFunction = "MIN"
	AggregateMax   AggregateFunction = "MAX"
)

// TimeBucket is the period a time-bucketed aggregation cuts a timestamp into.
type TimeBucket string

const (
	BucketDay   TimeBucket = "day"
	BucketWeek  TimeBucket = "week"
	BucketMonth TimeBucket = "month"
)

// MaxAggregateRows bounds the rows one aggregation answers: groups times buckets. A year of days
// over a dozen statuses fits; past it the answer says it was cut rather than growing without end.
const MaxAggregateRows = 5000

// Aggregation is a summary of the entries a query keeps: one function over one measure, optionally
// per value of a grouping field and per period of a timestamp.
//
// It reads the same Spec's scope and filter; its own fields are parsed into this catalogue before
// any adapter sees them, so the compiler meets nothing but typed constants and fields.
type Aggregation struct {
	Function AggregateFunction
	// Measure is the summed field - a custom field holding a number, money or a duration. Zero for
	// COUNT.
	Measure Field
	// GroupBy is the field the rows are split by; zero for one total.
	GroupBy Field
	// Bucket and BucketField cut the rows by period; both or neither.
	Bucket      TimeBucket
	BucketField Field
}

// IsGrouped reports whether the aggregation is split by a field.
func (a Aggregation) IsGrouped() bool { return a.GroupBy.Name != "" }

// IsBucketed reports whether the aggregation is split by period.
func (a Aggregation) IsBucketed() bool { return a.Bucket != "" }

// ParseAggregation reads `{function, field?, group_by?, bucket?: {field, period}}`.
func ParseAggregation(raw any, path string) (Aggregation, error) {
	document, ok := raw.(map[string]any)
	if !ok {
		return Aggregation{}, fieldError(path, "query.aggregate_malformed", nil)
	}

	function := AggregateFunction(stringOf(document["function"]))
	if !slices.Contains([]AggregateFunction{
		AggregateCount, AggregateSum, AggregateAvg, AggregateMin, AggregateMax,
	}, function) {
		return Aggregation{}, fieldError(path+"/function", "query.aggregate_function_unknown", nil)
	}
	result := Aggregation{Function: function}

	measure, present := document["field"]
	switch {
	case function == AggregateCount && present && measure != nil:
		return Aggregation{}, fieldError(path+"/field", "query.aggregate_field_unexpected", nil)
	case function != AggregateCount:
		target, err := field(stringOf(measure), path+"/field")
		if err != nil {
			return Aggregation{}, err
		}
		// Only a custom field holds a quantity today: number, money, duration. Its shape is the
		// definitions' answer, checked in Shaped.
		if _, custom := CustomField(target.Name); !custom {
			return Aggregation{}, fieldError(path+"/field", "query.field_not_aggregatable",
				map[string]string{"field": target.Name})
		}
		result.Measure = target
	}

	if name := stringOf(document["group_by"]); name != "" {
		target, err := field(name, path+"/group_by")
		if err != nil {
			return Aggregation{}, err
		}
		if !target.Groupable {
			return Aggregation{}, fieldError(path+"/group_by", "query.field_not_groupable",
				map[string]string{"field": target.Name})
		}
		result.GroupBy = target
	}

	if raw, present := document["bucket"]; present && raw != nil {
		bucket, ok := raw.(map[string]any)
		if !ok {
			return Aggregation{}, fieldError(path+"/bucket", "query.aggregate_malformed", nil)
		}
		period := TimeBucket(stringOf(bucket["period"]))
		if !slices.Contains([]TimeBucket{BucketDay, BucketWeek, BucketMonth}, period) {
			return Aggregation{}, fieldError(path+"/bucket/period", "query.bucket_period_unknown", nil)
		}
		target, err := field(stringOf(bucket["field"]), path+"/bucket/field")
		if err != nil {
			return Aggregation{}, err
		}
		if _, custom := CustomField(target.Name); !custom && target.Kind != KindTimestamp {
			return Aggregation{}, fieldError(path+"/bucket/field", "query.field_not_bucketable",
				map[string]string{"field": target.Name})
		}
		result.Bucket, result.BucketField = period, target
	}
	return result, nil
}

// CustomKeys answers the custom keys the aggregation names, for the use case to shape.
func (a Aggregation) CustomKeys() []string {
	var keys []string
	for _, field := range []Field{a.Measure, a.GroupBy, a.BucketField} {
		if key, ok := CustomField(field.Name); ok && !slices.Contains(keys, key) {
			keys = append(keys, key)
		}
	}
	return keys
}

// Shaped puts the definitions' shapes on the custom fields the aggregation names and refuses what
// the shapes make meaningless: a measure that is not one number per entry, a grouping by a set or
// prose, a bucket over anything but a date. An undefined key is no value on any entry, as in a
// filter: its sum is empty rather than refused.
func (a Aggregation) Shaped(shapes map[string]CustomShape) (Aggregation, error) {
	shape := func(field Field) Field {
		key, _ := CustomField(field.Name)
		found, known := shapes[key]
		if !known {
			found = CustomShape{Resolved: true}
		}
		field.Custom = found
		return field
	}

	if _, custom := CustomField(a.Measure.Name); custom {
		a.Measure = shape(a.Measure)
		s := a.Measure.Custom
		if s.Defined && (s.Ambiguous || s.Set || s.Column != CustomNumber) {
			return Aggregation{}, fieldError("/aggregate/field", "query.field_not_aggregatable",
				map[string]string{"field": a.Measure.Name})
		}
	}
	if _, custom := CustomField(a.GroupBy.Name); custom {
		a.GroupBy = shape(a.GroupBy)
		if !a.GroupBy.Custom.groups() {
			return Aggregation{}, fieldError("/aggregate/group_by", "query.field_not_groupable",
				map[string]string{"field": a.GroupBy.Name})
		}
	}
	if _, custom := CustomField(a.BucketField.Name); custom {
		a.BucketField = shape(a.BucketField)
		s := a.BucketField.Custom
		if s.Defined && (s.Ambiguous || s.Set || s.Column != CustomInstant) {
			return Aggregation{}, fieldError("/aggregate/bucket/field", "query.field_not_bucketable",
				map[string]string{"field": a.BucketField.Name})
		}
	}
	return a, nil
}
