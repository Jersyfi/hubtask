// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	jobservice "github.com/Jersyfi/hubtask/core/application/service/job"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/job"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The sentences E-01 exists to make true, against real memberships and real rows: "show me" and
// "stop it" are different questions, and a job answers only to the tenant that asked for it.

var (
	jobViewer      = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000e1")
	jobViewerGrant = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000e2")
)

type jobAcceptance struct {
	read   jobservice.GetJob
	cancel jobservice.CancelJob
}

// seedJobRoles adds a viewer beside the administrator seedMemberships already grants: two roles at
// the tenant, which is where a job's permission is asked because a job is anchored to nothing.
func seedJobRoles(ctx context.Context, t *testing.T) {
	t.Helper()
	seedMemberships(ctx, t)
	admin := adminPool(ctx, t)

	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Cleo')
		ON CONFLICT (id) DO NOTHING`, jobViewer.String(), tenantA.String()); err != nil {
		t.Fatalf("seeding the viewer: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO membership (id, tenant_id, account_id, scope_type, role)
		VALUES ($1, $2, $3, 'TENANT', 'VIEWER') ON CONFLICT (id) DO NOTHING`,
		jobViewerGrant.String(), tenantA.String(), jobViewer.String()); err != nil {
		t.Fatalf("seeding the viewer's role: %v", err)
	}
}

func jobAcceptanceHarness(ctx context.Context, t *testing.T) jobAcceptance {
	t.Helper()

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(queueClock.Now())
	sink := postgres.NewAuditSink(clockadapter.NewUUIDv7(clockadapter.System{}))
	authorizer := access.Service{
		Memberships: postgres.NewMembershipRepository(),
		UnitOfWork:  unitOfWork, Audit: sink, Clock: fixed,
	}
	records := postgres.NewJobRepository()

	return jobAcceptance{
		read: jobservice.GetJob{
			Jobs: records, Authorizer: authorizer, UnitOfWork: unitOfWork,
		},
		cancel: jobservice.CancelJob{
			Jobs: records, Authorizer: authorizer, Audit: sink,
			Clock: fixed, UnitOfWork: unitOfWork,
		},
	}
}

func actorIn(tenant, account shared.ID, scopes ...string) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenant, AccountID: account,
		AccountName: "fixture", Scopes: scopes, Locale: "en", TimeZone: "UTC",
	}
}

// The distinct permission, proved where it is decided: a viewer may watch a job and may not stop
// it, and the administrator beside them may do both.
func TestAViewerMayWatchAJobAndMayNotStopIt(t *testing.T) {
	ctx := context.Background()
	seedJobRoles(ctx, t)
	harness := jobAcceptanceHarness(ctx, t)
	id := enqueueFor(ctx, t, tenantA, freshName(t))

	watcher := actorIn(tenantA, jobViewer, "jobs:read", "jobs:cancel")
	job, err := harness.read.Execute(ctx, watcher, jobservice.Query{JobID: id})
	if err != nil {
		t.Fatalf("the viewer could not read the job: %v", err)
	}
	if job.State != domain.StateQueued {
		t.Fatalf("state %q", job.State)
	}

	// The scope is held; the role is not. Both bounds have to allow it (ADR-0005), and this is the
	// one that refuses.
	if _, err := harness.cancel.Execute(ctx, watcher, jobservice.Query{JobID: id}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("the viewer stopped the job: %v", err)
	}

	administrator := actorIn(tenantA, authorA, "jobs:read", "jobs:cancel")
	cancelled, err := harness.cancel.Execute(ctx, administrator, jobservice.Query{JobID: id})
	if err != nil {
		t.Fatalf("the administrator could not stop the job: %v", err)
	}
	if cancelled.State != domain.StateCancelled {
		t.Fatalf("state %q after the administrator stopped it", cancelled.State)
	}
}

// The second bound, on its own: a role that allows it and a token that does not.
func TestATokenWithoutTheCancelScopeMayStillWatch(t *testing.T) {
	ctx := context.Background()
	seedJobRoles(ctx, t)
	harness := jobAcceptanceHarness(ctx, t)
	id := enqueueFor(ctx, t, tenantA, freshName(t))

	watching := actorIn(tenantA, authorA, "jobs:read")
	if _, err := harness.read.Execute(ctx, watching, jobservice.Query{JobID: id}); err != nil {
		t.Fatalf("a token scoped to read could not read: %v", err)
	}
	if _, err := harness.cancel.Execute(ctx, watching, jobservice.Query{JobID: id}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a token scoped to read stopped the job: %v", err)
	}
}

// The tenant boundary through the use case rather than through the statement, and the refusal a
// terminal job gets - the two answers a client acts on differently.
func TestAJobAnswersOnlyToItsOwnTenantAndOnlyWhileItIsRunning(t *testing.T) {
	ctx := context.Background()
	seedJobRoles(ctx, t)
	harness := jobAcceptanceHarness(ctx, t)
	id := enqueueFor(ctx, t, tenantA, freshName(t))

	stranger := actorIn(tenantB, authorB, "jobs:read", "jobs:cancel")
	if _, err := harness.read.Execute(ctx, stranger, jobservice.Query{JobID: id}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("another tenant's owner read the job: %v", err)
	}
	if _, err := harness.cancel.Execute(ctx, stranger, jobservice.Query{JobID: id}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("another tenant's owner stopped the job: %v", err)
	}

	administrator := actorIn(tenantA, authorA, "jobs:read", "jobs:cancel")
	if _, err := harness.cancel.Execute(ctx, administrator, jobservice.Query{JobID: id}); err != nil {
		t.Fatalf("stopping the job: %v", err)
	}

	_, err := harness.cancel.Execute(ctx, administrator, jobservice.Query{JobID: id})
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("stopping it twice: %v, want a conflict", err)
	}
	if code := shared.AsError(err).DetailCode; code != "jobs.already_finished" {
		t.Fatalf("detail code %q", code)
	}
}

// The entry an auditor is looking for after "why did the backup not run on Tuesday", written to
// the real trail rather than to a double.
func TestStoppingAJobIsInTheTrail(t *testing.T) {
	ctx := context.Background()
	seedJobRoles(ctx, t)
	harness := jobAcceptanceHarness(ctx, t)
	id := enqueueFor(ctx, t, tenantA, freshName(t))

	administrator := actorIn(tenantA, authorA, "jobs:read", "jobs:cancel")
	if _, err := harness.cancel.Execute(ctx, administrator, jobservice.Query{JobID: id}); err != nil {
		t.Fatalf("stopping the job: %v", err)
	}

	var actions, severity, outcome string
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT action, severity, outcome FROM audit_log
		WHERE tenant_id = $1 AND target_type = 'job' AND target_id = $2`,
		tenantA.String(), id.String()).Scan(&actions, &severity, &outcome); err != nil {
		t.Fatalf("reading the trail: %v", err)
	}
	switch {
	case actions != string(jobservice.JobCancelledAction):
		t.Errorf("action %q", actions)
	case severity != "NOTICE":
		t.Errorf("severity %q", severity)
	case outcome != "SUCCESS":
		t.Errorf("outcome %q", outcome)
	}
}
