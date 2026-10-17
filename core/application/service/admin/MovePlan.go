// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	quotarepo "github.com/Jersyfi/hubtask/core/application/repository/quota"
	"github.com/Jersyfi/hubtask/core/application/service/platform"
	quotaservice "github.com/Jersyfi/hubtask/core/application/service/quota"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/plan"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	MoveTenantPlanName = "MoveTenantPlan"

	// TenantPlanChangedAction is written into the workspace's own trail and the instance journal:
	// its people are entitled to see when their limits and features changed and from which plan to
	// which (UC-INS-13 check 6).
	TenantPlanChangedAction audit.Action = "tenant.plan_changed"
)

// MoveTenantPlanCommand puts one workspace on a plan, or on none where Plan is empty.
type MoveTenantPlanCommand struct {
	TenantID shared.ID
	Plan     string
	// ExpectedVersion is the workspace's version as the caller last read it; nil moves whatever
	// stands. A stale one is refused, so a late retry of an older move cannot undo a newer one.
	ExpectedVersion *int
}

// MovedTenant is what a move answers: the workspace on its plan, the features its plan now leaves
// out, and the limits for which it keeps an operator's exception - the exception stays the last
// word, so a platform that sold an add-on sees where to set it again (multi-tenancy.md §4.3).
type MovedTenant struct {
	Tenant          adminrepo.TenantRecord
	FeaturesResting []plan.Feature
	ExceptionsKept  []string
	// LimitsExceeded is every counted capacity the workspace now holds more of than its limit
	// allows: nothing is removed, the next creation is refused (P-03).
	LimitsExceeded []LimitExceeded
}

// LimitExceeded is one capacity over its resolved limit, the operator's exception included.
type LimitExceeded struct {
	Quota string
	Limit int64
	Used  int64
}

// MoveTenantPlan moves a workspace between plans. Nothing is copied into the workspace and nothing
// is deleted: what the plan decides applies at the workspace's next request (UC-INS-13 checks 2, 4).
type MoveTenantPlan struct {
	Writer  PlanWriter
	Tenants adminrepo.Tenants
	Quotas  quotarepo.Store
	// Usage counts what the workspace holds, for the limits it is now over. Nil answers none.
	Usage quotarepo.Usage
	Audit audit.Sink
	// Platform tells a purchase platform the workspace moved (UC-INS-14).
	Platform platform.Recorder
}

// Execute moves it.
func (h MoveTenantPlan) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd MoveTenantPlanCommand,
) (MovedTenant, error) {
	if err := h.Writer.authorize(ctx, actor); err != nil {
		return MovedTenant{}, err
	}
	if cmd.TenantID.IsZero() {
		return MovedTenant{}, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	}

	var moved MovedTenant
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

		var target plan.Plan
		if cmd.Plan != "" {
			planID, err := h.Writer.Plans.Hold(ctx, cmd.Plan)
			if errors.Is(err, shared.ErrNotFound) {
				return unknownPlan(cmd.Plan)
			}
			if err != nil {
				return err
			}
			if target, err = h.Writer.Plans.Find(ctx, cmd.Plan); err != nil {
				return err
			}
			target.ID = planID
		}

		// To the plan it is on: the state wanted already stands, and nothing is recorded - an entry
		// per retry would put noise where evidence lives (the lifecycle's own rule).
		if record.PlanName != cmd.Plan {
			now := h.Writer.Instance.Clock.Now()
			written, err := h.Tenants.SetPlan(ctx, target.ID, record.Version, now)
			if err != nil {
				return err
			}
			if !written {
				return shared.ErrVersionConflict.WithDetail("admin.tenant_version_moved")
			}
			if err := h.record(ctx, actor, record, cmd.Plan, now); err != nil {
				return err
			}
			previous := record.PlanName
			if record, err = h.Tenants.Find(ctx); err != nil {
				return err
			}
			var from any
			if previous != "" {
				from = previous
			}
			if err := h.Platform.Record(ctx, event.WorkspacePlanChanged,
				event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now, map[string]any{"previous_plan": from},
			); err != nil {
				return err
			}
		}

		levels, err := quotaservice.ReadLevels(ctx, h.Quotas)
		if err != nil {
			return err
		}
		exceeded, err := h.exceeded(ctx, levels)
		if err != nil {
			return err
		}
		moved = MovedTenant{
			Tenant: record, FeaturesResting: target.Values.LeftOut, ExceptionsKept: exceptionsOf(levels.Exception),
			LimitsExceeded: exceeded,
		}
		return nil
	})
	return moved, err
}

// exceeded answers the counted capacities above the limit in force after the move. Rates and day
// budgets pass with the hour or the day, and running exports end; at the limit is not over it.
func (h MoveTenantPlan) exceeded(ctx context.Context, levels quotaservice.Levels) ([]LimitExceeded, error) {
	exceeded := []LimitExceeded{}
	if h.Usage == nil {
		return exceeded, nil
	}
	limits := quotaservice.Resolve(levels, env.TenancyMulti)
	for _, counted := range []struct {
		quota string
		count func(context.Context) (int64, error)
	}{
		{quotaservice.Items, h.Usage.Items},
		{quotaservice.MediaBytes, h.Usage.MediaBytes},
		{quotaservice.WebhookTargets, h.Usage.WebhookTargets},
	} {
		limit := limits.Of(counted.quota)
		if limit == quotaservice.Unlimited {
			continue
		}
		used, err := counted.count(ctx)
		if err != nil {
			return nil, err
		}
		if used > limit {
			exceeded = append(exceeded, LimitExceeded{Quota: counted.quota, Limit: limit, Used: used})
		}
	}
	return exceeded, nil
}

// record writes the move into the workspace's own trail and the instance journal: the plans by
// their short names, which are identifiers and never a person's content.
func (h MoveTenantPlan) record(
	ctx context.Context, actor appshared.ActorContext, record adminrepo.TenantRecord, to string, now time.Time,
) error {
	if err := h.Audit.Append(ctx, audit.Entry{
		TenantID: record.ID, OccurredAt: now, Action: TenantPlanChangedAction,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityNotice,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: tenantTarget, TargetID: record.ID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(audit.Change{
			Field: "plan", Classification: audit.Open, From: record.PlanName, To: to,
		}),
	}); err != nil {
		return err
	}
	return h.Writer.Instance.Journal.Record(ctx, adminrepo.InstanceEvent{
		ID: h.Writer.Instance.IDs.NewID(), OccurredAt: now, Action: string(TenantPlanChangedAction),
		TenantID: record.ID, TenantSlug: record.Slug, ActorLabel: actor.AccountName,
		Details: map[string]any{"plan_from": record.PlanName, "plan_to": to},
	})
}

// exceptionsOf names the limits an operator set for this one workspace, in the contract's order.
func exceptionsOf(overrides quotarepo.Overrides) []string {
	set := map[string]*int64{
		quotaservice.APIRequestsPerMinute:  overrides.APIRequestsPerMinute,
		quotaservice.Items:                 overrides.Items,
		quotaservice.MediaBytes:            overrides.MediaBytes,
		quotaservice.AutomationRunsPerHour: overrides.AutomationRunsPerHour,
		quotaservice.WebhookTargets:        overrides.WebhookTargets,
		quotaservice.ExportJobs:            overrides.ExportJobs,
		quotaservice.AiTokensPerDay:        overrides.AiTokensPerDay,
	}
	kept := []string{}
	for _, name := range quotaservice.OperatorNames() {
		if set[name] != nil {
			kept = append(kept, name)
		}
	}
	return kept
}

// Descriptor is the catalogue entry.
func (h MoveTenantPlan) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: MoveTenantPlanName,
		Summary: "Puts a workspace on another plan, or on none: what the plan decides applies at the " +
			"workspace's next request, nothing is copied into it and nothing is deleted. With " +
			"`expected_version`, a move on a workspace that changed since it was read is refused. " +
			"Answers the features the plan now leaves out and the limits for which the workspace " +
			"keeps an operator's exception. Multi mode only.",
		SideEffects: "Writes the workspace's plan, its trail and the instance journal.",
		TokenScope:  adminTenantsScope,
		Input: []usecase.Field{
			{Name: "tenant_id", Kind: usecase.KindID, Required: true},
			{Name: "plan", Kind: usecase.KindString,
				Description: "The plan's short name; empty or absent for none."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true,
				Description: "The workspace's version as last read; a stale one is refused with version_conflict."},
		},
		Audit: usecase.AuditDeclaration{
			Action: TenantPlanChangedAction, TargetType: tenantTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "the control plane acts on workspaces, not on items."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			tenantID, err := in.ID("tenant_id")
			if err != nil {
				return nil, err
			}
			cmd := MoveTenantPlanCommand{TenantID: tenantID, Plan: in.String("plan")}
			if in.Present("expected_version") {
				version := in.Int("expected_version")
				cmd.ExpectedVersion = &version
			}
			moved, err := h.Execute(ctx, actor, cmd)
			if err != nil {
				return nil, err
			}
			resting := make([]any, 0, len(moved.FeaturesResting))
			for _, feature := range moved.FeaturesResting {
				resting = append(resting, string(feature))
			}
			exceeded := make([]usecase.Output, 0, len(moved.LimitsExceeded))
			for _, over := range moved.LimitsExceeded {
				exceeded = append(exceeded, usecase.Output{"quota": over.Quota, "limit": over.Limit, "used": over.Used})
			}
			return usecase.Output{
				"tenant": adminTenantOutput(moved.Tenant), "features_resting": resting,
				"exceptions_kept": moved.ExceptionsKept, "limits_exceeded": exceeded,
			}, nil
		}),
	}
}
