// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	metarepo "github.com/Jersyfi/hubtask/core/application/repository/meta"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	ListPresetsName = "ListPresets"
	presetTarget    = "preset"
)

// PresetsReadAction is the audit code of a refused read of the presets.
const PresetsReadAction audit.Action = "presets.read"

// ListPresets answers the presets a person can start from, as this installation offers them:
// Hubtask's four, the installation's, the workspace's and the person's own (UC-WRK-30 check 2),
// each with its origin.
//
// Open to any member of the workspace, like GetAccount: the workspace's are offered to everyone in
// it, and a person's own are answered to them alone. A requester is no member and is refused
// (access.Request.AnyMember) - the workspace's saved presets are the team's. What a credential
// still bounds is the token scope.
type ListPresets struct {
	// Authorizer asks whether the reader is a member of the workspace; nil refuses everybody.
	Authorizer Authorizer
	Profiles   metarepo.CapabilityProfiles
	// Presets reads the saved ones; a build without it answers Hubtask's four alone.
	Presets repository.Presets
	// Installation reads the installation's; a build without it answers none.
	Installation repository.InstallationPresets
	UnitOfWork   persistence.UnitOfWork
}

// Execute answers Hubtask's four in a stable order, then the installation's, the workspace's and
// the person's own, each by name.
func (h ListPresets) Execute(ctx context.Context, actor appshared.ActorContext) ([]domain.Preset, error) {
	if err := actor.RequireScope(containersRead); err != nil {
		return nil, err
	}
	if h.Authorizer == nil {
		return nil, shared.ErrForbidden.WithDetail("access.not_permitted").
			WithParams(map[string]string{"permission": string(service.PermissionRead)})
	}
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		AnyMember: true, Action: PresetsReadAction, TokenScope: containersRead, TargetType: presetTarget,
	}); err != nil {
		return nil, err
	}
	var bound domain.StructureBound
	var saved []domain.Preset
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		read, err := structureBound(ctx, h.Profiles)
		if err != nil {
			return err
		}
		bound = read
		if h.Installation != nil {
			installation, err := h.Installation.List(ctx)
			if err != nil {
				return err
			}
			for _, preset := range installation {
				saved = append(saved, preset.Preset)
			}
		}
		if h.Presets == nil {
			return nil
		}
		offered, err := h.Presets.ListOffered(ctx, actor.AccountID)
		saved = append(saved, offered...)
		return err
	})
	if err != nil {
		return nil, err
	}

	presets := append(domain.BuiltInPresets(), saved...)
	offered := make([]domain.Preset, 0, len(presets))
	for _, preset := range presets {
		offered = append(offered, preset.Offered(bound))
	}
	return offered, nil
}

// structureBound reads what the installation allows: the system profiles and the ones in force.
func structureBound(ctx context.Context, profiles metarepo.CapabilityProfiles) (domain.StructureBound, error) {
	inForce, err := profiles.List(ctx)
	if err != nil {
		return domain.StructureBound{}, err
	}
	system, err := profiles.ListSystem(ctx)
	if err != nil {
		return domain.StructureBound{}, err
	}
	return domain.NewStructureBound(inForce, system), nil
}

// Descriptor is the catalogue entry.
func (h ListPresets) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListPresetsName,
		Summary: "The presets a hub or collection can start from - Hubtask's List, Tasks, Project " +
			"and Flow, the installation's, the workspace's and the caller's own, each with its origin and " +
			"the values of its nine settings (levels and their names, fields, views, " +
			"what counts, completion, capture, celebration, automation, entry types) as this installation " +
			"offers them. Any member may read them; a requester is no member and is refused. " +
			"Choosing one copies its values; nothing follows a preset afterwards.",
		SideEffects: "None. Reads only.",
		TokenScope:  containersRead,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: PresetsReadAction, TargetType: presetTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListPresets) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	presets, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	rows := make([]usecase.Output, 0, len(presets))
	for _, preset := range presets {
		rows = append(rows, presetOutput(preset))
	}
	return usecase.Output{"data": rows}, nil
}
