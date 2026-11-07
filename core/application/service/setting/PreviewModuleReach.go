// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package setting

import (
	"context"
	"errors"

	automationrepo "github.com/Jersyfi/hubtask/core/application/repository/automation"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/automation"
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

// Rules pages through the workspace's rules; only the switched-on ones are asked for.
type Rules interface {
	List(ctx context.Context, query automationrepo.Query) (automationrepo.Page, error)
}

// Actions says which module the use case behind an action kind belongs to.
type Actions interface {
	ByAutomationAction(kind string) (usecase.Descriptor, bool)
}

// rulePage is how many rules one read of the walk takes.
const rulePage = 200

// PreviewModuleReach answers how many people switching a module off at the workspace or one hub
// would leave without it, before the switch: everyone who reaches the place while the module is
// on there, and nobody where it is off already. Leaving it to the level above (INHERIT) counts
// the same people when the level above has it off, and nobody when the module stays on. A count,
// never who (rule 10). It also counts the switched-on rules the switch quiets: one with an action
// of the module, which is skipped while it is off, or a trigger of it, which never fires.
type PreviewModuleReach struct {
	Modules Modules
	People  People
	Rules   Rules
	Actions Actions
}

// ModuleReach is the answer.
type ModuleReach struct {
	Module module.Key
	HubID  shared.ID
	On     bool
	People int
	Rules  int
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
	if h.People == nil || h.Rules == nil || h.Actions == nil {
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
		if err != nil {
			return err
		}
		reach.Rules, err = h.quieted(ctx, key, hubID)
		return err
	})
	return reach, err
}

// quieted counts the switched-on rules that depend on the module where the switch reaches: every
// one for the workspace; for a hub, the workspace's and a person's, the hub's and its collections'.
func (h PreviewModuleReach) quieted(ctx context.Context, key module.Key, hubID shared.ID) (int, error) {
	enabled := true
	query := automationrepo.Query{Enabled: &enabled, Size: rulePage}
	hubs := map[shared.ID]shared.ID{}
	count := 0
	for {
		page, err := h.Rules.List(ctx, query)
		if err != nil {
			return 0, err
		}
		for _, rule := range page.Rules {
			if !h.dependsOn(rule, key) {
				continue
			}
			reaches, err := h.reaches(ctx, rule.Scope, hubID, hubs)
			if err != nil {
				return 0, err
			}
			if reaches {
				count++
			}
		}
		if !page.HasMore || page.NextCursor == "" {
			return count, nil
		}
		query.Cursor = page.NextCursor
	}
}

func (h PreviewModuleReach) dependsOn(rule automation.Rule, key module.Key) bool {
	if rule.Trigger.Kind.Module() == key {
		return true
	}
	for _, action := range rule.Actions {
		if descriptor, ok := h.Actions.ByAutomationAction(action.Kind); ok && descriptor.Module == key {
			return true
		}
	}
	return false
}

// reaches answers whether a rule of the scope acts at the hub; a collection's hub is read once.
func (h PreviewModuleReach) reaches(
	ctx context.Context, scope automation.Scope, hubID shared.ID, hubs map[shared.ID]shared.ID,
) (bool, error) {
	if hubID.IsZero() {
		return true, nil
	}
	switch scope.Type {
	case automation.ScopeHub:
		return scope.ID == hubID, nil
	case automation.ScopeCollection:
		parent, read := hubs[scope.ID]
		if !read {
			collection, err := h.Modules.Containers.Find(ctx, scope.ID)
			if errors.Is(err, shared.ErrNotFound) {
				// A rule of a collection gone since acts nowhere.
				return false, nil
			}
			if err != nil {
				return false, err
			}
			parent = collection.ParentID
			hubs[scope.ID] = parent
		}
		return parent == hubID, nil
	default:
		return true, nil
	}
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
			"while the module is on, and nobody where it is off already, and the switched-on rules " +
			"there that use the module and go quiet. Counts, never who.",
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
			out := usecase.Output{
				"module": string(reach.Module), "on": reach.On, "people": reach.People, "rules": reach.Rules,
			}
			if !reach.HubID.IsZero() {
				out["hub_id"] = reach.HubID.String()
			}
			return out, nil
		}),
	}
}
