// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package setting

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	domain "github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	PreviewModuleReachName = "PreviewModuleReach"

	// ModuleReachPreviewedAction is the code a refused preview is recorded under.
	ModuleReachPreviewedAction audit.Action = "module.reach_previewed"
)

// People counts who holds a role at a place: in the workspace, or within one hub when one is
// named (at it, at a collection or entry in it, or at the workspace unless the hub is private).
type People interface {
	PeopleReaching(ctx context.Context, hubID shared.ID) (int, error)
}

// PreviewModuleReach answers how many people switching a module off at the workspace or one hub
// would leave without it, before the switch: everyone who reaches the place while the module is
// on there, and nobody where it is off already. Leaving it to the level above (INHERIT) counts
// the same people when the level above has it off, and nobody when the module stays on. A count,
// never who (rule 10).
type PreviewModuleReach struct {
	Modules Modules
	People  People
}

// ModuleReach is the answer.
type ModuleReach struct {
	Module module.Key
	HubID  shared.ID
	On     bool
	People int
}

// Execute counts in a read-only transaction, asked as the switch itself is.
func (h PreviewModuleReach) Execute(
	ctx context.Context, actor appshared.ActorContext, key module.Key, hubID shared.ID, to Switch,
) (ModuleReach, error) {
	m := h.Modules
	if !key.Valid() {
		return ModuleReach{}, shared.ErrValidation.WithDetail("module.switch_invalid").
			WithFields(shared.FieldError{Path: "/module", Code: "module.switch_invalid"})
	}
	if to != SwitchOff && to != SwitchInherit {
		return ModuleReach{}, shared.ErrValidation.WithDetail("module.switch_invalid").
			WithFields(shared.FieldError{Path: "/switch", Code: "module.switch_invalid"})
	}
	where, err := m.read(ctx, actor, hubID)
	if err != nil {
		return ModuleReach{}, err
	}
	if err := m.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure, Path: where.path, Action: ModuleReachPreviewedAction,
		TokenScope: workspaceWriteScope, TargetType: moduleTarget, TargetID: where.target,
	}); err != nil {
		return ModuleReach{}, err
	}
	if h.People == nil {
		return ModuleReach{}, shared.ErrInternal.WithDetail("module.reach_unwired")
	}
	reach := ModuleReach{Module: key, HubID: hubID}
	err = m.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		layers, err := m.layers(ctx, where.walk)
		if err != nil {
			return err
		}
		resolved := domain.Resolve(key.Definition(), layers...)
		reach.On, _ = resolved.Effective.(bool)
		if !reach.On {
			return nil
		}
		if to == SwitchInherit {
			// The place's own layer is the last one; without it the level above decides.
			above := domain.Resolve(key.Definition(), layers[:len(layers)-1]...)
			if stays, _ := above.Effective.(bool); stays {
				return nil
			}
		}
		reach.People, err = h.People.PeopleReaching(ctx, hubID)
		return err
	})
	return reach, err
}

// Descriptor is the catalogue entry.
func (h PreviewModuleReach) Descriptor() usecase.Descriptor {
	modules := make([]string, 0, len(module.Keys()))
	for _, key := range module.Keys() {
		modules = append(modules, string(key))
	}
	return usecase.Descriptor{
		Name:       PreviewModuleReachName,
		Permission: service.PermissionStructure,
		Summary: "Answers how many people switching a module off at the workspace or one hub would " +
			"leave without it, before the switch: everyone who holds a role there - for a hub, at " +
			"it, a collection or an entry in it, or at the workspace unless the hub is private - " +
			"while the module is on, and nobody where it is off already. A count, never who.",
		SideEffects: "None.",
		ReadOnly:    true,
		TokenScope:  workspaceWriteScope,
		Input: []usecase.Field{
			{Name: "module", Kind: usecase.KindString, Required: true, Enum: modules,
				Description: "The module."},
			{Name: "hub_id", Kind: usecase.KindID, Description: "A hub of the workspace; omitted is the workspace."},
			{Name: "switch", Kind: usecase.KindString, Enum: []string{string(SwitchOff), string(SwitchInherit)},
				Description: "OFF, the default, or INHERIT to leave the module to the level above."},
		},
		Audit: usecase.AuditDeclaration{
			Action: ModuleReachPreviewedAction, TargetType: moduleTarget, Severity: audit.SeverityInfo, Required: false,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "a preview writes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			key, err := module.Parse(in.String("module"))
			if err != nil {
				return nil, err
			}
			hubID, err := optionalID(in, "hub_id")
			if err != nil {
				return nil, err
			}
			to := SwitchOff
			if raw := in.String("switch"); raw != "" {
				to = Switch(raw)
			}
			reach, err := h.Execute(ctx, actor, key, hubID, to)
			if err != nil {
				return nil, err
			}
			out := usecase.Output{"module": string(reach.Module), "on": reach.On, "people": reach.People}
			if !reach.HubID.IsZero() {
				out["hub_id"] = reach.HubID.String()
			}
			return out, nil
		}),
	}
}
