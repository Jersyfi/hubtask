// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	CancelTenantDeletionName = "CancelTenantDeletion"

	// TenantDeletionCancelledAction is written into the tenant's own trail: its members are
	// entitled to see that their workspace was to be deleted and was not (audit.md §6).
	TenantDeletionCancelledAction audit.Action = "tenant.deletion_cancelled"

	journalDeletionCancelled = "tenant.deletion_cancelled"
)

// PausedAutomations switches back on the rules a deletion request switched off, inside the
// cancel's transaction, and reports how many. Implemented by the automation package, which owns
// how a schedule's next moment is worked out.
type PausedAutomations interface {
	Resume(ctx context.Context, tenantID shared.ID, now time.Time) (int, error)
}

// CancelTenantDeletion is §5's edge back from PENDING_DELETION (UC-INS-08 check 5): the workspace
// returns to the status it was requested from - SUSPENDED where none was kept, so nobody regains
// access the operator did not choose - the deadline is cleared, and the rules the request switched
// off are switched back on.
//
// No step-up and no typed name: it removes nothing and returns access, as ResumeTenant does. No
// hold check either: a legal hold keeps a pending workspace pending, and this is the way out of
// that state (data-protection.md §5) - the holds stay as they are. What it may not meet is a purge
// that has begun; the grace job's mark in the row refuses it from then on.
type CancelTenantDeletion struct {
	Tenants     adminrepo.Tenants
	Automations PausedAutomations
	Journal     adminrepo.Journal
	Audit       audit.Sink
	UnitOfWork  persistence.UnitOfWork
	Clock       clock.Clock
	IDs         clock.IDGenerator
}

// Execute calls the deletion off and answers the status the workspace is in afterwards.
func (h CancelTenantDeletion) Execute(
	ctx context.Context, actor appshared.ActorContext, tenantID shared.ID,
) (domain.TenantStatus, error) {
	if err := actor.RequireScope(adminTenantsScope); err != nil {
		return "", err
	}
	if tenantID.IsZero() {
		return "", shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	}
	if h.Automations == nil {
		// Refused rather than half done: a cancel that could not switch the rules back on would
		// return a workspace that is not the one the deletion was requested for.
		return "", shared.ErrInternal.WithDetail("admin.automations_not_wired")
	}

	var status domain.TenantStatus
	scope := persistence.Scope{TenantID: tenantID, ActorID: actor.AccountID}
	err := h.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		record, err := h.find(ctx)
		if err != nil {
			return err
		}
		status = record.Status
		if err := refuseUnderway(record); err != nil {
			return err
		}
		if record.Status != domain.TenantPendingDeletion {
			// Idempotent: there is no deletion to call off, which is the state the operator wanted.
			return nil
		}

		now := h.Clock.Now()
		back, cancelled, err := h.Tenants.CancelDeletion(ctx, now)
		if err != nil {
			return err
		}
		if !cancelled {
			// Something wrote the row between the read and the statement - the grace job's mark,
			// or another operator's cancel. Read again which, and answer for it.
			again, err := h.find(ctx)
			if err != nil {
				return err
			}
			status = again.Status
			return refuseUnderway(again)
		}
		status = back

		resumed, err := h.Automations.Resume(ctx, tenantID, now)
		if err != nil {
			return err
		}

		if err := h.Audit.Append(ctx, audit.Entry{
			TenantID: tenantID, OccurredAt: now, Action: TenantDeletionCancelledAction,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: tenantTarget, TargetID: tenantID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(
				audit.Change{Field: "status", Classification: audit.Open,
					From: string(domain.TenantPendingDeletion), To: string(back)},
				audit.Change{Field: "purge_after", Classification: audit.Open,
					From: record.PurgeAfter.Format(time.RFC3339)},
			),
		}); err != nil {
			return err
		}
		return h.Journal.Record(ctx, adminrepo.InstanceEvent{
			ID: h.IDs.NewID(), OccurredAt: now, Action: journalDeletionCancelled,
			TenantID: tenantID, TenantSlug: record.Slug, ActorLabel: actor.AccountName,
			Details: map[string]any{
				"status":              string(back),
				"automations_resumed": resumed,
			},
		})
	})
	if err != nil {
		return "", err
	}
	return status, nil
}

// refuseUnderway refuses a workspace whose hard delete has begun: its bytes are already going.
func refuseUnderway(record adminrepo.TenantRecord) error {
	if record.Status == domain.TenantPendingDeletion && !record.PurgeStartedAt.IsZero() {
		return shared.ErrConflict.WithDetail("admin.tenant_deletion_underway")
	}
	return nil
}

func (h CancelTenantDeletion) find(ctx context.Context) (adminrepo.TenantRecord, error) {
	record, err := h.Tenants.Find(ctx)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return adminrepo.TenantRecord{}, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
		}
		return adminrepo.TenantRecord{}, err
	}
	return record, nil
}

// Descriptor is the catalogue entry.
func (h CancelTenantDeletion) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CancelTenantDeletionName,
		Summary: "Calls off a workspace's pending deletion: it returns to the status it was " +
			"requested from (suspended where that is not known), the deadline is cleared and the " +
			"automation rules the request switched off are switched back on. Possible until the " +
			"hard delete begins; a workspace that is not pending is left as it is.",
		SideEffects: "Moves the tenant out of PENDING_DELETION, switches its paused automation " +
			"rules back on, and records the act in the tenant's trail and the instance journal.",
		TokenScope: adminTenantsScope,
		Input: []usecase.Field{
			{Name: "tenant_id", Kind: usecase.KindID, Required: true},
		},
		Audit: usecase.AuditDeclaration{
			Action: TenantDeletionCancelledAction, TargetType: tenantTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the control plane acts on workspaces, not on items.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CancelTenantDeletion) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	tenantID, err := in.ID("tenant_id")
	if err != nil {
		return nil, err
	}
	status, err := h.Execute(ctx, actor, tenantID)
	if err != nil {
		return nil, err
	}
	return usecase.Output{"tenant_id": tenantID.String(), "status": string(status)}, nil
}
