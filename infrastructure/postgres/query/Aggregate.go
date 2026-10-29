// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package query

import (
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

// Aggregate compiles a summary over the entries the search keeps, or over the links between them:
// one row per group and period, each carrying the group key, the period's first day, the count and
// the function's value.
//
// The four columns are always there - a NULL::text where nothing groups or buckets, a
// NULL::numeric for a plain count - so one scan reads every shape. The period is cut in the time
// zone the use case resolved for the actor, bound like any other value; its unit is a constant
// chosen by the typed period. A Monday week is PostgreSQL's ISO week; a Sunday week is a seven-day
// bin from a Sunday, because shifting the ISO week by a day would need an operator this package
// never writes (FuzzCompile's alphabet). The limit is probe, so a caller asking for one row past
// its bound learns the answer was cut.
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
	b.write(` FROM work_item wi`)
	if aggregation.CountsLinks() {
		b.write(` JOIN entry_link lk ON lk.tenant_id = wi.tenant_id AND lk.source_id = wi.id`)
	}
	b.write(` WHERE `)
	b.predicates(search)
	if aggregation.CountsLinks() {
		// The target is held to the same predicates by an inner `wi` that shadows the outer one,
		// so a link counts only where the query keeps both its ends - an end the reader cannot see
		// is never counted (the scope and the restriction are part of the predicates).
		b.write(` AND lk.target_id IN (SELECT wi.id FROM work_item wi WHERE `)
		b.predicates(search)
		b.write(`)`)
	}
	// A plain total is not grouped, so an empty scope answers one row of zero rather than none.
	if aggregation.IsGrouped() || aggregation.IsBucketed() {
		b.write(` GROUP BY 1, 2 ORDER BY 1 ASC NULLS LAST, 2 ASC NULLS LAST`)
	}
	b.write(` LIMIT `)
	b.param(int64(probe))

	return b.statement()
}

func (b *builder) aggregateGroupKey(aggregation view.Aggregation) {
	if !aggregation.IsGrouped() {
		b.write(`NULL::text`)
		return
	}
	if aggregation.ByLinkType {
		b.write(`lk.link_type`)
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
	var opening, closing string
	switch {
	case aggregation.Bucket == view.BucketDay:
		opening, closing = `date_trunc('day', (`, `)`
	case aggregation.Bucket == view.BucketWeek && aggregation.WeekStart == view.WeekSunday:
		// 2 January 2000 was a Sunday.
		opening, closing = `date_bin(interval '7 days', (`, `, make_timestamp(2000, 1, 2, 0, 0, 0))`
	case aggregation.Bucket == view.BucketWeek:
		opening, closing = `date_trunc('week', (`, `)`
	case aggregation.Bucket == view.BucketMonth:
		opening, closing = `date_trunc('month', (`, `)`
	default:
		b.fail(errFieldNotCompilable(aggregation.BucketField))
		return
	}
	b.write(opening)
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
	b.write(`::text`, closing, `::date::text`)
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
	if aggregation.Quantity != "" {
		b.quantity(aggregation.Quantity)
		b.write(`)::numeric`)
		return
	}
	if shape := aggregation.Measure.Custom; shape.Resolved && !shape.Defined {
		b.write(`NULL::numeric`)
	} else {
		b.customValue(aggregation.Measure)
	}
	b.write(`)::numeric`)
}

// quantity writes a built-in measure: the span between two of the row's stamps in whole minutes,
// NULL where either is missing. timestamptz_mi is the subtraction operator's own function, spelled
// as a name for the reason customText spells jsonb_extract_path_text.
func (b *builder) quantity(quantity view.Quantity) {
	var from, to string
	switch quantity {
	case view.QuantityLeadTime:
		from, to = `wi.created_at`, `wi.completed_at`
	case view.QuantityScheduledSpan:
		from, to = `wi.start_at`, `wi.due_at`
	default:
		b.fail(shared.ErrInternal.WithDetail("query.quantity_unknown"))
		return
	}
	b.write(`div(extract(epoch FROM timestamptz_mi(`, to, `, `, from, `)), 60)`)
}
