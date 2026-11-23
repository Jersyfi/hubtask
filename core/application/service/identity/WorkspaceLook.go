// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const ReadWorkspaceLookName = "ReadWorkspaceLook"

// ReadWorkspaceLook answers the workspace's name and look to any member (UC-INS-22 checks 5 and
// 9, ADR-0112): a role on one hub is enough, as for the names behind identifiers, while the
// workspace's set-up (ReadWorkspace) needs READ at the workspace. Nothing beyond the name, the
// choice and the picture identifiers leaves, and where the workspace may not show its own look
// the answer is the product's, so a member learns nothing a header would not show them.
type ReadWorkspaceLook struct{ Writer WorkspaceWriter }

// WorkspaceLook is what a member sees of the workspace.
type WorkspaceLook struct {
	Name     string
	Branding domain.Branding
}

// Execute reads it.
func (h ReadWorkspaceLook) Execute(ctx context.Context, actor appshared.ActorContext) (WorkspaceLook, error) {
	w := h.Writer
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		AnyMember:  true,
		Action:     WorkspaceReadAction,
		TokenScope: workspaceRead,
		TargetType: workspaceTarget,
		TargetID:   actor.TenantID,
	}); err != nil {
		return WorkspaceLook{}, err
	}
	var found domain.Workspace
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		read, err := w.Workspaces.Find(ctx)
		found = read
		return err
	})
	if err != nil {
		return WorkspaceLook{}, err
	}
	look := WorkspaceLook{Name: found.DisplayName}
	if ownBrandingAllowed(w.resolvedPolicy(ctx, actor.TenantID)) {
		look.Branding = found.Branding
	}
	return look, nil
}

// Descriptor is the catalogue entry.
func (h ReadWorkspaceLook) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReadWorkspaceLookName,
		Summary: "The workspace's name and look as every member sees them: its display name, what " +
			"the header shows and the identifiers of the pictures it may draw - the product's name and " +
			"none where the workspace may not show its own. Any member reads it, a role on one hub is " +
			"enough.",
		SideEffects: "None. Reads only.",
		TokenScope:  workspaceRead,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: WorkspaceReadAction, TargetType: workspaceTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(func(
			ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
		) (usecase.Output, error) {
			look, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			out := brandingOutput(look.Branding, true)
			delete(out, "allowed")
			out["display_name"] = look.Name
			return out, nil
		}),
	}
}
