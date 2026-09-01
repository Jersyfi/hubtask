// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"strconv"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	quotarepo "github.com/Jersyfi/hubtask/core/application/repository/quota"
	quotaservice "github.com/Jersyfi/hubtask/core/application/service/quota"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	UpdateTenantQuotasName = "UpdateTenantQuotas"

	// TenantQuotasChangedAction is the trail's record that the wall moved. Warning, the backup
	// schedule's reasoning: a quota decides what a workspace may hold from now on, without
	// anybody being asked again.
	TenantQuotasChangedAction audit.Action = "tenant.quotas_changed"
)

// QuotaChange is one key's instruction: set a ceiling, or clear the override back to the mode's
// default. Absent keys are simply not in the list.
type QuotaChange struct {
	Quota string
	// Value is the new ceiling (0 = unlimited); nil clears the override.
	Value *int64
}

// UpdateTenantQuotasCommand is the input, typed.
type UpdateTenantQuotasCommand struct {
	TenantID shared.ID
	Changes  []QuotaChange
}

// UpdateTenantQuotas is the operator's write on a workspace's §4 ceilings (H-08): partial by
// design, version-guarded, audited field by field.
type UpdateTenantQuotas struct {
	Tenants    adminrepo.Tenants
	Store      quotarepo.Store
	Usage      quotarepo.Usage
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	Tenancy    env.TenancyMode
}

// Execute applies the changes and answers the resolved standings.
func (h UpdateTenantQuotas) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd UpdateTenantQuotasCommand,
) ([]quotaservice.Standing, error) {
	if err := actor.RequireScope(adminTenantsScope); err != nil {
		return nil, err
	}
	if cmd.TenantID.IsZero() {
		return nil, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	}
	for _, change := range cmd.Changes {
		if !knownQuota(change.Quota) {
			return nil, shared.ErrValidation.
				WithDetail("admin.quota_unknown").
				WithParams(map[string]string{"quota": change.Quota})
		}
		if change.Value != nil && *change.Value < 0 {
			return nil, shared.ErrValidation.
				WithDetail("admin.quota_negative").
				WithParams(map[string]string{"quota": change.Quota})
		}
	}

	var standings []quotaservice.Standing
	scope := persistence.Scope{TenantID: cmd.TenantID, ActorID: actor.AccountID}
	err := h.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		record, err := h.Tenants.Find(ctx)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return shared.ErrNotFound.WithDetail("admin.tenant_not_found")
			}
			return err
		}

		before, err := h.Store.Overrides(ctx)
		if err != nil {
			return err
		}
		after := before
		for _, change := range cmd.Changes {
			applyChange(&after, change)
		}

		now := h.Clock.Now()
		written, err := h.Store.SetOverrides(ctx, after, record.Version, now)
		if err != nil {
			return err
		}
		if !written {
			return shared.ErrConflict.WithDetail("admin.tenant_version_moved")
		}

		if err := h.recordAudit(ctx, actor, cmd, before, now); err != nil {
			return err
		}

		listed, err := quotaservice.Standings(ctx, h.Store, h.Usage, h.Tenancy, now)
		standings = listed
		return err
	})
	if err != nil {
		return nil, err
	}
	return standings, nil
}

// applyChange writes one instruction into the overrides.
func applyChange(overrides *quotarepo.Overrides, change QuotaChange) {
	target := map[string]**int64{
		quotaservice.APIRequestsPerMinute:  &overrides.APIRequestsPerMinute,
		quotaservice.Items:                 &overrides.Items,
		quotaservice.MediaBytes:            &overrides.MediaBytes,
		quotaservice.AutomationRunsPerHour: &overrides.AutomationRunsPerHour,
		quotaservice.WebhookTargets:        &overrides.WebhookTargets,
		quotaservice.ExportJobs:            &overrides.ExportJobs,
	}[change.Quota]
	if target == nil {
		return
	}
	*target = change.Value
}

func knownQuota(name string) bool {
	for _, known := range quotaservice.Names() {
		if name == known {
			return true
		}
	}
	return false
}

// recordAudit writes the wall's movement into the workspace's own trail, field by field: from
// what stood (the override, or "default"), to what stands now.
func (h UpdateTenantQuotas) recordAudit(
	ctx context.Context, actor appshared.ActorContext, cmd UpdateTenantQuotasCommand,
	before quotarepo.Overrides, now time.Time,
) error {
	changes := make([]audit.Change, 0, len(cmd.Changes))
	beforeOf := map[string]*int64{
		quotaservice.APIRequestsPerMinute:  before.APIRequestsPerMinute,
		quotaservice.Items:                 before.Items,
		quotaservice.MediaBytes:            before.MediaBytes,
		quotaservice.AutomationRunsPerHour: before.AutomationRunsPerHour,
		quotaservice.WebhookTargets:        before.WebhookTargets,
		quotaservice.ExportJobs:            before.ExportJobs,
	}
	for _, change := range cmd.Changes {
		changes = append(changes, audit.Change{
			Field: change.Quota, Classification: audit.Open,
			From: overrideLabel(beforeOf[change.Quota]), To: overrideLabel(change.Value),
		})
	}

	return h.Audit.Append(ctx, audit.Entry{
		TenantID: cmd.TenantID, OccurredAt: now, Action: TenantQuotasChangedAction,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityWarning,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: quotaTargetType, TargetID: cmd.TenantID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}

const quotaTargetType = "quota"

// overrideLabel renders one override state for the trail: a number, or the word for "the mode's
// default applies".
func overrideLabel(value *int64) any {
	if value == nil {
		return "default"
	}
	return strconv.FormatInt(*value, 10)
}

// Descriptor is the catalogue entry.
func (h UpdateTenantQuotas) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UpdateTenantQuotasName,
		Summary: "Sets a workspace's §4 ceilings: each provided key becomes the ceiling " +
			"(0 = unlimited), an explicit null clears the override back to the mode's default, " +
			"absent keys stay untouched. Answers the resolved standings.",
		SideEffects: "Rewrites the quotas key of the tenant's settings document and records " +
			"the movement, field by field, in the workspace's audit trail.",
		TokenScope: adminTenantsScope,
		Input: []usecase.Field{
			{Name: "tenant_id", Kind: usecase.KindID, Required: true},
			{
				Name: "quotas", Kind: usecase.KindObject, Required: true,
				Description: "The changes: quota name to ceiling, null to clear an override.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: TenantQuotasChangedAction, TargetType: quotaTargetType,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the control plane acts on workspaces, not on items (domain-model.md §3.5).",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h UpdateTenantQuotas) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	tenantID, err := in.ID("tenant_id")
	if err != nil {
		return nil, err
	}

	document, _ := in["quotas"].(map[string]any)
	changes := make([]QuotaChange, 0, len(document))
	for name, raw := range document {
		change := QuotaChange{Quota: name}
		switch value := raw.(type) {
		case nil:
			// An explicit null: clear the override.
		case float64:
			ceiling := int64(value)
			change.Value = &ceiling
		case int64:
			ceiling := value
			change.Value = &ceiling
		default:
			return nil, shared.ErrValidation.
				WithDetail("admin.quota_unknown").
				WithParams(map[string]string{"quota": name})
		}
		changes = append(changes, change)
	}

	standings, err := h.Execute(ctx, actor, UpdateTenantQuotasCommand{
		TenantID: tenantID, Changes: changes,
	})
	if err != nil {
		return nil, err
	}
	return usecase.Output{"data": quotaservice.StandingOutputs(standings)}, nil
}
