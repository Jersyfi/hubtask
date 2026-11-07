// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"context"
	"strconv"

	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ReadAgentPolicyName = "ReadAgentPolicy"
	SetAgentPolicyName  = "SetAgentPolicy"

	PolicyReadAction audit.Action = "agent_policy.read"
	PolicySetAction  audit.Action = "agent_policy.set"
	// UnregisteredRefusedAction is an act through MCP refused because its caller is no registered
	// agent while the workspace takes registered agents only.
	UnregisteredRefusedAction audit.Action = "agent.unregistered_refused"

	policyTarget = "agent_policy"
	policyPrefix = "agents."
)

// registeredOnly resolves the workspace's switch; without a settings store it is off.
func (d Desk) registeredOnly(ctx context.Context, tenant shared.ID) (setting.Resolution, error) {
	definition := domain.RegisteredOnlyDefinition()
	if d.Settings == nil {
		return setting.Resolve(definition), nil
	}
	instance, planLayer, err := d.Settings.Above(ctx, policyPrefix)
	if err != nil {
		return setting.Resolution{}, err
	}
	workspace := settingrepo.Place{Level: setting.LevelWorkspace, ScopeID: tenant}
	stored, err := d.Settings.At(ctx, []settingrepo.Place{workspace}, policyPrefix)
	if err != nil {
		return setting.Resolution{}, err
	}
	layers := append([]setting.Layer{instance, planLayer}, settingrepo.LayersOf([]settingrepo.Place{workspace}, stored)...)
	return setting.Resolve(definition, layers...), nil
}

func policyOutput(r setting.Resolution) usecase.Output {
	row := usecase.Output{"value": domain.RegisteredOnlyOf(r), "source": string(r.Source)}
	if r.Locked() {
		row["locked_by"] = string(r.LockedBy)
	}
	return usecase.Output{string(domain.RegisteredOnlyKey): row}
}

// ReadAgentPolicy answers the workspace's agent policy: whether MCP takes registered agents only,
// the level that decided it and the level that locked it.
type ReadAgentPolicy struct{ Desk Desk }

func (h ReadAgentPolicy) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReadAgentPolicyName, Module: moduleOf(), Permission: service.PermissionManageMembers,
		Summary: "Answers whether the workspace takes registered agents only on MCP, the level that " +
			"decided it and the level that locked it.",
		SideEffects: "None. Reads only.",
		TokenScope:  scopeRead, ReadOnly: true,
		Audit: usecase.AuditDeclaration{Action: PolicyReadAction, TargetType: policyTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			if err := h.Desk.administer(ctx, actor, PolicyReadAction, scopeRead, policyTarget, actor.TenantID); err != nil {
				return nil, err
			}
			var r setting.Resolution
			err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				r, err = h.Desk.registeredOnly(ctx, actor.TenantID)
				return err
			})
			if err != nil {
				return nil, err
			}
			return policyOutput(r), nil
		}),
	}
}

// SetAgentPolicy switches whether the workspace takes registered agents only on MCP. The
// installation or a plan may have switched it on and locked it; the workspace only tightens.
type SetAgentPolicy struct{ Desk Desk }

func (h SetAgentPolicy) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SetAgentPolicyName, Module: moduleOf(), Permission: service.PermissionManageMembers,
		Summary: "Switches whether the workspace takes registered agents only on MCP: on, a person's " +
			"own token or a service account no agent names is refused there with " +
			"agents.registration_required. Off by default; a level above may lock it on.",
		SideEffects: "Stores the setting and writes an audit entry.",
		TokenScope:  scopeWrite,
		Input: []usecase.Field{
			{Name: "registered_only", Kind: usecase.KindBool, Required: true, Description: "Whether MCP takes registered agents only."},
		},
		Audit:    usecase.AuditDeclaration{Action: PolicySetAction, TargetType: policyTarget, Severity: audit.SeverityNotice, Required: true},
		Activity: usecase.ActivityDeclaration{Exempt: "a setting is not an item; the change is in the trail."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := notAgent(actor, "agents.agent_decides_policy_not"); err != nil {
				return nil, err
			}
			if err := h.Desk.administer(ctx, actor, PolicySetAction, scopeWrite, policyTarget, actor.TenantID); err != nil {
				return nil, err
			}
			on := in.Bool("registered_only")
			definition := domain.RegisteredOnlyDefinition()
			var r setting.Resolution
			err := h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				instance, planLayer, err := h.Desk.Settings.Above(ctx, policyPrefix)
				if err != nil {
					return err
				}
				above := setting.Resolve(definition, instance, planLayer)
				entry := setting.Entry{Value: on, Hold: setting.HoldSet, By: actor.AccountID}
				if err := setting.Check(definition, setting.LevelWorkspace, above, entry); err != nil {
					return err
				}
				place := settingrepo.Place{Level: setting.LevelWorkspace, ScopeID: actor.TenantID}
				if err := h.Desk.Settings.Put(ctx, h.Desk.IDs.NewID(), place, domain.RegisteredOnlyKey, entry, h.Desk.Clock.Now()); err != nil {
					return err
				}
				if r, err = h.Desk.registeredOnly(ctx, actor.TenantID); err != nil {
					return err
				}
				return h.Desk.record(ctx, actor, PolicySetAction, policyTarget, actor.TenantID, audit.SeverityNotice,
					open(string(domain.RegisteredOnlyKey), strconv.FormatBool(on)))
			})
			if err != nil {
				return nil, err
			}
			return policyOutput(r), nil
		}),
	}
}
