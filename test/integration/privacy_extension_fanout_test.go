// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	auditrepository "github.com/Jersyfi/hubtask/core/application/repository/audit"
	privacyservice "github.com/Jersyfi/hubtask/core/application/service/privacy"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/presentation/worker"
)

// UC-PRV-01 check 10, the installation half: an installation-wide case extended by the operator is
// recorded in every workspace the person is a member of - through the runner, one transaction per
// workspace, in whatever state the workspace is - and a run that fails after its write leaves no
// second entry when it is retried.

// fanOutTenant seeds a workspace in which the address is a member.
func fanOutTenant(ctx context.Context, t *testing.T, email string) shared.ID {
	t.Helper()
	tenant := freshID(t)
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx,
		`INSERT INTO tenant (id, slug, display_name, default_time_zone) VALUES ($1, $2, 'Fan-out', 'Europe/Berlin')`,
		tenant.String(), slugOf(tenant)); err != nil {
		t.Fatalf("seeding the workspace: %v", err)
	}
	if _, err := admin.Exec(ctx,
		`INSERT INTO account (id, tenant_id, email, display_name) VALUES ($1, $2, $3, 'Anna Beispiel')`,
		freshID(t).String(), tenant.String(), email); err != nil {
		t.Fatalf("seeding the person: %v", err)
	}
	return tenant
}

// failingOnceAfterTheWrite lets the entry be written in one workspace and then fails that run, as
// a process dying between the write and the commit would.
type failingOnceAfterTheWrite struct {
	inner  queue.Handler
	tenant shared.ID
	failed *atomic.Bool
}

func (h failingOnceAfterTheWrite) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	result, err := h.inner.Run(ctx, job)
	if err != nil {
		return result, err
	}
	if job.TenantID == h.tenant && h.failed.CompareAndSwap(false, true) {
		return queue.Result{}, shared.ErrUnavailable.WithDetail("test.failed_after_the_write")
	}
	return result, nil
}

func TestAnInstallationWideExtensionIsRecordedInEveryWorkspaceOnce(t *testing.T) {
	ctx := context.Background()
	email := "fan-" + freshID(t).String() + "@example.org"
	own := fanOutTenant(ctx, t, email)
	active := fanOutTenant(ctx, t, email)
	suspended := fanOutTenant(ctx, t, email)
	leaving := fanOutTenant(ctx, t, email)
	gone := fanOutTenant(ctx, t, email)

	pool := appPool(ctx, t)
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	jobs := postgres.NewQueue(ids, clockadapter.System{})
	cases := privacyservice.Cases{
		Requests: privacyRepo(), Workspaces: postgres.NewWorkspaceSettingsRepository(),
		Subjects: privacyRepo(), Jobs: jobs, Authorizer: permissive{},
		Audit: postgres.NewAuditSink(generator{t}), UnitOfWork: postgres.NewUnitOfWork(pool),
		Clock: portclock.Fixed(extensionReceived), IDs: generator{t},
	}
	registry, err := usecase.NewRegistry(nil,
		privacyservice.CreateDataSubjectRequest{Cases: cases}.Descriptor(),
		privacyservice.ExtendDataSubjectRequest{Cases: cases}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	operator := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: own, AccountID: readerE, AccountName: "Olga Operator",
		Scopes: []string{"privacy:manage", "admin:tenants"},
	}

	recorded, err := registry.Invoke(ctx, privacyservice.CreateDataSubjectRequestName, operator, usecase.Input{
		"kind": string(domain.KindAccess), "scope": string(domain.ScopeInstallation), "subject_email": email,
	})
	if err != nil {
		t.Fatalf("recording: %v", err)
	}
	id := recorded.String("id")
	if _, err := registry.Invoke(ctx, privacyservice.ExtendDataSubjectRequestName, operator, usecase.Input{
		"request_id": id, "due_on": "2026-10-20", "reason": string(domain.ReasonComplexity),
		"informed_on": "2026-08-10",
	}); err != nil {
		t.Fatalf("extending: %v", err)
	}

	// One job per other workspace, none for the case's own.
	if queued := countIn(ctx, t,
		`SELECT count(*) FROM job WHERE kind = 'privacy.extension_entry' AND dedupe_key LIKE $1`,
		"dsr-extended:"+id+":%"); queued != 4 {
		t.Fatalf("%d entry jobs were queued, want 4", queued)
	}

	// A case without an address names nobody anywhere else, and queues nothing.
	unnamed, err := registry.Invoke(ctx, privacyservice.CreateDataSubjectRequestName, operator, usecase.Input{
		"kind": string(domain.KindAccess), "scope": string(domain.ScopeInstallation),
		"subject_account_id": freshID(t).String(),
	})
	if err != nil {
		t.Fatalf("recording without an address: %v", err)
	}
	if _, err := registry.Invoke(ctx, privacyservice.ExtendDataSubjectRequestName, operator, usecase.Input{
		"request_id": unnamed.String("id"), "due_on": "2026-10-20",
		"reason": string(domain.ReasonComplexity), "informed_on": "2026-08-10",
	}); err != nil {
		t.Fatalf("extending without an address: %v", err)
	}
	if queued := countIn(ctx, t, `SELECT count(*) FROM job WHERE dedupe_key LIKE $1`,
		"dsr-extended:"+unnamed.String("id")+":%"); queued != 0 {
		t.Errorf("a case without an address queued %d entry jobs", queued)
	}

	// The workspaces move before the jobs run: one suspended, one awaiting deletion, one gone.
	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql    string
		tenant shared.ID
	}{
		{`UPDATE tenant SET status = 'SUSPENDED' WHERE id = $1`, suspended},
		{`UPDATE tenant SET status = 'PENDING_DELETION', purge_after = now() + interval '30 days' WHERE id = $1`, leaving},
		{`DELETE FROM tenant WHERE id = $1`, gone},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.tenant.String()); err != nil {
			t.Fatalf("moving the workspace: %v", err)
		}
	}

	failed := &atomic.Bool{}
	runner := worker.Runner{
		Queue: jobs, UnitOfWork: postgres.NewUnitOfWork(pool), Clock: clockadapter.System{},
		Handlers: map[queue.Kind]queue.Handler{
			queue.KindPrivacyExtensionEntry: failingOnceAfterTheWrite{
				inner: worker.PrivacyExtensionEntry{Record: privacyservice.RecordExtensionEntry{
					Workspaces: postgres.NewWorkspaceSettingsRepository(),
					Audit:      postgres.NewAuditSink(generator{t}),
					Clock:      portclock.Fixed(extensionReceived.Add(time.Minute)),
				}},
				tenant: active, failed: failed,
			},
		},
		Batch: 10, PollInterval: 50 * time.Millisecond, JobTimeout: 5 * time.Second,
		Lease:       30 * time.Second,
		NextAttempt: func(int) time.Duration { return 100 * time.Millisecond },
	}
	runCtx, stop := context.WithCancel(ctx)
	var running sync.WaitGroup
	running.Add(1)
	go func() {
		defer running.Done()
		runner.Run(runCtx)
	}()
	waitFor(t, 20*time.Second, "every entry job to succeed", func() bool {
		return countIn(ctx, t,
			`SELECT count(*) FROM job WHERE kind = 'privacy.extension_entry' AND dedupe_key LIKE $1
			   AND state = 'SUCCEEDED'`, "dsr-extended:"+id+":%") == 4
	})
	stop()
	running.Wait()

	if !failed.Load() {
		t.Error("the run that should fail after its write never ran")
	}
	for name, c := range map[string]struct {
		tenant shared.ID
		want   int
	}{
		"the case's own workspace":      {own, 1},
		"an active workspace, retried":  {active, 1},
		"a suspended workspace":         {suspended, 1},
		"a workspace awaiting deletion": {leaving, 1},
		"a workspace that is gone":      {gone, 0},
	} {
		entries := countIn(ctx, t,
			`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'dsr.extended' AND target_id = $2`,
			c.tenant.String(), id)
		if entries != c.want {
			t.Errorf("%s holds %d entries, want %d", name, entries, c.want)
		}
	}

	// Each workspace's trail, read and verified under its own context: the entry names the
	// operator, and the chain over it holds - the rolled-back attempt left no gap.
	for _, tenant := range []shared.ID{active, suspended, leaving} {
		var label, reason string
		if err := admin.QueryRow(ctx,
			`SELECT actor_label, changes->'extension_reason'->>'to' FROM audit_log
			 WHERE tenant_id = $1 AND action = 'dsr.extended'`, tenant.String()).Scan(&label, &reason); err != nil {
			t.Fatalf("reading the entry: %v", err)
		}
		if label != "Olga Operator" || reason != "COMPLEXITY" {
			t.Errorf("the entry names %q for %q", label, reason)
		}
		verified, err := verifierFor(t).Execute(ctx, auditActor(tenant), auditrepository.Period{
			From: extensionReceived.Add(-time.Hour), To: extensionReceived.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("verifying: %v", err)
		}
		if !verified.Valid || verified.Checked != 1 {
			t.Errorf("the trail of %s reads %+v", tenant, verified)
		}
	}
}
