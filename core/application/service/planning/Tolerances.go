// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"context"
	"strconv"
	"time"

	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
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
// latest baseline's: beyond the tolerance the plan is in exception (standards.md P12).
type ReadPlanTolerance struct{ Planner Planner }

// Execute reads it.
func (h ReadPlanTolerance) Execute(ctx context.Context, actor appshared.ActorContext, q ScheduleQuery) (ToleranceView, error) {
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
		if view.Resolution, err = h.Planner.tolerance(ctx, walk); err != nil {
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
		if err := h.Planner.record(ctx, actor, ToleranceExceededAction, toleranceTarget, plan.ID, h.Planner.Clock.Now(),
			audit.Change{Field: "slip_days", Classification: audit.Open, To: strconv.Itoa(exception.SlipDays)},
			audit.Change{Field: "tolerance_days", Classification: audit.Open, To: strconv.Itoa(days)}); err != nil {
			return ToleranceView{}, err
		}
	}
	return view, nil
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
		Input:       scheduleInput(),
		Audit:       usecase.AuditDeclaration{Action: ToleranceExceededAction},
		Activity:    usecase.ActivityDeclaration{Exempt: "a read changes no entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			q, err := scheduleQueryOf(in)
			if err != nil {
				return nil, err
			}
			view, err := h.Execute(ctx, actor, q)
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
// asked, or leaves it to the level above (standards.md P12). It is the place's shape, so STRUCTURE
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
	definition := domain.TimeToleranceDefinition()
	now := h.Planner.Clock.Now()
	var view ToleranceView
	err = h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		above, err := h.Planner.tolerance(ctx, walk[:len(walk)-1])
		if err != nil {
			return err
		}
		if cmd.Days == nil {
			if _, err := h.Planner.Settings.Remove(ctx, at, definition.Key); err != nil {
				return err
			}
		} else {
			hold := setting.HoldSet
			if cmd.Lock {
				hold = setting.HoldLock
			}
			entry := setting.Entry{Value: *cmd.Days, Hold: hold, By: actor.AccountID}
			if err := setting.Check(definition, at.Level, above, entry); err != nil {
				return err
			}
			if err := h.Planner.Settings.Put(ctx, h.Planner.IDs.NewID(), at, definition.Key, entry, now); err != nil {
				return err
			}
		}
		if view.Resolution, err = h.Planner.tolerance(ctx, walk); err != nil {
			return err
		}
		to := ""
		if cmd.Days != nil {
			to = strconv.Itoa(*cmd.Days)
		}
		return h.Planner.record(ctx, actor, ToleranceSetAction, toleranceTarget, plan.ID, now,
			audit.Change{Field: string(definition.Key), Classification: audit.Open, To: to},
			audit.Change{Field: "locked", Classification: audit.Open, To: strconv.FormatBool(cmd.Lock && cmd.Days != nil)})
	})
	return view, err
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
