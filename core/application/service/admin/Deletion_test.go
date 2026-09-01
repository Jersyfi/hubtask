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
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/stepup"
)

type automationsFake struct {
	disabled int
	calls    int
}

func (a *automationsFake) DisableAll(context.Context, time.Time) (int, error) {
	a.calls++
	return a.disabled, nil
}

type jobsFake struct{ requests []queue.Request }

func (j *jobsFake) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	j.requests = append(j.requests, request)
	return shared.ID("018f2a1b-0000-7000-8000-0000000000f0"), nil
}

// stepUpFake proves whatever token it was configured to expect.
type stepUpFake struct {
	expect   string
	consumed int
}

func (s *stepUpFake) Available() bool { return true }

func (s *stepUpFake) Satisfied(_ context.Context, _ shared.ID, token string) (bool, error) {
	if token != s.expect {
		return false, nil
	}
	s.consumed++
	return true, nil
}

type deletionFixture struct {
	handler     RequestTenantDeletion
	tenants     *tenantsStore
	journal     *journalStore
	automations *automationsFake
	jobs        *jobsFake
	stepUp      *stepUpFake
	audit       *auditSink
	work        *unitOfWork
}

func newDeletionFixture(status domain.TenantStatus) *deletionFixture {
	f := &deletionFixture{
		tenants: &tenantsStore{
			record: adminrepo.TenantRecord{
				ID: lifecycleTenant, Slug: "acme", DisplayName: "Acme GmbH", Status: status,
			},
			moveOK: true,
		},
		journal: &journalStore{}, automations: &automationsFake{disabled: 3},
		jobs: &jobsFake{}, stepUp: &stepUpFake{expect: "hbt_sup_proof"},
		audit: &auditSink{}, work: &unitOfWork{},
	}
	f.tenants.deleteOK = true
	f.handler = RequestTenantDeletion{
		Tenants: f.tenants, Journal: f.journal, Automations: f.automations,
		Jobs: f.jobs, StepUp: f.stepUp, Audit: f.audit,
		UnitOfWork: f.work, Clock: clock.Fixed(now), IDs: &sequentialIDs{},
	}
	return f
}

func deletionCommand() RequestTenantDeletionCommand {
	return RequestTenantDeletionCommand{
		TenantID: lifecycleTenant, Confirmation: "Acme GmbH", StepUpToken: "hbt_sup_proof",
	}
}

func TestTheDeletionRequestSchedulesTheWholeSection5Phase(t *testing.T) {
	f := newDeletionFixture(domain.TenantActive)

	scheduled, err := f.handler.Execute(t.Context(), operator(), deletionCommand())
	if err != nil {
		t.Fatalf("requesting: %v", err)
	}

	wantPurge := now.Add(DeletionGrace).UTC()
	if !scheduled.PurgeAfter.Equal(wantPurge) {
		t.Errorf("purge after %v, want %v", scheduled.PurgeAfter, wantPurge)
	}
	if len(f.tenants.deletions) != 1 || !f.tenants.deletions[0].Equal(wantPurge) {
		t.Errorf("deletion writes %v", f.tenants.deletions)
	}
	if f.automations.calls != 1 {
		t.Error("the automations were not disabled")
	}
	if f.stepUp.consumed != 1 {
		t.Error("the step-up proof was not consumed")
	}

	// Two jobs, seeded by the request's own write (decision 6): the grace job at the deadline,
	// and the tenant's backup poller pulled forward for the final archive.
	if len(f.jobs.requests) != 2 {
		t.Fatalf("%d jobs, want 2: %+v", len(f.jobs.requests), f.jobs.requests)
	}
	grace, wake := f.jobs.requests[0], f.jobs.requests[1]
	if grace.Kind != queue.KindTenantHardDelete || !grace.RunAt.Equal(wantPurge) ||
		grace.TenantID != lifecycleTenant || grace.DedupeKey != lifecycleTenant.String() {
		t.Errorf("grace job %+v", grace)
	}
	if wake.Kind != queue.KindBackupSchedule || wake.TenantID != lifecycleTenant {
		t.Errorf("backup wake %+v", wake)
	}

	if len(f.audit.entries) != 1 || f.audit.entries[0].Action != TenantDeletionRequestedAction ||
		f.audit.entries[0].Severity != audit.SeverityCritical {
		t.Errorf("audit %+v", f.audit.entries)
	}
	if len(f.journal.entries) != 1 ||
		f.journal.entries[0].Details["automations_disabled"] != 3 {
		t.Errorf("journal %+v", f.journal.entries)
	}
}

// A suspended workspace may leave too: §5's one edge with two origins.
func TestASuspendedWorkspaceMayRequestDeletion(t *testing.T) {
	f := newDeletionFixture(domain.TenantSuspended)

	if _, err := f.handler.Execute(t.Context(), operator(), deletionCommand()); err != nil {
		t.Fatalf("requesting from suspension: %v", err)
	}
}

func TestTheTypedNameGuardsTheDeletion(t *testing.T) {
	f := newDeletionFixture(domain.TenantActive)
	cmd := deletionCommand()
	cmd.Confirmation = "acme gmbh"

	_, err := f.handler.Execute(t.Context(), operator(), cmd)

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "admin.deletion_confirmation_required" {
		t.Errorf("answer %v, want the confirmation refusal", err)
	}
	// A mistyped name must not burn a step-up the operator then has to earn again.
	if f.stepUp.consumed != 0 {
		t.Error("the proof was consumed for a mistyped name")
	}
	if len(f.jobs.requests) != 0 || len(f.tenants.deletions) != 0 {
		t.Error("something was written for a mistyped name")
	}
}

func TestTheDeletionDemandsAFreshStepUp(t *testing.T) {
	f := newDeletionFixture(domain.TenantActive)
	cmd := deletionCommand()
	cmd.StepUpToken = "hbt_sup_stale"

	_, err := f.handler.Execute(t.Context(), operator(), cmd)

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != stepup.CodeRequired {
		t.Errorf("answer %v, want %s", err, stepup.CodeRequired)
	}
	if len(f.jobs.requests) != 0 {
		t.Error("a job was seeded without the proof")
	}
}

func TestASecondDeletionRequestReportsTheStandingOne(t *testing.T) {
	f := newDeletionFixture(domain.TenantPendingDeletion)

	_, err := f.handler.Execute(t.Context(), operator(), deletionCommand())

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "admin.tenant_leaving" {
		t.Errorf("answer %v, want admin.tenant_leaving", err)
	}
}

func TestTheDeletionDemandsTheAdminScope(t *testing.T) {
	f := newDeletionFixture(domain.TenantActive)
	actor := operator()
	actor.Scopes = nil

	if _, err := f.handler.Execute(t.Context(), actor, deletionCommand()); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("answer %v, want forbidden", err)
	}
}
