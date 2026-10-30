// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"slices"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
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
	// AggregateP50, AggregateP85 and AggregateP95 are continuous percentiles: a flow metric reads
	// a cycle time's median and its 85th percentile, not only its mean.
	AggregateP50 AggregateFunction = "P50"
	AggregateP85 AggregateFunction = "P85"
	AggregateP95 AggregateFunction = "P95"
)

// Percentile answers the fraction a percentile function reads; false for every other function.
func (f AggregateFunction) Percentile() (float64, bool) {
	switch f {
	case AggregateP50:
		return 0.5, true
	case AggregateP85:
		return 0.85, true
	case AggregateP95:
		return 0.95, true
	}
	return 0, false
}

// TimeBucket is the period a time-bucketed aggregation cuts a timestamp into.
type TimeBucket string

const (
	BucketDay   TimeBucket = "day"
	BucketWeek  TimeBucket = "week"
	BucketMonth TimeBucket = "month"
)

// WeekStart is the day a weekly bucket begins on. Monday is ISO 8601's and the default; Sunday is
// the other week a calendar draws.
type WeekStart string

const (
	WeekMonday WeekStart = "monday"
	WeekSunday WeekStart = "sunday"
)

// Quantity is a measure every entry carries without a definition: a span between two of its own
// stamps in whole minutes, or the requester's satisfaction score. A closed vocabulary like the
// functions; the compiler writes one constant expression per quantity.
type Quantity string

const (
	// QuantityLeadTime is creation to completion - what a flow metric calls lead time. An open
	// entry holds none.
	QuantityLeadTime Quantity = "lead_time"
	// QuantityScheduledSpan is start to due. An entry missing either holds none.
	QuantityScheduledSpan Quantity = "scheduled_span"
	// QuantitySatisfaction is the requester's score of the resolved entry, 1 to 5 (UC-SVC-07):
	// AVG over a queue's view is that queue's satisfaction. An entry nobody rated holds none.
	QuantitySatisfaction Quantity = "satisfaction"
	// QuantityCycleTime is the first entry into a status of the IN_PROGRESS category to completion.
	// Read from the status history, which is written from migration 0250 on: an entry that never
	// entered such a status since then, or is open, holds none.
	QuantityCycleTime Quantity = "cycle_time"
	// QuantityWipAge is how long an open entry has been in progress: its first entry into an
	// IN_PROGRESS status to now. A completed entry, or one never started since then, holds none.
	QuantityWipAge Quantity = "wip_age"
	// QuantityTimeInStatus is how long an open entry has stood in its current status: its latest
	// entry into it to now. A completed entry, or one without history, holds none.
	QuantityTimeInStatus Quantity = "time_in_status"
)

// Quantities lists the built-in measures, for a door that offers them.
func Quantities() []Quantity {
	return []Quantity{
		QuantityLeadTime, QuantityScheduledSpan, QuantitySatisfaction,
		QuantityCycleTime, QuantityWipAge, QuantityTimeInStatus,
	}
}

// ReadsFlow reports whether a quantity is read from the status history, so the use case hands
// the compiler the started statuses and the moment it asks.
func (q Quantity) ReadsFlow() bool {
	return q == QuantityCycleTime || q == QuantityWipAge || q == QuantityTimeInStatus
}

// AggregateSource is what an aggregation counts: the entries a query keeps, the links between
// them, the time recorded on them, or the pages of the spaces in scope (F-9). Records are entries.
type AggregateSource string

const (
	SourceEntries AggregateSource = "entries"
	// SourceLinks counts the links whose two ends the query both keeps - the links a view of those
	// entries draws (`POST /links:query`), so an end the reader cannot see is never counted.
	SourceLinks AggregateSource = "links"
	// SourceTime counts the time records on the entries the query keeps (UC-TIM-04): the reader's
	// own, and other people's only where the use case says the reader sees them.
	SourceTime AggregateSource = "time"
	// SourcePages counts the pages of the hubs' spaces the use case found the reader may read
	// (UC-DOC-01); a page is no entry, so the entries' filter does not apply to it.
	SourcePages AggregateSource = "pages"
)

// PageGroup is a pages summary's grouping: a page's own field.
type PageGroup string

const (
	PageByOwner    PageGroup = "owner"
	PageByTemplate PageGroup = "template"
	// PageByVerification is `verified`, `expired` or `unverified` at the moment of asking.
	PageByVerification PageGroup = "verification"
)

// GroupByLinkType is the one grouping a links summary has beside its source end's fields.
const GroupByLinkType = "link_type"

// TimeGroup is a grouping a time summary has beside its entry's fields: the record's own.
type TimeGroup string

const (
	TimeByWorkType TimeGroup = "work_type"
	TimeByBillable TimeGroup = "billable"
	// TimeByPerson names who recorded it; refused where the workspace shows others' time only as
	// sums (UC-TIM-04 check 4).
	TimeByPerson TimeGroup = "person"
)

// TimeMeasure is a time summary's one measure, the record's minutes; TimeDay its one bucket of its
// own, the day the time was spent on.
const (
	TimeMeasure = "minutes"
	TimeDay     = "day"
)

// TimeSight is who a time summary counts, decided by the use case and never parsed: the reader's
// own records, other people's on entries of the Shown collections, and whether a group of fewer
// than three people (the reader alone aside) is withheld.
type TimeSight struct {
	Reader   shared.ID
	Shown    []shared.ID
	Withhold bool
}

// MaxAggregateRows bounds the rows one aggregation answers: groups times buckets. A year of days
// over a dozen statuses fits; past it the answer says it was cut rather than growing without end.
const MaxAggregateRows = 5000

// Aggregation is a summary of the entries a query keeps: one function over one measure, optionally
// per value of a grouping field and per period of a timestamp.
//
// It reads the same Spec's scope and filter; its own fields are parsed into this catalogue before
// any adapter sees them, so the compiler meets nothing but typed constants and fields.
type Aggregation struct {
	// Source is SourceEntries or SourceLinks; a links summary is a COUNT, and its fields are its
	// source end's.
	Source   AggregateSource
	Function AggregateFunction
	// Measure is the summed field - a custom field holding a number, money or a duration. Zero for
	// COUNT and for a built-in quantity.
	Measure Field
	// Quantity is the built-in measure instead of a custom field; empty otherwise.
	Quantity Quantity
	// GroupBy is the field the rows are split by; zero for one total.
	GroupBy Field
	// ByLinkType splits a links summary by the link's type instead of a field.
	ByLinkType bool
	// TimeGroup splits a time summary by the record's own field instead of the entry's.
	TimeGroup TimeGroup
	// Minutes is a time summary's measure; ByDay buckets it by the record's day.
	Minutes bool
	ByDay   bool
	// Sight is a time summary's reach, set by the use case.
	Sight TimeSight
	// PageGroup splits a pages summary; Hubs are the spaces it counts and Reader the person a
	// restricted page must admit to be counted, both set by the use case.
	PageGroup PageGroup
	Hubs      []shared.ID
	Reader    shared.ID
	// Bucket and BucketField cut the rows by period; both or neither.
	Bucket      TimeBucket
	BucketField Field
	// WeekStart is the first day of a weekly bucket; empty for every other period.
	WeekStart WeekStart
	// Started and Now are set by the use case for a flow quantity, never parsed from a request:
	// the status keys of the IN_PROGRESS category in the workspace's workflows, and the moment of
	// asking, from its clock (rule 4).
	Started []string
	Now     time.Time
}

// IsGrouped reports whether the aggregation is split by a field.
func (a Aggregation) IsGrouped() bool {
	return a.GroupBy.Name != "" || a.ByLinkType || a.TimeGroup != "" || a.PageGroup != ""
}

// CountsPages reports whether the aggregation counts pages rather than entries.
func (a Aggregation) CountsPages() bool { return a.Source == SourcePages }

// pageBuckets are the stamps a pages summary buckets by.
var pageBuckets = []string{"created_at", "updated_at"}

// CountsLinks reports whether the aggregation counts links rather than entries.
func (a Aggregation) CountsLinks() bool { return a.Source == SourceLinks }

// CountsTime reports whether the aggregation counts time records rather than entries.
func (a Aggregation) CountsTime() bool { return a.Source == SourceTime }

// IsBucketed reports whether the aggregation is split by period.
func (a Aggregation) IsBucketed() bool { return a.Bucket != "" }

// ParseAggregation reads `{source?, function, field?, group_by?, bucket?: {field, period,
// week_start?}}`. The field is a custom field or a built-in Quantity; a links summary is a COUNT
// and may group by GroupByLinkType; a time summary measures TimeMeasure alone, may group by a
// TimeGroup and bucket by TimeDay.
func ParseAggregation(raw any, path string) (Aggregation, error) {
	document, ok := raw.(map[string]any)
	if !ok {
		return Aggregation{}, fieldError(path, "query.aggregate_malformed", nil)
	}

	source := AggregateSource(stringOf(document["source"]))
	switch {
	case source == "":
		source = SourceEntries
	case !slices.Contains([]AggregateSource{SourceEntries, SourceLinks, SourceTime, SourcePages}, source):
		return Aggregation{}, fieldError(path+"/source", "query.aggregate_source_unknown", nil)
	}

	function := AggregateFunction(stringOf(document["function"]))
	if !slices.Contains([]AggregateFunction{
		AggregateCount, AggregateSum, AggregateAvg, AggregateMin, AggregateMax,
		AggregateP50, AggregateP85, AggregateP95,
	}, function) {
		return Aggregation{}, fieldError(path+"/function", "query.aggregate_function_unknown", nil)
	}
	// A link holds no measure of its own; summing its source end's would count an entry once per
	// link, which is no question anybody asks.
	if source == SourceLinks && function != AggregateCount {
		return Aggregation{}, fieldError(path+"/function", "query.aggregate_links_count_only", nil)
	}
	if source == SourcePages && function != AggregateCount {
		return Aggregation{}, fieldError(path+"/function", "query.aggregate_pages_count_only", nil)
	}
	result := Aggregation{Source: source, Function: function}

	measure, present := document["field"]
	switch {
	case function == AggregateCount && present && measure != nil:
		return Aggregation{}, fieldError(path+"/field", "query.aggregate_field_unexpected", nil)
	case function != AggregateCount && source == SourceTime:
		// A record's minutes are its one quantity; an entry's measure would count it once per
		// record.
		if stringOf(measure) != TimeMeasure {
			return Aggregation{}, fieldError(path+"/field", "query.aggregate_time_minutes_only", nil)
		}
		result.Minutes = true
	case function != AggregateCount && slices.Contains(Quantities(), Quantity(stringOf(measure))):
		result.Quantity = Quantity(stringOf(measure))
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

	name := stringOf(document["group_by"])
	if source == SourcePages && name != "" {
		group := PageGroup(name)
		if !slices.Contains([]PageGroup{PageByOwner, PageByTemplate, PageByVerification}, group) {
			return Aggregation{}, fieldError(path+"/group_by", "query.field_not_groupable",
				map[string]string{"field": name})
		}
		result.PageGroup = group
	} else if group := TimeGroup(name); source == SourceTime &&
		slices.Contains([]TimeGroup{TimeByWorkType, TimeByBillable, TimeByPerson}, group) {
		result.TimeGroup = group
	} else if name == GroupByLinkType && source == SourceLinks {
		result.ByLinkType = true
	} else if name != "" {
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
		if source == SourceTime && stringOf(bucket["field"]) == TimeDay {
			result.Bucket, result.ByDay = period, true
		} else {
			target, err := field(stringOf(bucket["field"]), path+"/bucket/field")
			if err != nil {
				return Aggregation{}, err
			}
			_, custom := CustomField(target.Name)
			if (!custom && target.Kind != KindTimestamp) ||
				(source == SourcePages && !slices.Contains(pageBuckets, target.Name)) {
				return Aggregation{}, fieldError(path+"/bucket/field", "query.field_not_bucketable",
					map[string]string{"field": target.Name})
			}
			result.Bucket, result.BucketField = period, target
		}
		start, present := bucket["week_start"]
		switch {
		case !present || start == nil:
			if period == BucketWeek {
				result.WeekStart = WeekMonday
			}
		case period != BucketWeek:
			return Aggregation{}, fieldError(path+"/bucket/week_start", "query.week_start_unexpected", nil)
		case !slices.Contains([]WeekStart{WeekMonday, WeekSunday}, WeekStart(stringOf(start))):
			return Aggregation{}, fieldError(path+"/bucket/week_start", "query.week_start_unknown", nil)
		default:
			result.WeekStart = WeekStart(stringOf(start))
		}
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
