// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package setting

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ListContainerModulesName = "ListContainerModules"

	containersReadScope = "containers:read"
	containerTarget     = "container"
)

// ListContainerModules answers the modules on at a hub, or at the hub a collection is in, to
// whoever may read it (UC-INS-21 check 3): the set a hub's and a collection's screens are drawn
// for, where the capabilities' `modules_on` is the workspace's. ListModules answers the same
// resolution with its sources, and only to whoever may shape the workspace.
type ListContainerModules struct{ Modules Modules }

// Execute reads the place, asks for READ on it and resolves its hub's modules.
func (h ListContainerModules) Execute(ctx context.Context, actor appshared.ActorContext, containerID shared.ID) (module.Set, error) {
	var container work.Container
	err := h.Modules.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Modules.Containers.Find(ctx, containerID)
		container = found
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := h.Modules.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead, Path: service.ContainerScopes(container),
		Action: ModulesReadAction, TokenScope: containersReadScope,
		TargetType: containerTarget, TargetID: container.ID,
	}); err != nil {
		return nil, err
	}
	hubID := container.ID
	if container.Type != work.ContainerHub {
		hubID = container.ParentID
	}
	return h.Modules.On(ctx, actor, hubID)
}

// Descriptor is ListContainerModules' catalogue entry.
func (h ListContainerModules) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ListContainerModulesName,
		Permission: service.PermissionRead,
		Summary: "Lists the modules of the suite on at a hub, or at the hub a collection is in: what " +
			"its screens, menus and fields are drawn for. A hub narrows the workspace.",
		SideEffects: "None. Reads only.",
		ReadOnly:    true,
		TokenScope:  containersReadScope,
		Input: []usecase.Field{
			{Name: "container_id", Kind: usecase.KindID, Required: true, Description: "The hub or collection."},
		},
		Audit: usecase.AuditDeclaration{Action: ModulesReadAction, TargetType: containerTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a read changes nothing, and a module is not an item.",
		},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("container_id")
			if err != nil {
				return nil, err
			}
			on, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			keys := make([]string, 0, len(on))
			for _, key := range on {
				keys = append(keys, string(key))
			}
			return usecase.Output{"modules_on": keys}, nil
		}),
	}
}
