// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	lifecycleservice "github.com/Jersyfi/hubtask/core/application/service/lifecycle"
	privacyservice "github.com/Jersyfi/hubtask/core/application/service/privacy"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/text"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/presentation/worker"
)

// Releasing a hold carries out the rest of the erasure by itself, and the case records it
// (UC-PRV-03 check 12, data-protection.md §4.1): the release queues the job in its own transaction,
// the runner runs it in the tenant's scope, and what no hold keeps any more goes.

type remainderRig struct {
	ctx      context.Context
	t        *testing.T
	jobs     postgres.Queue
	registry *usecase.Registry
	subject  person
}

func newRemainderRig(t *testing.T) *remainderRig {
	t.Helper()
	ctx := context.Background()
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	jobs := postgres.NewQueue(ids, clockadapter.System{})
	holds := lifecycleservice.Holds{
		Holds:      postgres.NewLegalHoldRepository(),
		Remainders: privacyservice.ErasureRemainders{Kept: privacyRepo(), Jobs: jobs},
		Authorizer: permissive{}, Audit: postgres.NewAuditSink(generator{t}),
		UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)), Clock: portclock.Fixed(created.Add(time.Hour)),
		IDs: generator{t}, Text: text.Composing{},
	}
	registry, err := usecase.NewRegistry(nil, lifecycleservice.ReleaseLegalHold{Holds: holds}.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	return &remainderRig{
		ctx: ctx, t: t, jobs: jobs, registry: registry,
		subject: seedSubject(ctx, t, "rest-"+freshID(t).String()+"@example.test"),
	}
}

func (r *remainderRig) owner() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: r.subject.tenant, AccountID: freshID(r.t),
		AccountName: "Olga Owner", Scopes: []string{"retention:manage"},
	}
}

func (r *remainderRig) release(hold shared.ID) {
	r.t.Helper()
	if _, err := r.registry.Invoke(r.ctx, lifecycleservice.ReleaseLegalHoldName, r.owner(), usecase.Input{
		"hold_id": hold.String(), "reason": "The proceedings ended",
	}); err != nil {
		r.t.Fatalf("releasing the hold: %v", err)
	}
}

func (r *remainderRig) queued(request shared.ID) int {
	return countIn(r.ctx, r.t, `SELECT count(*) FROM job WHERE kind = 'privacy.erasure_remainder'
		AND payload->>'request_id' = $1`, request.String())
}

// runJobs runs the queue until every remainder job of the case has succeeded.
func (r *remainderRig) runJobs(request shared.ID) {
	r.t.Helper()
	runner := worker.Runner{
		Queue: r.jobs, UnitOfWork: postgres.NewUnitOfWork(appPool(r.ctx, r.t)), Clock: clockadapter.System{},
		Handlers: map[queue.Kind]queue.Handler{
			queue.KindPrivacyErasureRemainder: worker.PrivacyErasureRemainder{Resume: privacyservice.ResumeErasure{
				Requests: privacyRepo(), Kept: privacyRepo(), Eraser: eraserFor(r.t),
				UnitOfWork: postgres.NewUnitOfWork(appPool(r.ctx, r.t)), Clock: portclock.Fixed(created.Add(2 * time.Hour)),
			}},
		},
		Batch: 10, PollInterval: 50 * time.Millisecond, JobTimeout: 10 * time.Second,
		Lease: 30 * time.Second, NextAttempt: func(int) time.Duration { return 100 * time.Millisecond },
	}
	runCtx, stop := context.WithCancel(r.ctx)
	var running sync.WaitGroup
	running.Add(1)
	go func() {
		defer running.Done()
		runner.Run(runCtx)
	}()
	waitFor(r.t, 20*time.Second, "the remainder jobs to succeed", func() bool {
		return countIn(r.ctx, r.t, `SELECT count(*) FROM job WHERE kind = 'privacy.erasure_remainder'
			AND payload->>'request_id' = $1 AND state <> 'SUCCEEDED'`, request.String()) == 0
	})
	stop()
	running.Wait()
}

func (r *remainderRig) exists(query string, args ...any) bool {
	return countIn(r.ctx, r.t, query, args...) == 1
}

func TestReleasingAHoldCarriesOutTheRestOfTheErasure(t *testing.T) {
	r := newRemainderRig(t)
	s := r.subject
	hub := holdOn(r.ctx, t, s.tenant, "CONTAINER", s.hub)
	entry := holdOn(r.ctx, t, s.tenant, "ITEM", s.item)
	request := erasureRequest(r.ctx, t, s, domain.ModeFullDelete)
	if _, err := performerFor(t).Perform(r.ctx, privacyservice.PerformInput{
		RequestID: request.ID, TenantID: s.tenant,
	}); err != nil {
		t.Fatalf("carrying out the case: %v", err)
	}
	if !r.exists(`SELECT count(*) FROM comment WHERE id = $1`, s.comment.String()) {
		t.Fatal("the held comment went with the erasure")
	}

	// Both holds cover the task; the record names the older one, the hub's. Lifting it changes
	// nothing on the ground - the entry's hold still keeps it all - and the case's record moves
	// the part under the hold that still keeps it.
	if r.exists(`SELECT count(*) FROM erasure_kept WHERE request_id = $1 AND hold_id = $2`,
		request.ID.String(), entry.String()) {
		t.Error("the younger hold was named for a row the older one covers")
	}
	r.release(hub)
	if r.queued(request.ID) != 1 {
		t.Fatalf("%d remainder jobs were queued, want 1", r.queued(request.ID))
	}
	r.runJobs(request.ID)
	if !r.exists(`SELECT count(*) FROM comment WHERE id = $1`, s.comment.String()) {
		t.Error("the comment went while the entry's hold still keeps it")
	}
	if !r.exists(`SELECT count(*) FROM erasure_kept WHERE request_id = $1 AND hold_id = $2 AND erased_at IS NOT NULL`,
		request.ID.String(), hub.String()) {
		t.Error("the lifted hold's part is not marked done")
	}
	if !r.exists(`SELECT count(*) FROM erasure_kept WHERE request_id = $1 AND hold_id = $2 AND erased_at IS NULL`,
		request.ID.String(), entry.String()) {
		t.Error("the entry's hold does not keep the part the hub's held")
	}

	// Lifting the last one carries out the rest: the comment, the assignment, the account.
	r.release(entry)
	r.runJobs(request.ID)
	if r.exists(`SELECT count(*) FROM comment WHERE id = $1`, s.comment.String()) {
		t.Error("the comment survived the last hold")
	}
	if r.exists(`SELECT count(*) FROM account WHERE id = $1`, s.account.String()) {
		t.Error("the account survived the last hold of a full deletion")
	}
	if !r.exists(`SELECT count(*) FROM data_subject_request WHERE id = $1 AND subject_account_id IS NULL`,
		request.ID.String()) {
		t.Error("the case still names an account that is gone")
	}
	if countIn(r.ctx, t, `SELECT count(*) FROM erasure_kept WHERE request_id = $1 AND erased_at IS NULL`,
		request.ID.String()) != 0 {
		t.Error("the case still says something is kept")
	}
	if countIn(r.ctx, t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'dsr.remainder_erased'
		AND target_id = $2`, s.tenant.String(), request.ID.String()) != 2 {
		t.Error("the trail does not record each run of the rest")
	}
}

// A release that kept nothing queues nothing; a remainder nothing seeded is seeded by the pass.
func TestTheRemainderIsSeededOnlyWhereSomethingWasKept(t *testing.T) {
	r := newRemainderRig(t)
	s := r.subject
	unrelated := holdOn(r.ctx, t, s.tenant, "ITEM", freshID(t))
	hub := holdOn(r.ctx, t, s.tenant, "CONTAINER", s.hub)
	request := erasureRequest(r.ctx, t, s, domain.ModeAnonymize)
	if _, err := performerFor(t).Perform(r.ctx, privacyservice.PerformInput{
		RequestID: request.ID, TenantID: s.tenant,
	}); err != nil {
		t.Fatalf("carrying out the case: %v", err)
	}

	r.release(unrelated)
	if r.queued(request.ID) != 0 {
		t.Errorf("a hold that kept nothing queued %d jobs", r.queued(request.ID))
	}

	// The hub's hold is lifted behind this binary's back, as an older pod or a restore would.
	if _, err := adminPool(r.ctx, t).Exec(r.ctx,
		`UPDATE legal_hold SET released_at = now(), released_by = $2, released_reason = 'old pod' WHERE id = $1`,
		hub.String(), freshID(t).String()); err != nil {
		t.Fatalf("lifting behind its back: %v", err)
	}
	if err := write(r.ctx, t, s.tenant, func(ctx context.Context) error {
		active, err := postgres.NewLifecycleRepository().Active(ctx)
		if err != nil {
			return err
		}
		return privacyservice.ErasureRemainders{Kept: privacyRepo(), Jobs: r.jobs}.Reconcile(ctx, s.tenant, active)
	}); err != nil {
		t.Fatalf("reconciling: %v", err)
	}
	if r.queued(request.ID) != 1 {
		t.Fatalf("the pass queued %d jobs for a remainder nothing seeded, want 1", r.queued(request.ID))
	}
	r.runJobs(request.ID)
	if r.exists(`SELECT count(*) FROM work_item WHERE id = $1 AND assignee_id = $2`, s.item.String(), s.account.String()) {
		t.Error("the assignment survived the rest of the erasure")
	}
}

// The rest of a full deletion meets a rule the kept account has come to run: nothing goes, the case
// says why the rest waits, and the job does not retry into the same refusal.
func TestARemainderARuleStandsInTheWayOfWaitsAndSaysWhy(t *testing.T) {
	r := newRemainderRig(t)
	s := r.subject
	person := holdOn(r.ctx, t, s.tenant, "ACCOUNT", s.account)
	request := erasureRequest(r.ctx, t, s, domain.ModeFullDelete)
	if _, err := performerFor(t).Perform(r.ctx, privacyservice.PerformInput{
		RequestID: request.ID, TenantID: s.tenant,
	}); err != nil {
		t.Fatalf("carrying out the case: %v", err)
	}
	if _, err := adminPool(r.ctx, t).Exec(r.ctx, `
		INSERT INTO automation_rule (id, tenant_id, name, scope_type, run_as, trigger, actions, created_by)
		VALUES ($1, $2, 'later', 'TENANT', $3, '{}'::jsonb, '[]'::jsonb, $3)`,
		freshID(t).String(), s.tenant.String(), s.account.String()); err != nil {
		t.Fatalf("seeding the rule: %v", err)
	}

	r.release(person)
	r.runJobs(request.ID)
	if !r.exists(`SELECT count(*) FROM account WHERE id = $1`, s.account.String()) {
		t.Fatal("the account went although a rule acts as it")
	}
	if !r.exists(`SELECT count(*) FROM erasure_kept WHERE request_id = $1 AND erased_at IS NULL
		AND blocked_code = 'privacy.erasure_blocked_by_rule' AND blocked_params->>'rules' = '1'`,
		request.ID.String()) {
		t.Error("the case does not say why the rest waits")
	}
}
