// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package setting holds the use cases of the settings catalogue (domain-model.md §2, "The settings catalogue"): reading
// what a place resolves, and writing one level's entry, with the module switches first
// (UC-INS-21).
package setting

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"time"

	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
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

	// Entity is the change log's name for a place's module switches (offline-sync.md §3.1): one
	// record for the workspace and one for each hub, identified by the place.
	Entity = "setting"

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
	// Epochs is the workspace's synchronisation epoch, advanced when a module whose records leave
	// a device's copy turns on or off at the workspace (offline-sync.md §8). Nil advances nothing.
	Epochs Epochs
	// Hold holds the workspace's configuration, the modules among it, for a switch's transaction:
	// two switches at once otherwise each judge what a module needs on what the other has not
	// committed yet. Nil holds nothing.
	Hold Holder
	// Changes and HLC tell the other devices of a switch, so that a module switched off leaves
	// them without a reload (UC-INS-21 check 12). A nil Changes records nothing.
	Changes changelog.ChangeLog
	HLC     clock.HLCSource
}

// Holder holds the workspace's configuration until the transaction ends.
type Holder interface {
	Hold(ctx context.Context) error
}

// Epochs advances the workspace's synchronisation epoch inside the caller's transaction.
type Epochs interface {
	Advance(ctx context.Context) (int64, error)
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
	// HubIDs are the existing hubs a workspace's switch on switches the module on in as well.
	HubIDs []shared.ID
}

// SetModule switches one module on or off for the workspace or one hub, or leaves it to the level
// above. Off hides the module's screens, fields and actions and deletes nothing (UC-INS-21
// check 4); a lock above refuses with its origin (check 6); a hub decides inside what the
// workspace has on (check 3). A workspace's switch changes no hub that exists but the chosen
// ones, and off then on gives each hub the state it had (check 13): whenever what the workspace
// resolves to turns, every hub storing no value of its own gets the state it had as its own.
type SetModule struct{ Modules Modules }

// Execute writes the switch and answers every module at the place afterwards.
func (h SetModule) Execute(ctx context.Context, actor appshared.ActorContext, cmd SetModuleCommand) ([]module.State, error) {
	m := h.Modules
	if !cmd.Module.Valid() || (cmd.Switch != SwitchOn && cmd.Switch != SwitchOff && cmd.Switch != SwitchInherit) {
		return nil, shared.ErrValidation.WithDetail("module.switch_invalid").
			WithFields(shared.FieldError{Path: "/switch", Code: "module.switch_invalid"})
	}
	if len(cmd.HubIDs) > 0 && (cmd.Switch != SwitchOn || !cmd.HubID.IsZero()) {
		return nil, shared.ErrValidation.WithDetail("module.hubs_with_on_only").
			WithFields(shared.FieldError{Path: "/hub_ids", Code: "module.hubs_with_on_only"})
	}
	where, err := m.authorised(ctx, actor, cmd.HubID)
	if err != nil {
		return nil, err
	}
	chosen, err := m.chosen(ctx, actor, cmd.HubIDs)
	if err != nil {
		return nil, err
	}
	var states []module.State
	err = m.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if m.Hold != nil {
			if err := m.Hold.Hold(ctx); err != nil {
				return err
			}
		}
		layers, err := m.layers(ctx, where.walk)
		if err != nil {
			return err
		}
		now := m.Clock.Now()
		var written []shared.ID
		if cmd.Switch == SwitchOn {
			// What the module needs first, so a lock on it refuses before anything is written.
			for _, needed := range cmd.Module.Needs() {
				hubs, err := m.switchOnNeeded(ctx, actor, where, layers, needed, chosen, now)
				if err != nil {
					return err
				}
				written = append(written, hubs...)
			}
			if layers, err = m.layers(ctx, where.walk); err != nil {
				return err
			}
		}
		definition := cmd.Module.Definition()
		above := domain.Resolve(definition, layers[:len(layers)-1]...)
		before := domain.Resolve(definition, layers...)
		if cmd.Switch == SwitchInherit {
			if err := refuseUnmet(layers, definition.Key, cmd.Switch); err != nil {
				return err
			}
			if _, err := m.Settings.Remove(ctx, where.at, definition.Key); err != nil {
				return err
			}
		} else {
			entry := domain.Entry{Value: cmd.Switch == SwitchOn, Hold: domain.HoldSet, By: actor.AccountID}
			if err := domain.Check(definition, where.at.Level, above, entry); err != nil {
				return err
			}
			if cmd.Switch == SwitchOff {
				if err := refuseUnmet(layers, definition.Key, cmd.Switch); err != nil {
					return err
				}
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
		pinned, err := m.keepHubs(ctx, where, cmd.Module, before, after, now)
		if err != nil {
			return err
		}
		if err := m.switchOnChosen(ctx, actor, chosen, cmd.Module, after, now); err != nil {
			return err
		}
		if err := m.resynchronise(ctx, cmd, before, after); err != nil {
			return err
		}
		if err := m.recordAudit(ctx, actor, where, cmd, before, after, reach{pinned: pinned, chosen: chosen}, now); err != nil {
			return err
		}
		if err := m.announce(ctx, actor, where.target, where.at, states); err != nil {
			return err
		}
		return m.announceHubs(ctx, actor, append(written, pinned...), chosen)
	})
	return states, err
}

// authorised finds the place and asks whether the actor may shape it, outside the transaction a
// refusal's audit entry would be rolled back with.
func (m Modules) authorised(ctx context.Context, actor appshared.ActorContext, hubID shared.ID) (place, error) {
	where, err := m.read(ctx, actor, hubID)
	if err != nil {
		return place{}, err
	}
	return where, m.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure, Path: where.path, Action: ModuleSwitchedAction,
		TokenScope: workspaceWriteScope, TargetType: moduleTarget, TargetID: where.target,
	})
}

// chosen answers the hubs a workspace's switch names, each once and each one the actor may shape:
// one that is not refuses the whole switch (rule 2).
func (m Modules) chosen(ctx context.Context, actor appshared.ActorContext, hubs []shared.ID) ([]shared.ID, error) {
	chosen := make([]shared.ID, 0, len(hubs))
	for _, hub := range hubs {
		if hub.IsZero() {
			return nil, shared.ErrValidation.WithDetail("usecase.field_required").
				WithFields(shared.FieldError{Path: "/hub_ids", Code: "usecase.field_required"})
		}
		if slices.Contains(chosen, hub) {
			continue
		}
		if _, err := m.authorised(ctx, actor, hub); err != nil {
			return nil, err
		}
		chosen = append(chosen, hub)
	}
	return chosen, nil
}

// reach is what a workspace's switch did to the hubs, for the trail: those given their state as
// their own, and those chosen.
type reach struct {
	pinned []shared.ID
	chosen []shared.ID
}

// keepHubs writes, when the workspace's switch turned what the workspace resolves to, the state
// each hub had as the hub's own value where it stores none, and answers the hubs written: off
// before the module comes on, so no existing hub changes; on before it goes off, so that on again
// gives each hub the state it had (UC-INS-21 check 13).
func (m Modules) keepHubs(
	ctx context.Context, where place, key module.Key, before, after domain.Resolution, now time.Time,
) ([]shared.ID, error) {
	was, _ := before.Effective.(bool)
	is, _ := after.Effective.(bool)
	if where.at.Level != domain.LevelWorkspace || was == is {
		return nil, nil
	}
	pin := settingrepo.HubPin{Key: key.SettingKey(), On: was}
	if !was {
		// A hub that stores on for a module needing this one keeps this one on with it, or that
		// module would come back without what it needs.
		for _, other := range module.Keys() {
			if slices.Contains(other.Needs(), key) {
				pin.OnWhere = append(pin.OnWhere, other.SettingKey())
			}
		}
	}
	// Not through domain.Check, which refuses a hub's on under a workspace's off: the value is the
	// hub's state for when the workspace has the module on again, and resolves to off until then.
	return m.Settings.PinHubs(ctx, pin, now)
}

// switchOnChosen switches the module on in each chosen hub, under what the workspace now resolves
// to.
func (m Modules) switchOnChosen(
	ctx context.Context, actor appshared.ActorContext, chosen []shared.ID, key module.Key,
	workspace domain.Resolution, now time.Time,
) error {
	if len(chosen) == 0 {
		return nil
	}
	definition := key.Definition()
	entry := domain.Entry{Value: true, Hold: domain.HoldSet, By: actor.AccountID}
	if err := domain.Check(definition, domain.LevelHub, workspace, entry); err != nil {
		return err
	}
	for _, hub := range chosen {
		at := settingrepo.Place{Level: domain.LevelHub, ScopeID: hub}
		if err := m.Settings.Put(ctx, m.IDs.NewID(), at, definition.Key, entry, now); err != nil {
			return err
		}
	}
	return nil
}

// announce records a place's switches as they now stand: filed under the hub for a hub's, under
// no container for the workspace's, which every member receives.
func (m Modules) announce(
	ctx context.Context, actor appshared.ActorContext, target shared.ID, at settingrepo.Place, states []module.State,
) error {
	return m.record(ctx, actor, target, at, SwitchesRecord(at, states))
}

func (m Modules) record(
	ctx context.Context, actor appshared.ActorContext, target shared.ID, at settingrepo.Place, payload map[string]any,
) error {
	if m.Changes == nil {
		return nil
	}
	change := changelog.Change{
		TenantID: actor.TenantID, Entity: Entity, EntityID: target, Op: changelog.Upsert,
		ActorID: actor.AccountID, HLC: m.HLC.Next(), Payload: payload,
	}
	if at.Level == domain.LevelHub {
		change.ContainerID = target
	}
	return m.Changes.Record(ctx, change)
}

// announceHubs records the switches of each hub a workspace's switch wrote a value into, so that
// the record a device holds of a hub is the hub's own decision from then on.
func (m Modules) announceHubs(ctx context.Context, actor appshared.ActorContext, written ...[]shared.ID) error {
	if m.Changes == nil {
		return nil
	}
	hubs := slices.Concat(written...)
	slices.Sort(hubs)
	hubs = slices.Compact(hubs)
	if len(hubs) == 0 {
		return nil
	}
	records, err := m.Records(ctx, actor, hubs)
	if err != nil {
		return err
	}
	for _, hub := range hubs {
		at := settingrepo.Place{Level: domain.LevelHub, ScopeID: hub}
		if err := m.record(ctx, actor, hub, at, records[hub]); err != nil {
			return err
		}
	}
	return nil
}

// SwitchesRecord is a place's module switches as a device holds them: each module and whether it
// is on there - what the capabilities and ListContainerModules answer whoever reads the place.
// Keys and booleans only: which level decided or locked a module is ListModules' to answer, to
// whoever may shape the place.
func SwitchesRecord(at settingrepo.Place, states []module.State) map[string]any {
	rows := make([]any, 0, len(states))
	for _, state := range states {
		rows = append(rows, map[string]any{"module": string(state.Module), "on": state.On()})
	}
	out := map[string]any{"hub_id": nil, "modules": rows}
	if at.Level == domain.LevelHub {
		out["hub_id"] = at.ScopeID.String()
	}
	return out
}

// Records answers the switches' record of the workspace and of each hub named, keyed by the
// place, inside the caller's transaction: what a device starting from nothing is handed. Who
// receives which is the reader's to decide.
func (m Modules) Records(ctx context.Context, actor appshared.ActorContext, hubs []shared.ID) (map[shared.ID]map[string]any, error) {
	workspace := settingrepo.Place{Level: domain.LevelWorkspace, ScopeID: actor.TenantID}
	places := make([]settingrepo.Place, 0, len(hubs)+1)
	places = append(places, workspace)
	for _, hub := range hubs {
		places = append(places, settingrepo.Place{Level: domain.LevelHub, ScopeID: hub})
	}
	instance, plan, err := m.Settings.Above(ctx, module.Area+".")
	if err != nil {
		return nil, err
	}
	stored, err := m.Settings.At(ctx, places, module.Area+".")
	if err != nil {
		return nil, err
	}
	layers := settingrepo.LayersOf(places, stored)
	records := make(map[shared.ID]map[string]any, len(places))
	records[actor.TenantID] = SwitchesRecord(workspace, module.Resolve(instance, plan, layers[0]))
	for i, hub := range hubs {
		records[hub] = SwitchesRecord(places[i+1], module.Resolve(instance, plan, layers[0], layers[i+1]))
	}
	return records, nil
}

// refuseUnmet refuses a switch off, or back to the level above, that would leave a module on
// without one it needs (a desk without its workflows), before anything is written.
func refuseUnmet(layers []domain.Layer, key domain.Key, to Switch) error {
	prospect := slices.Clone(layers)
	last := prospect[len(prospect)-1]
	entries := maps.Clone(last.Entries)
	if entries == nil {
		entries = map[domain.Key]domain.Entry{}
	}
	if to == SwitchInherit {
		delete(entries, key)
	} else {
		entries[key] = domain.Entry{Value: false, Hold: domain.HoldSet}
	}
	prospect[len(prospect)-1] = domain.Layer{Level: last.Level, Entries: entries}
	on, needed, unmet := module.Unmet(module.Resolve(prospect...))
	if !unmet {
		return nil
	}
	return shared.ErrConflict.WithDetail("module.needed").
		WithFields(shared.FieldError{Path: "/switch", Code: "module.needed"}).
		WithParams(map[string]string{"module": string(needed), "needed_by": string(on)})
}

// switchOnNeeded switches on at the place a module another one needs, where it is not on already,
// and in each chosen hub, with its own audit entry for either; it answers the hubs it wrote a kept
// state into.
func (m Modules) switchOnNeeded(
	ctx context.Context, actor appshared.ActorContext, where place, layers []domain.Layer, needed module.Key,
	chosen []shared.ID, now time.Time,
) ([]shared.ID, error) {
	definition := needed.Definition()
	before := domain.Resolve(definition, layers...)
	if on, _ := before.Effective.(bool); on {
		if len(chosen) == 0 {
			return nil, nil
		}
		if err := m.switchOnChosen(ctx, actor, chosen, needed, before, now); err != nil {
			return nil, err
		}
		// The place had it on, the chosen hubs gain it: the trail names it with them (check 8).
		cmd := SetModuleCommand{Module: needed, Switch: SwitchOn}
		return nil, m.recordAudit(ctx, actor, where, cmd, before, before, reach{chosen: chosen}, now)
	}
	entry := domain.Entry{Value: true, Hold: domain.HoldSet, By: actor.AccountID}
	if err := domain.Check(definition, where.at.Level, domain.Resolve(definition, layers[:len(layers)-1]...), entry); err != nil {
		return nil, err
	}
	if err := m.Settings.Put(ctx, m.IDs.NewID(), where.at, definition.Key, entry, now); err != nil {
		return nil, err
	}
	stored, err := m.layers(ctx, where.walk)
	if err != nil {
		return nil, err
	}
	after := domain.Resolve(definition, stored...)
	pinned, err := m.keepHubs(ctx, where, needed, before, after, now)
	if err != nil {
		return nil, err
	}
	if err := m.switchOnChosen(ctx, actor, chosen, needed, after, now); err != nil {
		return nil, err
	}
	cmd := SetModuleCommand{Module: needed, Switch: SwitchOn}
	return pinned, m.recordAudit(ctx, actor, where, cmd, before, after, reach{pinned: pinned, chosen: chosen}, now)
}

// resynchronise advances the epoch when the workspace's switch of a module the replica narrows
// changes what it resolves to: every device walks again, and holds the module's records exactly
// while it is on - a delta could not hand back what the stream withheld while it was off.
func (m Modules) resynchronise(ctx context.Context, cmd SetModuleCommand, before, after domain.Resolution) error {
	if m.Epochs == nil || !cmd.HubID.IsZero() || !cmd.Module.NarrowsReplica() {
		return nil
	}
	was, _ := before.Effective.(bool)
	is, _ := after.Effective.(bool)
	if was == is {
		return nil
	}
	_, err := m.Epochs.Advance(ctx)
	return err
}

func (m Modules) recordAudit(
	ctx context.Context, actor appshared.ActorContext, where place, cmd SetModuleCommand,
	before, after domain.Resolution, hubs reach, now time.Time,
) error {
	changes := []audit.Change{{
		Field: string(cmd.Module.SettingKey()), Classification: audit.Open,
		From: before.Effective, To: after.Effective,
	}}
	// Counts and identifiers only (rule 10). A chosen hub was written twice - its state kept, then
	// switched on - and counts as chosen.
	kept := 0
	for _, hub := range hubs.pinned {
		if !slices.Contains(hubs.chosen, hub) {
			kept++
		}
	}
	if kept > 0 {
		changes = append(changes, audit.Change{Field: "hubs_kept", Classification: audit.Open, To: kept})
	}
	if len(hubs.chosen) > 0 {
		ids := make([]string, 0, len(hubs.chosen))
		for _, hub := range hubs.chosen {
			ids = append(ids, hub.String())
		}
		changes = append(changes, audit.Change{Field: "hub_ids", Classification: audit.Open, To: ids})
	}
	return m.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: now, Action: ModuleSwitchedAction,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: moduleTarget, TargetID: where.target,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}

// Descriptor is ListModules' catalogue entry.
func (h ListModules) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ListModulesName,
		Permission: service.PermissionStructure,
		Summary: "Lists every module of the suite at the workspace, or at one hub, with whether it " +
			"is on, which level decided it, and which level locked it.",
		ReadOnly:   true,
		TokenScope: workspaceReadScope,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Description: "A hub of the workspace; omitted is the workspace."},
		},
		Audit: usecase.AuditDeclaration{Action: ModulesReadAction, TargetType: moduleTarget, Severity: audit.SeverityInfo},
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
		Name:       SetModuleName,
		Permission: service.PermissionStructure,
		Summary: "Switches one module of the suite on or off for the workspace or one hub, or " +
			"leaves it to the level above. On at the workspace allows the module and is what a hub " +
			"made afterwards starts with; it changes no existing hub but those in hub_ids. Off at " +
			"the workspace takes it from every hub, and on again gives each hub the state it had. " +
			"A hub never switches on what the workspace has off; a module a plan or the " +
			"installation locks is refused as setting.locked.",
		SideEffects: "Stores the switch and writes an audit entry. Where the workspace's switch " +
			"turns the module on or off, stores each hub's state as the hub's own value where it " +
			"has none. Switching off hides the module and deletes none of its data.",
		TokenScope: workspaceWriteScope,
		Input: []usecase.Field{
			{Name: "module", Kind: usecase.KindString, Required: true, Enum: modules,
				Description: "The module."},
			{Name: "switch", Kind: usecase.KindString, Required: true,
				Enum:        []string{string(SwitchOn), string(SwitchOff), string(SwitchInherit)},
				Description: "ON, OFF, or INHERIT to leave it to the level above."},
			{Name: "hub_id", Kind: usecase.KindID, Description: "A hub of the workspace; omitted is the workspace."},
			{Name: "hub_ids", Kind: usecase.KindIDList,
				Description: "With switch ON at the workspace: the existing hubs to switch the module on in as well."},
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
			hubIDs, err := in.IDList("hub_ids")
			if err != nil {
				return nil, err
			}
			states, err := h.Execute(ctx, actor, SetModuleCommand{
				Module: key, Switch: Switch(strings.ToUpper(in.String("switch"))), HubID: hubID, HubIDs: hubIDs,
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

// ModuleGate refuses an operation whose module is off where it acts (UC-INS-21 checks 3 and 5):
// one place, on the registry every door comes through - REST, MCP, `hubctl` and a rule's action -
// with one message code, the way the plan's EntitlementGate refuses a feature.
//
// Where it acts is the hub the operation's input names, directly or through a collection or an
// entry in it (placeFields); an operation that names none is judged at the workspace. A place
// that cannot be found is judged at the workspace too, and the handler answers for it - the gate
// is no second way to learn whether something exists.
type ModuleGate struct {
	Modules Modules
	// Items finds the collection an entry is in. Nil judges an operation that names only an entry
	// at the workspace.
	Items GateItems
	// Objects finds the hub of a module's own object by the input field naming it (a page, a
	// board), read after the places: an operation that names only its object is judged at that
	// object's hub, not the workspace.
	Objects map[string]GateObject
}

// GateObject answers the hub an object is in.
type GateObject func(ctx context.Context, id shared.ID) (shared.ID, error)

// GateItems is the one read of the item store the gate needs.
type GateItems interface {
	Find(ctx context.Context, id shared.ID) (work.WorkItem, error)
}

// placeFields are the input fields that locate an operation's place, in the order they are read:
// a hub, a container (a collection, or a plan that is a hub or a collection), an entry.
var placeFields = []struct {
	name string
	kind string
}{
	{"hub_id", "container"}, {"collection_id", "container"}, {"container_id", "container"},
	{"plan_id", "container"}, {"item_id", "item"}, {"entry_id", "item"},
}

// hubOf answers the hub an operation's input locates, or empty for the workspace.
func (g ModuleGate) hubOf(ctx context.Context, descriptor usecase.Descriptor, in usecase.Input) shared.ID {
	declared := map[string]bool{}
	for _, field := range descriptor.Input {
		declared[field.Name] = true
	}
	for _, field := range placeFields {
		if !declared[field.name] || in.String(field.name) == "" {
			continue
		}
		id, err := in.ID(field.name)
		if err != nil {
			return ""
		}
		if field.kind == "item" {
			if g.Items == nil {
				return ""
			}
			item, err := g.Items.Find(ctx, id)
			if err != nil {
				return ""
			}
			id = item.CollectionID
		}
		container, err := g.Modules.Containers.Find(ctx, id)
		if err != nil {
			return ""
		}
		if container.Type == work.ContainerHub {
			return container.ID
		}
		return container.ParentID
	}
	for _, field := range descriptor.Input {
		find, ok := g.Objects[field.Name]
		if !ok || in.String(field.Name) == "" {
			continue
		}
		id, err := in.ID(field.Name)
		if err != nil {
			return ""
		}
		hubID, err := find(ctx, id)
		if err != nil {
			return ""
		}
		return hubID
	}
	return ""
}

// answer reads the place the operation acts at and the modules on there in one transaction: On
// joins it, so a gated call costs one pool acquisition before the handler's own, not two.
func (g ModuleGate) answer(
	ctx context.Context, actor appshared.ActorContext, descriptor usecase.Descriptor, in usecase.Input,
) (module.Set, error) {
	if g.Modules.UnitOfWork == nil || actor.TenantID.IsZero() {
		return g.Modules.On(ctx, actor, "")
	}
	var on module.Set
	err := g.Modules.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var hubID shared.ID
		if g.Modules.Containers != nil {
			hubID = g.hubOf(ctx, descriptor, in)
		}
		var err error
		on, err = g.Modules.On(ctx, actor, hubID)
		return err
	})
	return on, err
}

// Wrap puts the gate inside the composition root's middleware.
func (g ModuleGate) Wrap(observe usecase.Middleware) usecase.Middleware {
	return func(descriptor usecase.Descriptor, next usecase.Handler) usecase.Handler {
		handler := next
		if descriptor.Module != "" {
			key := descriptor.Module
			handler = usecase.HandlerFunc(func(
				ctx context.Context, actor appshared.ActorContext, in usecase.Input,
			) (usecase.Output, error) {
				on, err := g.answer(ctx, actor, descriptor, in)
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
