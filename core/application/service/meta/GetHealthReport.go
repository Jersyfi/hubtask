// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package meta

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	health "github.com/Jersyfi/hubtask/core/port/health"
)

// GetHealthReportName is the catalogue name, and it is `Get…` rather than `Read…` because every
// channel identity is derived from it: the REST operation the contract has declared since A-06 is
// `getHealthReport`.
const GetHealthReportName = "GetHealthReport"

// HealthReportReadAction is what a refusal is recorded against. A successful read writes no entry -
// an ordinary read is not an auditable event (audit.md §4), and this one is polled by a banner in
// the browser, so an entry per read would be a trail of nothing but this.
const HealthReportReadAction audit.Action = "health.report_read"

const (
	// opsReadScope is the scope a credential needs for the reduced answer. Deliberately not
	// `admin:…`: `catalogue.SessionScopes` filters on that prefix, so a scope named that way could
	// never be carried by a session - and a session is exactly the reader this answer exists for
	// (0.6.0 decision 6, K-06).
	opsReadScope = "ops:read"
	// adminTenantsScope is the installation's own credential, and holding it is what makes a reader
	// the operator. No membership in any workspace can answer for an installation, so the bound is
	// the credential - the shape `privacy.requireInstanceScope` already uses.
	adminTenantsScope = "admin:tenants"

	healthTarget = "installation"
)

// Reporter is the slice of the health registry this use case needs.
//
// An interface here rather than the registry itself, so that the application layer names what it
// asks for and `core/port/health` stays what it is: a port with no adapter knowledge in it.
type Reporter interface {
	Report(ctx context.Context) health.Report
}

// Authorizer is the authorisation service as this use case needs it.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// GetHealthReport answers the deep self-diagnosis at `/api/v1/meta/health`, in one of two shapes.
//
// The report itself has existed since A-04 and is served on the operations listener; what was
// missing until K-06 is the authenticated door the contract declares, left "until A-06" in a
// comment that outlived A-06 by six milestones (#507).
//
// **One route, two answers**, the way `GetCapabilities` puts it: the same endpoint, a different
// answer, decided by the scope the use case opens rather than by a branch in an adapter.
//
//   - The installation's operator, holding `admin:tenants`, reads the whole report: every
//     dependency with its latency and its last error code, the circuit states, the backlogs, the
//     migration state and the configuration warnings.
//   - Everybody else reads `status`, `version` and `degraded_features`. Which features are degraded
//     is a statement about what the reader is about to try - it is what `HealthNotice.svelte`
//     renders, and it is why F1 could say "fed where the actor may read it" and mean it. The
//     dependency names, the backlogs and the warnings are the installation's internals and stay
//     the operator's, which is what keeps this route from becoming a way across a tenant boundary
//     in multi mode.
//
// The reduction happens here and not in the adapter, because deciding what a reader may see is
// authorisation (rule 2, ADR-0005). An adapter that trimmed the report would be an adapter making
// that decision, and the next channel would make it again, differently.
type GetHealthReport struct {
	Health     Reporter
	Authorizer Authorizer
}

// Execute answers the report the actor may read.
func (h GetHealthReport) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (health.Report, error) {
	if actor.HasScope(adminTenantsScope) {
		return h.Health.Report(ctx), nil
	}

	request := access.Request{
		// STRUCTURE, the permission a workspace administrator holds, for the reason the AI
		// provider's configuration is read under it: what this installation is currently able to
		// do is something somebody who shapes the workspace needs and an ordinary member does not.
		Permission: service.PermissionStructure,
		// A-4: an auditor reads what a workspace is configured to do without gaining the right to
		// change it, and "which features are degraded right now" is the same kind of question.
		Alternative: service.PermissionReadConfiguration,
		Path:        []identity.Scope{identity.TenantScope()},
		Action:      HealthReportReadAction,
		TokenScope:  opsReadScope,
		TargetType:  healthTarget,
	}
	if err := h.Authorizer.Authorize(ctx, actor, request); err != nil {
		return health.Report{}, err
	}
	return reduced(h.Health.Report(ctx)), nil
}

// reduced is the answer for a reader who is not the installation's operator.
//
// Built by naming what stays rather than by clearing what goes: a field added to the report next
// year is then absent from this answer until somebody decides it belongs, which is the safe
// direction for a struct that grows.
func reduced(report health.Report) health.Report {
	return health.Report{
		Status:           report.Status,
		Version:          report.Version,
		DegradedFeatures: report.DegradedFeatures,
	}
}

// Descriptor is the catalogue entry.
func (h GetHealthReport) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: GetHealthReportName,
		Summary: "Reads the installation's deep self-diagnosis: its status, its version and the " +
			"features it is currently not serving fully. A credential holding `admin:tenants` " +
			"reads the whole report as well - every dependency with its latency, last error code " +
			"and circuit state, the migration state, the backlogs and the configuration warnings.",
		SideEffects: "None. Reads only.",
		TokenScope:  opsReadScope,
		ReadOnly:    true,
		// Required is false and the action is still named: a successful read is not an auditable
		// event, and a refused one is - recorded by the authorisation service against this action.
		Audit: usecase.AuditDeclaration{
			Action: HealthReportReadAction, TargetType: healthTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the installation's health is not an entry's history (domain-model.md §3.5).",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h GetHealthReport) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	report, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	return healthOutput(report), nil
}

// healthOutput renders the report for the channels that speak `usecase.Output` - MCP and the rule
// engine. The REST adapter maps the report itself, because it has a hand-written wire format the
// contract test validates (presentation/rest/HealthReport.go).
//
// Absent rather than empty, exactly as the JSON is: a reader who received `dependencies: []` would
// conclude that nothing is monitored, which is a different statement from "you may not see this".
func healthOutput(report health.Report) usecase.Output {
	out := usecase.Output{"status": string(report.Status), "version": report.Version}

	degraded := make([]usecase.Output, 0, len(report.DegradedFeatures))
	for _, feature := range report.DegradedFeatures {
		degraded = append(degraded, usecase.Output{
			"feature":     feature.Feature,
			"reason_code": feature.ReasonCode,
			"since":       feature.Since.UTC(),
		})
	}
	out["degraded_features"] = degraded

	if len(report.Dependencies) == 0 {
		return out
	}

	dependencies := make([]usecase.Output, 0, len(report.Dependencies))
	for _, dependency := range report.Dependencies {
		entry := usecase.Output{
			"name":     dependency.Name,
			"required": dependency.Required,
			"status":   string(dependency.Status),
		}
		if dependency.ErrorCode != "" {
			entry["last_error_code"] = dependency.ErrorCode
		}
		if dependency.CircuitState != "" {
			entry["circuit_state"] = dependency.CircuitState
		}
		dependencies = append(dependencies, entry)
	}
	out["dependencies"] = dependencies
	out["migration"] = usecase.Output{
		"applied":  report.Migration.Applied,
		"expected": report.Migration.Expected,
		"status":   report.Migration.Status,
	}
	out["backlogs"] = usecase.Output{
		"outbox_pending":        report.Backlogs.OutboxPending,
		"outbox_lag_seconds":    report.Backlogs.OutboxLagSeconds,
		"job_queue_depth":       report.Backlogs.JobQueueDepth,
		"dead_letter_total":     report.Backlogs.DeadLetterTotal,
		"webhook_retry_backlog": report.Backlogs.WebhookRetryBacklog,
	}

	warnings := make([]usecase.Output, 0, len(report.Warnings))
	for _, warning := range report.Warnings {
		warnings = append(warnings, usecase.Output{
			"code": warning.Code, "severity": warning.Severity, "params": warning.Params,
		})
	}
	out["warnings"] = warnings
	return out
}
