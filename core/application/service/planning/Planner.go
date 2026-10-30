// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package planning is the planning module's use cases (M2): a plan - a hub or a collection - read
// as a schedule with its critical path and the float of every entry (PLN-02, PLN-03), the moves
// the scheduler proposes applied by a person (P-04), a dependency's timing and an entry's mode
// set, milestones marked (PLN-05), and baselines taken and compared (PLN-04).
//
// A plan shows only the entries its reader may see; a dependency with an end out of reach is left
// out, so a schedule never tells of an entry the reader cannot see (T-04). Moving an entry runs
// through UpdateWorkItem, so every move is checked, recorded and announced as one by hand is.
package planning

import (
	"context"
	"strconv"
	"time"

	planrepo "github.com/Jersyfi/hubtask/core/application/repository/planning"
	deskrepo "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	domain "github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The token scopes: an entry's to read and change a plan, as its entries are read and changed.
const (
	itemsRead  = "items:read"
	itemsWrite = "items:write"
)

const (
	planTarget     = "plan"
	baselineTarget = "plan_baseline"
	linkTarget     = "entry_link"
	itemTarget     = "item"
)

// The stable audit codes (audit.md §2). A baseline's name is a person's words and goes as a
// fingerprint (rule 10).
const (
	PlanReadAction         audit.Action = "plan.read"
	PlanScheduledAction    audit.Action = "plan.scheduled"
	DependencyTimedAction  audit.Action = "entry_link.timed"
	EntryScheduledAction   audit.Action = "item.scheduling_set"
	BaselineTakenAction    audit.Action = "plan_baseline.taken"
	BaselineDeletedAction  audit.Action = "plan_baseline.deleted"
	BaselinesReadAction    audit.Action = "plan_baseline.read"
	BaselineComparedAction audit.Action = "plan_baseline.compared"
	itemReadAction         audit.Action = "item.read"
	maxLinksOfAPlan                     = 8000
	maxMovesApplied                     = work.MaxBulkOperations
	updateWorkItemName                  = "UpdateWorkItem"
)

// Authorizer records a refusal and answers it.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// Reader answers which of many paths a reader may see, without recording a refusal.
type Reader interface {
	Permitted(ctx context.Context, actor appshared.ActorContext, request access.Request, paths [][]identity.Scope) ([]bool, error)
}

// Catalogue runs one use case by name: how a proposed move is applied, through UpdateWorkItem.
type Catalogue interface {
	Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error)
}

// Planner is what every planning use case shares.
type Planner struct {
	Plans      planrepo.Plans
	Baselines  planrepo.Baselines
	Sprints    planrepo.Sprints
	Items      workrepo.Items
	Containers workrepo.Containers
	LinkTypes  workrepo.LinkTypes
	Links      workrepo.EntryLinks
	References workrepo.References
	// Calendars names the business calendars a plan may be counted on (UC-SVC-01); nil counts
	// Monday to Friday.
	Calendars  deskrepo.Calendars
	Catalogue  Catalogue
	Authorizer Authorizer
	Reader     Reader
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	Text       text.Normalizer
	// Changes and HLC tell offline devices of a dependency's new timing and an entry's new mode;
	// nil records nothing.
	Changes changelog.ChangeLog
	HLC     clock.HLCSource
}

func (p Planner) record(
	ctx context.Context, actor appshared.ActorContext, action audit.Action, target string, id shared.ID,
	now time.Time, changes ...audit.Change,
) error {
	return p.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: now, Action: action,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: target, TargetID: id,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}

// plan reads the hub or collection a plan is and asks the permission at it.
func (p Planner) plan(
	ctx context.Context, actor appshared.ActorContext, planID shared.ID, permission service.Permission,
	action audit.Action, scope string,
) (work.Container, error) {
	if planID.IsZero() {
		return work.Container{}, invalid("/plan_id", "planning.plan_required")
	}
	var container work.Container
	if err := p.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := p.Containers.Find(ctx, planID)
		container = found
		return err
	}); err != nil {
		return work.Container{}, err
	}
	if container.DeletedAt != nil {
		return work.Container{}, shared.ErrNotFound.WithDetail("containers.not_found")
	}
	if err := p.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: permission, Path: service.ContainerScopes(container), Action: action,
		TokenScope: scope, TargetType: planTarget, TargetID: container.ID,
	}); err != nil {
		return work.Container{}, err
	}
	return container, nil
}

// weekdays is the calendar a plan counts on when it names none: Monday to Friday.
type weekdays struct{}

func (weekdays) WorksOn(year int, month time.Month, day int) bool {
	weekday := time.Date(year, month, day, 12, 0, 0, 0, time.UTC).Weekday()
	return weekday != time.Saturday && weekday != time.Sunday
}

// Loaded is a plan read for the scheduler: what the reader may see of it, on its calendar.
type Loaded struct {
	Plan     domain.Plan
	Zone     *time.Location
	Entries  map[shared.ID]planrepo.PlanEntry
	External map[shared.ID]bool
	Links    []work.EntryLink
}

// load reads a plan's visible entries, their dependencies - with the visible entries outside the
// plan they wait on, kept fixed - and the calendar it counts on.
func (p Planner) load(
	ctx context.Context, actor appshared.ActorContext, planID shared.ID, calendarKey, zoneName string,
) (Loaded, error) {
	zone := time.UTC
	if zoneName != "" {
		loaded, err := time.LoadLocation(zoneName)
		if err != nil {
			return Loaded{}, invalid("/zone", "planning.zone_unknown")
		}
		zone = loaded
	}
	var (
		entries  []planrepo.PlanEntry
		links    []work.EntryLink
		registry work.LinkTypes
		days     domain.Workdays = weekdays{}
	)
	err := p.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if entries, err = p.Plans.Entries(ctx, planID, domain.MaxEntries+1); err != nil {
			return err
		}
		if len(entries) > domain.MaxEntries {
			return shared.ErrValidation.WithDetail("planning.too_many_entries").
				WithParams(map[string]string{"maximum": strconv.Itoa(domain.MaxEntries)})
		}
		if calendarKey != "" {
			if p.Calendars == nil {
				return invalid("/calendar", "calendars.not_found")
			}
			found, err := p.Calendars.Find(ctx, calendarKey)
			if err != nil {
				return err
			}
			days, zone = found.Calendar, found.Calendar.Zone()
		}
		rows, err := p.LinkTypes.List(ctx)
		if err != nil {
			return err
		}
		registry = work.NewLinkTypes(rows)
		ids := make([]shared.ID, 0, len(entries))
		for _, e := range entries {
			ids = append(ids, e.ID)
		}
		links, err = p.Links.Touching(ctx, ids, maxLinksOfAPlan)
		return err
	})
	if err != nil {
		return Loaded{}, err
	}
	paths := make([][]identity.Scope, 0, len(entries))
	for _, e := range entries {
		paths = append(paths, entryPath(e.HubID, e.CollectionID, e.ID))
	}
	visible, err := p.permitted(ctx, actor, paths)
	if err != nil {
		return Loaded{}, err
	}
	out := Loaded{Zone: zone, Entries: map[shared.ID]planrepo.PlanEntry{}, External: map[shared.ID]bool{}}
	out.Plan.Calendar = days
	for i, e := range entries {
		if visible[i] {
			out.Entries[e.ID] = e
		}
	}
	var outside []shared.ID
	waiting := make([]work.EntryLink, 0, len(links))
	for _, link := range links {
		t, ok := registry.Find(link.Type)
		if !ok || !t.Schedules() {
			continue
		}
		waiting = append(waiting, link)
		for _, end := range []shared.ID{link.SourceID, link.TargetID} {
			if _, in := out.Entries[end]; !in && !out.External[end] {
				out.External[end] = true
				outside = append(outside, end)
			}
		}
	}
	if err := p.loadOutside(ctx, actor, outside, &out); err != nil {
		return Loaded{}, err
	}
	for _, e := range entries {
		if entry, ok := out.Entries[e.ID]; ok {
			out.Plan.Entries = append(out.Plan.Entries, schedulerEntry(entry, zone))
		}
	}
	for _, id := range outside {
		if entry, ok := out.Entries[id]; ok {
			fixed := schedulerEntry(entry, zone)
			fixed.Mode, fixed.ParentID = domain.Manual, ""
			out.Plan.Entries = append(out.Plan.Entries, fixed)
		}
	}
	for _, link := range waiting {
		t, _ := registry.Find(link.Type)
		waiter, waitedFor, _ := link.Dependency(t)
		_, inWaiter := out.Entries[waiter]
		_, inWaited := out.Entries[waitedFor]
		if !inWaiter || !inWaited || (out.External[waiter] && out.External[waitedFor]) {
			continue
		}
		out.Links = append(out.Links, link)
		out.Plan.Dependencies = append(out.Plan.Dependencies, domain.Dependency{
			Predecessor: waitedFor, Successor: waiter, Timing: link.Timing,
		})
	}
	return out, nil
}

// loadOutside adds the entries outside the plan that a plan's entry waits on or blocks, where the
// reader may see them; the rest stay out, and the dependency with them.
func (p Planner) loadOutside(ctx context.Context, actor appshared.ActorContext, ids []shared.ID, out *Loaded) error {
	if len(ids) == 0 {
		return nil
	}
	var (
		summaries []workrepo.EntrySummary
		schedules map[shared.ID]planrepo.EntrySchedule
	)
	if err := p.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if summaries, err = p.References.Entries(ctx, ids); err != nil {
			return err
		}
		schedules, err = p.Plans.Schedules(ctx, ids)
		return err
	}); err != nil {
		return err
	}
	paths := make([][]identity.Scope, 0, len(summaries))
	for _, s := range summaries {
		paths = append(paths, entryPath(s.HubID, s.CollectionID, s.ID))
	}
	visible, err := p.permitted(ctx, actor, paths)
	if err != nil {
		return err
	}
	for i, s := range summaries {
		if !visible[i] || s.Archived {
			continue
		}
		out.Entries[s.ID] = planrepo.PlanEntry{
			ID: s.ID, CollectionID: s.CollectionID, HubID: s.HubID, Title: s.Title, StartAt: s.StartAt, DueAt: s.DueAt,
			Done: s.Completed, Mode: domain.Manual, Milestone: schedules[s.ID].Milestone,
		}
	}
	return nil
}

func (p Planner) permitted(ctx context.Context, actor appshared.ActorContext, paths [][]identity.Scope) ([]bool, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	allowed, err := p.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead, Action: itemReadAction, TokenScope: itemsRead, TargetType: itemTarget,
		On: access.ItemSubject{Does: service.ItemRead},
	}, paths)
	if err != nil {
		return nil, err
	}
	for len(allowed) < len(paths) {
		allowed = append(allowed, false)
	}
	return allowed, nil
}

// entryPath is the scope chain an entry is judged on, ending on the entry, so that an entry shared
// with the reader alone is visible.
func entryPath(hub, collection, entry shared.ID) []identity.Scope {
	path := []identity.Scope{identity.TenantScope()}
	if !hub.IsZero() {
		path = append(path, identity.HubScope(hub))
	}
	return append(path, identity.CollectionScope(collection), identity.ItemScope(entry))
}

// schedulerEntry reads an entry's dates as days: an all-day due date in its own zone, every other
// instant in the plan's.
func schedulerEntry(e planrepo.PlanEntry, zone *time.Location) domain.Entry {
	entry := domain.Entry{ID: e.ID, ParentID: e.ParentID, Mode: e.Mode, Milestone: e.Milestone, Done: e.Done}
	if !entry.Mode.Valid() {
		entry.Mode = domain.Manual
	}
	if e.StartAt != nil {
		day := domain.DayOf(*e.StartAt, zone)
		entry.Start = &day
	}
	if e.DueAt != nil {
		dueZone := zone
		if e.DueDateOnly && e.DueTimeZone != "" {
			if loaded, err := time.LoadLocation(e.DueTimeZone); err == nil {
				dueZone = loaded
			}
		}
		day := domain.DayOf(*e.DueAt, dueZone)
		entry.Finish = &day
	}
	return entry
}

func invalid(path, code string) error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}

// descriptor is the shape every planning use case shares.
func descriptor(d usecase.Descriptor, target string) usecase.Descriptor {
	d.Module = module.Planning
	if d.Audit.Action != "" {
		d.Audit.TargetType, d.Audit.Severity, d.Audit.Required = target, audit.SeverityInfo, !d.ReadOnly
	}
	if d.ReadOnly && d.Activity.Exempt == "" {
		d.Activity.Exempt = "a read changes nothing."
	}
	return d
}
