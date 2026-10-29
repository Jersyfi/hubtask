// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package query

import (
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

// Aggregate compiles a summary over the entries the search keeps: one row per group and period,
// each carrying the group key, the period's first day, the entry count and the function's value.
//
// The four columns are always there - a NULL::text where nothing groups or buckets, a
// NULL::numeric for a plain count - so one scan reads every shape. The period is cut in the time
// zone the use case resolved for the actor, bound like any other value; its unit is a constant
// chosen by the typed period. A week starts on Monday (PostgreSQL's ISO week). The limit is probe,
// so a caller asking for one row past its bound learns the answer was cut.
func Aggregate(
	search repository.ItemSearch, aggregation view.Aggregation, timeZone string, probe int,
) (Statement, error) {
	b := newBuilder(search)

	b.write(`SELECT `)
	b.aggregateGroupKey(aggregation)
	b.write(`, `)
	b.aggregateBucket(aggregation, timeZone)
	b.write(`, count(*)::bigint, `)
	b.aggregateValue(aggregation)
	b.write(` FROM work_item wi WHERE `)
	b.predicates(search)
	b.write(` GROUP BY 1, 2 ORDER BY 1 ASC NULLS LAST, 2 ASC NULLS LAST LIMIT `)
	b.param(int64(probe))

	return b.statement()
}

func (b *builder) aggregateGroupKey(aggregation view.Aggregation) {
	if !aggregation.IsGrouped() {
		b.write(`NULL::text`)
		return
	}
	if _, custom := view.CustomField(aggregation.GroupBy.Name); custom {
		b.customGroupKey(aggregation.GroupBy)
		return
	}
	group, ok := column(aggregation.GroupBy, itemPrefix)
	if !ok {
		b.fail(errFieldNotCompilable(aggregation.GroupBy))
		return
	}
	b.write(group, `::text`)
}

func (b *builder) aggregateBucket(aggregation view.Aggregation, timeZone string) {
	if !aggregation.IsBucketed() {
		b.write(`NULL::text`)
		return
	}
	var unit string
	switch aggregation.Bucket {
	case view.BucketDay:
		unit = `'day'`
	case view.BucketWeek:
		unit = `'week'`
	case view.BucketMonth:
		unit = `'month'`
	default:
		b.fail(errFieldNotCompilable(aggregation.BucketField))
		return
	}
	b.write(`date_trunc(`, unit, `, (`)
	if _, custom := view.CustomField(aggregation.BucketField.Name); custom {
		if shape := aggregation.BucketField.Custom; shape.Resolved && !shape.Defined {
			b.write(`NULL::timestamptz`)
		} else {
			b.customValue(aggregation.BucketField)
		}
	} else {
		moment, ok := column(aggregation.BucketField, itemPrefix)
		if !ok {
			b.fail(errFieldNotCompilable(aggregation.BucketField))
			return
		}
		b.write(moment)
	}
	b.write(`) AT TIME ZONE `)
	b.param(timeZone)
	b.write(`::text)::date::text`)
}

func (b *builder) aggregateValue(aggregation view.Aggregation) {
	var function string
	switch aggregation.Function {
	case view.AggregateCount:
		b.write(`NULL::numeric`)
		return
	case view.AggregateSum:
		function = `sum(`
	case view.AggregateAvg:
		function = `avg(`
	case view.AggregateMin:
		function = `min(`
	case view.AggregateMax:
		function = `max(`
	default:
		b.fail(errFieldNotCompilable(aggregation.Measure))
		return
	}
	b.write(function)
	if shape := aggregation.Measure.Custom; shape.Resolved && !shape.Defined {
		b.write(`NULL::numeric`)
	} else {
		b.customValue(aggregation.Measure)
	}
	b.write(`)::numeric`)
}
