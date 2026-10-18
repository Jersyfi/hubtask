// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"slices"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	ReadUsageName = "ReadUsage"

	usageReadAction audit.Action = "instance.usage_read"

	// maxUsageDays bounds a read to 366 days, the first and the last included.
	maxUsageDays     = 366
	defaultUsagePage = 50
	maxUsagePage     = 200
)

// UsageMetric is how one metric is counted and by which day (multi-tenancy.md §4.3).
type UsageMetric struct {
	Name      string
	Aggregate string
	Day       string
}

// UsageMetrics is the vocabulary in the contract's order: the quotas' own names.
func UsageMetrics() []UsageMetric {
	return []UsageMetric{
		{"items", "HIGHEST", "UTC"},
		{"media_bytes", "HIGHEST", "UTC"},
		{"accounts", "HIGHEST", "UTC"},
		{"ai_tokens_per_day", "SUM", "WORKSPACE"},
		{"ai_own_tokens_per_day", "SUM", "WORKSPACE"},
		{"automation_runs_per_hour", "SUM", "UTC"},
	}
}

// ReadUsage answers each workspace's use per metric and day for billing (UC-INS-14 checks 4 and 8):
// only while the operator has switched metering on, a page of workspaces at a time.
type ReadUsage struct {
	Writer  InstanceWriter
	Ledger  adminrepo.UsageLedger
	Tenancy env.TenancyMode
}

// UsageQuery is what a read asks.
type UsageQuery struct {
	From, To time.Time
	Metric   string
	Tenant   shared.ID
	Cursor   string
	Size     int
}

// WorkspaceUsage is one workspace's days, per metric.
type WorkspaceUsage struct {
	Tenant  shared.ID
	Purged  bool
	Metrics map[string][]UsageDay
}

// UsageDay is one metric's value on one day.
type UsageDay struct {
	Day   time.Time
	Value int64
}

// UsagePage is one read.
type UsagePage struct {
	Workspaces   []WorkspaceUsage
	Metrics      []UsageMetric
	FinalThrough time.Time
	Next         string
	More         bool
}

// Execute reads one page.
func (h ReadUsage) Execute(ctx context.Context, actor appshared.ActorContext, query UsageQuery) (UsagePage, error) {
	if err := h.Writer.authorize(ctx, actor, platformScopes...); err != nil {
		return UsagePage{}, err
	}
	if err := requireMulti(h.Tenancy); err != nil {
		return UsagePage{}, err
	}
	metrics, err := usageMetricsOf(query.Metric)
	if err != nil {
		return UsagePage{}, err
	}
	from, to := dayOf(query.From), dayOf(query.To)
	if to.Before(from) || to.Sub(from) > (maxUsageDays-1)*24*time.Hour {
		return UsagePage{}, shared.ErrValidation.WithDetail("admin.usage_range_invalid").
			WithFields(shared.FieldError{Path: "/to", Code: "admin.usage_range_invalid"})
	}
	size := query.Size
	switch {
	case size < 1:
		size = defaultUsagePage
	case size > maxUsagePage:
		size = maxUsagePage
	}

	var rows []adminrepo.UsageRow
	err = h.Writer.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		level, err := h.Writer.Settings.Read(ctx)
		if err != nil {
			return err
		}
		if !level.Metering {
			return shared.ErrConflict.WithDetail("admin.metering_off")
		}
		rows, err = h.Ledger.Read(ctx, adminrepo.UsageQuery{
			From: from, To: to, Metric: query.Metric, Tenant: query.Tenant, Cursor: query.Cursor, Size: size + 1,
		})
		return err
	})
	if err != nil {
		return UsagePage{}, err
	}

	page := UsagePage{Metrics: metrics, FinalThrough: finalThrough(h.Writer.Clock.Now())}
	page.Workspaces = usageOf(rows, metrics, from, to)
	if len(page.Workspaces) > size {
		page.Workspaces, page.More = page.Workspaces[:size], true
	}
	if page.More {
		page.Next = h.Ledger.CursorAfter(page.Workspaces[size-1].Tenant)
	}
	return page, nil
}

// finalThrough is the last day that has ended in every zone: UTC-12 is the earliest, so the day
// before the UTC date twelve hours ago.
func finalThrough(now time.Time) time.Time {
	return dayOf(now.UTC().Add(-12*time.Hour)).AddDate(0, 0, -1)
}

func dayOf(at time.Time) time.Time {
	year, month, day := at.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func usageMetricsOf(wanted string) ([]UsageMetric, error) {
	all := UsageMetrics()
	if wanted == "" {
		return all, nil
	}
	at := slices.IndexFunc(all, func(m UsageMetric) bool { return m.Name == wanted })
	if at < 0 {
		return nil, shared.ErrValidation.WithDetail("admin.usage_metric_unknown").
			WithParams(map[string]string{"metric": wanted}).
			WithFields(shared.FieldError{Path: "/metric", Code: "admin.usage_metric_unknown"})
	}
	return all[at : at+1], nil
}

// usageOf lays the rows out as every day of the range per workspace and metric. A count kept as the
// day's highest carries the last closing value over a day without a row - the count did not change
// that day; a sum is 0 there.
func usageOf(rows []adminrepo.UsageRow, metrics []UsageMetric, from, to time.Time) []WorkspaceUsage {
	type key struct {
		tenant shared.ID
		metric string
	}
	byDay := map[key]map[time.Time]adminrepo.UsageRow{}
	carried := map[key]int64{}
	var order []shared.ID
	purged := map[shared.ID]bool{}
	for _, row := range rows {
		if _, seen := purged[row.Tenant]; !seen {
			order = append(order, row.Tenant)
		}
		purged[row.Tenant] = purged[row.Tenant] || row.Purged
		k := key{row.Tenant, row.Metric}
		day := dayOf(row.Day)
		if day.Before(from) {
			if row.Closing != nil {
				carried[k] = *row.Closing
			}
			continue
		}
		if byDay[k] == nil {
			byDay[k] = map[time.Time]adminrepo.UsageRow{}
		}
		byDay[k][day] = row
	}

	workspaces := make([]WorkspaceUsage, 0, len(order))
	for _, tenant := range order {
		usage := WorkspaceUsage{Tenant: tenant, Purged: purged[tenant], Metrics: map[string][]UsageDay{}}
		for _, metric := range metrics {
			k := key{tenant, metric.Name}
			last := carried[k]
			days := make([]UsageDay, 0, int(to.Sub(from).Hours()/24)+1)
			for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
				row, held := byDay[k][day]
				switch {
				case held && metric.Aggregate == "HIGHEST":
					days = append(days, UsageDay{Day: day, Value: row.Value})
					if row.Closing != nil {
						last = *row.Closing
					}
				case held:
					days = append(days, UsageDay{Day: day, Value: row.Value})
				case metric.Aggregate == "HIGHEST":
					days = append(days, UsageDay{Day: day, Value: last})
				default:
					days = append(days, UsageDay{Day: day})
				}
			}
			usage.Metrics[metric.Name] = days
		}
		workspaces = append(workspaces, usage)
	}
	return workspaces
}

// Descriptor is the catalogue entry.
func (h ReadUsage) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReadUsageName,
		Summary: "Each workspace's use per metric and day over a range, for billing: entries, stored " +
			"bytes and people as the day's highest count, the AI budget's tokens and the automation " +
			"runs as the day's sum, each saying its day basis, with the last day that is final. A " +
			"purged workspace is answered by its bare id for 13 months. Only while the operator has " +
			"switched metering on.",
		SideEffects: "None. Reads only.",
		TokenScope:  adminTenantsScope,
		AlsoScope:   provisioningScope,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "from", Kind: usecase.KindString, Required: true, Description: "The first day, YYYY-MM-DD."},
			{Name: "to", Kind: usecase.KindString, Required: true,
				Description: "The last day, YYYY-MM-DD; 366 days at most, the first included."},
			{Name: "metric", Kind: usecase.KindString, Description: "One metric's name; absent for all."},
			{Name: "workspace", Kind: usecase.KindID, Description: "One workspace; absent for every one."},
			{Name: "cursor", Kind: usecase.KindString, Description: "Where the last page stopped."},
			{Name: "limit", Kind: usecase.KindInt, Description: "Workspaces per page: fifty by default, two hundred at most."},
		},
		Audit: usecase.AuditDeclaration{
			Action: usageReadAction, TargetType: instanceTarget, Severity: audit.SeverityInfo, Required: false,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "a read of the control plane touches no item."},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h ReadUsage) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	// Asked before the days are read, so that a caller without the scope learns that rather than
	// how a range is written.
	if err := h.Writer.authorize(ctx, actor, platformScopes...); err != nil {
		return nil, err
	}
	query := UsageQuery{Metric: in.String("metric"), Cursor: in.String("cursor"), Size: in.Int("limit")}
	for _, field := range []struct {
		name   string
		target *time.Time
	}{{"from", &query.From}, {"to", &query.To}} {
		parsed, err := time.Parse(time.DateOnly, in.String(field.name))
		if err != nil {
			return nil, shared.ErrValidation.WithDetail("admin.usage_range_invalid").
				WithFields(shared.FieldError{Path: "/" + field.name, Code: "admin.usage_range_invalid"})
		}
		*field.target = parsed
	}
	if in.Present("workspace") {
		id, err := in.ID("workspace")
		if err != nil {
			return nil, err
		}
		query.Tenant = id
	}
	page, err := h.Execute(ctx, actor, query)
	if err != nil {
		return nil, err
	}
	data := make([]any, 0, len(page.Workspaces))
	for _, workspace := range page.Workspaces {
		metrics := usecase.Output{}
		for name, days := range workspace.Metrics {
			rows := make([]any, 0, len(days))
			for _, day := range days {
				rows = append(rows, usecase.Output{"day": day.Day.Format(time.DateOnly), "value": day.Value})
			}
			metrics[name] = rows
		}
		data = append(data, usecase.Output{
			"workspace_id": workspace.Tenant.String(), "purged": workspace.Purged, "metrics": metrics,
		})
	}
	described := make([]any, 0, len(page.Metrics))
	for _, metric := range page.Metrics {
		described = append(described, usecase.Output{"name": metric.Name, "aggregate": metric.Aggregate, "day": metric.Day})
	}
	var next any
	if page.Next != "" {
		next = page.Next
	}
	return usecase.Output{
		"data": data, "metrics": described, "final_through": page.FinalThrough.Format(time.DateOnly),
		"page": usecase.Output{"next_cursor": next, "has_more": page.More},
	}, nil
}
