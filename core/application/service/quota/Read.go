// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package quota

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/quota"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	ReadQuotasName = "ReadQuotas"
	quotaTarget    = "quota"
	quotasRead     = "quotas:read"

	// QuotaReadAction exists for the refusal's sake: a denied read is recorded against it.
	QuotaReadAction audit.Action = "quota.read"
)

// Authorizer is the slice of the authorisation service this use case needs.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// Standing is one quota's answer: the resolved ceiling, the live count where one exists, and
// whether the workspace carries its own ceiling.
type Standing struct {
	Quota string
	Limit int64
	// Used is nil where the limit is enforced elsewhere - the request rate lives in the
	// limiter's own headers.
	Used       *int64
	Configured bool
}

// Ratio answers used/limit for the metric and the response, nil while either half is missing.
func (s Standing) Ratio() *float64 {
	if s.Used == nil || s.Limit == Unlimited {
		return nil
	}
	ratio := Ratio(s.Limit, *s.Used)
	return &ratio
}

// ReadQuotas answers the workspace's own quota standing (H-08): every §4 limit as it applies
// here. A quota is workspace configuration, so the auditor's read-only configuration permission
// opens it too (G-12's pair).
type ReadQuotas struct {
	Store      repository.Store
	Usage      repository.Usage
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	Tenancy    env.TenancyMode
}

// Execute lists the standings, in the contract's order.
func (h ReadQuotas) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]Standing, error) {
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission:  service.PermissionStructure,
		Alternative: service.PermissionReadConfiguration,
		Path:        []identity.Scope{identity.TenantScope()},
		Action:      QuotaReadAction,
		TokenScope:  quotasRead,
		TargetType:  quotaTarget,
	}); err != nil {
		return nil, err
	}

	var standings []Standing
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		listed, err := Standings(ctx, h.Store, h.Usage, h.Tenancy, h.Clock.Now())
		standings = listed
		return err
	})
	if err != nil {
		return nil, err
	}
	return standings, nil
}

// Standings resolves and counts, shared with the operator's write - the answer after a write is
// the same answer a read gives.
func Standings(
	ctx context.Context, store repository.Store, usage repository.Usage,
	mode env.TenancyMode, now time.Time,
) ([]Standing, error) {
	overrides, err := store.Overrides(ctx)
	if err != nil {
		return nil, err
	}
	limits := Resolve(overrides, mode)

	configured := map[string]bool{
		APIRequestsPerMinute:  overrides.APIRequestsPerMinute != nil,
		Items:                 overrides.Items != nil,
		MediaBytes:            overrides.MediaBytes != nil,
		AutomationRunsPerHour: overrides.AutomationRunsPerHour != nil,
		WebhookTargets:        overrides.WebhookTargets != nil,
		ExportJobs:            overrides.ExportJobs != nil,
	}
	counters := map[string]func(context.Context) (int64, error){
		Items:          usage.Items,
		MediaBytes:     usage.MediaBytes,
		WebhookTargets: usage.WebhookTargets,
		ExportJobs:     usage.LiveExports,
		AutomationRunsPerHour: func(ctx context.Context) (int64, error) {
			return usage.AutomationRunsSince(ctx, now.Add(-time.Hour))
		},
	}

	standings := make([]Standing, 0, len(Names()))
	for _, name := range Names() {
		standing := Standing{Quota: name, Limit: limits.Of(name), Configured: configured[name]}
		if counter := counters[name]; counter != nil {
			used, err := counter(ctx)
			if err != nil {
				return nil, err
			}
			standing.Used = &used
		}
		standings = append(standings, standing)
	}
	return standings, nil
}

// StandingOutputs is the contract's shape, shared by both channels that answer standings.
func StandingOutputs(standings []Standing) []usecase.Output {
	rows := make([]usecase.Output, 0, len(standings))
	for _, standing := range standings {
		row := usecase.Output{
			"quota":      standing.Quota,
			"limit":      standing.Limit,
			"used":       nil,
			"ratio":      nil,
			"configured": standing.Configured,
		}
		if standing.Used != nil {
			row["used"] = *standing.Used
		}
		if ratio := standing.Ratio(); ratio != nil {
			row["ratio"] = *ratio
		}
		rows = append(rows, row)
	}
	return rows
}

// Descriptor is the catalogue entry.
func (h ReadQuotas) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReadQuotasName,
		Summary: "Answers every §4 limit as it applies to this workspace: the resolved ceiling " +
			"(0 means unlimited), the live count where one exists, and the approach ratio.",
		TokenScope: quotasRead,
		ReadOnly:   true,
		Audit: usecase.AuditDeclaration{
			Action: QuotaReadAction, TargetType: quotaTarget, Severity: audit.SeverityInfo,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a quota is workspace configuration, not an item; the history is an item's " +
				"(domain-model.md §3.5).",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ReadQuotas) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	standings, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	return usecase.Output{"data": StandingOutputs(standings)}, nil
}
