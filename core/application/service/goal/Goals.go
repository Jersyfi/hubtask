// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"context"
	"errors"
	"sort"

	goalrepo "github.com/Jersyfi/hubtask/core/application/repository/goal"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The catalogue names.
const (
	ListGoalPeriodsName  = "ListGoalPeriods"
	DefineGoalPeriodName = "DefineGoalPeriod"
	CloseGoalPeriodName  = "CloseGoalPeriod"
	ListGoalsName        = "ListGoals"
	ReadGoalName         = "ReadGoal"
	CreateGoalName       = "CreateGoal"
	UpdateGoalName       = "UpdateGoal"
	DeleteGoalName       = "DeleteGoal"
	LinkGoalEntriesName  = "LinkGoalEntries"
	UnlinkGoalEntryName  = "UnlinkGoalEntry"
	CheckInGoalName      = "CheckInGoal"
	ListGoalCheckInsName = "ListGoalCheckIns"
	ListEntryGoalsName   = "ListEntryGoals"
)

var workspace = []identity.Scope{identity.TenantScope()}

// PeriodOutput is a period on the wire.
func PeriodOutput(p domain.Period) usecase.Output {
	return usecase.Output{
		"id": p.ID.String(), "name": p.Name, "start_on": p.StartOn.String(), "end_on": p.EndOn.String(),
		"state": string(p.State), "version": p.Version,
	}
}

// ListGoalPeriods answers the workspace's periods, the latest first, to its readers.
type ListGoalPeriods struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h ListGoalPeriods) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListGoalPeriodsName, Permission: service.PermissionRead, ReadOnly: true,
		Summary:     "The workspace's goal periods - quarters, years - the latest first, open and closed.",
		SideEffects: "None. Reads only.", TokenScope: itemsRead,
		Audit: usecase.AuditDeclaration{Action: PeriodsReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			s := h.Steward
			if err := s.authorize(ctx, actor, service.PermissionRead, workspace, PeriodsReadAction, itemsRead, periodTarget, ""); err != nil {
				return nil, err
			}
			var periods []domain.Period
			if err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				periods, err = s.Store.Periods(ctx, maxPeriods)
				return err
			}); err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(periods))
			for _, p := range periods {
				data = append(data, PeriodOutput(p))
			}
			return usecase.Output{"data": data}, nil
		}),
	}, periodTarget)
}

// DefineGoalPeriod creates a period, or renames and re-dates an open one, with STRUCTURE at the
// workspace.
type DefineGoalPeriod struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h DefineGoalPeriod) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DefineGoalPeriodName, Permission: service.PermissionStructure,
		Summary: "Creates a goal period with a name and its first and last day, or - with period_id and " +
			"the version read - changes an open one. At most two years long. Needs STRUCTURE at the workspace.",
		SideEffects: "Writes the period and an audit entry.", TokenScope: itemsWrite,
		Input: []usecase.Field{
			{Name: "period_id", Kind: usecase.KindID, Description: "The open period to change; a new one without."},
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "The period's name, e.g. 2026 Q4."},
			{Name: "start_on", Kind: usecase.KindString, Required: true, Description: "The first day, YYYY-MM-DD."},
			{Name: "end_on", Kind: usecase.KindString, Required: true, Description: "The last day, YYYY-MM-DD."},
			{Name: "expected_version", Kind: usecase.KindAny, CallerOnly: true, Description: "The version read, when changing one."},
		},
		Audit: usecase.AuditDeclaration{Action: PeriodDefinedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			id, err := optionalID(in, "period_id")
			if err != nil {
				return nil, err
			}
			start, err := dayOf(in, "start_on", "/start_on")
			if err != nil || start == nil {
				return nil, invalid("/start_on", "goals.day_invalid")
			}
			end, err := dayOf(in, "end_on", "/end_on")
			if err != nil || end == nil {
				return nil, invalid("/end_on", "goals.day_invalid")
			}
			p, err := domain.CheckPeriod(domain.Period{ID: id, Name: in.String("name"), StartOn: *start, EndOn: *end}, s.Text)
			if err != nil {
				return nil, err
			}
			if err := s.authorize(ctx, actor, service.PermissionStructure, workspace, PeriodDefinedAction, itemsWrite, periodTarget, id); err != nil {
				return nil, err
			}
			expected, err := versionOf(in, !id.IsZero())
			if err != nil {
				return nil, err
			}
			now := s.Clock.Now()
			err = s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if id.IsZero() {
					p.ID = s.IDs.NewID()
					if err := s.Store.InsertPeriod(ctx, p, actor.AccountID, now); err != nil {
						return err
					}
				} else {
					existing, err := s.Store.Period(ctx, id)
					if err != nil {
						return err
					}
					if existing.State == domain.PeriodClosed {
						return shared.ErrConflict.WithDetail("goal_periods.closed")
					}
					if err := s.Store.UpdatePeriod(ctx, p, expected, now); err != nil {
						return err
					}
				}
				var err error
				if p, err = s.Store.Period(ctx, p.ID); err != nil {
					return err
				}
				return s.record(ctx, actor, PeriodDefinedAction, periodTarget, p.ID, now)
			})
			if err != nil {
				return nil, err
			}
			return PeriodOutput(p), nil
		}),
	}, periodTarget)
}

// CloseGoalPeriod closes a period and grades every goal of it: the grade given, or the goal's
// progress as the closer sees it - a goal with no progress grades 0. A closed period's goals take
// no check-in.
type CloseGoalPeriod struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h CloseGoalPeriod) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: CloseGoalPeriodName, Permission: service.PermissionStructure,
		Summary: "Closes an open goal period and grades each of its goals 0 to 1: the grade given per " +
			"goal, else the goal's progress. Needs STRUCTURE at the workspace.",
		SideEffects: "Closes the period, grades its goals and writes an audit entry.", TokenScope: itemsWrite,
		Input: []usecase.Field{
			{Name: "period_id", Kind: usecase.KindID, Required: true, Description: "The open period."},
			{Name: "grades", Kind: usecase.KindObject, Description: "A grade 0 to 1 per goal identifier; a goal left out is graded by its progress."},
			{Name: "expected_version", Kind: usecase.KindAny, Required: true, CallerOnly: true, Description: "The version read."},
		},
		Audit: usecase.AuditDeclaration{Action: PeriodClosedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			id, err := in.ID("period_id")
			if err != nil {
				return nil, err
			}
			expected, err := versionOf(in, true)
			if err != nil {
				return nil, err
			}
			given := map[shared.ID]float64{}
			raw, _ := in["grades"].(map[string]any)
			for key, value := range raw {
				goalID, err := shared.ParseID(key)
				if err != nil {
					return nil, invalid("/grades", "goal_periods.grade_goal_invalid")
				}
				grade, ok := number(value)
				if !ok {
					return nil, invalid("/grades", "goal_periods.grade_out_of_range")
				}
				if err := domain.CheckGrade(grade); err != nil {
					return nil, err
				}
				given[goalID] = grade
			}
			if err := s.authorize(ctx, actor, service.PermissionStructure, workspace, PeriodClosedAction, itemsWrite, periodTarget, id); err != nil {
				return nil, err
			}
			sight, err := s.sight(ctx, actor, nil)
			if err != nil {
				return nil, err
			}
			now := s.Clock.Now()
			var period domain.Period
			err = s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				goals, err := s.Store.Goals(ctx, goalrepo.GoalFilter{PeriodID: id, Max: maxGoals})
				if err != nil {
					return err
				}
				grades := map[shared.ID]float64{}
				for _, g := range goals {
					if grade, ok := given[g.ID]; ok {
						grades[g.ID] = grade
					} else if p, ok := sight.Progress(g.ID); ok {
						grades[g.ID] = p
					} else {
						grades[g.ID] = 0
					}
				}
				for goalID := range given {
					if _, ok := grades[goalID]; !ok {
						return invalid("/grades", "goal_periods.grade_goal_invalid")
					}
				}
				if err := s.Store.ClosePeriod(ctx, id, expected, grades, actor.AccountID, now); err != nil {
					return err
				}
				if period, err = s.Store.Period(ctx, id); err != nil {
					return err
				}
				return s.record(ctx, actor, PeriodClosedAction, periodTarget, id, now)
			})
			if err != nil {
				return nil, err
			}
			return PeriodOutput(period), nil
		}),
	}, periodTarget)
}

// ListGoals answers the goals a reader may see, narrowed by a period, a hub, an owner or a parent,
// each with its progress for that reader - the alignment tree is drawn from parent_id.
type ListGoals struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h ListGoals) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListGoalsName, Permission: service.PermissionRead, ReadOnly: true,
		Summary: "The goals the caller may read - the workspace's and those of hubs they read - narrowed " +
			"by period, hub, owner or parent, each with its key results and its progress as the caller sees it.",
		SideEffects: "None. Reads only.", TokenScope: itemsRead,
		Input: []usecase.Field{
			{Name: "period_id", Kind: usecase.KindID, Description: "Only this period's goals."},
			{Name: "hub_id", Kind: usecase.KindID, Description: "Only this hub's goals."},
			{Name: "workspace_only", Kind: usecase.KindBool, Description: "Only the workspace's own goals, in no hub."},
			{Name: "owner_id", Kind: usecase.KindID, Description: "Only the goals this person owns."},
			{Name: "parent_id", Kind: usecase.KindID, Description: "Only the goals under this goal."},
		},
		Audit: usecase.AuditDeclaration{Action: GoalReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := person(actor); err != nil {
				return nil, err
			}
			if err := actor.RequireScope(itemsRead); err != nil {
				return nil, err
			}
			filter := map[string]shared.ID{}
			for _, field := range []string{"period_id", "hub_id", "owner_id", "parent_id"} {
				id, err := optionalID(in, field)
				if err != nil {
					return nil, err
				}
				filter[field] = id
			}
			sight, err := h.Steward.sight(ctx, actor, nil)
			if err != nil {
				return nil, err
			}
			data := []usecase.Output{}
			for _, id := range sight.Order {
				g := sight.Goals[id]
				switch {
				case !filter["period_id"].IsZero() && g.PeriodID != filter["period_id"],
					!filter["hub_id"].IsZero() && g.HubID != filter["hub_id"],
					in.Bool("workspace_only") && !g.HubID.IsZero(),
					!filter["owner_id"].IsZero() && g.OwnerID != filter["owner_id"],
					!filter["parent_id"].IsZero() && g.ParentID != filter["parent_id"]:
					continue
				}
				data = append(data, sight.GoalOutput(g))
			}
			return usecase.Output{"data": data}, nil
		}),
	}, goalTarget)
}

// readable reads a goal the reader may see; one they may not is not found.
func (s Steward) readable(ctx context.Context, actor appshared.ActorContext, id shared.ID) (domain.Goal, error) {
	if err := person(actor); err != nil {
		return domain.Goal{}, err
	}
	if err := actor.RequireScope(itemsRead); err != nil {
		return domain.Goal{}, err
	}
	var g domain.Goal
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		g, err = s.Store.Goal(ctx, id)
		return err
	})
	if err != nil {
		return domain.Goal{}, err
	}
	reads, err := s.readsPlaces(ctx, actor, []shared.ID{g.HubID}, itemsRead, goalTarget)
	if err != nil {
		return domain.Goal{}, err
	}
	if !reads[g.HubID] {
		return domain.Goal{}, notFound("goals.not_found")
	}
	return g, nil
}

// writable reads a goal and asks WRITE_ITEMS at its place.
func (s Steward) writable(ctx context.Context, actor appshared.ActorContext, id shared.ID, action audit.Action) (domain.Goal, error) {
	g, err := s.readable(ctx, actor, id)
	if err != nil {
		return domain.Goal{}, err
	}
	if err := s.authorize(ctx, actor, service.PermissionWriteItems, placePath(g.HubID), action, itemsWrite, goalTarget, g.ID); err != nil {
		return domain.Goal{}, err
	}
	return g, nil
}

// ReadGoal answers one goal with its key results, its progress, the linked entries the reader may
// read and its latest check-ins.
type ReadGoal struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h ReadGoal) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadGoalName, Permission: service.PermissionRead, ReadOnly: true,
		Summary: "One goal with its key results and progress, the linked entries the caller may read and " +
			"its latest check-ins. A goal the caller may not read answers not found.",
		SideEffects: "None. Reads only.", TokenScope: itemsRead,
		Input: []usecase.Field{{Name: "goal_id", Kind: usecase.KindID, Required: true, Description: "The goal."}},
		Audit: usecase.AuditDeclaration{Action: GoalReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			id, err := in.ID("goal_id")
			if err != nil {
				return nil, err
			}
			g, err := s.readable(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			sight, err := s.sight(ctx, actor, []shared.ID{id})
			if err != nil {
				return nil, err
			}
			var checkIns []domain.CheckIn
			if err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				checkIns, err = s.Store.CheckIns(ctx, id, 20)
				return err
			}); err != nil {
				return nil, err
			}
			out := sight.GoalOutput(g)
			entries := []usecase.Output{}
			for _, e := range sight.Entries[id] {
				key := any(nil)
				if e.Handle != "" {
					key = e.Handle + "-" + itoa(int(e.Number))
				}
				entries = append(entries, usecase.Output{
					"id": e.ID.String(), "title": e.Title, "key": key, "completed": e.Completed,
					"collection_id": e.CollectionID.String(),
				})
			}
			out["linked_entries"] = entries
			out["check_ins"] = checkInsOutput(checkIns)
			return out, nil
		}),
	}, goalTarget)
}

// goalFields reads a goal's fields; key results keep the identifier they came with or get one.
func (s Steward) goalFields(in usecase.Input) (domain.GoalFields, error) {
	f := domain.GoalFields{Title: in.String("title"), Description: in.String("description"), Status: domain.Status(in.String("status"))}
	var err error
	for _, field := range []struct {
		name string
		to   *shared.ID
	}{{"period_id", &f.PeriodID}, {"hub_id", &f.HubID}, {"parent_id", &f.ParentID}, {"owner_id", &f.OwnerID}} {
		if *field.to, err = optionalID(in, field.name); err != nil {
			return domain.GoalFields{}, err
		}
	}
	raw, _ := in["key_results"].([]any)
	for i, item := range raw {
		at := "/key_results/" + itoa(i)
		doc, ok := item.(map[string]any)
		if !ok {
			return domain.GoalFields{}, invalid(at, "key_results.malformed")
		}
		r := domain.KeyResult{
			Title: stringOf(doc["title"]), Kind: domain.ResultKind(stringOf(doc["kind"])),
			Unit: stringOf(doc["unit"]), Source: domain.ResultSource(stringOf(doc["source"])),
		}
		for _, n := range []struct {
			name string
			to   *float64
		}{{"start", &r.Start}, {"target", &r.Target}, {"current", &r.Current}} {
			if v, present := doc[n.name]; present && v != nil {
				value, ok := number(v)
				if !ok {
					return domain.GoalFields{}, invalid(at+"/"+n.name, "key_results.value_invalid")
				}
				*n.to = value
			}
		}
		for _, n := range []struct {
			name string
			to   *shared.ID
		}{{"id", &r.ID}, {"view_id", &r.ViewID}, {"entry_id", &r.EntryID}} {
			if text := stringOf(doc[n.name]); text != "" {
				id, err := shared.ParseID(text)
				if err != nil {
					return domain.GoalFields{}, invalid(at+"/"+n.name, "key_results.id_invalid")
				}
				*n.to = id
			}
		}
		if r.ID.IsZero() {
			r.ID = s.IDs.NewID()
		}
		f.KeyResults = append(f.KeyResults, r)
	}
	return domain.CheckGoal(f, s.Text)
}

func stringOf(v any) string {
	text, _ := v.(string)
	return text
}

// parent checks a goal's parent: one the writer may read, never the goal or a goal under it.
func (s Steward) parent(ctx context.Context, actor appshared.ActorContext, goal, parent shared.ID) error {
	if parent.IsZero() {
		return nil
	}
	if _, err := s.readable(ctx, actor, parent); err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return shared.ErrNotFound.WithDetail("goals.parent_not_found").WithFields(shared.FieldError{Path: "/parent_id", Code: "goals.parent_not_found"})
		}
		return err
	}
	var chain []shared.ID
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		for next := parent; !next.IsZero() && len(chain) <= domain.MaxDepth; {
			chain = append(chain, next)
			g, err := s.Store.Goal(ctx, next)
			if err != nil {
				return err
			}
			next = g.ParentID
		}
		return nil
	})
	if err != nil {
		return err
	}
	return domain.Parenting(goal, chain)
}

// period checks a goal's period exists and is open.
func (s Steward) period(ctx context.Context, actor appshared.ActorContext, id shared.ID) error {
	var p domain.Period
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		p, err = s.Store.Period(ctx, id)
		return err
	})
	if errors.Is(err, shared.ErrNotFound) {
		return shared.ErrNotFound.WithDetail("goal_periods.not_found").WithFields(shared.FieldError{Path: "/period_id", Code: "goal_periods.not_found"})
	}
	if err != nil {
		return err
	}
	if p.State == domain.PeriodClosed {
		return shared.ErrConflict.WithDetail("goal_periods.closed")
	}
	return nil
}

// sources checks a writer may read what a key result is fed by: a milestone's entry.
func (s Steward) sources(ctx context.Context, actor appshared.ActorContext, results []domain.KeyResult) error {
	var entries []shared.ID
	for _, r := range results {
		if r.Kind == domain.ResultMilestone {
			entries = append(entries, r.EntryID)
		}
	}
	readable, err := s.readableEntries(ctx, actor, entries)
	if err != nil {
		return err
	}
	for i, r := range results {
		if r.Kind == domain.ResultMilestone {
			if _, ok := readable[r.EntryID]; !ok {
				return invalid("/key_results/"+itoa(i)+"/entry_id", "items.not_found")
			}
		}
	}
	return nil
}

var goalInput = []usecase.Field{
	{Name: "period_id", Kind: usecase.KindID, Required: true, Description: "The period the goal is for."},
	{Name: "title", Kind: usecase.KindString, Required: true, Description: "The objective, at most 200 characters."},
	{Name: "description", Kind: usecase.KindString, Description: "Why it matters."},
	{Name: "owner_id", Kind: usecase.KindID, Description: "Who owns it."},
	{Name: "parent_id", Kind: usecase.KindID, Description: "The goal it contributes to; never itself or one of its own."},
	{Name: "status", Kind: usecase.KindString, Enum: []string{"NONE", "ON_TRACK", "AT_RISK", "OFF_TRACK", "DONE", "DROPPED"}, Description: "How it stands; a check-in sets it too."},
	{Name: "key_results", Kind: usecase.KindList, Description: "At most 20, in order: title, kind (NUMBER, PERCENT, BINARY, MILESTONE), start, target, current, unit, source (MANUAL or VIEW with view_id, a saved summary view whose total is the value), entry_id for a MILESTONE; id to keep one."},
}

// CreateGoal sets a goal for a period at the workspace or in a hub, with WRITE_ITEMS there.
type CreateGoal struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h CreateGoal) Descriptor() usecase.Descriptor {
	input := append([]usecase.Field{{Name: "hub_id", Kind: usecase.KindID, Description: "The hub the goal belongs to; the workspace's without."}}, goalInput...)
	return descriptor(usecase.Descriptor{
		Name: CreateGoalName, Permission: service.PermissionWriteItems,
		Summary: "Sets a goal for an open period, at the workspace or in a hub, with its key results and an " +
			"optional parent goal. Needs WRITE_ITEMS at the workspace or the hub.",
		SideEffects: "Writes the goal and an audit entry.", TokenScope: itemsWrite, Input: input,
		Audit: usecase.AuditDeclaration{Action: GoalCreatedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			if err := person(actor); err != nil {
				return nil, err
			}
			f, err := s.goalFields(in)
			if err != nil {
				return nil, err
			}
			if err := s.hub(ctx, actor, f.HubID); err != nil {
				return nil, err
			}
			if err := s.authorize(ctx, actor, service.PermissionWriteItems, placePath(f.HubID), GoalCreatedAction, itemsWrite, goalTarget, ""); err != nil {
				return nil, err
			}
			id := s.IDs.NewID()
			if err := s.period(ctx, actor, f.PeriodID); err != nil {
				return nil, err
			}
			if err := s.parent(ctx, actor, id, f.ParentID); err != nil {
				return nil, err
			}
			if err := s.sources(ctx, actor, f.KeyResults); err != nil {
				return nil, err
			}
			g := domain.Goal{
				ID: id, PeriodID: f.PeriodID, HubID: f.HubID, ParentID: f.ParentID, Title: f.Title,
				Description: f.Description, OwnerID: f.OwnerID, Status: f.Status, Confidence: -1, KeyResults: f.KeyResults,
			}
			if g.OwnerID.IsZero() {
				g.OwnerID = actor.AccountID
			}
			now := s.Clock.Now()
			if err := s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := s.Store.InsertGoal(ctx, g, actor.AccountID, now); err != nil {
					return err
				}
				return s.record(ctx, actor, GoalCreatedAction, goalTarget, id, now)
			}); err != nil {
				return nil, err
			}
			return h.Steward.answer(ctx, actor, id)
		}),
	}, goalTarget)
}

// answer is a goal as its writer sees it after the write.
func (s Steward) answer(ctx context.Context, actor appshared.ActorContext, id shared.ID) (usecase.Output, error) {
	sight, err := s.sight(ctx, actor, []shared.ID{id})
	if err != nil {
		return nil, err
	}
	g, ok := sight.Goals[id]
	if !ok {
		return nil, notFound("goals.not_found")
	}
	return sight.GoalOutput(g), nil
}

// UpdateGoal replaces a goal's fields and key results, held to the version read; its hub stays.
type UpdateGoal struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h UpdateGoal) Descriptor() usecase.Descriptor {
	input := append([]usecase.Field{
		{Name: "goal_id", Kind: usecase.KindID, Required: true, Description: "The goal."},
		{Name: "expected_version", Kind: usecase.KindAny, Required: true, CallerOnly: true, Description: "The version read."},
	}, goalInput...)
	return descriptor(usecase.Descriptor{
		Name: UpdateGoalName, Permission: service.PermissionWriteItems,
		Summary: "Replaces a goal's period, title, description, owner, parent, status and key results, held " +
			"to the version read; a key result keeps its values by its id. A graded goal does not change. " +
			"Needs WRITE_ITEMS at the goal's place.",
		SideEffects: "Writes the goal and an audit entry.", TokenScope: itemsWrite, Input: input,
		Audit: usecase.AuditDeclaration{Action: GoalUpdatedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			id, err := in.ID("goal_id")
			if err != nil {
				return nil, err
			}
			expected, err := versionOf(in, true)
			if err != nil {
				return nil, err
			}
			f, err := s.goalFields(in)
			if err != nil {
				return nil, err
			}
			g, err := s.writable(ctx, actor, id, GoalUpdatedAction)
			if err != nil {
				return nil, err
			}
			if g.Grade != nil {
				return nil, shared.ErrConflict.WithDetail("goals.graded")
			}
			if f.PeriodID != g.PeriodID {
				if err := s.period(ctx, actor, f.PeriodID); err != nil {
					return nil, err
				}
			}
			if f.ParentID != g.ParentID {
				if err := s.parent(ctx, actor, id, f.ParentID); err != nil {
					return nil, err
				}
			}
			if err := s.sources(ctx, actor, f.KeyResults); err != nil {
				return nil, err
			}
			g.PeriodID, g.ParentID, g.Title, g.Description, g.Status, g.KeyResults =
				f.PeriodID, f.ParentID, f.Title, f.Description, f.Status, f.KeyResults
			if !f.OwnerID.IsZero() {
				g.OwnerID = f.OwnerID
			}
			now := s.Clock.Now()
			if err := s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := s.Store.UpdateGoal(ctx, g, expected, actor.AccountID, now); err != nil {
					return err
				}
				return s.record(ctx, actor, GoalUpdatedAction, goalTarget, id, now)
			}); err != nil {
				return nil, err
			}
			return s.answer(ctx, actor, id)
		}),
	}, goalTarget)
}

// DeleteGoal removes a goal with its links and check-ins; the goals under it move to its parent.
type DeleteGoal struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h DeleteGoal) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DeleteGoalName, Permission: service.PermissionWriteItems, Destructive: true,
		Summary: "Deletes a goal, its links to entries and its check-ins, held to the version read; the goals " +
			"under it move up to its parent. The entries stay. Needs WRITE_ITEMS at the goal's place.",
		SideEffects: "Deletes the goal and writes an audit entry.", TokenScope: itemsWrite,
		Input: []usecase.Field{
			{Name: "goal_id", Kind: usecase.KindID, Required: true, Description: "The goal."},
			{Name: "expected_version", Kind: usecase.KindAny, Required: true, CallerOnly: true, Description: "The version read."},
		},
		Audit: usecase.AuditDeclaration{Action: GoalDeletedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			id, err := in.ID("goal_id")
			if err != nil {
				return nil, err
			}
			expected, err := versionOf(in, true)
			if err != nil {
				return nil, err
			}
			g, err := s.writable(ctx, actor, id, GoalDeletedAction)
			if err != nil {
				return nil, err
			}
			now := s.Clock.Now()
			if err := s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := s.Store.DeleteGoal(ctx, g, expected); err != nil {
					return err
				}
				return s.record(ctx, actor, GoalDeletedAction, goalTarget, id, now)
			}); err != nil {
				return nil, err
			}
			return usecase.Output{"deleted": true}, nil
		}),
	}, goalTarget)
}

// LinkGoalEntries links entries the writer may read to a goal; its progress counts the share done.
type LinkGoalEntries struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h LinkGoalEntries) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: LinkGoalEntriesName, Permission: service.PermissionWriteItems,
		Summary: "Links entries to a goal - at most 500 per goal -, each one the caller may read; one linked " +
			"already stays. The goal's progress counts the share of its linked entries completed. Needs " +
			"WRITE_ITEMS at the goal's place.",
		SideEffects: "Writes the links and an audit entry.", TokenScope: itemsWrite,
		Input: []usecase.Field{
			{Name: "goal_id", Kind: usecase.KindID, Required: true, Description: "The goal."},
			{Name: "entry_ids", Kind: usecase.KindIDList, Required: true, Description: "The entries."},
		},
		Audit: usecase.AuditDeclaration{Action: GoalEntriesLinkedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			id, err := in.ID("goal_id")
			if err != nil {
				return nil, err
			}
			entries, err := in.IDList("entry_ids")
			if err != nil {
				return nil, err
			}
			if len(entries) == 0 || len(entries) > domain.MaxLinkedEntries {
				return nil, invalid("/entry_ids", "goals.entries_count")
			}
			if _, err := s.writable(ctx, actor, id, GoalEntriesLinkedAction); err != nil {
				return nil, err
			}
			readable, err := s.readableEntries(ctx, actor, entries)
			if err != nil {
				return nil, err
			}
			for _, e := range entries {
				if _, ok := readable[e]; !ok {
					return nil, appshared.ItemNotFound(e)
				}
			}
			now := s.Clock.Now()
			if err := s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				linked, err := s.Store.LinkedEntries(ctx, []shared.ID{id})
				if err != nil {
					return err
				}
				union := map[shared.ID]bool{}
				for _, e := range append(linked[id], entries...) {
					union[e] = true
				}
				if len(union) > domain.MaxLinkedEntries {
					return invalid("/entry_ids", "goals.entries_count")
				}
				if err := s.Store.LinkEntries(ctx, id, entries, actor.AccountID, now); err != nil {
					return err
				}
				return s.record(ctx, actor, GoalEntriesLinkedAction, goalTarget, id, now)
			}); err != nil {
				return nil, err
			}
			return s.answer(ctx, actor, id)
		}),
	}, goalTarget)
}

// UnlinkGoalEntry removes an entry's link to a goal.
type UnlinkGoalEntry struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h UnlinkGoalEntry) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: UnlinkGoalEntryName, Permission: service.PermissionWriteItems,
		Summary:     "Removes an entry's link to a goal; the entry stays. Needs WRITE_ITEMS at the goal's place.",
		SideEffects: "Removes the link and writes an audit entry.", TokenScope: itemsWrite,
		Input: []usecase.Field{
			{Name: "goal_id", Kind: usecase.KindID, Required: true, Description: "The goal."},
			{Name: "entry_id", Kind: usecase.KindID, Required: true, Description: "The entry."},
		},
		Audit: usecase.AuditDeclaration{Action: GoalEntryUnlinkedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			id, err := in.ID("goal_id")
			if err != nil {
				return nil, err
			}
			entry, err := in.ID("entry_id")
			if err != nil {
				return nil, err
			}
			if _, err := s.writable(ctx, actor, id, GoalEntryUnlinkedAction); err != nil {
				return nil, err
			}
			now := s.Clock.Now()
			if err := s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				removed, err := s.Store.UnlinkEntry(ctx, id, entry)
				if err != nil {
					return err
				}
				if !removed {
					return notFound("goals.entry_not_linked")
				}
				return s.record(ctx, actor, GoalEntryUnlinkedAction, goalTarget, id, now)
			}); err != nil {
				return nil, err
			}
			return s.answer(ctx, actor, id)
		}),
	}, goalTarget)
}

// ListEntryGoals answers the goals an entry is linked to, beside its links: the ones the reader may
// read, each with its progress as the reader sees it.
type ListEntryGoals struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h ListEntryGoals) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListEntryGoalsName, Permission: service.PermissionRead, ReadOnly: true,
		Summary: "The goals an entry is linked to that the caller may read, each with its key results and its " +
			"progress as the caller sees it. An entry the caller may not read is not found.",
		SideEffects: "None. Reads only.", TokenScope: itemsRead,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The entry."},
		},
		Audit: usecase.AuditDeclaration{Action: GoalReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			if err := person(actor); err != nil {
				return nil, err
			}
			if err := actor.RequireScope(itemsRead); err != nil {
				return nil, err
			}
			entry, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			readable, err := s.readableEntries(ctx, actor, []shared.ID{entry})
			if err != nil {
				return nil, err
			}
			if _, ok := readable[entry]; !ok {
				return nil, appshared.ItemNotFound(entry)
			}
			var ids []shared.ID
			if err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				ids, err = s.Store.GoalsOfEntry(ctx, entry, maxGoals)
				return err
			}); err != nil {
				return nil, err
			}
			data := []usecase.Output{}
			if len(ids) == 0 {
				return usecase.Output{"data": data}, nil
			}
			linked := make(map[shared.ID]bool, len(ids))
			for _, id := range ids {
				linked[id] = true
			}
			sight, err := s.sight(ctx, actor, nil)
			if err != nil {
				return nil, err
			}
			for _, id := range sight.Order {
				if linked[id] {
					data = append(data, sight.GoalOutput(sight.Goals[id]))
				}
			}
			return usecase.Output{"data": data}, nil
		}),
	}, goalTarget)
}

func checkInsOutput(checkIns []domain.CheckIn) []usecase.Output {
	out := make([]usecase.Output, 0, len(checkIns))
	for _, c := range checkIns {
		keys := make([]string, 0, len(c.Values))
		for id := range c.Values {
			keys = append(keys, id.String())
		}
		sort.Strings(keys)
		values := make([]usecase.Output, 0, len(keys))
		for _, k := range keys {
			values = append(values, usecase.Output{"key_result_id": k, "value": c.Values[shared.ID(k)]})
		}
		out = append(out, usecase.Output{
			"id": c.ID.String(), "goal_id": c.GoalID.String(), "author_id": idText(c.AuthorID),
			"status": string(c.Status), "confidence": c.Confidence, "note": c.Note, "values": values,
			"created_at": c.At,
		})
	}
	return out
}

// CheckInGoal records how a goal stands: a status, a confidence 0 to 10, a note and the values of
// its MANUAL key results. The goal takes the status, the confidence and the values.
type CheckInGoal struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h CheckInGoal) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: CheckInGoalName, Permission: service.PermissionWriteItems,
		Summary: "Checks in on a goal: its status, a confidence from 0 to 10, a note, and new values of its " +
			"key results that are reported by hand. Kept as history. A graded goal takes none. Needs " +
			"WRITE_ITEMS at the goal's place.",
		SideEffects: "Writes the check-in, updates the goal and writes an audit entry.", TokenScope: itemsWrite,
		Input: []usecase.Field{
			{Name: "goal_id", Kind: usecase.KindID, Required: true, Description: "The goal."},
			{Name: "status", Kind: usecase.KindString, Required: true, Enum: []string{"ON_TRACK", "AT_RISK", "OFF_TRACK", "DONE", "DROPPED"}, Description: "How it stands."},
			{Name: "confidence", Kind: usecase.KindInt, Required: true, Description: "How confident, 0 to 10."},
			{Name: "note", Kind: usecase.KindString, Description: "What happened, at most 5000 characters."},
			{Name: "values", Kind: usecase.KindList, Description: "key_result_id and value per key result reported by hand."},
		},
		Audit: usecase.AuditDeclaration{Action: GoalCheckedInAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			id, err := in.ID("goal_id")
			if err != nil {
				return nil, err
			}
			confidence, ok := intOf(in["confidence"])
			if !ok {
				return nil, invalid("/confidence", "check_ins.confidence_out_of_range")
			}
			values := map[shared.ID]float64{}
			raw, _ := in["values"].([]any)
			for i, item := range raw {
				doc, _ := item.(map[string]any)
				resultID, err := shared.ParseID(stringOf(doc["key_result_id"]))
				if err != nil {
					return nil, invalid("/values/"+itoa(i)+"/key_result_id", "check_ins.key_result_unknown")
				}
				value, ok := number(doc["value"])
				if !ok {
					return nil, invalid("/values/"+itoa(i)+"/value", "key_results.value_invalid")
				}
				values[resultID] = value
			}
			g, err := s.writable(ctx, actor, id, GoalCheckedInAction)
			if err != nil {
				return nil, err
			}
			c, err := domain.CheckCheckIn(domain.CheckIn{
				ID: s.IDs.NewID(), GoalID: id, AuthorID: actor.AccountID, Status: domain.Status(in.String("status")),
				Confidence: confidence, Note: in.String("note"), Values: values,
			}, g, s.Text)
			if err != nil {
				return nil, err
			}
			now := s.Clock.Now()
			c.At = now
			expected := g.Version
			g.Apply(c)
			if err := s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := s.Store.AddCheckIn(ctx, c); err != nil {
					return err
				}
				if err := s.Store.UpdateGoal(ctx, g, expected, actor.AccountID, now); err != nil {
					return err
				}
				return s.record(ctx, actor, GoalCheckedInAction, goalTarget, id, now,
					audit.Change{Field: "status", To: string(c.Status)},
					audit.Change{Field: "confidence", To: itoa(c.Confidence)})
			}); err != nil {
				return nil, err
			}
			return s.answer(ctx, actor, id)
		}),
	}, goalTarget)
}

// ListGoalCheckIns answers a goal's check-ins, the latest first.
type ListGoalCheckIns struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h ListGoalCheckIns) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListGoalCheckInsName, Permission: service.PermissionRead, ReadOnly: true,
		Summary:     "A goal's check-ins, the latest first, at most 200: status, confidence, note and the values reported.",
		SideEffects: "None. Reads only.", TokenScope: itemsRead,
		Input: []usecase.Field{{Name: "goal_id", Kind: usecase.KindID, Required: true, Description: "The goal."}},
		Audit: usecase.AuditDeclaration{Action: GoalReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			id, err := in.ID("goal_id")
			if err != nil {
				return nil, err
			}
			if _, err := s.readable(ctx, actor, id); err != nil {
				return nil, err
			}
			var checkIns []domain.CheckIn
			if err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				checkIns, err = s.Store.CheckIns(ctx, id, maxCheckIns)
				return err
			}); err != nil {
				return nil, err
			}
			return usecase.Output{"data": checkInsOutput(checkIns)}, nil
		}),
	}, goalTarget)
}
