// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"context"
	"strconv"
	"time"

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
	ListCapacitiesName = "ListCapacities"
	SetCapacityName    = "SetCapacity"
	RemoveCapacityName = "RemoveCapacity"
	ListAbsencesName   = "ListAbsences"
	RecordAbsenceName  = "RecordAbsence"
	UpdateAbsenceName  = "UpdateAbsence"
	RemoveAbsenceName  = "RemoveAbsence"
	ReadWorkloadName   = "ReadWorkload"

	capacityTarget = "time_capacity"
	absenceTarget  = "time_absence"
	workloadTarget = "plan"

	maxCapacitiesRead = 5000
	maxAbsencesRead   = 5000
	maxCapacitiesKept = 100
)

// The stable audit codes (audit.md §2).
const (
	CapacitySetAction     audit.Action = "time_capacity.set"
	CapacityRemovedAction audit.Action = "time_capacity.removed"
	CapacityReadAction    audit.Action = "time_capacity.read"
	AbsenceRecordedAction audit.Action = "time_absence.recorded"
	AbsenceUpdatedAction  audit.Action = "time_absence.updated"
	AbsenceRemovedAction  audit.Action = "time_absence.removed"
	AbsencesReadAction    audit.Action = "time_absence.read"
	WorkloadReadAction    audit.Action = "time_workload.read"
)

// selfOrManager lets a person write their own capacity and absences, and whoever manages the
// workspace's members write anybody's (UC-TIM-06, UC-TIM-07).
func (k Keeper) selfOrManager(ctx context.Context, actor appshared.ActorContext, account shared.ID, action audit.Action, target string) error {
	if err := person(actor); err != nil {
		return err
	}
	if account == actor.AccountID {
		return nil
	}
	return k.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers, Path: []identity.Scope{identity.TenantScope()},
		Action: action, TokenScope: workspaceManage, TargetType: target, TargetID: account,
	})
}

// ListCapacities answers a person's capacities: one's own to everybody, anybody's to whoever
// approves time or manages members at the workspace.
type ListCapacities struct{ Keeper Keeper }

// Execute reads them; a zero account is the caller.
func (h ListCapacities) Execute(ctx context.Context, actor appshared.ActorContext, account shared.ID) ([]domain.Capacity, error) {
	if err := person(actor); err != nil {
		return nil, err
	}
	if account.IsZero() {
		account = actor.AccountID
	}
	if account != actor.AccountID {
		if err := h.Keeper.refuseAsApprover(ctx, actor, CapacityReadAction, itemsRead, account); err != nil {
			return nil, err
		}
	}
	var out []domain.Capacity
	err := h.Keeper.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		out, err = h.Keeper.Resources.Capacities(ctx, []shared.ID{account}, maxCapacitiesKept)
		return err
	})
	return out, err
}

// CapacityOutput is a capacity on the wire.
func CapacityOutput(c domain.Capacity) usecase.Output {
	minutes := make([]int, 7)
	copy(minutes, c.WeekMinutes[:])
	return usecase.Output{"account_id": c.AccountID.String(), "from": c.From.String(), "week_minutes": minutes, "version": c.Version}
}

// Descriptor is the catalogue entry.
func (h ListCapacities) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListCapacitiesName,
		Summary: "Lists a person's capacities - the minutes of each weekday from a day on - the earliest first. " +
			"One's own to everybody; another person's needs APPROVE_TIME or MANAGE_MEMBERS at the workspace.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "account_id", Kind: usecase.KindID, Description: "The person; the caller without one."},
		},
		Audit: usecase.AuditDeclaration{Action: CapacityReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			account, err := optionalID(in, "account_id")
			if err != nil {
				return nil, err
			}
			list, err := h.Execute(ctx, actor, account)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(list))
			for _, c := range list {
				data = append(data, CapacityOutput(c))
			}
			return usecase.Output{"data": data}, nil
		}),
	}, capacityTarget)
}

// SetCapacity writes the minutes a person can work on each weekday from a day on, until their
// next capacity begins (UC-TIM-06).
type SetCapacity struct{ Keeper Keeper }

// Execute writes it.
func (h SetCapacity) Execute(ctx context.Context, actor appshared.ActorContext, c domain.Capacity) (domain.Capacity, error) {
	if c.AccountID.IsZero() {
		c.AccountID = actor.AccountID
	}
	if err := domain.CheckCapacity(c); err != nil {
		return domain.Capacity{}, err
	}
	if err := h.Keeper.selfOrManager(ctx, actor, c.AccountID, CapacitySetAction, capacityTarget); err != nil {
		return domain.Capacity{}, err
	}
	now := h.Keeper.Clock.Now()
	err := h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		held, err := h.Keeper.Resources.Capacities(ctx, []shared.ID{c.AccountID}, maxCapacitiesKept+1)
		if err != nil {
			return err
		}
		known := false
		for _, other := range held {
			if other.From == c.From {
				known, c.Version = true, other.Version+1
			}
		}
		if !known {
			if len(held) >= maxCapacitiesKept {
				return shared.ErrValidation.WithDetail("time.too_many_capacities").
					WithParams(map[string]string{"maximum": strconv.Itoa(maxCapacitiesKept)})
			}
			c.Version = 1
		}
		if err := h.Keeper.Resources.PutCapacity(ctx, h.Keeper.IDs.NewID(), c, actor.AccountID, now); err != nil {
			return err
		}
		if err := h.Keeper.capacitiesChanged(ctx, actor, c.AccountID); err != nil {
			return err
		}
		total := 0
		for _, m := range c.WeekMinutes {
			total += m
		}
		return h.Keeper.record(ctx, actor, CapacitySetAction, capacityTarget, c.AccountID, now,
			audit.Change{Field: "from", Classification: audit.Open, To: c.From.String()},
			audit.Change{Field: "week_minutes", Classification: audit.Open, To: strconv.Itoa(total)})
	})
	return c, err
}

func weekMinutesOf(in usecase.Input) ([7]int, error) {
	var out [7]int
	list, ok := in["week_minutes"].([]any)
	if !ok || len(list) != 7 {
		return out, invalid("/week_minutes", "time.capacity_week_required")
	}
	for i, v := range list {
		n, ok := intField(usecase.Input{"v": v}, "v")
		if !ok {
			return out, invalid("/week_minutes", "time.capacity_week_required")
		}
		out[i] = n
	}
	return out, nil
}

// Descriptor is the catalogue entry.
func (h SetCapacity) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SetCapacityName,
		Summary: "Writes the minutes a person can work on each weekday, Monday to Sunday, from a day on until their " +
			"next capacity begins - a part-time person's four days of six hours. One's own needs nothing more; " +
			"another person's needs MANAGE_MEMBERS at the workspace.",
		SideEffects: "Writes the capacity and an audit entry.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "account_id", Kind: usecase.KindID, Description: "The person; the caller without one."},
			{Name: "from", Kind: usecase.KindString, Required: true, Description: "The first day it holds, YYYY-MM-DD."},
			{Name: "week_minutes", Kind: usecase.KindList, Required: true, Description: "Seven numbers of minutes, Monday first, each 0 to 1440."},
		},
		Audit: usecase.AuditDeclaration{Action: CapacitySetAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			account, err := optionalID(in, "account_id")
			if err != nil {
				return nil, err
			}
			from, err := dayField(in, "from", true)
			if err != nil {
				return nil, err
			}
			week, err := weekMinutesOf(in)
			if err != nil {
				return nil, err
			}
			c, err := h.Execute(ctx, actor, domain.Capacity{AccountID: account, From: from, WeekMinutes: week})
			if err != nil {
				return nil, err
			}
			return CapacityOutput(c), nil
		}),
	}, capacityTarget)
}

// RemoveCapacity removes the capacity a person has from a day; the one before it holds on.
type RemoveCapacity struct{ Keeper Keeper }

// Execute removes it.
func (h RemoveCapacity) Execute(ctx context.Context, actor appshared.ActorContext, account shared.ID, from domain.Day) error {
	if account.IsZero() {
		account = actor.AccountID
	}
	if err := h.Keeper.selfOrManager(ctx, actor, account, CapacityRemovedAction, capacityTarget); err != nil {
		return err
	}
	now := h.Keeper.Clock.Now()
	return h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		removed, err := h.Keeper.Resources.RemoveCapacity(ctx, account, from)
		if err != nil {
			return err
		}
		if !removed {
			return shared.ErrNotFound.WithDetail("time.capacity_not_found")
		}
		if err := h.Keeper.capacitiesChanged(ctx, actor, account); err != nil {
			return err
		}
		return h.Keeper.record(ctx, actor, CapacityRemovedAction, capacityTarget, account, now,
			audit.Change{Field: "from", Classification: audit.Open, From: from.String()})
	})
}

// Descriptor is the catalogue entry.
func (h RemoveCapacity) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: RemoveCapacityName,
		Summary: "Removes the capacity a person has from a day; the one before it holds on. One's own needs nothing " +
			"more; another person's needs MANAGE_MEMBERS at the workspace.",
		SideEffects: "Removes the capacity and writes an audit entry.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "account_id", Kind: usecase.KindID, Description: "The person; the caller without one."},
			{Name: "from", Kind: usecase.KindString, Required: true, Description: "The first day of the capacity, YYYY-MM-DD."},
		},
		Audit: usecase.AuditDeclaration{Action: CapacityRemovedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			account, err := optionalID(in, "account_id")
			if err != nil {
				return nil, err
			}
			from, err := dayField(in, "from", true)
			if err != nil {
				return nil, err
			}
			if err := h.Execute(ctx, actor, account, from); err != nil {
				return nil, err
			}
			return usecase.Output{"removed": true}, nil
		}),
	}, capacityTarget)
}

// ListAbsences answers the workspace's shared calendar of absences in a range (UC-TIM-07): who is
// away when, and only the kind - never a reason - to every member who records time.
type ListAbsences struct{ Keeper Keeper }

// Execute reads them; a zero account is everybody.
func (h ListAbsences) Execute(ctx context.Context, actor appshared.ActorContext, account shared.ID, from, to domain.Day) ([]domain.Absence, error) {
	if err := person(actor); err != nil {
		return nil, err
	}
	var out []domain.Absence
	err := h.Keeper.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		out, err = h.Keeper.Resources.Absences(ctx, account, from, to, maxAbsencesRead)
		return err
	})
	return out, err
}

// AbsenceOutput is an absence on the wire.
func AbsenceOutput(a domain.Absence) usecase.Output {
	return usecase.Output{
		"id": a.ID.String(), "account_id": a.AccountID.String(), "from": a.From.String(), "to": a.To.String(),
		"kind": string(a.Kind), "half": a.Half, "version": a.Version,
	}
}

// Descriptor is the catalogue entry.
func (h ListAbsences) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListAbsencesName,
		Summary: "Lists the absences that share a day with a range of at most a year - the shared calendar: who is " +
			"away, from when to when, wholly or half of each day, and only the kind (LEAVE, TRAINING, OTHER), never a reason.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "from", Kind: usecase.KindString, Required: true, Description: "The first day, YYYY-MM-DD."},
			{Name: "to", Kind: usecase.KindString, Required: true, Description: "The last day, YYYY-MM-DD."},
			{Name: "account_id", Kind: usecase.KindID, Description: "One person; everybody without one."},
		},
		Audit: usecase.AuditDeclaration{Action: AbsencesReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			from, to, err := dayRange(in)
			if err != nil {
				return nil, err
			}
			account, err := optionalID(in, "account_id")
			if err != nil {
				return nil, err
			}
			list, err := h.Execute(ctx, actor, account, from, to)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(list))
			for _, a := range list {
				data = append(data, AbsenceOutput(a))
			}
			return usecase.Output{"data": data}, nil
		}),
	}, absenceTarget)
}

// RecordAbsence records a span a person is away once, for every plan (UC-TIM-07): the workload
// and every schedule read it from here.
type RecordAbsence struct{ Keeper Keeper }

// Execute records it.
func (h RecordAbsence) Execute(ctx context.Context, actor appshared.ActorContext, a domain.Absence) (domain.Absence, error) {
	if a.AccountID.IsZero() {
		a.AccountID = actor.AccountID
	}
	if err := domain.CheckAbsence(a); err != nil {
		return domain.Absence{}, err
	}
	if err := h.Keeper.selfOrManager(ctx, actor, a.AccountID, AbsenceRecordedAction, absenceTarget); err != nil {
		return domain.Absence{}, err
	}
	a.ID, a.Version = h.Keeper.IDs.NewID(), 1
	now := h.Keeper.Clock.Now()
	err := h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Keeper.noOverlap(ctx, a); err != nil {
			return err
		}
		if err := h.Keeper.Resources.PutAbsence(ctx, a, 0, actor.AccountID, now); err != nil {
			return err
		}
		if err := h.Keeper.absenceChanged(ctx, actor, a.ID, &a); err != nil {
			return err
		}
		return h.Keeper.record(ctx, actor, AbsenceRecordedAction, absenceTarget, a.ID, now, absenceChanges(domain.Absence{}, a)...)
	})
	return a, err
}

func (k Keeper) noOverlap(ctx context.Context, a domain.Absence) error {
	others, err := k.Resources.Absences(ctx, a.AccountID, a.From, a.To, maxAbsencesRead)
	if err != nil {
		return err
	}
	for _, other := range others {
		if a.Overlaps(other) {
			return shared.ErrConflict.WithDetail("time.absence_overlaps").
				WithParams(map[string]string{"from": other.From.String(), "to": other.To.String()})
		}
	}
	return nil
}

func absenceChanges(before, after domain.Absence) []audit.Change {
	text := func(d domain.Day, set bool) string {
		if !set {
			return ""
		}
		return d.String()
	}
	had := !before.ID.IsZero()
	return []audit.Change{
		{Field: "from", Classification: audit.Open, From: text(before.From, had), To: after.From.String()},
		{Field: "to", Classification: audit.Open, From: text(before.To, had), To: after.To.String()},
		{Field: "kind", Classification: audit.Open, From: string(before.Kind), To: string(after.Kind)},
		{Field: "half", Classification: audit.Open, From: strconv.FormatBool(before.Half), To: strconv.FormatBool(after.Half)},
	}
}

func absenceFields(in usecase.Input) (domain.Absence, error) {
	account, err := optionalID(in, "account_id")
	if err != nil {
		return domain.Absence{}, err
	}
	from, err := dayField(in, "from", true)
	if err != nil {
		return domain.Absence{}, err
	}
	to, err := dayField(in, "to", true)
	if err != nil {
		return domain.Absence{}, err
	}
	half, _ := in["half"].(bool)
	return domain.Absence{AccountID: account, From: from, To: to, Kind: domain.AbsenceKind(in.String("kind")), Half: half}, nil
}

var absenceInput = []usecase.Field{
	{Name: "from", Kind: usecase.KindString, Required: true, Description: "The first day away, YYYY-MM-DD."},
	{Name: "to", Kind: usecase.KindString, Required: true, Description: "The last day away, YYYY-MM-DD; at most a year after the first."},
	{Name: "kind", Kind: usecase.KindString, Required: true, Enum: []string{"LEAVE", "TRAINING", "OTHER"}, Description: "Why, as far as the team may know: an illness is OTHER."},
	{Name: "half", Kind: usecase.KindBool, Description: "Away half of each day of the span."},
}

// Descriptor is the catalogue entry.
func (h RecordAbsence) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: RecordAbsenceName,
		Summary: "Records a span a person is away - leave, training or other, wholly or half of each day - once, for " +
			"every plan and the workload. A span sharing a day with another of the person's is refused. One's own " +
			"needs nothing more; another person's needs MANAGE_MEMBERS at the workspace.",
		SideEffects: "Writes the absence and an audit entry.",
		TokenScope:  itemsWrite,
		Input: append([]usecase.Field{
			{Name: "account_id", Kind: usecase.KindID, Description: "The person; the caller without one."},
		}, absenceInput...),
		Audit: usecase.AuditDeclaration{Action: AbsenceRecordedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			a, err := absenceFields(in)
			if err != nil {
				return nil, err
			}
			out, err := h.Execute(ctx, actor, a)
			if err != nil {
				return nil, err
			}
			return AbsenceOutput(out), nil
		}),
	}, absenceTarget)
}

// UpdateAbsence changes an absence's span, kind or half days.
type UpdateAbsence struct{ Keeper Keeper }

// Execute writes it held to the version read.
func (h UpdateAbsence) Execute(ctx context.Context, actor appshared.ActorContext, a domain.Absence, expected int) (domain.Absence, error) {
	if err := person(actor); err != nil {
		return domain.Absence{}, err
	}
	var before domain.Absence
	if err := h.Keeper.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		before, err = h.Keeper.Resources.FindAbsence(ctx, a.ID)
		return err
	}); err != nil {
		return domain.Absence{}, err
	}
	a.AccountID = before.AccountID
	if err := domain.CheckAbsence(a); err != nil {
		return domain.Absence{}, err
	}
	if err := h.Keeper.selfOrManager(ctx, actor, a.AccountID, AbsenceUpdatedAction, absenceTarget); err != nil {
		return domain.Absence{}, err
	}
	now := h.Keeper.Clock.Now()
	a.Version = expected + 1
	err := h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Keeper.noOverlap(ctx, a); err != nil {
			return err
		}
		if err := h.Keeper.Resources.PutAbsence(ctx, a, expected, actor.AccountID, now); err != nil {
			return err
		}
		if err := h.Keeper.absenceChanged(ctx, actor, a.ID, &a); err != nil {
			return err
		}
		return h.Keeper.record(ctx, actor, AbsenceUpdatedAction, absenceTarget, a.ID, now, absenceChanges(before, a)...)
	})
	return a, err
}

// Descriptor is the catalogue entry.
func (h UpdateAbsence) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: UpdateAbsenceName,
		Summary: "Changes an absence's days, kind or half days, held to the version read. One's own needs nothing " +
			"more; another person's needs MANAGE_MEMBERS at the workspace.",
		SideEffects: "Writes the absence and an audit entry.",
		TokenScope:  itemsWrite,
		Input: append([]usecase.Field{
			{Name: "absence_id", Kind: usecase.KindID, Required: true, Description: "The absence."},
			{Name: "expected_version", Kind: usecase.KindAny, Required: true, CallerOnly: true, Description: "The version read."},
		}, absenceInput...),
		Audit: usecase.AuditDeclaration{Action: AbsenceUpdatedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("absence_id")
			if err != nil {
				return nil, err
			}
			expected, ok := intField(in, "expected_version")
			if !ok {
				return nil, invalid("/expected_version", "time.version_required")
			}
			a, err := absenceFields(in)
			if err != nil {
				return nil, err
			}
			a.ID = id
			out, err := h.Execute(ctx, actor, a, expected)
			if err != nil {
				return nil, err
			}
			return AbsenceOutput(out), nil
		}),
	}, absenceTarget)
}

// RemoveAbsence removes an absence.
type RemoveAbsence struct{ Keeper Keeper }

// Execute removes it held to the version read.
func (h RemoveAbsence) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID, expected int) error {
	if err := person(actor); err != nil {
		return err
	}
	var before domain.Absence
	if err := h.Keeper.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		before, err = h.Keeper.Resources.FindAbsence(ctx, id)
		return err
	}); err != nil {
		return err
	}
	if err := h.Keeper.selfOrManager(ctx, actor, before.AccountID, AbsenceRemovedAction, absenceTarget); err != nil {
		return err
	}
	now := h.Keeper.Clock.Now()
	return h.Keeper.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Keeper.Resources.RemoveAbsence(ctx, id, expected); err != nil {
			return err
		}
		if err := h.Keeper.absenceChanged(ctx, actor, id, nil); err != nil {
			return err
		}
		return h.Keeper.record(ctx, actor, AbsenceRemovedAction, absenceTarget, id, now,
			audit.Change{Field: "kind", Classification: audit.Open, From: string(before.Kind)})
	})
}

// Descriptor is the catalogue entry.
func (h RemoveAbsence) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: RemoveAbsenceName,
		Summary: "Removes an absence, held to the version read. One's own needs nothing more; another person's " +
			"needs MANAGE_MEMBERS at the workspace.",
		SideEffects: "Removes the absence and writes an audit entry.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "absence_id", Kind: usecase.KindID, Required: true, Description: "The absence."},
			{Name: "expected_version", Kind: usecase.KindAny, Required: true, CallerOnly: true, Description: "The version read."},
		},
		Audit: usecase.AuditDeclaration{Action: AbsenceRemovedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("absence_id")
			if err != nil {
				return nil, err
			}
			expected, ok := intField(in, "expected_version")
			if !ok {
				return nil, invalid("/expected_version", "time.version_required")
			}
			if err := h.Execute(ctx, actor, id, expected); err != nil {
				return nil, err
			}
			return usecase.Output{"removed": true}, nil
		}),
	}, absenceTarget)
}

// ReadWorkload lays out who has room in a hub or a collection (UC-TIM-06): each person's weeks,
// what they can give against what their open entries' remaining estimates ask. The people are
// those the plan's open entries are assigned to and everybody whose capacity is known. Where the
// workspace shows only sums, a reader who does not approve time at the plan reads the team's
// weeks, naming nobody.
type ReadWorkload struct{ Keeper Keeper }

// WorkloadQuery is what the workload is asked.
type WorkloadQuery struct {
	PlanID   shared.ID
	From     domain.Day
	Weeks    int
	Calendar string
	Zone     *time.Location
}

// Workload is the answer: per person, or the team's sums.
type Workload struct {
	Cells []domain.WorkloadCell
	Team  []domain.TeamWeek
	// Aggregated says the answer is the team's sums.
	Aggregated bool
	// UnscheduledMinutes are remaining minutes of assigned open entries with neither a start nor a due day.
	UnscheduledMinutes int
}

// Execute reads it.
func (h ReadWorkload) Execute(ctx context.Context, actor appshared.ActorContext, q WorkloadQuery) (Workload, error) {
	if err := person(actor); err != nil {
		return Workload{}, err
	}
	if q.Zone == nil {
		q.Zone = time.UTC
	}
	at, err := h.Keeper.planPlace(ctx, actor, q.PlanID)
	if err != nil {
		return Workload{}, err
	}
	if err := h.Keeper.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead, Path: at.path(), Action: WorkloadReadAction, TokenScope: itemsRead,
		TargetType: workloadTarget, TargetID: q.PlanID,
	}); err != nil {
		return Workload{}, err
	}
	policy, err := h.Keeper.policy(ctx, actor)
	if err != nil {
		return Workload{}, err
	}
	aggregated := false
	if policy.Visibility == domain.VisibilityAggregated {
		allowed, err := h.Keeper.approves(ctx, actor, []place{at}, WorkloadReadAction, itemsRead)
		if err != nil {
			return Workload{}, err
		}
		aggregated = len(allowed) == 0 || !allowed[0]
	}
	var calendar planning.Workdays
	if q.Calendar != "" {
		if h.Keeper.Calendars == nil {
			return Workload{}, invalid("/calendar", "calendars.not_found")
		}
		if err := h.Keeper.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			found, err := h.Keeper.Calendars.Find(ctx, q.Calendar)
			calendar = found.Calendar
			return err
		}); err != nil {
			return Workload{}, err
		}
	}
	first := domain.WeekOf(q.From)
	weeks := q.Weeks
	if weeks < 1 || weeks > domain.MaxWorkloadWeeks {
		weeks = domain.MaxWorkloadWeeks
	}
	last := first + domain.Day(7*weeks) - 1
	var (
		entries    []planEntryAssigned
		capacities []domain.Capacity
		absences   []domain.Absence
		estimates  map[shared.ID]planning.Estimate
		minutes    map[shared.ID]int
	)
	if err := h.Keeper.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		rows, err := h.Keeper.Plans.Entries(ctx, q.PlanID, maxEntriesOfAPlan+1)
		if err != nil {
			return err
		}
		if len(rows) > maxEntriesOfAPlan {
			return shared.ErrValidation.WithDetail("planning.too_many_entries").
				WithParams(map[string]string{"maximum": strconv.Itoa(maxEntriesOfAPlan)})
		}
		var ids []shared.ID
		for _, r := range rows {
			if r.Done || r.AssigneeID.IsZero() {
				continue
			}
			entries = append(entries, planEntryAssigned{r.ID, r.HubID, r.CollectionID, r.AssigneeID, r.StartAt, r.DueAt})
			ids = append(ids, r.ID)
		}
		if estimates, err = h.Keeper.Estimates.Estimates(ctx, ids); err != nil {
			return err
		}
		if minutes, err = h.Keeper.Store.MinutesOn(ctx, ids); err != nil {
			return err
		}
		if capacities, err = h.Keeper.Resources.Capacities(ctx, nil, maxCapacitiesRead); err != nil {
			return err
		}
		absences, err = h.Keeper.Resources.Absences(ctx, "", first, last, maxAbsencesRead)
		return err
	}); err != nil {
		return Workload{}, err
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
		return Workload{}, err
	}
	people := peopleOf(capacities, absences)
	out := Workload{Aggregated: aggregated}
	var allocations []domain.Allocation
	for i, e := range entries {
		if i >= len(visible) || !visible[i] {
			continue
		}
		hours, ok := estimates[e.id].CountsIn(planning.Hours)
		if !ok {
			continue
		}
		remaining := int(hours*60) - minutes[e.id]
		if remaining <= 0 {
			continue
		}
		start, finish, scheduled := spanOf(e.start, e.due, q.Zone)
		if !scheduled {
			out.UnscheduledMinutes += remaining
			continue
		}
		people = withPerson(people, e.assignee)
		allocations = append(allocations, domain.Allocation{
			EntryID: e.id, AccountID: e.assignee, Start: start, Finish: finish, Minutes: remaining,
		})
	}
	cells := domain.Workload(people, calendar, allocations, q.From, weeks)
	if aggregated {
		out.Team = domain.SumByWeek(cells)
		return out, nil
	}
	out.Cells = cells
	return out, nil
}

type planEntryAssigned struct {
	id, hub, collection, assignee shared.ID
	start, due                    *time.Time
}

// spanOf answers an entry's scheduled days: its start to its due day, one of them where only one
// is set.
func spanOf(start, due *time.Time, zone *time.Location) (domain.Day, domain.Day, bool) {
	switch {
	case start != nil && due != nil:
		return planning.DayOf(*start, zone), planning.DayOf(*due, zone), true
	case due != nil:
		d := planning.DayOf(*due, zone)
		return d, d, true
	case start != nil:
		d := planning.DayOf(*start, zone)
		return d, d, true
	}
	return 0, 0, false
}

func peopleOf(capacities []domain.Capacity, absences []domain.Absence) []domain.Person {
	var people []domain.Person
	index := map[shared.ID]int{}
	for _, c := range capacities {
		i, ok := index[c.AccountID]
		if !ok {
			i = len(people)
			index[c.AccountID] = i
			people = append(people, domain.Person{AccountID: c.AccountID})
		}
		people[i].Capacities = append(people[i].Capacities, c)
	}
	for _, a := range absences {
		if i, ok := index[a.AccountID]; ok {
			people[i].Absences = append(people[i].Absences, a)
		}
	}
	return people
}

func withPerson(people []domain.Person, account shared.ID) []domain.Person {
	for _, p := range people {
		if p.AccountID == account {
			return people
		}
	}
	return append(people, domain.Person{AccountID: account})
}

// planPlace reads a hub or a collection that is not trashed and answers where it lies.
func (k Keeper) planPlace(ctx context.Context, actor appshared.ActorContext, planID shared.ID) (place, error) {
	if planID.IsZero() {
		return place{}, invalid("/plan_id", "planning.plan_required")
	}
	var found work.Container
	if err := k.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		found, err = k.Containers.Find(ctx, planID)
		return err
	}); err != nil {
		return place{}, err
	}
	if found.DeletedAt != nil {
		return place{}, shared.ErrNotFound.WithDetail("containers.not_found")
	}
	if found.Type == work.ContainerHub {
		return place{hub: found.ID}, nil
	}
	return place{hub: found.ParentID, collection: found.ID}, nil
}

// Descriptor is the catalogue entry.
func (h ReadWorkload) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadWorkloadName, Permission: service.PermissionRead,
		Summary: "Lays out who has room in a hub or a collection, week by week from a Monday, up to 26 weeks: each " +
			"person's available minutes (capacity, less public holidays of the business calendar and absences) against " +
			"the minutes their assigned open entries' remaining estimates in hours ask, spread over the entries' " +
			"scheduled days; over and under allocated marked. Needs READ at the plan; where the workspace shows only " +
			"sums, a reader without APPROVE_TIME or MANAGE_MEMBERS there reads the team's weeks only.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "plan_id", Kind: usecase.KindID, Required: true, Description: "The hub or the collection."},
			{Name: "from", Kind: usecase.KindString, Required: true, Description: "A day of the first week, YYYY-MM-DD."},
			{Name: "weeks", Kind: usecase.KindAny, Description: "How many weeks, 1 to 26; 26 without one."},
			{Name: "calendar", Kind: usecase.KindString, Description: "A business calendar's key, whose holidays nobody works."},
			{Name: "zone", Kind: usecase.KindString, Description: "The time zone the entries' dates are days in; UTC without one."},
		},
		Audit: usecase.AuditDeclaration{Action: WorkloadReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			plan, err := in.ID("plan_id")
			if err != nil {
				return nil, err
			}
			from, err := dayField(in, "from", true)
			if err != nil {
				return nil, err
			}
			zone, err := zoneOf(in)
			if err != nil {
				return nil, err
			}
			weeks, _ := intField(in, "weeks")
			w, err := h.Execute(ctx, actor, WorkloadQuery{PlanID: plan, From: from, Weeks: weeks, Calendar: in.String("calendar"), Zone: zone})
			if err != nil {
				return nil, err
			}
			return WorkloadOutput(plan, w), nil
		}),
	}, workloadTarget)
}

// WorkloadOutput is the workload on the wire.
func WorkloadOutput(plan shared.ID, w Workload) usecase.Output {
	people := make([]usecase.Output, 0, len(w.Cells))
	for _, c := range w.Cells {
		people = append(people, usecase.Output{
			"account_id": c.AccountID.String(), "week": c.Week.String(), "available_minutes": c.Available,
			"allocated_minutes": c.Allocated, "known": c.Known, "over": c.Over(), "under": c.Under(),
		})
	}
	team := make([]usecase.Output, 0, len(w.Team))
	for _, t := range w.Team {
		team = append(team, usecase.Output{
			"week": t.Week.String(), "available_minutes": t.Available, "allocated_minutes": t.Allocated, "people": t.People,
		})
	}
	return usecase.Output{
		"plan_id": plan.String(), "aggregated": w.Aggregated, "cells": people, "team": team,
		"unscheduled_minutes": w.UnscheduledMinutes,
	}
}
