// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
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

func (s *stepUpFake) Methods(_ context.Context, _, _ shared.ID) ([]stepup.Method, error) {
	return []stepup.Method{stepup.MethodPassword}, nil
}

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
		Jobs: f.jobs, StepUp: f.stepUp, Audit: f.audit, Holds: &holdsDouble{},
		UnitOfWork: f.work, Clock: clock.Fixed(now), IDs: &sequentialIDs{},
		Tenancy: env.TenancyMulti,
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

	// Two jobs, seeded by the request's own write (multi-tenancy.md §2.1): the grace job at the deadline,
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

// platformRegister answers the register's service-account rows; an empty-register owner, or a
// registered person, is not among them.
type platformRegister struct {
	rows   map[shared.ID]bool
	broken bool
	asked  int
}

func (r *platformRegister) RegisteredServiceAccount(_ context.Context, accountID shared.ID) (bool, error) {
	r.asked++
	if r.broken {
		return false, shared.ErrUnavailable.WithDetail("postgres.query_failed")
	}
	return r.rows[accountID], nil
}

const platformID = shared.ID("018f2a1b-0000-7000-8000-0000000000c7")

// platform is a purchase platform's service account, its token carrying the narrow scope.
func platform() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorServiceAccount, TenantID: operatorHome, AccountID: platformID,
		AccountName: "Shop", Scopes: []string{provisioningScope},
	}
}

func platformCommand() RequestTenantDeletionCommand {
	cmd := deletionCommand()
	cmd.StepUpToken = ""
	return cmd
}

// UC-INS-08 check 7: a service account the register names requests the deletion with the typed name
// and no step-up, and the request is the same §5 phase a person's is - the 30 days, the automations
// off, the final backups, the trail and the journal with the platform as the actor.
func TestARegisteredPlatformRequestsADeletionWithoutAStepUp(t *testing.T) {
	f := newDeletionFixture(domain.TenantActive)
	f.handler.Register = &platformRegister{rows: map[shared.ID]bool{platformID: true}}

	scheduled, err := f.handler.Execute(t.Context(), platform(), platformCommand())
	if err != nil {
		t.Fatalf("the platform's request was refused: %v", err)
	}
	if want := now.Add(DeletionGrace).UTC(); !scheduled.PurgeAfter.Equal(want) {
		t.Errorf("purge after %v, want the 30-day grace %v", scheduled.PurgeAfter, want)
	}
	if f.stepUp.consumed != 0 {
		t.Error("a proof was consumed for the platform")
	}
	if f.automations.calls != 1 || len(f.jobs.requests) != 2 {
		t.Errorf("automations disabled %d times, %d jobs", f.automations.calls, len(f.jobs.requests))
	}
	if len(f.audit.entries) != 1 || f.audit.entries[0].ActorID != platformID ||
		f.audit.entries[0].ActorKind != appshared.ActorServiceAccount {
		t.Errorf("the trail holds %+v", f.audit.entries)
	}
	if len(f.journal.entries) != 1 || f.journal.entries[0].ActorLabel != "Shop" {
		t.Errorf("the journal holds %+v", f.journal.entries)
	}
}

// Everybody else still proves themselves: a person in the register, an agent, a service account the
// register does not name by its own row (the empty register's owner among them), and any service
// account where the deletion is wired without a register. An unreadable register refuses.
func TestTheStepUpIsWaivedForNobodyElse(t *testing.T) {
	agent := platform()
	agent.Kind = appshared.ActorAIAgent
	agent.Scopes = append(agent.Scopes, "agent:destructive")
	person := operator()
	person.AccountID = platformID

	cases := []struct {
		name     string
		actor    appshared.ActorContext
		register *platformRegister
		want     string
	}{
		{"a registered person", person, &platformRegister{rows: map[shared.ID]bool{platformID: true}}, stepup.CodeRequired},
		{"an agent", agent, &platformRegister{rows: map[shared.ID]bool{platformID: true}}, stepup.CodeRequired},
		{"a service account the register does not name", platform(), &platformRegister{}, stepup.CodeRequired},
		{"no register wired", platform(), nil, stepup.CodeRequired},
		{"the register unreadable", platform(), &platformRegister{broken: true}, "postgres.query_failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDeletionFixture(domain.TenantActive)
			if tc.register != nil {
				f.handler.Register = tc.register
			}
			_, err := f.handler.Execute(t.Context(), tc.actor, platformCommand())
			if shared.AsError(err) == nil || shared.AsError(err).DetailCode != tc.want {
				t.Fatalf("answered %v, want %s", err, tc.want)
			}
			if len(f.tenants.deletions) != 0 || len(f.jobs.requests) != 0 || len(f.audit.entries) != 0 {
				t.Error("something was written without a proof")
			}
		})
	}
}

// The platform's request keeps every guard but the proof: the typed name, the legal hold, a request
// already standing.
func TestThePlatformsRequestKeepsTheNameAndTheHold(t *testing.T) {
	register := &platformRegister{rows: map[shared.ID]bool{platformID: true}}

	f := newDeletionFixture(domain.TenantActive)
	f.handler.Register = register
	cmd := platformCommand()
	cmd.Confirmation = "acme"
	if _, err := f.handler.Execute(t.Context(), platform(), cmd); shared.AsError(err) == nil ||
		shared.AsError(err).DetailCode != "admin.deletion_confirmation_required" {
		t.Errorf("a mistyped name answered %v", err)
	}

	f = newDeletionFixture(domain.TenantActive)
	f.handler.Register = register
	f.handler.Holds = &holdsDouble{inForce: anItemHold()}
	if _, err := f.handler.Execute(t.Context(), platform(), platformCommand()); shared.AsError(err) == nil ||
		shared.AsError(err).DetailCode != lifecycle.CodeLegalHold {
		t.Errorf("a workspace under a hold answered %v", err)
	}
	if len(f.tenants.deletions) != 0 {
		t.Error("a held workspace was moved")
	}

	f = newDeletionFixture(domain.TenantPendingDeletion)
	f.handler.Register = register
	if _, err := f.handler.Execute(t.Context(), platform(), platformCommand()); shared.AsError(err) == nil ||
		shared.AsError(err).DetailCode != "admin.tenant_leaving" {
		t.Errorf("a second request answered %v", err)
	}
}
