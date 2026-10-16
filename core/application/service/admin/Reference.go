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

const (
	defaultTenantPage = 50
	maxTenantPage     = 200
)

// TenantReader reads workspaces for the control plane: one by its identifier, or a page of those
// matching a platform's filters.
type TenantReader struct {
	Writer PlanWriter
	Finder adminrepo.TenantFinder
}

// ReadTenant answers one workspace.
type ReadTenant struct{ Reader TenantReader }

// QueryTenants answers a page of the workspaces matching the filters.
type QueryTenants struct{ Reader TenantReader }

func (r TenantReader) query(
	ctx context.Context, actor appshared.ActorContext, query adminrepo.TenantQuery,
) ([]adminrepo.TenantRecord, adminrepo.PageInfo, error) {
	if err := r.Writer.Instance.authorize(ctx, actor); err != nil {
		return nil, adminrepo.PageInfo{}, err
	}
	var records []adminrepo.TenantRecord
	var page adminrepo.PageInfo
	err := r.Writer.Instance.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			found, info, err := r.Finder.Query(ctx, query)
			records, page = found, info
			return err
		})
	if err != nil {
		return nil, adminrepo.PageInfo{}, err
	}
	// An operator's opening of the password is answered only while it stands, as the listing does.
	for i := range records {
		if !records[i].PasswordOpening.InForce(r.Writer.Instance.Clock.Now()) {
			records[i].PasswordOpening = domain.PasswordOpening{}
		}
	}
	return records, page, nil
}

// Execute reads one workspace.
func (h ReadTenant) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) (adminrepo.TenantRecord, error) {
	if id.IsZero() {
		return adminrepo.TenantRecord{}, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	}
	records, _, err := h.Reader.query(ctx, actor, adminrepo.TenantQuery{ID: id, Size: 1})
	if err != nil {
		return adminrepo.TenantRecord{}, err
	}
	if len(records) == 0 {
		return adminrepo.TenantRecord{}, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	}
	return records[0], nil
}

// Execute answers one page.
func (h QueryTenants) Execute(
	ctx context.Context, actor appshared.ActorContext, query adminrepo.TenantQuery,
) ([]adminrepo.TenantRecord, adminrepo.PageInfo, error) {
	switch {
	case query.Size <= 0:
		query.Size = defaultTenantPage
	case query.Size > maxTenantPage:
		query.Size = maxTenantPage
	}
	if query.Plan != "" && query.WithoutPlan {
		return nil, adminrepo.PageInfo{}, shared.ErrValidation.WithDetail("admin.tenant_query_conflicting").
			WithFields(shared.FieldError{Path: "/without_plan", Code: "admin.tenant_query_conflicting"})
	}
	if query.Status != "" && query.Status != domain.TenantActive && query.Status != domain.TenantSuspended &&
		query.Status != domain.TenantPendingDeletion {
		return nil, adminrepo.PageInfo{}, shared.ErrValidation.WithDetail("admin.tenant_status_unknown").
			WithFields(shared.FieldError{Path: "/status", Code: "admin.tenant_status_unknown"})
	}
	return h.Reader.query(ctx, actor, query)
}

// Descriptor is the catalogue entry.
func (h ReadTenant) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReadTenantName,
		Summary: "Reads one workspace as the control plane sees it: state, plan, version, the platform's " +
			"reference, the legal hold and the deletion dates.",
		TokenScope: adminTenantsScope,
		ReadOnly:   true,
		Input:      []usecase.Field{{Name: "tenant_id", Kind: usecase.KindID, Required: true}},
		Audit:      usecase.AuditDeclaration{Action: "tenant.read", TargetType: tenantTarget, Severity: audit.SeverityInfo},
		Activity:   usecase.ActivityDeclaration{Exempt: "a read of the control plane touches no item."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("tenant_id")
			if err != nil {
				return nil, err
			}
			record, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			return adminTenantOutput(record), nil
		}),
	}
}

// Descriptor is the catalogue entry.
func (h QueryTenants) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: QueryTenantsName,
		Summary: "Finds the workspaces matching every filter given - short name, the platform's " +
			"reference, plan, no plan, status - oldest first, one page at a time.",
		TokenScope: adminTenantsScope,
		ReadOnly:   true,
		Input: []usecase.Field{
			{Name: "slug", Kind: usecase.KindString},
			{Name: "external_reference", Kind: usecase.KindString},
			{Name: "plan", Kind: usecase.KindString},
			{Name: "without_plan", Kind: usecase.KindBool},
			{Name: "status", Kind: usecase.KindString},
			{Name: "size", Kind: usecase.KindInt},
			{Name: "cursor", Kind: usecase.KindString},
		},
		Audit:    usecase.AuditDeclaration{Action: "tenant.queried", TargetType: tenantTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: "a read of the control plane touches no item."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			records, page, err := h.Execute(ctx, actor, adminrepo.TenantQuery{
				Slug: in.String("slug"), Reference: in.String("external_reference"), Plan: in.String("plan"),
				WithoutPlan: in.Bool("without_plan"), Status: domain.TenantStatus(in.String("status")),
				Size: in.Int("size"), Cursor: in.String("cursor"),
			})
			if err != nil {
				return nil, err
			}
			rows := make([]usecase.Output, 0, len(records))
			for _, record := range records {
				rows = append(rows, adminTenantOutput(record))
			}
			return usecase.Output{
				"data": rows, "page": usecase.Output{"next_cursor": page.NextCursor, "has_more": page.HasMore},
			}, nil
		}),
	}
}
