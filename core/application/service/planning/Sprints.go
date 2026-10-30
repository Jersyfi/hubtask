// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"context"
	"strconv"
	"time"

	planrepo "github.com/Jersyfi/hubtask/core/application/repository/planning"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	CreateSprintName        = "CreateSprint"
	UpdateSprintName        = "UpdateSprint"
	ListSprintsName         = "ListSprints"
	ReadSprintName          = "ReadSprint"
	DeleteSprintName        = "DeleteSprint"
	AddSprintEntriesName    = "AddSprintEntries"
	RemoveSprintEntriesName = "RemoveSprintEntries"
	StartSprintName         = "StartSprint"
	CloseSprintName         = "CloseSprint"
	ReadSprintBacklogName   = "ReadSprintBacklog"
	ReadVelocityName        = "ReadVelocity"
)

const sprintTarget = "sprint"

// The stable audit codes of sprints (audit.md §2). A sprint's name and goal are a person's words
// and go as fingerprints (rule 10).
const (
	SprintCreatedAction        audit.Action = "sprint.created"
	SprintUpdatedAction        audit.Action = "sprint.updated"
	SprintDeletedAction        audit.Action = "sprint.deleted"
	SprintStartedAction        audit.Action = "sprint.started"
	SprintClosedAction         audit.Action = "sprint.closed"
	SprintEntriesAddedAction   audit.Action = "sprint.entries_added"
	SprintEntriesRemovedAction audit.Action = "sprint.entries_removed"
	SprintReadAction           audit.Action = "sprint.read"
	VelocityReadAction         audit.Action = "sprint.velocity_read"
	maxSprintsListed                        = 200
	maxBacklog                              = 500
)

// SprintOutput is a sprint as every door returns it (schema Sprint).
func SprintOutput(s domain.Sprint) usecase.Output {
	return usecase.Output{
		"id": s.ID.String(), "collection_id": s.CollectionID.String(), "name": s.Name, "goal": s.Goal,
		"start_on": s.StartOn.String(), "end_on": s.EndOn.String(), "state": string(s.State), "unit": string(s.Unit),
		"committed_value": s.CommittedValue, "committed_count": s.CommittedCount,
		"completed_value": s.CompletedValue, "completed_count": s.CompletedCount,
		"started_at": timeOrNil(s.StartedAt), "closed_at": timeOrNil(s.ClosedAt), "version": s.Version,
	}
}

// EstimateOutput is an estimate, or null for an entry without one.
func EstimateOutput(e domain.Estimate, ok bool) any {
	if !ok {
		return nil
	}
	var size any
	if e.Size != "" {
		size = e.Size
	}
	return usecase.Output{"unit": string(e.Unit), "value": e.Value, "size": size}
}

func parseDay(in usecase.Input, field string) (domain.Day, error) {
	raw := in.String(field)
	at, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return 0, invalid("/"+field, "sprints.date_invalid")
	}
	return domain.DayFrom(at.Year(), at.Month(), at.Day()), nil
}

// collection reads the collection a sprint belongs to and asks the permission at it; a hub has no
// sprints.
func (p Planner) collection(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, permission service.Permission,
	action audit.Action, scope string,
) (work.Container, error) {
	if id.IsZero() {
		return work.Container{}, invalid("/collection_id", "sprints.collection_required")
	}
	container, err := p.plan(ctx, actor, id, permission, action, scope)
	if err != nil {
		return work.Container{}, err
	}
	if container.Type != work.ContainerCollection {
		return work.Container{}, invalid("/collection_id", "sprints.collection_required")
	}
	return container, nil
}

// sprint reads a sprint and asks the permission at its collection.
func (p Planner) sprint(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, permission service.Permission,
	action audit.Action, scope string,
) (domain.Sprint, error) {
	if id.IsZero() {
		return domain.Sprint{}, invalid("/sprint_id", "sprints.id_required")
	}
	var sprint domain.Sprint
	if err := p.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := p.Sprints.Find(ctx, id)
		sprint = found
		return err
	}); err != nil {
		return domain.Sprint{}, err
	}
	if _, err := p.plan(ctx, actor, sprint.CollectionID, permission, action, scope); err != nil {
		return domain.Sprint{}, err
	}
	return sprint, nil
}

// calendar answers the working days and the zone a sprint's days are counted in.
func (p Planner) calendar(ctx context.Context, actor appshared.ActorContext, key, zoneName string) (domain.Workdays, *time.Location, error) {
	zone := time.UTC
	if zoneName != "" {
		loaded, err := time.LoadLocation(zoneName)
		if err != nil {
			return nil, nil, invalid("/zone", "planning.zone_unknown")
		}
		zone = loaded
	}
	if key == "" {
		return weekdays{}, zone, nil
	}
	if p.Calendars == nil {
		return nil, nil, invalid("/calendar", "calendars.not_found")
	}
	var days domain.Workdays
	err := p.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := p.Calendars.Find(ctx, key)
		if err != nil {
			return err
		}
		days, zone = found.Calendar, found.Calendar.Zone()
		return nil
	})
	return days, zone, err
}

func sprintFields(s domain.Sprint) domain.SprintFields {
	return domain.SprintFields{Name: s.Name, Goal: s.Goal, StartOn: s.StartOn, EndOn: s.EndOn, Unit: s.Unit}
}

// CreateSprint plans a sprint of a collection: its name, goal, days and the unit it counts in
// (PLN-06).
type CreateSprint struct{ Planner Planner }

// Execute plans the sprint.
func (h CreateSprint) Execute(ctx context.Context, actor appshared.ActorContext, collectionID shared.ID, fields domain.SprintFields) (domain.Sprint, error) {
	collection, err := h.Planner.collection(ctx, actor, collectionID, service.PermissionWriteItems, SprintCreatedAction, itemsWrite)
	if err != nil {
		return domain.Sprint{}, err
	}
	now := h.Planner.Clock.Now()
	sprint, err := domain.NewSprint(domain.NewSprintInput{
		ID: h.Planner.IDs.NewID(), TenantID: actor.TenantID, CollectionID: collection.ID, CreatedBy: actor.AccountID,
		Fields: fields, Now: now, Text: h.Planner.Text,
	})
	if err != nil {
		return domain.Sprint{}, err
	}
	err = h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Planner.Sprints.Insert(ctx, sprint); err != nil {
			return err
		}
		return h.Planner.record(ctx, actor, SprintCreatedAction, sprintTarget, sprint.ID, now,
			audit.Change{Field: "collection_id", Classification: audit.Open, To: collection.ID.String()},
			audit.Change{Field: "name", Classification: audit.Sensitive, To: sprint.Name},
			audit.Change{Field: "goal", Classification: audit.Sensitive, To: sprint.Goal},
			audit.Change{Field: "start_on", Classification: audit.Open, To: sprint.StartOn.String()},
			audit.Change{Field: "end_on", Classification: audit.Open, To: sprint.EndOn.String()})
	})
	if err != nil {
		return domain.Sprint{}, err
	}
	return sprint, nil
}

func sprintFieldInputs(required bool) []usecase.Field {
	return []usecase.Field{
		{Name: "name", Kind: usecase.KindString, Required: required, Description: "Up to 120 characters, on one line."},
		{Name: "goal", Kind: usecase.KindString, Description: "What the sprint is for, up to 500 characters."},
		{Name: "start_on", Kind: usecase.KindString, Required: required, Description: "The first day, YYYY-MM-DD."},
		{Name: "end_on", Kind: usecase.KindString, Required: required, Description: "The last day, YYYY-MM-DD; at most 92 days after the first."},
		{Name: "unit", Kind: usecase.KindString, Enum: []string{"POINTS", "HOURS"}, Description: "What the commitment and velocity count; POINTS without, T-shirt sizes among them."},
	}
}

// Descriptor is the catalogue entry.
func (h CreateSprint) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: CreateSprintName, Permission: service.PermissionWriteItems,
		Summary: "Plans a sprint of a collection: a name, a goal, its first and last day and whether it counts " +
			"points or hours. It starts PLANNED; entries join it before or while it runs.",
		SideEffects: "Writes the sprint and an audit entry.",
		TokenScope:  itemsWrite,
		Input: append([]usecase.Field{
			{Name: "collection_id", Kind: usecase.KindID, Required: true, Description: "The collection - the team's board - the sprint belongs to."},
		}, sprintFieldInputs(true)...),
		Audit:    usecase.AuditDeclaration{Action: SprintCreatedAction},
		Activity: usecase.ActivityDeclaration{Exempt: "a sprint is planned beside the entries and changes none."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			collection, err := in.ID("collection_id")
			if err != nil {
				return nil, err
			}
			start, err := parseDay(in, "start_on")
			if err != nil {
				return nil, err
			}
			end, err := parseDay(in, "end_on")
			if err != nil {
				return nil, err
			}
			sprint, err := h.Execute(ctx, actor, collection, domain.SprintFields{
				Name: in.String("name"), Goal: in.String("goal"), StartOn: start, EndOn: end,
				Unit: domain.EstimateUnit(in.String("unit")),
			})
			if err != nil {
				return nil, err
			}
			return SprintOutput(sprint), nil
		}),
	}, sprintTarget)
}

// UpdateSprint changes a sprint's name, goal, days or unit (PLN-06). A running sprint keeps its
// first day and unit; a closed one does not change.
type UpdateSprint struct{ Planner Planner }

// UpdateSprintCommand is the input, typed; a nil field keeps what stands.
type UpdateSprintCommand struct {
	SprintID       shared.ID
	Name, Goal     *string
	StartOn, EndOn *domain.Day
	Unit           *domain.EstimateUnit
	Version        int
}

// Execute changes the sprint.
func (h UpdateSprint) Execute(ctx context.Context, actor appshared.ActorContext, cmd UpdateSprintCommand) (domain.Sprint, error) {
	sprint, err := h.Planner.sprint(ctx, actor, cmd.SprintID, service.PermissionWriteItems, SprintUpdatedAction, itemsWrite)
	if err != nil {
		return domain.Sprint{}, err
	}
	if cmd.Version != 0 && cmd.Version != sprint.Version {
		return domain.Sprint{}, shared.ErrVersionConflict.WithDetail("sprints.version_conflict")
	}
	before := sprint
	fields := sprintFields(sprint)
	if cmd.Name != nil {
		fields.Name = *cmd.Name
	}
	if cmd.Goal != nil {
		fields.Goal = *cmd.Goal
	}
	if cmd.StartOn != nil {
		fields.StartOn = *cmd.StartOn
	}
	if cmd.EndOn != nil {
		fields.EndOn = *cmd.EndOn
	}
	if cmd.Unit != nil {
		fields.Unit = *cmd.Unit
	}
	now := h.Planner.Clock.Now()
	if err := sprint.Change(fields, now, h.Planner.Text); err != nil {
		return domain.Sprint{}, err
	}
	err = h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Planner.Sprints.Update(ctx, sprint, before.Version); err != nil {
			return err
		}
		return h.Planner.record(ctx, actor, SprintUpdatedAction, sprintTarget, sprint.ID, now,
			audit.Change{Field: "name", Classification: audit.Sensitive, From: before.Name, To: sprint.Name},
			audit.Change{Field: "goal", Classification: audit.Sensitive, From: before.Goal, To: sprint.Goal},
			audit.Change{Field: "start_on", Classification: audit.Open, From: before.StartOn.String(), To: sprint.StartOn.String()},
			audit.Change{Field: "end_on", Classification: audit.Open, From: before.EndOn.String(), To: sprint.EndOn.String()},
			audit.Change{Field: "unit", Classification: audit.Open, From: string(before.Unit), To: string(sprint.Unit)})
	})
	if err != nil {
		return domain.Sprint{}, err
	}
	sprint.Version = before.Version + 1
	return sprint, nil
}

// Descriptor is the catalogue entry.
func (h UpdateSprint) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: UpdateSprintName, Permission: service.PermissionWriteItems,
		Summary: "Changes a sprint's name, goal, days or unit; a field left out keeps what stands. A running " +
			"sprint keeps its first day and unit (sprints.running_start_fixed); a closed one is history (sprints.closed).",
		SideEffects: "Writes the sprint and an audit entry.",
		TokenScope:  itemsWrite,
		Input: append(append([]usecase.Field{
			{Name: "sprint_id", Kind: usecase.KindID, Required: true, Description: "The sprint."},
		}, sprintFieldInputs(false)...),
			usecase.Field{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read; a stale one is a conflict."}),
		Audit:    usecase.AuditDeclaration{Action: SprintUpdatedAction},
		Activity: usecase.ActivityDeclaration{Exempt: "a sprint's own fields change no entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("sprint_id")
			if err != nil {
				return nil, err
			}
			cmd := UpdateSprintCommand{SprintID: id, Name: in.OptionalString("name"), Goal: in.OptionalString("goal"), Version: in.Int("expected_version")}
			for field, target := range map[string]**domain.Day{"start_on": &cmd.StartOn, "end_on": &cmd.EndOn} {
				if in.Present(field) {
					day, err := parseDay(in, field)
					if err != nil {
						return nil, err
					}
					*target = &day
				}
			}
			if in.Present("unit") {
				unit := domain.EstimateUnit(in.String("unit"))
				cmd.Unit = &unit
			}
			sprint, err := h.Execute(ctx, actor, cmd)
			if err != nil {
				return nil, err
			}
			return SprintOutput(sprint), nil
		}),
	}, sprintTarget)
}

// ListSprints reads a collection's sprints, the latest first (PLN-06).
type ListSprints struct{ Planner Planner }

// Execute lists them.
func (h ListSprints) Execute(ctx context.Context, actor appshared.ActorContext, collectionID shared.ID, state domain.SprintState) ([]domain.Sprint, error) {
	if state != "" && state != domain.SprintPlanned && state != domain.SprintActive && state != domain.SprintClosed {
		return nil, invalid("/state", "sprints.state_unknown")
	}
	if _, err := h.Planner.collection(ctx, actor, collectionID, service.PermissionRead, SprintReadAction, itemsRead); err != nil {
		return nil, err
	}
	var out []domain.Sprint
	err := h.Planner.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		out, err = h.Planner.Sprints.List(ctx, collectionID, state, maxSprintsListed)
		return err
	})
	return out, err
}

// Descriptor is the catalogue entry.
func (h ListSprints) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListSprintsName, Permission: service.PermissionRead,
		Summary:     "Lists a collection's sprints, the latest first, at most 200: their days, state, goal, commitment and result.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "collection_id", Kind: usecase.KindID, Required: true, Description: "The collection."},
			{Name: "state", Kind: usecase.KindString, Enum: []string{"PLANNED", "ACTIVE", "CLOSED"}, Description: "Only sprints in this state; every state without."},
		},
		Audit: usecase.AuditDeclaration{Action: SprintReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			collection, err := in.ID("collection_id")
			if err != nil {
				return nil, err
			}
			sprints, err := h.Execute(ctx, actor, collection, domain.SprintState(in.String("state")))
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(sprints))
			for _, s := range sprints {
				data = append(data, SprintOutput(s))
			}
			return usecase.Output{"data": data}, nil
		}),
	}, sprintTarget)
}

// SprintView is a sprint with the entries its reader may see and its burndown.
type SprintView struct {
	Sprint    domain.Sprint
	Entries   []SprintViewEntry
	Burndown  []domain.BurndownPoint
	Estimates map[shared.ID]domain.Estimate
}

// SprintViewEntry is one visible membership with its entry's title.
type SprintViewEntry struct {
	domain.SprintEntry
	Title string
}

// visibleMembers answers the members a reader may see, with their titles.
func (p Planner) visibleMembers(ctx context.Context, actor appshared.ActorContext, members []planrepo.SprintMember) ([]SprintViewEntry, error) {
	ids := make([]shared.ID, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.EntryID)
	}
	titles := map[shared.ID]string{}
	if len(ids) > 0 {
		if err := p.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			summaries, err := p.References.Entries(ctx, ids)
			for _, s := range summaries {
				titles[s.ID] = s.Title
			}
			return err
		}); err != nil {
			return nil, err
		}
	}
	paths := make([][]identity.Scope, 0, len(members))
	for _, m := range members {
		paths = append(paths, entryPath(m.HubID, m.CollectionID, m.EntryID))
	}
	visible, err := p.permitted(ctx, actor, paths)
	if err != nil {
		return nil, err
	}
	out := make([]SprintViewEntry, 0, len(members))
	for i, m := range members {
		if visible[i] {
			out = append(out, SprintViewEntry{SprintEntry: m.SprintEntry, Title: titles[m.EntryID]})
		}
	}
	return out, nil
}

// ReadSprint reads a sprint with the entries its reader may see, their estimates and outcomes,
// and its burndown day by day (PLN-06, PLN-07). An entry the reader may not see is left out of
// the list and the burndown; the commitment and result are the sprint's own record.
type ReadSprint struct{ Planner Planner }

// Execute reads the sprint.
func (h ReadSprint) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID, calendarKey, zoneName string) (SprintView, error) {
	sprint, err := h.Planner.sprint(ctx, actor, id, service.PermissionRead, SprintReadAction, itemsRead)
	if err != nil {
		return SprintView{}, err
	}
	days, zone, err := h.Planner.calendar(ctx, actor, calendarKey, zoneName)
	if err != nil {
		return SprintView{}, err
	}
	var members []planrepo.SprintMember
	if err := h.Planner.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		members, err = h.Planner.Sprints.Entries(ctx, sprint.ID, domain.MaxSprintEntries)
		return err
	}); err != nil {
		return SprintView{}, err
	}
	entries, err := h.Planner.visibleMembers(ctx, actor, members)
	if err != nil {
		return SprintView{}, err
	}
	ids := make([]shared.ID, 0, len(entries))
	plain := make([]domain.SprintEntry, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.EntryID)
		plain = append(plain, e.SprintEntry)
	}
	var estimates map[shared.ID]domain.Estimate
	if err := h.Planner.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		estimates, err = h.Planner.Sprints.Estimates(ctx, ids)
		return err
	}); err != nil {
		return SprintView{}, err
	}
	today := domain.DayOf(h.Planner.Clock.Now(), zone)
	return SprintView{
		Sprint: sprint, Entries: entries, Estimates: estimates,
		Burndown: domain.Burndown(sprint, plain, estimates, days, zone, today),
	}, nil
}

// Output is the shape every door returns (schema SprintDetail).
func (v SprintView) Output() usecase.Output {
	entries := make([]usecase.Output, 0, len(v.Entries))
	for _, e := range v.Entries {
		var outcome any
		if e.Outcome != "" {
			outcome = string(e.Outcome)
		}
		estimate, ok := v.Estimates[e.EntryID]
		entries = append(entries, usecase.Output{
			"item_id": e.EntryID.String(), "title": e.Title, "added_at": e.AddedAt.UTC(), "removed_at": timeOrNil(e.RemovedAt),
			"committed": e.Committed, "outcome": outcome, "done": e.Done, "completed_at": timeOrNil(e.CompletedAt),
			"estimate": EstimateOutput(estimate, ok),
		})
	}
	burndown := make([]usecase.Output, 0, len(v.Burndown))
	for _, p := range v.Burndown {
		burndown = append(burndown, usecase.Output{"day": p.Day.String(), "remaining": p.Remaining, "scope": p.Scope, "ideal": p.Ideal})
	}
	return usecase.Output{"sprint": SprintOutput(v.Sprint), "entries": entries, "burndown": burndown}
}

// Descriptor is the catalogue entry.
func (h ReadSprint) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadSprintName, Permission: service.PermissionRead,
		Summary: "Reads a sprint with every entry in it or once in it the caller may see - when it joined and " +
			"left, whether it was committed, its estimate, whether it is done and what became of it at the " +
			"close - and the burndown: each day's work left, the scope and the ideal line over working days.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "sprint_id", Kind: usecase.KindID, Required: true, Description: "The sprint."},
			{Name: "calendar", Kind: usecase.KindString, Description: "A business calendar's key whose working days the ideal line counts; Monday to Friday without."},
			{Name: "zone", Kind: usecase.KindString, Description: "The IANA time zone the days are local to when no calendar names one; UTC without."},
		},
		Audit: usecase.AuditDeclaration{Action: SprintReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("sprint_id")
			if err != nil {
				return nil, err
			}
			view, err := h.Execute(ctx, actor, id, in.String("calendar"), in.String("zone"))
			if err != nil {
				return nil, err
			}
			return view.Output(), nil
		}),
	}, sprintTarget)
}

// DeleteSprint removes a planned sprint; its entries return to the backlog untouched (PLN-06). A
// running or closed sprint is the team's record and stays.
type DeleteSprint struct{ Planner Planner }

// Execute deletes it.
func (h DeleteSprint) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) error {
	sprint, err := h.Planner.sprint(ctx, actor, id, service.PermissionWriteItems, SprintDeletedAction, itemsWrite)
	if err != nil {
		return err
	}
	if sprint.State != domain.SprintPlanned {
		return shared.ErrConflict.WithDetail("sprints.not_planned")
	}
	now := h.Planner.Clock.Now()
	return h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Planner.Sprints.Delete(ctx, sprint.ID); err != nil {
			return err
		}
		return h.Planner.record(ctx, actor, SprintDeletedAction, sprintTarget, sprint.ID, now,
			audit.Change{Field: "collection_id", Classification: audit.Open, From: sprint.CollectionID.String()})
	})
}

// Descriptor is the catalogue entry.
func (h DeleteSprint) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DeleteSprintName, Permission: service.PermissionWriteItems,
		Summary: "Deletes a planned sprint; its entries go back to the backlog unchanged. A running or closed " +
			"sprint is the team's record and is not deleted (sprints.not_planned).",
		SideEffects: "Deletes the sprint with its memberships and writes an audit entry.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "sprint_id", Kind: usecase.KindID, Required: true, Description: "The sprint."},
		},
		Audit:    usecase.AuditDeclaration{Action: SprintDeletedAction},
		Activity: usecase.ActivityDeclaration{Exempt: "deleting a planned sprint changes no entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("sprint_id")
			if err != nil {
				return nil, err
			}
			return usecase.Output{}, h.Execute(ctx, actor, id)
		}),
	}, sprintTarget)
}

// AddSprintEntries puts entries of the sprint's collection into it (PLN-06). An entry is in one
// open sprint at a time; one joining a running sprint is not part of its commitment.
type AddSprintEntries struct{ Planner Planner }

// Execute adds them, all or none.
func (h AddSprintEntries) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID, items []shared.ID) (domain.Sprint, error) {
	sprint, err := h.Planner.sprint(ctx, actor, id, service.PermissionWriteItems, SprintEntriesAddedAction, itemsWrite)
	if err != nil {
		return domain.Sprint{}, err
	}
	if !sprint.Open() {
		return domain.Sprint{}, shared.ErrConflict.WithDetail("sprints.closed")
	}
	if err := checkBatch(items); err != nil {
		return domain.Sprint{}, err
	}
	if err := h.Planner.entriesOf(ctx, actor, sprint.CollectionID, items); err != nil {
		return domain.Sprint{}, err
	}
	now := h.Planner.Clock.Now()
	err = h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		open, err := h.Planner.Sprints.OpenMemberships(ctx, items)
		if err != nil {
			return err
		}
		already := map[shared.ID]bool{}
		for _, m := range open {
			if m.SprintID != sprint.ID {
				return shared.ErrConflict.WithDetail("sprints.entry_in_another_sprint").
					WithParams(map[string]string{"item_id": m.EntryID.String()})
			}
			already[m.EntryID] = true
		}
		added := 0
		for _, item := range items {
			if already[item] {
				continue
			}
			if err := h.Planner.Sprints.Add(ctx, sprint.ID, item, now, false); err != nil {
				return err
			}
			added++
		}
		return h.Planner.record(ctx, actor, SprintEntriesAddedAction, sprintTarget, sprint.ID, now,
			audit.Change{Field: "entries", Classification: audit.Open, To: strconv.Itoa(added)})
	})
	return sprint, err
}

// checkBatch bounds the entries one call names.
func checkBatch(items []shared.ID) error {
	if len(items) == 0 {
		return invalid("/item_ids", "sprints.entries_required")
	}
	if len(items) > work.MaxBulkOperations {
		return shared.ErrValidation.WithDetail("sprints.too_many_entries").
			WithParams(map[string]string{"maximum": strconv.Itoa(work.MaxBulkOperations)})
	}
	return nil
}

// entriesOf checks that every entry named lives in the collection and that the actor may change
// it; one out of reach answers as not found (T-04).
func (p Planner) entriesOf(ctx context.Context, actor appshared.ActorContext, collectionID shared.ID, items []shared.ID) error {
	var summaries []workrepo.EntrySummary
	if err := p.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		summaries, err = p.References.Entries(ctx, items)
		return err
	}); err != nil {
		return err
	}
	found := make(map[shared.ID]workrepo.EntrySummary, len(summaries))
	for _, s := range summaries {
		found[s.ID] = s
	}
	paths := make([][]identity.Scope, 0, len(items))
	for _, item := range items {
		s := found[item]
		paths = append(paths, entryPath(s.HubID, s.CollectionID, item))
	}
	visible, err := p.permitted(ctx, actor, paths)
	if err != nil {
		return err
	}
	for i, item := range items {
		s, ok := found[item]
		if !ok || !visible[i] {
			return appshared.ItemNotFound(item)
		}
		if s.CollectionID != collectionID {
			return invalid("/item_ids", "sprints.entry_elsewhere")
		}
	}
	return nil
}

// Descriptor is the catalogue entry.
func (h AddSprintEntries) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: AddSprintEntriesName, Permission: service.PermissionWriteItems,
		Summary: "Puts entries of the sprint's collection into a planned or running sprint, all or none, at most " +
			"500. An entry is in one open sprint at a time (sprints.entry_in_another_sprint); one joining a " +
			"running sprint widens its scope, not its commitment.",
		SideEffects: "Writes the memberships and an audit entry.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "sprint_id", Kind: usecase.KindID, Required: true, Description: "The sprint."},
			{Name: "item_ids", Kind: usecase.KindIDList, Required: true, Description: "The entries, of the sprint's collection."},
		},
		Audit:    usecase.AuditDeclaration{Action: SprintEntriesAddedAction},
		Activity: usecase.ActivityDeclaration{Exempt: "a membership is the sprint's; the entry itself does not change."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("sprint_id")
			if err != nil {
				return nil, err
			}
			items, err := in.IDList("item_ids")
			if err != nil {
				return nil, err
			}
			sprint, err := h.Execute(ctx, actor, id, items)
			if err != nil {
				return nil, err
			}
			return SprintOutput(sprint), nil
		}),
	}, sprintTarget)
}

// RemoveSprintEntries takes entries out of a planned or running sprint, back to the backlog
// (PLN-06). The sprint remembers when they left; its burndown counts them until then.
type RemoveSprintEntries struct{ Planner Planner }

// Execute removes them.
func (h RemoveSprintEntries) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID, items []shared.ID) (domain.Sprint, error) {
	sprint, err := h.Planner.sprint(ctx, actor, id, service.PermissionWriteItems, SprintEntriesRemovedAction, itemsWrite)
	if err != nil {
		return domain.Sprint{}, err
	}
	if !sprint.Open() {
		return domain.Sprint{}, shared.ErrConflict.WithDetail("sprints.closed")
	}
	if err := checkBatch(items); err != nil {
		return domain.Sprint{}, err
	}
	now := h.Planner.Clock.Now()
	err = h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		removed := 0
		for _, item := range items {
			ok, err := h.Planner.Sprints.Remove(ctx, sprint.ID, item, now)
			if err != nil {
				return err
			}
			if ok {
				removed++
			}
		}
		return h.Planner.record(ctx, actor, SprintEntriesRemovedAction, sprintTarget, sprint.ID, now,
			audit.Change{Field: "entries", Classification: audit.Open, To: strconv.Itoa(removed)})
	})
	return sprint, err
}

// Descriptor is the catalogue entry.
func (h RemoveSprintEntries) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: RemoveSprintEntriesName, Permission: service.PermissionWriteItems,
		Summary: "Takes entries out of a planned or running sprint and back to the backlog, at most 500; an entry " +
			"not in it is passed over. The sprint keeps when each left.",
		SideEffects: "Writes the memberships and an audit entry.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "sprint_id", Kind: usecase.KindID, Required: true, Description: "The sprint."},
			{Name: "item_ids", Kind: usecase.KindIDList, Required: true, Description: "The entries."},
		},
		Audit:    usecase.AuditDeclaration{Action: SprintEntriesRemovedAction},
		Activity: usecase.ActivityDeclaration{Exempt: "a membership is the sprint's; the entry itself does not change."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("sprint_id")
			if err != nil {
				return nil, err
			}
			items, err := in.IDList("item_ids")
			if err != nil {
				return nil, err
			}
			sprint, err := h.Execute(ctx, actor, id, items)
			if err != nil {
				return nil, err
			}
			return SprintOutput(sprint), nil
		}),
	}, sprintTarget)
}

// openMembers reads a sprint's open memberships with their estimates.
func (p Planner) openMembers(ctx context.Context, sprintID shared.ID) ([]domain.SprintEntry, map[shared.ID]domain.Estimate, error) {
	members, err := p.Sprints.Entries(ctx, sprintID, domain.MaxSprintEntries)
	if err != nil {
		return nil, nil, err
	}
	open := make([]domain.SprintEntry, 0, len(members))
	ids := make([]shared.ID, 0, len(members))
	for _, m := range members {
		if m.RemovedAt == nil && m.Outcome == "" {
			open = append(open, m.SprintEntry)
			ids = append(ids, m.EntryID)
		}
	}
	estimates, err := p.Sprints.Estimates(ctx, ids)
	return open, estimates, err
}

// StartSprint runs a planned sprint and records the commitment: its entries and their estimate
// sum now (PLN-06). One sprint runs per collection at a time.
type StartSprint struct{ Planner Planner }

// Execute starts it.
func (h StartSprint) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) (domain.Sprint, error) {
	sprint, err := h.Planner.sprint(ctx, actor, id, service.PermissionWriteItems, SprintStartedAction, itemsWrite)
	if err != nil {
		return domain.Sprint{}, err
	}
	now := h.Planner.Clock.Now()
	err = h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		entries, estimates, err := h.Planner.openMembers(ctx, sprint.ID)
		if err != nil {
			return err
		}
		version := sprint.Version
		if err := sprint.Start(entries, estimates, now); err != nil {
			return err
		}
		if err := h.Planner.Sprints.Update(ctx, sprint, version); err != nil {
			return err
		}
		if err := h.Planner.Sprints.Commit(ctx, sprint.ID); err != nil {
			return err
		}
		sprint.Version = version + 1
		return h.Planner.record(ctx, actor, SprintStartedAction, sprintTarget, sprint.ID, now,
			audit.Change{Field: "committed_count", Classification: audit.Open, To: strconv.Itoa(sprint.CommittedCount)},
			audit.Change{Field: "committed_value", Classification: audit.Open, To: strconv.FormatFloat(sprint.CommittedValue, 'f', -1, 64)})
	})
	if err != nil {
		return domain.Sprint{}, err
	}
	return sprint, nil
}

// Descriptor is the catalogue entry.
func (h StartSprint) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: StartSprintName, Permission: service.PermissionWriteItems,
		Summary: "Starts a planned sprint and records the commitment - how many entries it holds and their " +
			"estimate sum in its unit. One sprint runs per collection at a time (sprints.another_running).",
		SideEffects: "Writes the sprint, marks its entries committed and writes an audit entry.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "sprint_id", Kind: usecase.KindID, Required: true, Description: "The sprint."},
		},
		Audit:    usecase.AuditDeclaration{Action: SprintStartedAction},
		Activity: usecase.ActivityDeclaration{Exempt: "starting a sprint changes no entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("sprint_id")
			if err != nil {
				return nil, err
			}
			sprint, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			return SprintOutput(sprint), nil
		}),
	}, sprintTarget)
}

// CloseSprint ends a running sprint: what is done is its result; every unfinished entry moves into
// the planned sprint named, or back to the backlog without one (PLN-06).
type CloseSprint struct{ Planner Planner }

// CloseSprintResult is the closed sprint and where its unfinished entries went.
type CloseSprintResult struct {
	Sprint   domain.Sprint
	Carried  []shared.ID
	Returned []shared.ID
	CarryTo  shared.ID
}

// Execute closes it.
func (h CloseSprint) Execute(ctx context.Context, actor appshared.ActorContext, id, carryTo shared.ID) (CloseSprintResult, error) {
	sprint, err := h.Planner.sprint(ctx, actor, id, service.PermissionWriteItems, SprintClosedAction, itemsWrite)
	if err != nil {
		return CloseSprintResult{}, err
	}
	if !carryTo.IsZero() {
		if carryTo == sprint.ID {
			return CloseSprintResult{}, invalid("/carry_to_sprint_id", "sprints.carry_to_itself")
		}
		var next domain.Sprint
		if err := h.Planner.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			found, err := h.Planner.Sprints.Find(ctx, carryTo)
			next = found
			return err
		}); err != nil {
			return CloseSprintResult{}, err
		}
		if next.CollectionID != sprint.CollectionID || next.State != domain.SprintPlanned {
			return CloseSprintResult{}, invalid("/carry_to_sprint_id", "sprints.carry_to_not_planned")
		}
	}
	now := h.Planner.Clock.Now()
	result := CloseSprintResult{CarryTo: carryTo}
	err = h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		entries, estimates, err := h.Planner.openMembers(ctx, sprint.ID)
		if err != nil {
			return err
		}
		version := sprint.Version
		outcomes, err := sprint.Close(entries, estimates, !carryTo.IsZero(), now)
		if err != nil {
			return err
		}
		if err := h.Planner.Sprints.Update(ctx, sprint, version); err != nil {
			return err
		}
		sprint.Version = version + 1
		var done, carried, returned []shared.ID
		for _, e := range entries {
			switch outcomes[e.EntryID] {
			case domain.OutcomeDone:
				done = append(done, e.EntryID)
			case domain.OutcomeCarried:
				carried = append(carried, e.EntryID)
			default:
				returned = append(returned, e.EntryID)
			}
		}
		for outcome, ids := range map[domain.SprintOutcome][]shared.ID{
			domain.OutcomeDone: done, domain.OutcomeCarried: carried, domain.OutcomeReturned: returned,
		} {
			if err := h.Planner.Sprints.SetOutcomes(ctx, sprint.ID, outcome, ids); err != nil {
				return err
			}
		}
		for _, item := range carried {
			if err := h.Planner.Sprints.Add(ctx, carryTo, item, now, false); err != nil {
				return err
			}
		}
		result.Carried, result.Returned = carried, returned
		return h.Planner.record(ctx, actor, SprintClosedAction, sprintTarget, sprint.ID, now,
			audit.Change{Field: "completed_count", Classification: audit.Open, To: strconv.Itoa(sprint.CompletedCount)},
			audit.Change{Field: "completed_value", Classification: audit.Open, To: strconv.FormatFloat(sprint.CompletedValue, 'f', -1, 64)},
			audit.Change{Field: "carried", Classification: audit.Open, To: strconv.Itoa(len(carried))},
			audit.Change{Field: "returned", Classification: audit.Open, To: strconv.Itoa(len(returned))})
	})
	if err != nil {
		return CloseSprintResult{}, err
	}
	result.Sprint = sprint
	return result, nil
}

func idStrings(ids []shared.ID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}

// Descriptor is the catalogue entry.
func (h CloseSprint) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: CloseSprintName, Permission: service.PermissionWriteItems,
		Summary: "Closes a running sprint and records its result - the entries done and their estimate sum. " +
			"Every unfinished entry moves into the planned sprint of the same collection named, or back to " +
			"the backlog without one. A closed sprint is history.",
		SideEffects: "Writes the sprint, each entry's outcome, the carried entries' new memberships and an audit entry.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "sprint_id", Kind: usecase.KindID, Required: true, Description: "The sprint."},
			{Name: "carry_to_sprint_id", Kind: usecase.KindID, Description: "A planned sprint of the same collection the unfinished entries move into; the backlog without."},
		},
		Audit:    usecase.AuditDeclaration{Action: SprintClosedAction},
		Activity: usecase.ActivityDeclaration{Exempt: "closing a sprint changes no entry; the memberships are the sprints'."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("sprint_id")
			if err != nil {
				return nil, err
			}
			var carryTo shared.ID
			if in.Present("carry_to_sprint_id") {
				if carryTo, err = in.ID("carry_to_sprint_id"); err != nil {
					return nil, err
				}
			}
			result, err := h.Execute(ctx, actor, id, carryTo)
			if err != nil {
				return nil, err
			}
			var to any
			if !result.CarryTo.IsZero() {
				to = result.CarryTo.String()
			}
			return usecase.Output{
				"sprint": SprintOutput(result.Sprint), "carried": idStrings(result.Carried),
				"returned": idStrings(result.Returned), "carried_to": to,
			}, nil
		}),
	}, sprintTarget)
}

// BacklogEntry is an open entry of a collection that no open sprint holds, with its estimate.
type BacklogEntry struct {
	ID       shared.ID
	Title    string
	Estimate domain.Estimate
	Has      bool
}

// ReadSprintBacklog reads a collection's backlog for planning a sprint: its open entries the
// reader may see that no open sprint holds, in the collection's order, with their estimates
// (PLN-06, PLN-07).
type ReadSprintBacklog struct{ Planner Planner }

// Execute reads the backlog.
func (h ReadSprintBacklog) Execute(ctx context.Context, actor appshared.ActorContext, collectionID shared.ID) ([]BacklogEntry, error) {
	if _, err := h.Planner.collection(ctx, actor, collectionID, service.PermissionRead, SprintReadAction, itemsRead); err != nil {
		return nil, err
	}
	var entries []planrepo.PlanEntry
	if err := h.Planner.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		entries, err = h.Planner.Plans.Entries(ctx, collectionID, domain.MaxEntries)
		return err
	}); err != nil {
		return nil, err
	}
	paths := make([][]identity.Scope, 0, len(entries))
	for _, e := range entries {
		paths = append(paths, entryPath(e.HubID, e.CollectionID, e.ID))
	}
	visible, err := h.Planner.permitted(ctx, actor, paths)
	if err != nil {
		return nil, err
	}
	open := make([]planrepo.PlanEntry, 0, len(entries))
	ids := make([]shared.ID, 0, len(entries))
	for i, e := range entries {
		if visible[i] && !e.Done {
			open = append(open, e)
			ids = append(ids, e.ID)
		}
	}
	var (
		held      []planrepo.OpenMembership
		estimates map[shared.ID]domain.Estimate
	)
	if err := h.Planner.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if held, err = h.Planner.Sprints.OpenMemberships(ctx, ids); err != nil {
			return err
		}
		estimates, err = h.Planner.Sprints.Estimates(ctx, ids)
		return err
	}); err != nil {
		return nil, err
	}
	inSprint := make(map[shared.ID]bool, len(held))
	for _, m := range held {
		inSprint[m.EntryID] = true
	}
	out := make([]BacklogEntry, 0, len(open))
	for _, e := range open {
		if inSprint[e.ID] {
			continue
		}
		if len(out) == maxBacklog {
			break
		}
		estimate, ok := estimates[e.ID]
		out = append(out, BacklogEntry{ID: e.ID, Title: e.Title, Estimate: estimate, Has: ok})
	}
	return out, nil
}

// Descriptor is the catalogue entry.
func (h ReadSprintBacklog) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadSprintBacklogName, Permission: service.PermissionRead,
		Summary: "Reads a collection's backlog for planning a sprint: its open entries the caller may see that no " +
			"planned or running sprint holds, in the collection's order, at most 500, each with its estimate.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "collection_id", Kind: usecase.KindID, Required: true, Description: "The collection."},
		},
		Audit: usecase.AuditDeclaration{Action: SprintReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			collection, err := in.ID("collection_id")
			if err != nil {
				return nil, err
			}
			backlog, err := h.Execute(ctx, actor, collection)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(backlog))
			for _, b := range backlog {
				data = append(data, usecase.Output{"item_id": b.ID.String(), "title": b.Title, "estimate": EstimateOutput(b.Estimate, b.Has)})
			}
			return usecase.Output{"data": data}, nil
		}),
	}, sprintTarget)
}

// ReadVelocity reads how much a team got done in its latest closed sprints, and the average of
// the latest three (PLN-07).
type ReadVelocity struct{ Planner Planner }

// Execute reads the velocity.
func (h ReadVelocity) Execute(ctx context.Context, actor appshared.ActorContext, collectionID shared.ID) (domain.Velocity, error) {
	if _, err := h.Planner.collection(ctx, actor, collectionID, service.PermissionRead, VelocityReadAction, itemsRead); err != nil {
		return domain.Velocity{}, err
	}
	var closed []domain.Sprint
	if err := h.Planner.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		closed, err = h.Planner.Sprints.List(ctx, collectionID, domain.SprintClosed, domain.MaxVelocitySprints)
		return err
	}); err != nil {
		return domain.Velocity{}, err
	}
	return domain.VelocityOf(closed), nil
}

// Descriptor is the catalogue entry.
func (h ReadVelocity) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadVelocityName, Permission: service.PermissionRead,
		Summary: "Reads a collection's velocity: its latest closed sprints, at most 20, newest first, each with " +
			"what it committed and completed, and the average completed of the latest three in the unit the " +
			"newest counted.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "collection_id", Kind: usecase.KindID, Required: true, Description: "The collection."},
		},
		Audit: usecase.AuditDeclaration{Action: VelocityReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			collection, err := in.ID("collection_id")
			if err != nil {
				return nil, err
			}
			v, err := h.Execute(ctx, actor, collection)
			if err != nil {
				return nil, err
			}
			sprints := make([]usecase.Output, 0, len(v.Sprints))
			for _, s := range v.Sprints {
				sprints = append(sprints, SprintOutput(s))
			}
			return usecase.Output{"unit": string(v.Unit), "average": v.Average, "sprints": sprints}, nil
		}),
	}, sprintTarget)
}
