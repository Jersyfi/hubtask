// SPDX-License-Identifier: Apache-2.0
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
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// A purchase platform's own reference on a workspace, and how it finds workspaces again
// (multi-tenancy.md §4.3, UC-INS-13 check 10). The reference may name a customer: it goes into no
// log, metric or journal entry, and the trail records only that it changed.

const (
	UpdateTenantName = "UpdateTenant"
	ReadTenantName   = "ReadTenant"
	QueryTenantsName = "QueryTenants"

	// TenantReferenceChangedAction is written into the workspace's trail - masked - and the journal,
	// without the value.
	TenantReferenceChangedAction audit.Action = "tenant.reference_changed"
)

// UpdateTenant sets or clears a platform's reference on a workspace.
type UpdateTenant struct {
	Writer  PlanWriter
	Tenants adminrepo.Tenants
	Finder  adminrepo.TenantFinder
	Audit   audit.Sink
}

// UpdateTenantCommand carries the reference; empty clears it.
type UpdateTenantCommand struct {
	TenantID        shared.ID
	Reference       string
	ExpectedVersion *int
}

// Execute sets it. A workspace leaving can still have its reference cleared - it stays taken through
// the grace, and a platform provisioning that customer again clears it first - until its purge begins.
func (h UpdateTenant) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd UpdateTenantCommand,
) (adminrepo.TenantRecord, error) {
	if err := h.Writer.authorize(ctx, actor); err != nil {
		return adminrepo.TenantRecord{}, err
	}
	if cmd.TenantID.IsZero() {
		return adminrepo.TenantRecord{}, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	}
	if cmd.Reference != "" {
		if err := domain.ValidTenantReference(cmd.Reference); err != nil {
			return adminrepo.TenantRecord{}, err
		}
	}

	var answered adminrepo.TenantRecord
	scope := persistence.Scope{TenantID: cmd.TenantID, ActorID: actor.AccountID}
	err := h.Writer.Instance.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		record, err := h.Tenants.Find(ctx)
		if errors.Is(err, shared.ErrNotFound) {
			return shared.ErrNotFound.WithDetail("admin.tenant_not_found")
		}
		if err != nil {
			return err
		}
		if !record.PurgeStartedAt.IsZero() {
			return shared.ErrConflict.WithDetail("admin.tenant_deletion_underway")
		}
		if cmd.ExpectedVersion != nil && *cmd.ExpectedVersion != record.Version {
			return shared.ErrVersionConflict.WithDetail("admin.tenant_version_moved")
		}
		current, err := oneTenant(ctx, h.Finder, cmd.TenantID)
		if err != nil {
			return err
		}
		if current.ExternalReference != cmd.Reference {
			now := h.Writer.Instance.Clock.Now()
			written, err := h.Tenants.SetReference(ctx, cmd.Reference, record.Version, now)
			if err != nil {
				return err
			}
			if !written {
				return shared.ErrVersionConflict.WithDetail("admin.tenant_version_moved")
			}
			if err := h.Audit.Append(ctx, audit.Entry{
				TenantID: record.ID, OccurredAt: now, Action: TenantReferenceChangedAction,
				Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
				ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
				TargetType: tenantTarget, TargetID: record.ID,
				Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
				Changes: audit.Changes(audit.Change{
					Field: "external_reference", Classification: audit.Sensitive,
					From: current.ExternalReference, To: cmd.Reference,
				}),
			}); err != nil {
				return err
			}
			if err := h.Writer.Instance.Journal.Record(ctx, adminrepo.InstanceEvent{
				ID: h.Writer.Instance.IDs.NewID(), OccurredAt: now, Action: string(TenantReferenceChangedAction),
				TenantID: record.ID, TenantSlug: record.Slug, ActorLabel: actor.AccountName,
			}); err != nil {
				return err
			}
		}
		answered, err = oneTenant(ctx, h.Finder, cmd.TenantID)
		return err
	})
	return answered, err
}

// oneTenant reads one workspace through the control plane's query: the row with its plan, legal
// hold and deletion dates, as the listing answers it.
func oneTenant(ctx context.Context, finder adminrepo.TenantFinder, id shared.ID) (adminrepo.TenantRecord, error) {
	records, _, err := finder.Query(ctx, adminrepo.TenantQuery{ID: id, Size: 1})
	if err != nil {
		return adminrepo.TenantRecord{}, err
	}
	if len(records) == 0 {
		return adminrepo.TenantRecord{}, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	}
	return records[0], nil
}

// Descriptor is the catalogue entry.
func (h UpdateTenant) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UpdateTenantName,
		Summary: "Sets or clears a purchase platform's own reference on a workspace - its customer or " +
			"subscription id - unique on the installation. With `expected_version`, a change on a " +
			"workspace that changed since it was read is refused. Multi mode only.",
		SideEffects: "Writes the workspace's reference, its trail (the value masked) and the journal " +
			"(without the value).",
		TokenScope: adminTenantsScope,
		Input: []usecase.Field{
			{Name: "tenant_id", Kind: usecase.KindID, Required: true},
			{Name: "external_reference", Kind: usecase.KindString,
				Description: "The platform's reference; empty or null clears it."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true,
				Description: "The workspace's version as last read; a stale one is refused with version_conflict."},
		},
		Audit: usecase.AuditDeclaration{
			Action: TenantReferenceChangedAction, TargetType: tenantTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "the control plane acts on workspaces, not on items."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			tenantID, err := in.ID("tenant_id")
			if err != nil {
				return nil, err
			}
			cmd := UpdateTenantCommand{TenantID: tenantID, Reference: in.String("external_reference")}
			if in.Present("expected_version") {
				version := in.Int("expected_version")
				cmd.ExpectedVersion = &version
			}
			record, err := h.Execute(ctx, actor, cmd)
			if err != nil {
				return nil, err
			}
			return adminTenantOutput(record), nil
		}),
	}
}
