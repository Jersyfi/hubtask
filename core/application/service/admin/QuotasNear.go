// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"

	quotaservice "github.com/Jersyfi/hubtask/core/application/service/quota"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ListWorkspacesNearQuotaName = "ListWorkspacesNearQuota"

	quotasNearReadAction audit.Action = "instance.quotas_near_read"
)

// QuotaCensus is quota.Census as this use case reads it.
type QuotaCensus interface {
	Count(ctx context.Context) (map[string]quotaservice.Band, []quotaservice.Near, error)
}

// ListWorkspacesNearQuota names the workspaces A-18 counts: the alert carries no tenant, so the
// operator asks here which workspace and which wall.
type ListWorkspacesNearQuota struct {
	Writer InstanceWriter
	Census QuotaCensus
}

// Execute reads every active workspace's standing, now.
func (h ListWorkspacesNearQuota) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]quotaservice.Near, error) {
	if err := h.Writer.authorize(ctx, actor); err != nil {
		return nil, err
	}
	_, near, err := h.Census.Count(ctx)
	return near, err
}

func (h ListWorkspacesNearQuota) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListWorkspacesNearQuotaName,
		Summary: "The workspaces at 90% or more of a counted quota, each with the quotas that put " +
			"it there: ceiling, use and ratio. Numbers and the workspace's address, never content.",
		SideEffects: "None. Reads only.",
		TokenScope:  adminTenantsScope,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: quotasNearReadAction, TargetType: instanceTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListWorkspacesNearQuota) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	near, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	rows := make([]usecase.Output, 0, len(near))
	for _, workspace := range near {
		rows = append(rows, usecase.Output{
			"tenant_id": workspace.TenantID.String(),
			"slug":      workspace.Slug,
			"quotas":    quotaservice.StandingOutputs(workspace.Quotas),
		})
	}
	return usecase.Output{"data": rows}, nil
}
