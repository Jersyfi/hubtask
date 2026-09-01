// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"

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
	ListTenantsName   = "ListTenants"
	SuspendTenantName = "SuspendTenant"
	ResumeTenantName  = "ResumeTenant"

	// TenantSuspendedAction and TenantResumedAction are written into the tenant's own trail:
	// its members are entitled to see that their workspace was suspended, and when it came back
	// (audit.md §6). The suspension is WARNING - it is the entry somebody investigating "why
	// did every call fail at 14:02" is looking for.
	TenantSuspendedAction audit.Action = "tenant.suspended"
	TenantResumedAction   audit.Action = "tenant.resumed"

	journalSuspended = "tenant.suspended"
	journalResumed   = "tenant.resumed"
)

// ListTenants answers the installation's workspaces - the one legitimate enumerator (0.6.0
// decision 6), behind the admin scope, through the installation-scoped read path.
type ListTenants struct {
	Tenants    adminrepo.Tenants
	UnitOfWork persistence.UnitOfWork
}

// Execute lists the workspaces, oldest first.
func (h ListTenants) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]adminrepo.TenantRecord, error) {
	if err := actor.RequireScope(adminTenantsScope); err != nil {
		return nil, err
	}

	var records []adminrepo.TenantRecord
	err := h.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			listed, err := h.Tenants.List(ctx)
			records = listed
			return err
		})
	if err != nil {
		return nil, err
	}
	return records, nil
}

// Descriptor is the catalogue entry.
func (h ListTenants) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListTenantsName,
		Summary: "Lists the installation's workspaces, oldest first: identifier, slug, display " +
			"name, lifecycle status, defaults, and the purge deadline while a deletion request " +
			"stands. The one legitimate tenant enumerator, for the control plane alone.",
		TokenScope: adminTenantsScope,
		ReadOnly:   true,
		Audit: usecase.AuditDeclaration{
			Action: "tenant.listed", TargetType: tenantTarget, Severity: audit.SeverityInfo,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a read of the control plane touches no item; the history is an item's " +
				"(domain-model.md §3.5).",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListTenants) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	records, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	rows := make([]usecase.Output, 0, len(records))
	for _, record := range records {
		rows = append(rows, adminTenantOutput(record))
	}
	return usecase.Output{"data": rows}, nil
}

// LifecycleShift is the shared shape of the two one-write edges: suspend and resume.
type LifecycleShift struct {
	Tenants    adminrepo.Tenants
	Journal    adminrepo.Journal
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// shift moves one edge. Moving to the status that already stands is a no-op success: the state
// the operator wanted is the state there is, and an operator's retry must not become an error
// (§5, "reactivation is one write"). A workspace already leaving refuses both edges.
func (s LifecycleShift) shift(
	ctx context.Context, actor appshared.ActorContext, tenantID shared.ID,
	from, to domain.TenantStatus, action audit.Action, severity audit.Severity, journal string,
) error {
	if err := actor.RequireScope(adminTenantsScope); err != nil {
		return err
	}
	if tenantID.IsZero() {
		return shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	}

	scope := persistence.Scope{TenantID: tenantID, ActorID: actor.AccountID}
	return s.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		record, err := s.Tenants.Find(ctx)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return shared.ErrNotFound.WithDetail("admin.tenant_not_found")
			}
			return err
		}
		switch record.Status {
		case to:
			// Idempotent: the state the operator wanted already stands, and nothing is recorded
			// - an entry per retry would put noise where evidence lives.
			return nil
		case domain.TenantPendingDeletion:
			return shared.ErrConflict.WithDetail("admin.tenant_leaving")
		case from:
			// The edge this shift exists for.
		default:
			return shared.ErrConflict.WithDetail("admin.tenant_leaving")
		}

		now := s.Clock.Now()
		moved, err := s.Tenants.SetStatus(ctx, from, to, now)
		if err != nil {
			return err
		}
		if !moved {
			// The state moved between our read and our write; whoever moved it answered for it.
			return shared.ErrConflict.WithDetail("admin.tenant_leaving")
		}

		if err := s.Audit.Append(ctx, audit.Entry{
			TenantID: tenantID, OccurredAt: now, Action: action,
			Outcome: audit.OutcomeSuccess, Severity: severity,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: tenantTarget, TargetID: tenantID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(
				audit.Change{Field: "status", Classification: audit.Open,
					From: string(from), To: string(to)},
			),
		}); err != nil {
			return err
		}
		return s.Journal.Record(ctx, adminrepo.InstanceEvent{
			ID: s.IDs.NewID(), OccurredAt: now, Action: journal,
			TenantID: tenantID, TenantSlug: record.Slug, ActorLabel: actor.AccountName,
		})
	})
}

// SuspendTenant flips the middleware: the tenant's people answer 403 tenant_suspended on their
// very next request, the data remains, the read export still works (multi-tenancy.md §5).
type SuspendTenant struct{ LifecycleShift }

// Execute suspends the workspace.
func (h SuspendTenant) Execute(
	ctx context.Context, actor appshared.ActorContext, tenantID shared.ID,
) error {
	return h.shift(ctx, actor, tenantID,
		domain.TenantActive, domain.TenantSuspended,
		TenantSuspendedAction, audit.SeverityWarning, journalSuspended)
}

// Descriptor is the catalogue entry.
func (h SuspendTenant) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SuspendTenantName,
		Summary: "Suspends a workspace: every API call of its people answers 403 " +
			"tenant_suspended from their next request on. The data remains, and suspending an " +
			"already suspended workspace changes nothing.",
		SideEffects: "Flips the tenant's lifecycle status; records the act in the tenant's " +
			"audit trail and the instance journal.",
		TokenScope: adminTenantsScope,
		Input: []usecase.Field{
			{Name: "tenant_id", Kind: usecase.KindID, Required: true},
		},
		Audit: usecase.AuditDeclaration{
			Action: TenantSuspendedAction, TargetType: tenantTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the control plane acts on workspaces, not on items (domain-model.md §3.5).",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h SuspendTenant) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	tenantID, err := in.ID("tenant_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, tenantID); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}

// ResumeTenant is §5's one write back: the next request of the tenant's people works again.
type ResumeTenant struct{ LifecycleShift }

// Execute reactivates the workspace.
func (h ResumeTenant) Execute(
	ctx context.Context, actor appshared.ActorContext, tenantID shared.ID,
) error {
	return h.shift(ctx, actor, tenantID,
		domain.TenantSuspended, domain.TenantActive,
		TenantResumedAction, audit.SeverityInfo, journalResumed)
}

// Descriptor is the catalogue entry.
func (h ResumeTenant) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ResumeTenantName,
		Summary: "Reactivates a suspended workspace with one write. Resuming an active " +
			"workspace changes nothing; a workspace with a standing deletion request refuses.",
		SideEffects: "Flips the tenant's lifecycle status back; records the act in the " +
			"tenant's audit trail and the instance journal.",
		TokenScope: adminTenantsScope,
		Input: []usecase.Field{
			{Name: "tenant_id", Kind: usecase.KindID, Required: true},
		},
		Audit: usecase.AuditDeclaration{
			Action: TenantResumedAction, TargetType: tenantTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the control plane acts on workspaces, not on items (domain-model.md §3.5).",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ResumeTenant) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	tenantID, err := in.ID("tenant_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, tenantID); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}
