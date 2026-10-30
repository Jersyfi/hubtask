// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"context"
	"strconv"

	timerepo "github.com/Jersyfi/hubtask/core/application/repository/timekeeping"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ReadTimeReportName         = "ReadTimeReport"
	ReadEstimatesVsActualsName = "ReadEstimatesVsActuals"
	ExportTimeRecordsName      = "ExportTimeRecords"
)

// ReadTimeReport sums time by up to two dimensions (UC-TIM-04): the caller's own, or everybody's
// the caller approves. Where the workspace shows only sums, a report over other people names
// nobody - no PERSON dimension, no single person's filter - and withholds a group of fewer than
// three people.
type ReadTimeReport struct{ Keeper Keeper }

// Execute reads the report.
func (h ReadTimeReport) Execute(
	ctx context.Context, actor appshared.ActorContext, f timerepo.Filter, mine bool, dims []domain.Dimension,
) (domain.Report, error) {
	if err := person(actor); err != nil {
		return domain.Report{}, err
	}
	if err := domain.CheckDimensions(dims); err != nil {
		return domain.Report{}, err
	}
	if mine {
		f.AccountID = actor.AccountID
	}
	policy, err := h.Keeper.policy(ctx, actor)
	if err != nil {
		return domain.Report{}, err
	}
	others := f.AccountID != actor.AccountID
	aggregated := others && policy.Visibility == domain.VisibilityAggregated
	if aggregated && !f.AccountID.IsZero() {
		return domain.Report{}, shared.ErrForbidden.WithDetail("time.aggregated_only")
	}
	if aggregated {
		for _, d := range dims {
			if d == domain.ByPerson {
				return domain.Report{}, shared.ErrForbidden.WithDetail("time.aggregated_only")
			}
		}
	}
	var lines []domain.Line
	if err := h.Keeper.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		lines, err = h.Keeper.Store.Lines(ctx, f, maxLinesSummed+1)
		return err
	}); err != nil {
		return domain.Report{}, err
	}
	if len(lines) > maxLinesSummed {
		return domain.Report{}, shared.ErrValidation.WithDetail("time.report_too_large").
			WithParams(map[string]string{"maximum": strconv.Itoa(maxLinesSummed)})
	}
	if others {
		if lines, err = h.Keeper.approvedLines(ctx, actor, lines); err != nil {
			return domain.Report{}, err
		}
	}
	return domain.Summarise(lines, dims, aggregated, actor.AccountID), nil
}

// approvedLines keeps the caller's own lines and the others' where the caller approves time.
func (k Keeper) approvedLines(ctx context.Context, actor appshared.ActorContext, lines []domain.Line) ([]domain.Line, error) {
	placed := make([]timerepo.Placed, len(lines))
	for i, l := range lines {
		placed[i] = timerepo.Placed{Record: domain.Record{AccountID: l.AccountID}, HubID: l.HubID, CollectionID: l.CollectionID}
	}
	index := map[place]int{}
	var places []place
	for _, p := range placed {
		at := place{hub: p.HubID, collection: p.CollectionID}
		if _, ok := index[at]; !ok && p.AccountID != actor.AccountID {
			index[at] = len(places)
			places = append(places, at)
		}
	}
	allowed, err := k.approves(ctx, actor, places, ReportReadAction, itemsRead)
	if err != nil {
		return nil, err
	}
	kept := lines[:0]
	for _, l := range lines {
		if l.AccountID == actor.AccountID || allowed[index[place{hub: l.HubID, collection: l.CollectionID}]] {
			kept = append(kept, l)
		}
	}
	return kept, nil
}

// titles answers the titles of the entries among the ids that the caller may read.
func (k Keeper) titles(ctx context.Context, actor appshared.ActorContext, ids []shared.ID) (map[shared.ID]string, error) {
	out := map[shared.ID]string{}
	if len(ids) == 0 {
		return out, nil
	}
	var summaries []workrepo.EntrySummary
	if err := k.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		summaries, err = k.References.Entries(ctx, ids)
		return err
	}); err != nil {
		return nil, err
	}
	paths := make([][]identity.Scope, len(summaries))
	for i, s := range summaries {
		paths[i] = entryPath(s.HubID, s.CollectionID, s.ID)
	}
	allowed, err := k.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead, Action: itemReadAction, TokenScope: itemsRead, TargetType: itemTarget,
		On: access.ItemSubject{Does: service.ItemRead},
	}, paths)
	if err != nil {
		return nil, err
	}
	for i, s := range summaries {
		if i < len(allowed) && allowed[i] {
			out[s.ID] = s.Title
		}
	}
	return out, nil
}

// Descriptor is the catalogue entry.
func (h ReadTimeReport) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadTimeReportName,
		Summary: "Sums time records of a range of days by up to two of PERSON, ENTRY, COLLECTION, HUB, WORK_TYPE, " +
			"DAY, WEEK (from Monday), MONTH and BILLABLE: minutes, billable minutes and records per group, with the " +
			"titles of the entries the caller may read. The caller's own with `mine`, else also other people's where " +
			"the caller approves time. Where the workspace shows only sums, a report over other people takes no " +
			"PERSON and no other person's filter (time.aggregated_only) and withholds groups of fewer than three people.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead, ReadOnly: true,
		Input: append(filterInput(false), usecase.Field{
			Name: "group_by", Kind: usecase.KindAny, Description: "Up to two dimensions, in order.",
		}),
		Audit: usecase.AuditDeclaration{Action: ReportReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			f, err := filterOf(in)
			if err != nil {
				return nil, err
			}
			var dims []domain.Dimension
			switch v := in["group_by"].(type) {
			case nil:
			case []any:
				for _, d := range v {
					s, _ := d.(string)
					dims = append(dims, domain.Dimension(s))
				}
			case []string:
				for _, s := range v {
					dims = append(dims, domain.Dimension(s))
				}
			case string:
				dims = append(dims, domain.Dimension(v))
			default:
				return nil, invalid("/group_by", "time.dimension_unknown")
			}
			mine, _ := in["mine"].(bool)
			report, err := h.Execute(ctx, actor, f, mine, dims)
			if err != nil {
				return nil, err
			}
			return h.Keeper.reportOutput(ctx, actor, report)
		}),
	}, recordTarget)
}

func (k Keeper) reportOutput(ctx context.Context, actor appshared.ActorContext, r domain.Report) (usecase.Output, error) {
	var entries []shared.ID
	for _, g := range r.Groups {
		for i, d := range r.Dimensions {
			if d == domain.ByEntry && g.Keys[i] != "" {
				entries = append(entries, shared.ID(g.Keys[i]))
			}
		}
	}
	titles, err := k.titles(ctx, actor, entries)
	if err != nil {
		return nil, err
	}
	dims := make([]string, len(r.Dimensions))
	for i, d := range r.Dimensions {
		dims[i] = string(d)
	}
	groups := make([]usecase.Output, 0, len(r.Groups))
	for _, g := range r.Groups {
		keys := make([]usecase.Output, len(g.Keys))
		for i, key := range g.Keys {
			entry := usecase.Output{"dimension": dims[i], "value": key}
			if r.Dimensions[i] == domain.ByEntry {
				entry["label"] = titles[shared.ID(key)]
			}
			keys[i] = entry
		}
		groups = append(groups, usecase.Output{
			"keys": keys, "minutes": g.Minutes, "billable_minutes": g.BillableMinutes, "records": g.Records,
		})
	}
	return usecase.Output{
		"group_by": dims, "groups": groups, "minutes": r.Minutes,
		"withheld_minutes": r.WithheldMinutes, "withheld_groups": r.WithheldGroups,
	}, nil
}

// ReadEstimatesVsActuals answers each entry of a plan - a hub or a collection - with its estimate
// in hours against everybody's time on it, rolled up its parents (UC-TIM-04). Where the workspace
// shows only sums, only an approver of the plan reads it: an entry one person worked on would tell
// their minutes.
type ReadEstimatesVsActuals struct{ Keeper Keeper }

// ActualsRow is one entry of the answer.
type ActualsRow struct {
	domain.Actual
	ParentID shared.ID
	Title    string
	Done     bool
}

// Execute reads it.
func (h ReadEstimatesVsActuals) Execute(ctx context.Context, actor appshared.ActorContext, planID shared.ID) ([]ActualsRow, error) {
	if err := person(actor); err != nil {
		return nil, err
	}
	if planID.IsZero() {
		return nil, invalid("/plan_id", "planning.plan_required")
	}
	var plan workContainer
	if err := h.Keeper.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Keeper.Containers.Find(ctx, planID)
		plan = workContainer{found.ID, found.ParentID, found.DeletedAt != nil, found.Type == work.ContainerHub}
		return err
	}); err != nil {
		return nil, err
	}
	if plan.deleted {
		return nil, shared.ErrNotFound.WithDetail("containers.not_found")
	}
	policy, err := h.Keeper.policy(ctx, actor)
	if err != nil {
		return nil, err
	}
	at := place{hub: plan.parent, collection: plan.id}
	if plan.hub {
		at = place{hub: plan.id}
	}
	request := access.Request{
		Permission: service.PermissionRead, Path: at.path(), Action: ActualsReadAction, TokenScope: itemsRead,
		TargetType: "plan", TargetID: planID,
	}
	if policy.Visibility == domain.VisibilityAggregated {
		request = approverRequest(ActualsReadAction, itemsRead)
		request.Path, request.TargetType, request.TargetID = at.path(), "plan", planID
	}
	if err := h.Keeper.Authorizer.Authorize(ctx, actor, request); err != nil {
		return nil, err
	}
	var (
		entries   []planEntry
		estimates map[shared.ID]planning.Estimate
		minutes   map[shared.ID]int
	)
	if err := h.Keeper.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		rows, err := h.Keeper.Plans.Entries(ctx, planID, maxEntriesOfAPlan+1)
		if err != nil {
			return err
		}
		if len(rows) > maxEntriesOfAPlan {
			return shared.ErrValidation.WithDetail("planning.too_many_entries").
				WithParams(map[string]string{"maximum": strconv.Itoa(maxEntriesOfAPlan)})
		}
		for _, r := range rows {
			entries = append(entries, planEntry{r.ID, r.ParentID, r.HubID, r.CollectionID, r.Title, r.Done})
		}
		ids := make([]shared.ID, len(entries))
		for i, e := range entries {
			ids[i] = e.id
		}
		if estimates, err = h.Keeper.Estimates.Estimates(ctx, ids); err != nil {
			return err
		}
		minutes, err = h.Keeper.Store.MinutesOn(ctx, ids)
		return err
	}); err != nil {
		return nil, err
	}
	paths := make([][]identity.Scope, len(entries))
	for i, e := range entries {
		paths[i] = entryPath(e.hub, e.collection, e.id)
	}
	visible, err := h.Keeper.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead, Action: itemReadAction, TokenScope: itemsRead, TargetType: itemTarget,
		On: access.ItemSubject{Does: service.ItemRead},
	}, paths)
	if err != nil {
		return nil, err
	}
	planned := make([]domain.Planned, 0, len(entries))
	kept := make([]planEntry, 0, len(entries))
	for i, e := range entries {
		if i >= len(visible) || !visible[i] {
			continue
		}
		p := domain.Planned{ID: e.id, ParentID: e.parent}
		if hours, ok := estimates[e.id].CountsIn(planning.Hours); ok {
			p.Hours = &hours
		}
		planned = append(planned, p)
		kept = append(kept, e)
	}
	actuals := domain.Actuals(planned, minutes)
	out := make([]ActualsRow, len(actuals))
	for i, a := range actuals {
		out[i] = ActualsRow{Actual: a, ParentID: kept[i].parent, Title: kept[i].title, Done: kept[i].done}
	}
	return out, nil
}

type workContainer struct {
	id, parent shared.ID
	deleted    bool
	hub        bool
}

type planEntry struct {
	id, parent, hub, collection shared.ID
	title                       string
	done                        bool
}

// Descriptor is the catalogue entry.
func (h ReadEstimatesVsActuals) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadEstimatesVsActualsName, Permission: service.PermissionRead,
		Summary: "Answers each entry of a hub or a collection the caller may read with its estimate in hours, " +
			"everybody's recorded minutes on it, both rolled up with the entries below it, the hours remaining and " +
			"the variance. Needs READ at the plan; where the workspace shows only sums, APPROVE_TIME or MANAGE_MEMBERS there.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "plan_id", Kind: usecase.KindID, Required: true, Description: "The hub or the collection."},
		},
		Audit: usecase.AuditDeclaration{Action: ActualsReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("plan_id")
			if err != nil {
				return nil, err
			}
			rows, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(rows))
			var estimate float64
			var spent int
			for _, r := range rows {
				var hours any
				if r.EstimateHours != nil {
					hours = *r.EstimateHours
					estimate += *r.EstimateHours
				}
				spent += r.Minutes
				data = append(data, usecase.Output{
					"entry_id": r.ID.String(), "parent_id": idText(r.ParentID), "title": r.Title, "done": r.Done,
					"estimate_hours": hours, "minutes": r.Minutes, "rolled_estimate_hours": r.RolledHours,
					"rolled_minutes": r.RolledMinutes, "remaining_hours": r.RemainingHours, "variance_hours": r.VarianceHours,
				})
			}
			return usecase.Output{"plan_id": id.String(), "data": data, "estimate_hours": estimate, "minutes": spent}, nil
		}),
	}, "plan")
}

// ExportTimeRecords hands time records to accounting (UC-TIM-05): the same records a listing
// reads, at most 50000, in fixed columns. Where the workspace shows only sums, only the caller's
// own: a record is one person's.
type ExportTimeRecords struct{ Keeper Keeper }

// Execute reads the rows.
func (h ExportTimeRecords) Execute(ctx context.Context, actor appshared.ActorContext, f timerepo.Filter, mine bool) ([]timerepo.Placed, bool, error) {
	if err := person(actor); err != nil {
		return nil, false, err
	}
	if mine {
		f.AccountID = actor.AccountID
	}
	others := f.AccountID != actor.AccountID
	if others {
		policy, err := h.Keeper.policy(ctx, actor)
		if err != nil {
			return nil, false, err
		}
		if policy.Visibility == domain.VisibilityAggregated {
			return nil, false, shared.ErrForbidden.WithDetail("time.aggregated_only")
		}
	}
	var placed []timerepo.Placed
	if err := h.Keeper.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		placed, err = h.Keeper.Store.Records(ctx, f, maxRecordsExported+1)
		return err
	}); err != nil {
		return nil, false, err
	}
	truncated := len(placed) > maxRecordsExported
	if truncated {
		placed = placed[:maxRecordsExported]
	}
	if others {
		var err error
		if placed, err = h.Keeper.approvedRecords(ctx, actor, placed, RecordsExportedAction, itemsRead); err != nil {
			return nil, false, err
		}
	}
	now := h.Keeper.Clock.Now()
	err := h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return h.Keeper.record(ctx, actor, RecordsExportedAction, recordTarget, "", now,
			audit.Change{Field: "from", Classification: audit.Open, To: f.From.String()},
			audit.Change{Field: "to", Classification: audit.Open, To: f.To.String()},
			audit.Change{Field: "records", Classification: audit.Open, To: strconv.Itoa(len(placed))})
	})
	return placed, truncated, err
}

// ExportColumns are the export's columns, fixed so that a sheet built on them keeps its shape.
var ExportColumns = []string{
	"record_id", "day", "account_id", "hub_id", "collection_id", "entry_id", "entry_title", "work_type",
	"minutes", "hours", "billable", "source", "corrects", "note",
}

// Descriptor is the catalogue entry.
func (h ExportTimeRecords) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ExportTimeRecordsName,
		Summary: "Exports time records of a range of days for accounting, at most 50000, in fixed columns: the " +
			"caller's own with `mine`, else also other people's where the caller approves time; where the workspace " +
			"shows only sums, the caller's own only (time.aggregated_only). No invoice is made.",
		SideEffects: "Writes an audit entry of the export.",
		TokenScope:  itemsRead,
		Input:       filterInput(true),
		Audit:       usecase.AuditDeclaration{Action: RecordsExportedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			f, err := filterOf(in)
			if err != nil {
				return nil, err
			}
			mine, _ := in["mine"].(bool)
			placed, truncated, err := h.Execute(ctx, actor, f, mine)
			if err != nil {
				return nil, err
			}
			rows := make([]usecase.Output, 0, len(placed))
			for _, p := range placed {
				rows = append(rows, usecase.Output{
					"record_id": p.ID.String(), "day": p.Day.String(), "account_id": p.AccountID.String(),
					"hub_id": idText(p.HubID), "collection_id": idText(p.CollectionID), "entry_id": idText(p.EntryID),
					"entry_title": p.EntryTitle, "work_type": p.WorkType, "minutes": p.Minutes,
					"hours": float64(p.Minutes) / 60, "billable": p.Billable, "source": string(p.Source),
					"corrects": idText(p.Corrects), "note": p.Note,
				})
			}
			return usecase.Output{"columns": ExportColumns, "rows": rows, "truncated": truncated}, nil
		}),
	}, recordTarget)
}

// TimeReach answers a query's time summary whose time it counts (F-9): per collection, whether the
// reader approves time there and so sees other people's, and whether the workspace shows that
// only as sums - the question ReadTimeReport asks of its own lines, asked of the summary's places.
type TimeReach struct{ Keeper Keeper }

// Reach answers, per collection, whether other people's time on its entries is the reader's to see.
func (r TimeReach) Reach(ctx context.Context, actor appshared.ActorContext, collections []work.Container) ([]bool, bool, error) {
	if err := person(actor); err != nil {
		return nil, false, err
	}
	policy, err := r.Keeper.policy(ctx, actor)
	if err != nil {
		return nil, false, err
	}
	places := make([]place, len(collections))
	for i, c := range collections {
		places[i] = place{hub: c.ParentID, collection: c.ID}
	}
	shown, err := r.Keeper.approves(ctx, actor, places, ReportReadAction, itemsRead)
	if err != nil {
		return nil, false, err
	}
	return shown, policy.Visibility == domain.VisibilityAggregated, nil
}
