// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package query

import (
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

// Aggregate compiles a summary over the entries the search keeps, over the links between them or
// over the time recorded on them: one row per group and period, each carrying the group key, the period's first day, the count and
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
	if aggregation.CountsPages() {
		b.pages(search, aggregation, timeZone, probe)
		return b.statement()
	}

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
	if aggregation.CountsTime() {
		b.write(` JOIN time_record tr ON tr.tenant_id = wi.tenant_id AND tr.entry_id = wi.id`)
	}
	b.write(` WHERE `)
	b.predicates(search)
	if aggregation.CountsTime() {
		// The reader's own records, and other people's only on entries of the collections the use
		// case found the reader sees them in (rule 2): the decision is bound, never made here.
		b.write(` AND (tr.account_id = `)
		b.param(aggregation.Sight.Reader.String())
		b.write(`::uuid OR wi.collection_id = ANY(`)
		b.param(identifiers(aggregation.Sight.Shown))
		b.write(`::uuid[]))`)
	}
	if aggregation.CountsLinks() {
		// The target is held to the same predicates by an inner `wi` that shadows the outer one,
		// so a link counts only where the query keeps both its ends - an end the reader cannot see
		// is never counted (the scope and the restriction are part of the predicates).
		b.write(` AND lk.target_id IN (SELECT wi.id FROM work_item wi WHERE `)
		b.predicates(search)
		b.write(`)`)
	}
	// A plain total is not grouped, so an empty scope answers one row of zero rather than none.
	grouped := aggregation.IsGrouped() || aggregation.IsBucketed()
	if grouped {
		b.write(` GROUP BY 1, 2`)
	}
	if aggregation.CountsTime() && aggregation.Sight.Withhold {
		// A group of fewer than three people names them by its sum (UC-TIM-04 check 4); the
		// reader's own time alone is theirs to see.
		b.write(` HAVING count(DISTINCT tr.account_id) >= 3 OR bool_and(tr.account_id = `)
		b.param(aggregation.Sight.Reader.String())
		b.write(`::uuid)`)
	}
	if grouped {
		b.write(` ORDER BY 1 ASC NULLS LAST, 2 ASC NULLS LAST`)
	}
	b.write(` LIMIT `)
	b.param(int64(probe))

	return b.statement()
}

// pages compiles a pages summary: the live pages of the bound spaces - archived ones too where the
// search keeps archived entries, trashed ones never - per group and period. Which spaces is the use
// case's answer (rule 2); the entries' filter does not reach a page.
func (b *builder) pages(search repository.ItemSearch, aggregation view.Aggregation, timeZone string, probe int) {
	b.write(`SELECT `)
	switch aggregation.PageGroup {
	case "":
		b.write(`NULL::text`)
	case view.PageByOwner:
		b.write(`p.owner_id::text`)
	case view.PageByTemplate:
		b.write(`p.template::text`)
	case view.PageByVerification:
		b.write(`CASE WHEN p.verified_until IS NULL THEN 'unverified' WHEN p.verified_until > `)
		b.param(aggregation.Now.UTC())
		b.write(`::timestamptz THEN 'verified' ELSE 'expired' END`)
	default:
		b.fail(shared.ErrInternal.WithDetail("query.page_group_unknown"))
	}
	b.write(`, `)
	b.aggregateBucket(aggregation, timeZone)
	b.write(`, count(*)::bigint, NULL::numeric FROM page p WHERE p.hub_id = ANY(`)
	b.param(identifiers(aggregation.Hubs))
	b.write(`::uuid[]) AND p.deleted_at IS NULL`)
	// A restricted page is counted only for somebody it admits: a count, too, says it is there.
	b.write(` AND (NOT p.restricted AND p.parent_id IS NULL OR page_admits(p.id, `)
	if aggregation.Reader.IsZero() {
		// Nobody to admit: only the pages no restriction narrows are counted.
		b.write(`NULL`)
	} else {
		b.param(aggregation.Reader.String())
	}
	b.write(`::uuid))`)
	if !search.Spec.IncludeArchived {
		b.write(` AND p.archived_at IS NULL`)
	}
	if aggregation.IsGrouped() || aggregation.IsBucketed() {
		b.write(` GROUP BY 1, 2 ORDER BY 1 ASC NULLS LAST, 2 ASC NULLS LAST`)
	}
	b.write(` LIMIT `)
	b.param(int64(probe))
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
	switch aggregation.TimeGroup {
	case view.TimeByWorkType:
		b.write(`tr.work_type`)
		return
	case view.TimeByBillable:
		b.write(`tr.billable::text`)
		return
	case view.TimeByPerson:
		b.write(`tr.account_id::text`)
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
	if aggregation.CountsPages() {
		switch aggregation.BucketField.Name {
		case view.FieldCreatedAt:
			b.write(`p.created_at`)
		case view.FieldUpdatedAt:
			b.write(`p.updated_at`)
		default:
			b.fail(errFieldNotCompilable(aggregation.BucketField))
			return
		}
		b.write(`) AT TIME ZONE `)
		b.param(timeZone)
		b.write(`::text`, closing, `::date::text`)
		return
	}
	if aggregation.ByDay {
		// A day is a calendar date already: no zone moves it.
		b.write(`tr.day::timestamp)`, closing, `::date::text`)
		return
	}
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
	if aggregation.Minutes {
		b.write(`tr.minutes)::numeric`)
		return
	}
	if aggregation.Quantity != "" {
		b.quantity(aggregation)
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
// NULL where either is missing, or the requester's score. timestamptz_mi is the subtraction operator's own function, spelled
// as a name for the reason customText spells jsonb_extract_path_text.
func (b *builder) quantity(aggregation view.Aggregation) {
	if aggregation.Quantity.ReadsFlow() {
		b.flowQuantity(aggregation)
		return
	}
	var from, to string
	switch aggregation.Quantity {
	case view.QuantityLeadTime:
		from, to = `wi.created_at`, `wi.completed_at`
	case view.QuantityScheduledSpan:
		from, to = `wi.start_at`, `wi.due_at`
	case view.QuantitySatisfaction:
		// A score, not a span.
		b.write(satisfactionScore)
		return
	default:
		b.fail(shared.ErrInternal.WithDetail("query.quantity_unknown"))
		return
	}
	b.write(`div(extract(epoch FROM timestamptz_mi(`, to, `, `, from, `)), 60)`)
}

// flowQuantity writes a measure read from the status history (migration 0250): the first entry
// into a started status to completion (cycle time) or to the moment of asking (WIP age), or the
// latest entry into the current status to that moment (time in status), in whole minutes. The
// started keys and the moment are bound; an entry without the rows, or the wrong side of
// completion, holds NULL.
func (b *builder) flowQuantity(aggregation view.Aggregation) {
	started := aggregation.Started
	if started == nil {
		started = []string{}
	}
	firstStart := func() {
		b.write(`(SELECT min(sc.entered_at) FROM entry_status_change sc WHERE sc.tenant_id = wi.tenant_id `,
			`AND sc.item_id = wi.id AND sc.status = ANY(`)
		b.param(started)
		b.write(`::text[]))`)
	}
	now := func() {
		b.param(aggregation.Now.UTC())
		b.write(`::timestamptz`)
	}
	switch aggregation.Quantity {
	case view.QuantityCycleTime:
		b.write(`div(extract(epoch FROM timestamptz_mi(wi.completed_at, `)
		firstStart()
		b.write(`)), 60)`)
	case view.QuantityWipAge:
		b.write(`CASE WHEN wi.completed_at IS NULL THEN div(extract(epoch FROM timestamptz_mi(`)
		now()
		b.write(`, `)
		firstStart()
		b.write(`)), 60) END`)
	case view.QuantityTimeInStatus:
		b.write(`CASE WHEN wi.completed_at IS NULL THEN div(extract(epoch FROM timestamptz_mi(`)
		now()
		b.write(`, (SELECT max(sc.entered_at) FROM entry_status_change sc WHERE sc.tenant_id = wi.tenant_id `,
			`AND sc.item_id = wi.id AND sc.status = wi.status))), 60) END`)
	default:
		b.fail(shared.ErrInternal.WithDetail("query.quantity_unknown"))
	}
}
