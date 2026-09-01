// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

var exportTarget = shared.ID("018f2a1b-0000-7000-8000-0000000000d1")

type exportLoadFake struct{ live int }

func (l *exportLoadFake) LiveExports(context.Context) (int, error) { return l.live, nil }

type exportFixture struct {
	handler ExportTenant
	tenants *tenantsStore
	load    *exportLoadFake
	jobs    *jobsFake
	audit   *auditSink
	work    *unitOfWork
}

func newExportFixture(status domain.TenantStatus) *exportFixture {
	f := &exportFixture{
		tenants: &tenantsStore{record: adminrepo.TenantRecord{
			ID: lifecycleTenant, Slug: "acme", DisplayName: "Acme GmbH", Status: status,
		}},
		load: &exportLoadFake{}, jobs: &jobsFake{}, audit: &auditSink{}, work: &unitOfWork{},
	}
	f.handler = ExportTenant{
		Tenants: f.tenants, Load: f.load, Jobs: f.jobs, Audit: f.audit,
		UnitOfWork: f.work, Clock: clock.Fixed(now), IDs: &sequentialIDs{},
		Tenancy: env.TenancyMulti,
	}
	return f
}

func exportCommand() ExportTenantCommand {
	return ExportTenantCommand{TenantID: lifecycleTenant, TargetID: exportTarget}
}

// Every lifecycle state exports - the suspended and the leaving are exactly who needs it (§5).
func TestEveryLifecycleStateExports(t *testing.T) {
	for _, status := range []domain.TenantStatus{
		domain.TenantActive, domain.TenantSuspended, domain.TenantPendingDeletion,
	} {
		t.Run(string(status), func(t *testing.T) {
			f := newExportFixture(status)

			accepted, err := f.handler.Execute(t.Context(), operator(), exportCommand())
			if err != nil {
				t.Fatalf("exporting a %s workspace: %v", status, err)
			}
			if accepted.JobID.IsZero() || accepted.ExportID.IsZero() {
				t.Errorf("accepted %+v", accepted)
			}

			if len(f.jobs.requests) != 1 {
				t.Fatalf("jobs %+v", f.jobs.requests)
			}
			job := f.jobs.requests[0]
			if job.Kind != queue.KindTenantExport || job.TenantID != lifecycleTenant {
				t.Errorf("job %+v", job)
			}
			if job.Payload["export_id"] != accepted.ExportID.String() ||
				job.Payload["target_id"] != exportTarget.String() {
				t.Errorf("payload %+v", job.Payload)
			}

			// Audited with its target, never its content.
			if len(f.audit.entries) != 1 || f.audit.entries[0].Action != TenantExportedAction ||
				f.audit.entries[0].Severity != audit.SeverityWarning {
				t.Fatalf("audit %+v", f.audit.entries)
			}
		})
	}
}

// The §4 quota: a workspace at its limit is told to wait, and no job is created.
func TestTheExportQuotaBoundsTheLiveJobs(t *testing.T) {
	f := newExportFixture(domain.TenantActive)
	f.load.live = exportQuotaMulti

	_, err := f.handler.Execute(t.Context(), operator(), exportCommand())

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "capacity.export_jobs" {
		t.Errorf("answer %v, want capacity.export_jobs", err)
	}
	if !errors.Is(err, shared.ErrRateLimited) {
		t.Errorf("category %v, want rate limited", err)
	}
	if len(f.jobs.requests) != 0 {
		t.Error("a job was created over the quota")
	}

	// Single mode allows more (multi-tenancy.md §4's two columns).
	f = newExportFixture(domain.TenantActive)
	f.handler.Tenancy = env.TenancySingle
	f.load.live = exportQuotaMulti
	if _, err := f.handler.Execute(t.Context(), operator(), exportCommand()); err != nil {
		t.Errorf("single mode refused below its own limit: %v", err)
	}
}

func TestTheExportDemandsATargetAndAKnownTenant(t *testing.T) {
	f := newExportFixture(domain.TenantActive)
	cmd := exportCommand()
	cmd.TargetID = ""
	var domainErr *shared.Error
	if _, err := f.handler.Execute(t.Context(), operator(), cmd); !errors.As(err, &domainErr) ||
		domainErr.DetailCode != "admin.export_target_required" {
		t.Errorf("answer %v, want the target refusal", err)
	}

	f.tenants.findErr = shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	if _, err := f.handler.Execute(t.Context(), operator(), exportCommand()); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("answer %v, want the indistinguishable not found", err)
	}

	actor := operator()
	actor.Scopes = nil
	if _, err := f.handler.Execute(t.Context(), actor, exportCommand()); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("answer %v, want forbidden", err)
	}
}

func TestTheArchiveNameStaysOutsideTheBackupPrefix(t *testing.T) {
	name := ExportArchiveName(lifecycleTenant, time.Date(2026, 9, 1, 8, 30, 0, 0, time.UTC))
	want := "hubtask-export-" + lifecycleTenant.String() + "-20260901T083000Z"
	if name != want {
		t.Errorf("name %q, want %q", name, want)
	}
}

func TestTheExportPayloadRoundTrips(t *testing.T) {
	request, err := ExportRequestOf(map[string]any{
		"export_id": exportTarget.String(), "target_id": lifecycleTenant.String(),
	}, lifecycleTenant)
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if request.ExportID != exportTarget || request.TargetID != lifecycleTenant ||
		request.TenantID != lifecycleTenant {
		t.Errorf("request %+v", request)
	}

	if _, err := ExportRequestOf(map[string]any{"export_id": "nope"}, lifecycleTenant); err == nil {
		t.Error("an unreadable payload was accepted")
	}
}
