// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"context"
	"strconv"
	"time"

	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ReadPlanToleranceName = "ReadPlanTolerance"
	SetPlanToleranceName  = "SetPlanTolerance"
	toleranceTarget       = "plan_tolerance"
)

// The stable audit codes of a plan's tolerance (audit.md §2). An exceeded tolerance is recorded
// when a read finds it, so the exception stands in the trail an auditor and a manager filter on.
const (
	ToleranceSetAction      audit.Action = "plan_tolerance.set"
	ToleranceExceededAction audit.Action = "plan_tolerance.exceeded"
)

// walkOf is where a plan's tolerance resolves through: the workspace, the hub, and for a
// collection the collection itself.
func walkOf(actor appshared.ActorContext, plan work.Container) (settingrepo.Place, []settingrepo.Place) {
	workspace := settingrepo.Place{Level: setting.LevelWorkspace, ScopeID: actor.TenantID}
	if plan.Type == work.ContainerHub {
		at := settingrepo.Place{Level: setting.LevelHub, ScopeID: plan.ID}
		return at, []settingrepo.Place{workspace, at}
	}
	hub := settingrepo.Place{Level: setting.LevelHub, ScopeID: plan.ParentID}
	at := settingrepo.Place{Level: setting.LevelCollection, ScopeID: plan.ID}
	return at, []settingrepo.Place{workspace, hub, at}
}

// tolerance resolves a plan's time tolerance inside the caller's transaction.
func (p Planner) tolerance(ctx context.Context, walk []settingrepo.Place) (setting.Resolution, error) {
	definition := domain.TimeToleranceDefinition()
	if p.Settings == nil {
		return setting.Resolve(definition), nil
	}
	instance, plan, err := p.Settings.Above(ctx, string(definition.Key))
	if err != nil {
		return setting.Resolution{}, err
	}
	stored, err := p.Settings.At(ctx, walk, string(definition.Key))
	if err != nil {
		return setting.Resolution{}, err
	}
	return setting.Resolve(definition, append([]setting.Layer{instance, plan}, settingrepo.LayersOf(walk, stored)...)...), nil
}

// ToleranceView is a plan's tolerance with where it comes from and the plan's standing against
// its latest baseline.
type ToleranceView struct {
	Resolution setting.Resolution
	Baseline   *domain.Baseline
	Exception  *domain.Exception
}

// Output is the shape every door returns (schema PlanTolerance).
func (v ToleranceView) Output() usecase.Output {
	var days, locked, baseline, exception any
	if n, ok := domain.TimeToleranceOf(v.Resolution); ok {
		days = n
	}
	if v.Resolution.LockedBy != "" {
		locked = string(v.Resolution.LockedBy)
	}
	if v.Baseline != nil {
		baseline = v.Baseline.ID.String()
	}
	if e := v.Exception; e != nil {
		exception = usecase.Output{
			"agreed_finish": e.Agreed.String(), "forecast_finish": e.Forecast.String(), "slip_days": e.SlipDays,
			"tolerance_days": e.Tolerance, "exceeded": e.Exceeded,
		}
	}
	return usecase.Output{
		"time_tolerance_days": days, "source": string(v.Resolution.Source), "locked_by": locked,
		"baseline_id": baseline, "exception": exception,
	}
}

// ReadPlanTolerance reads a plan's time tolerance - its value, the level it comes from and the
// level that locked it - and, when the plan keeps a baseline, compares the forecast end with the
// latest baseline's: beyond the tolerance the plan is in exception (configuration.md §10).
type ReadPlanTolerance struct{ Planner Planner }

// Execute reads it.
func (h ReadPlanTolerance) Execute(ctx context.Context, actor appshared.ActorContext, q ScheduleQuery) (ToleranceView, error) {
	return h.read(ctx, actor, q, false)
}

// read reads it; alone reads the tolerance without the plan's standing, which writes nothing - the
// configuration document's read, inside a read-only transaction.
func (h ReadPlanTolerance) read(ctx context.Context, actor appshared.ActorContext, q ScheduleQuery, alone bool) (ToleranceView, error) {
	plan, err := h.Planner.plan(ctx, actor, q.PlanID, service.PermissionRead, PlanReadAction, itemsRead)
	if err != nil {
		return ToleranceView{}, err
	}
	_, walk := walkOf(actor, plan)
	var (
		view   ToleranceView
		latest []domain.Baseline
	)
	if err := h.Planner.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if view.Resolution, err = h.Planner.tolerance(ctx, walk); err != nil || alone {
			return err
		}
		if latest, err = h.Planner.Baselines.List(ctx, plan.ID); err != nil || len(latest) == 0 {
			return err
		}
		found, err := h.Planner.Baselines.Find(ctx, latest[0].ID)
		view.Baseline = &found
		return err
	}); err != nil {
		return ToleranceView{}, err
	}
	days, set := domain.TimeToleranceOf(view.Resolution)
	if view.Baseline == nil || !set {
		return view, nil
	}
	scheduled, err := h.Planner.schedule(ctx, actor, q)
	if err != nil || len(scheduled.Result.Placements) == 0 {
		return view, err
	}
	agreed, ok := agreedFinish(*view.Baseline, scheduled.Loaded.Zone)
	if !ok {
		return view, nil
	}
	exception := domain.TimeException(agreed, scheduled.Result.Finish, days)
	view.Exception = &exception
	if exception.Exceeded {
		now := h.Planner.Clock.Now()
		if err := h.Planner.record(ctx, actor, ToleranceExceededAction, toleranceTarget, plan.ID, now,
			audit.Change{Field: "slip_days", Classification: audit.Open, To: strconv.Itoa(exception.SlipDays)},
			audit.Change{Field: "tolerance_days", Classification: audit.Open, To: strconv.Itoa(days)}); err != nil {
			return ToleranceView{}, err
		}
		if err := h.Planner.tell(ctx, actor, walk, view, now); err != nil {
			return ToleranceView{}, err
		}
	}
	return view, nil
}

// tell sends the exception to whoever decided the tolerance at the level it comes from - the level
// above the plan where a hub or the workspace set it. A tolerance of the installation or the plan
// has nobody to tell.
func (p Planner) tell(ctx context.Context, actor appshared.ActorContext, walk []settingrepo.Place, view ToleranceView, now time.Time) error {
	if p.Exceptions == nil || p.Settings == nil || view.Baseline == nil {
		return nil
	}
	return p.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		stored, err := p.Settings.At(ctx, walk, string(domain.TimeToleranceKey))
		if err != nil {
			return err
		}
		var decider shared.ID
		for _, row := range stored {
			if row.Place.Level == view.Resolution.Source && row.Key == domain.TimeToleranceKey {
				decider = row.Entry.By
			}
		}
		if decider.IsZero() {
			return nil
		}
		return p.Exceptions.PlanExceeded(ctx, actor.TenantID, view.Baseline.ID, decider, actor.AccountID, now)
	})
}

// agreedFinish is a baseline's last due day in the plan's zone.
func agreedFinish(b domain.Baseline, zone *time.Location) (domain.Day, bool) {
	var last domain.Day
	for _, e := range b.Entries {
		if e.DueAt == nil {
			continue
		}
		if day := domain.DayOf(*e.DueAt, zone); day > last {
			last = day
		}
	}
	return last, last != 0
}

// Descriptor is the catalogue entry.
func (h ReadPlanTolerance) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadPlanToleranceName, Permission: service.PermissionRead,
		Summary: "Reads a plan's time tolerance - its days, the level it comes from and the level that locked it - " +
			"and, when the plan keeps a baseline, how far the forecast end lies past the latest baseline's and whether " +
			"that is beyond the tolerance: the plan is then in exception.",
		SideEffects: "Writes an audit entry when the plan is found in exception.",
		TokenScope:  itemsRead,
		Input: append(scheduleInput(), usecase.Field{
			Name: "tolerance_only", Kind: usecase.KindBool,
			Description: "Read the tolerance alone, without the plan's standing against its baseline; nothing is recorded.",
		}),
		Audit:    usecase.AuditDeclaration{Action: ToleranceExceededAction},
		Activity: usecase.ActivityDeclaration{Exempt: "a read changes no entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			q, err := scheduleQueryOf(in)
			if err != nil {
				return nil, err
			}
			view, err := h.read(ctx, actor, q, in.Bool("tolerance_only"))
			if err != nil {
				return nil, err
			}
			return view.Output(), nil
		}),
	}, toleranceTarget)
}

// SetPlanToleranceCommand is the input, typed; a nil Days leaves the tolerance to the level above.
type SetPlanToleranceCommand struct {
	PlanID shared.ID
	Days   *int
	Lock   bool
}

// SetPlanTolerance sets a hub's or a collection's time tolerance, locked for the levels below when
// asked, or leaves it to the level above (configuration.md §10). It is the place's shape, so STRUCTURE
// at the place decides it; a level above that locked it refuses.
type SetPlanTolerance struct{ Planner Planner }

// Execute sets it.
func (h SetPlanTolerance) Execute(ctx context.Context, actor appshared.ActorContext, cmd SetPlanToleranceCommand) (ToleranceView, error) {
	plan, err := h.Planner.plan(ctx, actor, cmd.PlanID, service.PermissionStructure, ToleranceSetAction, itemsWrite)
	if err != nil {
		return ToleranceView{}, err
	}
	if h.Planner.Settings == nil {
		return ToleranceView{}, shared.ErrUnavailable.WithDetail("planning.tolerance_unavailable")
	}
	at, walk := walkOf(actor, plan)
	var view ToleranceView
	err = h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if view.Resolution, err = h.Planner.putTolerance(ctx, actor, at, walk, cmd.Days, cmd.Lock); err != nil {
			return err
		}
		return h.Planner.recordTolerance(ctx, actor, plan.ID, cmd.Days, cmd.Lock)
	})
	return view, err
}

// putTolerance stores a level's tolerance, or removes it, inside the caller's transaction, after
// the levels above - all of walk but its last - had their say; it answers the resolution at the
// level.
func (p Planner) putTolerance(
	ctx context.Context, actor appshared.ActorContext, at settingrepo.Place, walk []settingrepo.Place, days *int, lock bool,
) (setting.Resolution, error) {
	definition := domain.TimeToleranceDefinition()
	above, err := p.tolerance(ctx, walk[:len(walk)-1])
	if err != nil {
		return setting.Resolution{}, err
	}
	if days == nil {
		if _, err := p.Settings.Remove(ctx, at, definition.Key); err != nil {
			return setting.Resolution{}, err
		}
	} else {
		hold := setting.HoldSet
		if lock {
			hold = setting.HoldLock
		}
		entry := setting.Entry{Value: *days, Hold: hold, By: actor.AccountID}
		if err := setting.Check(definition, at.Level, above, entry); err != nil {
			return setting.Resolution{}, err
		}
		if err := p.Settings.Put(ctx, p.IDs.NewID(), at, definition.Key, entry, p.Clock.Now()); err != nil {
			return setting.Resolution{}, err
		}
	}
	return p.tolerance(ctx, walk)
}

func (p Planner) recordTolerance(ctx context.Context, actor appshared.ActorContext, target shared.ID, days *int, lock bool) error {
	to := ""
	if days != nil {
		to = strconv.Itoa(*days)
	}
	return p.record(ctx, actor, ToleranceSetAction, toleranceTarget, target, p.Clock.Now(),
		audit.Change{Field: string(domain.TimeToleranceKey), Classification: audit.Open, To: to},
		audit.Change{Field: "locked", Classification: audit.Open, To: strconv.FormatBool(lock && days != nil)})
}

// Descriptor is the catalogue entry.
func (h SetPlanTolerance) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SetPlanToleranceName, Permission: service.PermissionStructure,
		Summary: "Sets a hub's or a collection's time tolerance in calendar days, 0 to 365, locked for the levels " +
			"below when asked, or - without days - leaves it to the level above. A level above that locked it " +
			"refuses (settings.locked).",
		SideEffects: "Writes the setting and an audit entry.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "plan_id", Kind: usecase.KindID, Required: true, Description: "The hub or collection."},
			{Name: "time_tolerance_days", Kind: usecase.KindInt, Description: "The days the forecast end may slip; absent leaves it to the level above."},
			{Name: "lock", Kind: usecase.KindBool, Description: "Fix it for the collections below."},
		},
		Audit:    usecase.AuditDeclaration{Action: ToleranceSetAction},
		Activity: usecase.ActivityDeclaration{Exempt: "a tolerance is the place's setting and changes no entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("plan_id")
			if err != nil {
				return nil, err
			}
			cmd := SetPlanToleranceCommand{PlanID: id, Lock: in.Bool("lock")}
			if in.Present("time_tolerance_days") && in["time_tolerance_days"] != nil {
				days := in.Int("time_tolerance_days")
				cmd.Days = &days
			}
			view, err := h.Execute(ctx, actor, cmd)
			if err != nil {
				return nil, err
			}
			return view.Output(), nil
		}),
	}, toleranceTarget)
}

const (
	ReadWorkspaceToleranceName = "ReadWorkspaceTolerance"
	SetWorkspaceToleranceName  = "SetWorkspaceTolerance"
	// ToleranceReadAction is a refused read of the workspace's tolerance.
	ToleranceReadAction audit.Action = "plan_tolerance.read"
)

// workspaceTolerance asks the permission at the workspace and answers where its tolerance lies.
func (p Planner) workspaceTolerance(
	ctx context.Context, actor appshared.ActorContext, permission service.Permission, action audit.Action, scope string,
) (settingrepo.Place, error) {
	if err := p.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: permission, Path: []identity.Scope{identity.TenantScope()}, Action: action,
		TokenScope: scope, TargetType: toleranceTarget, TargetID: actor.TenantID,
	}); err != nil {
		return settingrepo.Place{}, err
	}
	return settingrepo.Place{Level: setting.LevelWorkspace, ScopeID: actor.TenantID}, nil
}

// workspaceToleranceOutput is keyed by the catalogue's key, as the configuration document's
// settings area reads it: the value, the level it comes from and the level that locked it.
func workspaceToleranceOutput(r setting.Resolution) usecase.Output {
	var days, locked any
	if n, ok := domain.TimeToleranceOf(r); ok {
		days = n
	}
	if r.Locked() {
		locked = string(r.LockedBy)
	}
	return usecase.Output{string(domain.TimeToleranceKey): usecase.Output{
		"value": days, "source": string(r.Source), "locked_by": locked,
	}}
}

// ReadWorkspaceTolerance answers the time tolerance the workspace decides for every plan in it,
// with the level it comes from and the level that locked it (configuration.md §10).
type ReadWorkspaceTolerance struct{ Planner Planner }

// Descriptor is the catalogue entry.
func (h ReadWorkspaceTolerance) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadWorkspaceToleranceName, Permission: service.PermissionRead, ReadOnly: true,
		Summary: "Answers the time tolerance the workspace decides for every plan in it - the days a plan's forecast " +
			"end may slip past its latest baseline's - with the level it comes from and the level that locked it.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead,
		Audit:       usecase.AuditDeclaration{Action: ToleranceReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			at, err := h.Planner.workspaceTolerance(ctx, actor, service.PermissionRead, ToleranceReadAction, itemsRead)
			if err != nil {
				return nil, err
			}
			var r setting.Resolution
			if err := h.Planner.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				r, err = h.Planner.tolerance(ctx, []settingrepo.Place{at})
				return err
			}); err != nil {
				return nil, err
			}
			return workspaceToleranceOutput(r), nil
		}),
	}, toleranceTarget)
}

// SetWorkspaceTolerance sets the time tolerance every plan of the workspace inherits, locked for
// the hubs and collections below when asked, or leaves it to the installation and the plan.
// STRUCTURE at the workspace decides it, as the workspace's own shape.
type SetWorkspaceTolerance struct{ Planner Planner }

// Descriptor is the catalogue entry.
func (h SetWorkspaceTolerance) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SetWorkspaceToleranceName, Permission: service.PermissionStructure,
		Summary: "Sets the time tolerance in calendar days, 0 to 365, that every hub and collection of the workspace " +
			"inherits, locked for them when asked, or - without days - leaves it to the level above. A level above " +
			"that locked it refuses (settings.locked).",
		SideEffects: "Writes the setting and an audit entry.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "time_tolerance_days", Kind: usecase.KindInt, Description: "The days a plan's forecast end may slip; absent or null leaves it to the level above."},
			{Name: "lock", Kind: usecase.KindBool, Description: "Fix it for the hubs and collections below."},
		},
		Audit:    usecase.AuditDeclaration{Action: ToleranceSetAction},
		Activity: usecase.ActivityDeclaration{Exempt: "a tolerance is the workspace's setting and changes no entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			at, err := h.Planner.workspaceTolerance(ctx, actor, service.PermissionStructure, ToleranceSetAction, itemsWrite)
			if err != nil {
				return nil, err
			}
			if h.Planner.Settings == nil {
				return nil, shared.ErrUnavailable.WithDetail("planning.tolerance_unavailable")
			}
			var days *int
			if in.Present("time_tolerance_days") && in["time_tolerance_days"] != nil {
				n := in.Int("time_tolerance_days")
				days = &n
			}
			var r setting.Resolution
			err = h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				if r, err = h.Planner.putTolerance(ctx, actor, at, []settingrepo.Place{at}, days, in.Bool("lock")); err != nil {
					return err
				}
				return h.Planner.recordTolerance(ctx, actor, actor.TenantID, days, in.Bool("lock"))
			})
			if err != nil {
				return nil, err
			}
			return workspaceToleranceOutput(r), nil
		}),
	}, toleranceTarget)
}
