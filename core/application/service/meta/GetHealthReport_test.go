// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package meta

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domainshared "github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	health "github.com/Jersyfi/hubtask/core/port/health"
)

// reporter is the health registry as this use case sees it.
type reporter struct {
	report health.Report
	asked  int
}

func (r *reporter) Report(context.Context) health.Report {
	r.asked++
	return r.report
}

// permissive answers yes and records what it was asked, which is the half of the test that proves
// the question was the right one.
type permissive struct{ requests []access.Request }

func (p *permissive) Authorize(
	_ context.Context, _ appshared.ActorContext, request access.Request,
) error {
	p.requests = append(p.requests, request)
	return nil
}

// refusing is the workspace administrator's authorisation answering no.
type refusing struct{}

func (refusing) Authorize(context.Context, appshared.ActorContext, access.Request) error {
	return domainshared.ErrForbidden.WithDetail("access.denied")
}

func fullReport() health.Report {
	return health.Report{
		Status:    health.StatusDegraded,
		Version:   "0.7.5",
		Roles:     []string{"api"},
		Migration: health.MigrationState{Applied: 77, Expected: 77, Status: "ok"},
		Dependencies: []health.DependencyReport{
			{Name: "postgres", Required: true, Result: health.Result{Status: health.StatusOK}},
			{
				Name:   "object_storage",
				Result: health.Result{Status: health.StatusDown, ErrorCode: "storage.unreachable"},
			},
		},
		DegradedFeatures: []health.DegradedFeature{
			{
				Feature:    "media",
				ReasonCode: "dependency.unavailable",
				Since:      time.Unix(1_755_000_000, 0).UTC(),
			},
		},
		Backlogs: health.Backlogs{OutboxPending: 12, JobQueueDepth: 3},
		Warnings: []health.Warning{{Code: "config.backup_not_configured", Severity: "warn"}},
	}
}

func operator() appshared.ActorContext {
	return appshared.ActorContext{
		AccountID: domainshared.ID("018f2a1b-0000-7000-8000-00000000000f"),
		Scopes:    []string{adminTenantsScope},
	}
}

func administrator() appshared.ActorContext {
	return appshared.ActorContext{
		AccountID: domainshared.ID("018f2a1b-0000-7000-8000-00000000000e"),
		TenantID:  tenant,
		Scopes:    []string{opsReadScope},
	}
}

// The operator's answer is the whole report, and no permission is asked for: no membership in any
// workspace can answer for an installation, so the credential is the bound.
func TestTheOperatorReadsTheWholeReport(t *testing.T) {
	authorizer := &permissive{}
	handler := GetHealthReport{Health: &reporter{report: fullReport()}, Authorizer: authorizer}

	report, err := handler.Execute(context.Background(), operator())
	if err != nil {
		t.Fatalf("the operator was refused: %v", err)
	}

	if len(report.Dependencies) != 2 {
		t.Errorf("dependencies = %d, want 2", len(report.Dependencies))
	}
	if report.Backlogs.OutboxPending != 12 {
		t.Errorf("the backlogs are missing: %+v", report.Backlogs)
	}
	if len(report.Warnings) != 1 {
		t.Errorf("warnings = %d, want 1", len(report.Warnings))
	}
	if report.Migration.Applied != 77 {
		t.Errorf("the migration state is missing: %+v", report.Migration)
	}
	if len(authorizer.requests) != 0 {
		t.Errorf("a permission was asked for the installation's own credential: %+v",
			authorizer.requests)
	}
}

// The other reader gets what a banner needs and nothing that describes the installation. This is
// the whole of K-06's decision, so it is asserted field by field rather than by a length.
func TestAWorkspaceAdministratorReadsOnlyTheStatusAndWhatIsDegraded(t *testing.T) {
	authorizer := &permissive{}
	handler := GetHealthReport{Health: &reporter{report: fullReport()}, Authorizer: authorizer}

	report, err := handler.Execute(context.Background(), administrator())
	if err != nil {
		t.Fatalf("the administrator was refused: %v", err)
	}

	if report.Status != health.StatusDegraded {
		t.Errorf("status = %q, want degraded", report.Status)
	}
	if report.Version != "0.7.5" {
		t.Errorf("version = %q", report.Version)
	}
	if len(report.DegradedFeatures) != 1 || report.DegradedFeatures[0].Feature != "media" {
		t.Errorf("degraded_features = %+v", report.DegradedFeatures)
	}
	if report.Dependencies != nil {
		t.Errorf("the dependency names crossed the boundary: %+v", report.Dependencies)
	}
	if report.Warnings != nil {
		t.Errorf("the configuration warnings crossed the boundary: %+v", report.Warnings)
	}
	if report.Roles != nil {
		t.Errorf("the process roles crossed the boundary: %+v", report.Roles)
	}
	if report.Backlogs != (health.Backlogs{}) {
		t.Errorf("the backlogs crossed the boundary: %+v", report.Backlogs)
	}
	if report.Migration != (health.MigrationState{}) {
		t.Errorf("the migration state crossed the boundary: %+v", report.Migration)
	}
}

// The question asked of the authorisation service is the one the decision describes: a workspace
// administrator's permission, the auditor's read-only alternative, and the scope a session can
// carry.
func TestTheReducedAnswerAsksForTheAdministratorsPermission(t *testing.T) {
	authorizer := &permissive{}
	handler := GetHealthReport{Health: &reporter{report: fullReport()}, Authorizer: authorizer}

	if _, err := handler.Execute(context.Background(), administrator()); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if len(authorizer.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(authorizer.requests))
	}
	request := authorizer.requests[0]
	if request.Permission != service.PermissionStructure {
		t.Errorf("permission = %q", request.Permission)
	}
	if request.Alternative != service.PermissionReadConfiguration {
		t.Errorf("alternative = %q, want the auditor's read-only permission (A-4)",
			request.Alternative)
	}
	if request.TokenScope != opsReadScope {
		t.Errorf("token scope = %q, want %q", request.TokenScope, opsReadScope)
	}
	if request.Action != HealthReportReadAction {
		t.Errorf("action = %q", request.Action)
	}
}

// A reader who is neither is refused, and nothing about the installation is answered on the way.
func TestAMemberIsRefusedAndLearnsNothing(t *testing.T) {
	health := &reporter{report: fullReport()}
	handler := GetHealthReport{Health: health, Authorizer: refusing{}}

	report, err := handler.Execute(context.Background(), administrator())
	if err == nil {
		t.Fatal("a refused reader received a report")
	}
	if report.Status != "" || report.Version != "" {
		t.Errorf("a refusal carried part of the report: %+v", report)
	}
	if health.asked != 0 {
		t.Errorf("the registry was probed for a reader who may not read it (%d times)", health.asked)
	}
}

// The scope the reduced answer needs must not begin with `admin:`, because `catalogue.SessionScopes`
// filters on that prefix and a session is exactly the reader this answer exists for. Asserted here
// rather than left to the catalogue's test: the name is this file's decision.
func TestTheReadScopeCanBeCarriedByASession(t *testing.T) {
	if len(opsReadScope) >= 6 && opsReadScope[:6] == "admin:" {
		t.Errorf("%q is filtered out of every session's scopes (0.6.0 decision 6)", opsReadScope)
	}
}

// The catalogue entry is what makes the route reachable, and a round trip through it is what proves
// the handler is wired to the descriptor rather than only to the test above.
func TestTheDescriptorAnswersThroughTheRegistry(t *testing.T) {
	handler := GetHealthReport{Health: &reporter{report: fullReport()}, Authorizer: &permissive{}}
	registry, err := usecase.NewRegistry(nil, handler.Descriptor())
	if err != nil {
		t.Fatalf("the descriptor is not registrable: %v", err)
	}

	out, err := registry.Invoke(
		context.Background(), GetHealthReportName, operator(), usecase.Input{})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if out["status"] != string(health.StatusDegraded) {
		t.Errorf("status = %v", out["status"])
	}
	if _, present := out["dependencies"]; !present {
		t.Error("the operator's answer carries no dependencies")
	}

	reduced, err := registry.Invoke(
		context.Background(), GetHealthReportName, administrator(), usecase.Input{})
	if err != nil {
		t.Fatalf("invoke as an administrator: %v", err)
	}
	if _, present := reduced["dependencies"]; present {
		t.Error("the reduced answer carries dependencies")
	}
	if _, present := reduced["warnings"]; present {
		t.Error("the reduced answer carries configuration warnings")
	}
	if _, present := reduced["degraded_features"]; !present {
		t.Error("the reduced answer carries no degraded features, which is all it is for")
	}
}

// The descriptor says what it is, and two of its declarations matter enough to assert: a read is
// not an auditable event, and a route a banner polls must not write a trail entry per poll.
func TestTheDescriptorRecordsARefusalAndNotAPoll(t *testing.T) {
	descriptor := GetHealthReport{}.Descriptor()

	if !descriptor.ReadOnly {
		t.Error("the deep report is declared as something other than a read")
	}
	if descriptor.Audit.Required {
		t.Error("a successful read writes an audit entry, which a polled route would flood")
	}
	if descriptor.Audit.Action != HealthReportReadAction {
		t.Errorf("a refusal would be recorded against %q", descriptor.Audit.Action)
	}
	if descriptor.TokenScope != opsReadScope {
		t.Errorf("token scope = %q", descriptor.TokenScope)
	}
	if descriptor.RESTOperation() != "getHealthReport" {
		t.Errorf("the REST operation is %q, and the contract declares getHealthReport",
			descriptor.RESTOperation())
	}
}
