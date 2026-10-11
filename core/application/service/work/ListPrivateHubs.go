// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"

	lifecyclerepo "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	ListPrivateHubsName = "ListPrivateHubs"

	// PrivateHubsReadAction is the code a refused read is recorded under.
	PrivateHubsReadAction audit.Action = "private_hubs.read"

	membersRead = "members:read"
)

// ListPrivateHubs answers the administrators what ADR-0073 §3 lets them know of the workspace's
// private hubs: that they exist, whose they are, how much they hold, and when a trashed one goes -
// no name, no description, no content.
//
// MANAGE_MEMBERS rather than READ_CONFIGURATION: the list is for running the workspace - a person
// leaving, the storage they leave behind - which is membership management, and an auditor reads
// how the workspace is configured, not the people's hubs.
type ListPrivateHubs struct {
	Hubs       repository.PrivateHubs
	Policies   lifecyclerepo.Policies
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// PrivateHubRow is one row of the answer.
type PrivateHubRow struct {
	repository.PrivateHubSummary
	// PurgeOn is the day a trashed hub goes for good, by the workspace's trash period.
	PurgeOn string
}

// Execute lists them.
func (h ListPrivateHubs) Execute(ctx context.Context, actor appshared.ActorContext) ([]PrivateHubRow, error) {
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     PrivateHubsReadAction,
		TokenScope: membersRead,
		TargetType: "tenant",
		TargetID:   actor.TenantID,
	}); err != nil {
		return nil, err
	}

	var (
		hubs  []repository.PrivateHubSummary
		grace = trashDays(lifecycle.Policy{})
	)
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if hubs, err = h.Hubs.ListPrivateHubs(ctx); err != nil {
			return err
		}
		policy, err := h.Policies.Find(ctx, lifecycle.KindTrash)
		switch {
		case errors.Is(err, shared.ErrNotFound):
			return nil
		case err != nil:
			return err
		}
		grace = trashDays(policy)
		return nil
	})
	if err != nil {
		return nil, err
	}

	rows := make([]PrivateHubRow, 0, len(hubs))
	for _, hub := range hubs {
		row := PrivateHubRow{PrivateHubSummary: hub}
		if hub.TrashedAt != nil {
			row.PurgeOn = hub.TrashedAt.UTC().AddDate(0, 0, grace).Format("2006-01-02")
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// trashDays is the trash's period: the workspace's own, or the default it starts with
// (data-retention.md §3).
func trashDays(policy lifecycle.Policy) int {
	if policy.RetainDays > 0 {
		return policy.RetainDays
	}
	for _, standard := range lifecycle.DefaultPolicies() {
		if standard.DataKind == lifecycle.KindTrash {
			return standard.RetainDays
		}
	}
	return 0
}

// Descriptor is the catalogue entry.
func (h ListPrivateHubs) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListPrivateHubsName,
		Summary: "Lists the workspace's private hubs without their names: per hub its owners, how " +
			"many collections and entries it holds, the bytes of its attachments, when it was " +
			"created and, while it is in the trash, the day it goes for good. For running the " +
			"workspace; nothing in it can be read.",
		SideEffects: "None. Reads only.",
		TokenScope:  membersRead,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: PrivateHubsReadAction, TargetType: "tenant",
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListPrivateHubs) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	rows, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	data := make([]usecase.Output, 0, len(rows))
	for _, row := range rows {
		owners := make([]any, 0, len(row.Owners))
		for _, owner := range row.Owners {
			owners = append(owners, owner.String())
		}
		out := usecase.Output{
			"id": row.ID.String(), "owners": owners,
			"collections": row.Collections, "entries": row.Entries,
			"attachment_bytes": row.AttachmentBytes,
			"created_at":       row.CreatedAt,
			"trashed_at":       timeOrNil(row.TrashedAt),
			"purge_on":         nil,
		}
		if row.PurgeOn != "" {
			out["purge_on"] = row.PurgeOn
		}
		data = append(data, out)
	}
	return usecase.Output{"data": data}, nil
}
