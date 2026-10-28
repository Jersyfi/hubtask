// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package setting holds the use cases of the settings catalogue (domain-model.md §2.4): reading
// what a place resolves, and writing one level's entry, with the module switches first
// (UC-INS-21).
package setting

import (
	"context"
	"errors"
	"strings"
	"time"

	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	domain "github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ListModulesName = "ListModules"
	SetModuleName   = "SetModule"

	// ModuleSwitchedAction is the trail's code of a switch (UC-INS-21 check 8). Stable: an auditor
	// filters on it (audit.md §2).
	ModuleSwitchedAction audit.Action = "module.switched"
	// ModulesReadAction is the code a refused read is recorded under.
	ModulesReadAction audit.Action = "module.read"

	moduleTarget        = "module"
	workspaceReadScope  = "workspace:read"
	workspaceWriteScope = "workspace:manage"
)

// Authorizer is the one authorisation question this package asks.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// Containers is the one read of the container store this package needs: whether a hub is one.
type Containers interface {
	Find(ctx context.Context, id shared.ID) (work.Container, error)
}

// Modules is what both module use cases and the gate share: where a place's entries are, and how
// they resolve.
type Modules struct {
	Settings   settingrepo.Settings
	Containers Containers
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// place is the workspace or one hub of it, with the places a resolution walks through.
type place struct {
	at     settingrepo.Place
	walk   []settingrepo.Place
	path   []identity.Scope
	target shared.ID
}

func (m Modules) placeOf(ctx context.Context, actor appshared.ActorContext, hubID shared.ID) (place, error) {
	workspace := settingrepo.Place{Level: domain.LevelWorkspace, ScopeID: actor.TenantID}
	if hubID.IsZero() {
		return place{
			at: workspace, walk: []settingrepo.Place{workspace},
			path: []identity.Scope{identity.TenantScope()}, target: actor.TenantID,
		}, nil
	}
	hub, err := m.Containers.Find(ctx, hubID)
	if err != nil {
		return place{}, err
	}
	if hub.Type != work.ContainerHub {
		// A module is switched for a hub and never for one collection (UC-INS-21, *Where it ends*).
		return place{}, shared.ErrValidation.WithDetail("module.hub_only").
			WithFields(shared.FieldError{Path: "/hub_id", Code: "module.hub_only"})
	}
	at := settingrepo.Place{Level: domain.LevelHub, ScopeID: hubID}
	return place{
		at: at, walk: []settingrepo.Place{workspace, at},
		path: service.ContainerScopes(hub), target: hubID,
	}, nil
}

// read finds the place in its own read, so that a refusal's audit entry is written outside the
// transaction that would roll it back.
func (m Modules) read(ctx context.Context, actor appshared.ActorContext, hubID shared.ID) (place, error) {
	var where place
	err := m.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := m.placeOf(ctx, actor, hubID)
		where = found
		return err
	})
	return where, err
}

// layers reads every layer a place resolves through, top-down.
func (m Modules) layers(ctx context.Context, walk []settingrepo.Place) ([]domain.Layer, error) {
	instance, plan, err := m.Settings.Above(ctx, module.Area+".")
	if err != nil {
		return nil, err
	}
	stored, err := m.Settings.At(ctx, walk, module.Area+".")
	if err != nil {
		return nil, err
	}
	return append([]domain.Layer{instance, plan}, settingrepo.LayersOf(walk, stored)...), nil
}

// On answers the modules on at a place - the workspace, or the hub when one is named - in the
// transaction the caller runs in: the gate's and the capabilities' question.
func (m Modules) On(ctx context.Context, actor appshared.ActorContext, hubID shared.ID) (module.Set, error) {
	if m.Settings == nil || m.UnitOfWork == nil || actor.TenantID.IsZero() {
		return module.Set{}, nil
	}
	var on module.Set
	err := m.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		where, err := m.placeOf(ctx, actor, hubID)
		if err != nil {
			return err
		}
		layers, err := m.layers(ctx, where.walk)
		on = module.SetOf(module.Resolve(layers...))
		return err
	})
	return on, err
}

// ListModules answers every module at the workspace or one hub with its source, its lock and what
// is left to the place below (patterns A1, A2).
type ListModules struct{ Modules Modules }

// Execute reads the modules at the place.
func (h ListModules) Execute(ctx context.Context, actor appshared.ActorContext, hubID shared.ID) ([]module.State, error) {
	where, err := h.Modules.read(ctx, actor, hubID)
	if err != nil {
		return nil, err
	}
	if err := h.Modules.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure, Alternative: service.PermissionReadConfiguration,
		Path: where.path, Action: ModulesReadAction, TokenScope: workspaceReadScope,
		TargetType: moduleTarget, TargetID: where.target,
	}); err != nil {
		return nil, err
	}
	var states []module.State
	err = h.Modules.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		layers, err := h.Modules.layers(ctx, where.walk)
		states = module.Resolve(layers...)
		return err
	})
	return states, err
}

// Switch is what a place says about one module: on, off, or left to the level above.
type Switch string

const (
	SwitchOn      Switch = "ON"
	SwitchOff     Switch = "OFF"
	SwitchInherit Switch = "INHERIT"
)

// SetModuleCommand is the input, typed.
type SetModuleCommand struct {
	Module module.Key
	Switch Switch
	// HubID narrows the switch to one hub; zero is the workspace.
	HubID shared.ID
}

// SetModule switches one module on or off for the workspace or one hub, or leaves it to the level
// above. Off hides the module's screens, fields and actions and deletes nothing (UC-INS-21
// check 4); a lock above refuses with its origin (check 6); a hub only narrows (check 3).
type SetModule struct{ Modules Modules }

// Execute writes the switch and answers every module at the place afterwards.
func (h SetModule) Execute(ctx context.Context, actor appshared.ActorContext, cmd SetModuleCommand) ([]module.State, error) {
	m := h.Modules
	if !cmd.Module.Valid() || (cmd.Switch != SwitchOn && cmd.Switch != SwitchOff && cmd.Switch != SwitchInherit) {
		return nil, shared.ErrValidation.WithDetail("module.switch_invalid").
			WithFields(shared.FieldError{Path: "/switch", Code: "module.switch_invalid"})
	}
	where, err := m.read(ctx, actor, cmd.HubID)
	if err != nil {
		return nil, err
	}
	if err := m.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure, Path: where.path, Action: ModuleSwitchedAction,
		TokenScope: workspaceWriteScope, TargetType: moduleTarget, TargetID: where.target,
	}); err != nil {
		return nil, err
	}
	var states []module.State
	err = m.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		layers, err := m.layers(ctx, where.walk)
		if err != nil {
			return err
		}
		definition := cmd.Module.Definition()
		above := domain.Resolve(definition, layers[:len(layers)-1]...)
		before := domain.Resolve(definition, layers...)
		now := m.Clock.Now()
		if cmd.Switch == SwitchInherit {
			if _, err := m.Settings.Remove(ctx, where.at, definition.Key); err != nil {
				return err
			}
		} else {
			entry := domain.Entry{Value: cmd.Switch == SwitchOn, Hold: domain.HoldSet, By: actor.AccountID}
			if err := domain.Check(definition, where.at.Level, above, entry); err != nil {
				return err
			}
			if err := m.Settings.Put(ctx, m.IDs.NewID(), where.at, definition.Key, entry, now); err != nil {
				return err
			}
		}
		if layers, err = m.layers(ctx, where.walk); err != nil {
			return err
		}
		states = module.Resolve(layers...)
		after := domain.Resolve(definition, layers...)
		return m.recordAudit(ctx, actor, where, cmd, before, after, now)
	})
	return states, err
}

func (m Modules) recordAudit(
	ctx context.Context, actor appshared.ActorContext, where place, cmd SetModuleCommand,
	before, after domain.Resolution, now time.Time,
) error {
	return m.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: now, Action: ModuleSwitchedAction,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: moduleTarget, TargetID: where.target,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(audit.Change{
			Field: string(cmd.Module.SettingKey()), Classification: audit.Open,
			From: before.Effective, To: after.Effective,
		}),
	})
}

// Descriptor is ListModules' catalogue entry.
func (h ListModules) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListModulesName,
		Summary: "Lists every module of the suite at the workspace, or at one hub, with whether it " +
			"is on, which level decided it, and which level locked it.",
		ReadOnly:   true,
		TokenScope: workspaceReadScope,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Description: "A hub of the workspace; omitted is the workspace."},
		},
		Audit: usecase.AuditDeclaration{Action: ModulesReadAction, TargetType: moduleTarget},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a read changes nothing, and a module is not an item.",
		},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hubID, err := optionalID(in, "hub_id")
			if err != nil {
				return nil, err
			}
			states, err := h.Execute(ctx, actor, hubID)
			if err != nil {
				return nil, err
			}
			return StatesOutput(states), nil
		}),
	}
}

// Descriptor is SetModule's catalogue entry.
func (h SetModule) Descriptor() usecase.Descriptor {
	modules := make([]string, 0, len(module.Keys()))
	for _, key := range module.Keys() {
		modules = append(modules, string(key))
	}
	return usecase.Descriptor{
		Name: SetModuleName,
		Summary: "Switches one module of the suite on or off for the workspace or one hub, or " +
			"leaves it to the level above. A hub only switches off what the workspace has on; a " +
			"module a plan or the installation locks is refused as setting.locked.",
		SideEffects: "Stores the switch and writes an audit entry. Switching off hides the " +
			"module and deletes none of its data.",
		TokenScope: workspaceWriteScope,
		Input: []usecase.Field{
			{Name: "module", Kind: usecase.KindString, Required: true, Enum: modules,
				Description: "The module."},
			{Name: "switch", Kind: usecase.KindString, Required: true,
				Enum:        []string{string(SwitchOn), string(SwitchOff), string(SwitchInherit)},
				Description: "ON, OFF, or INHERIT to leave it to the level above."},
			{Name: "hub_id", Kind: usecase.KindID, Description: "A hub of the workspace; omitted is the workspace."},
		},
		Audit: usecase.AuditDeclaration{
			Action: ModuleSwitchedAction, TargetType: moduleTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a module is switched for a place, not for an item; the switch is in the audit trail.",
		},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			key, err := module.Parse(in.String("module"))
			if err != nil {
				return nil, err
			}
			hubID, err := optionalID(in, "hub_id")
			if err != nil {
				return nil, err
			}
			states, err := h.Execute(ctx, actor, SetModuleCommand{
				Module: key, Switch: Switch(strings.ToUpper(in.String("switch"))), HubID: hubID,
			})
			if err != nil {
				return nil, err
			}
			return StatesOutput(states), nil
		}),
	}
}

// StatesOutput is the answer both use cases give: `modules`, one row per module.
func StatesOutput(states []module.State) usecase.Output {
	rows := make([]usecase.Output, 0, len(states))
	for _, state := range states {
		row := usecase.Output{
			"module": string(state.Module),
			"on":     state.On(),
			"source": string(state.Resolution.Source),
		}
		if state.Resolution.Locked() {
			row["locked_by"] = string(state.Resolution.LockedBy)
			if !state.Resolution.LockedByPerson.IsZero() {
				row["locked_by_person"] = state.Resolution.LockedByPerson.String()
			}
		}
		rows = append(rows, row)
	}
	return usecase.Output{"modules": rows}
}

func optionalID(in usecase.Input, field string) (shared.ID, error) {
	if in.String(field) == "" {
		return "", nil
	}
	return in.ID(field)
}

// ModuleGate refuses an operation whose module is off at the caller's workspace (UC-INS-21
// check 5): one place, on the registry every door comes through - REST, MCP, `hubctl` and a rule's
// action - with one message code, the way the plan's EntitlementGate refuses a feature.
type ModuleGate struct{ Modules Modules }

// Wrap puts the gate inside the composition root's middleware.
func (g ModuleGate) Wrap(observe usecase.Middleware) usecase.Middleware {
	return func(descriptor usecase.Descriptor, next usecase.Handler) usecase.Handler {
		handler := next
		if descriptor.Module != "" {
			key := descriptor.Module
			handler = usecase.HandlerFunc(func(
				ctx context.Context, actor appshared.ActorContext, in usecase.Input,
			) (usecase.Output, error) {
				on, err := g.Modules.On(ctx, actor, "")
				if err != nil && !errors.Is(err, shared.ErrNotFound) {
					return nil, err
				}
				if err := on.Require(key); err != nil {
					return nil, err
				}
				return next.Invoke(ctx, actor, in)
			})
		}
		if observe == nil {
			return handler
		}
		return observe(descriptor, handler)
	}
}
