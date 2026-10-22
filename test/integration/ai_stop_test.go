// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	jobservice "github.com/Jersyfi/hubtask/core/application/service/job"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/job"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-AI-04 check 9 against PostgreSQL as the application role: the person who asked a question
// to AI watches and stops it wherever their role sits, as long as they may still read what it is
// about; nobody else gains anything; and another workspace's question is not there at all.

type stopScene struct {
	privateWorld
	// shared is an entry of the shared hub, hubOnly a person holding MEMBER on that hub alone.
	sharedEntry, hubOnly shared.ID
}

func seedStopScene(ctx context.Context, t *testing.T) stopScene {
	t.Helper()
	scene := stopScene{privateWorld: seedPrivateWorld(ctx, t), sharedEntry: freshID(t), hubOnly: freshID(t)}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, taskIn(tenantA, authorA, scene.sharedColl, scene.sharedEntry, "Tile the bathroom", "a0"))
	}); err != nil {
		t.Fatalf("seeding the shared entry: %v", err)
	}
	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Hanna Hub')`,
			[]any{scene.hubOnly.String(), tenantA.String()}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role)
		  VALUES ($1, $2, $3, 'HUB', $4, 'MEMBER')`,
			[]any{freshID(t).String(), tenantA.String(), scene.hubOnly.String(), scene.sharedHub.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding the hub's member: %v", err)
		}
	}
	return scene
}

// askedJob is a question to AI as the asking use case queues it: the kind, and a payload of
// references naming who asked about which entry.
func askedJob(ctx context.Context, t *testing.T, tenant, askedBy, entry shared.ID) shared.ID {
	t.Helper()
	id := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO job (id, tenant_id, kind, payload)
		VALUES ($1, $2, 'ai.suggest', jsonb_build_object(
		  'asked_by', $3::text, 'target_type', 'WORK_ITEM', 'target_id', $4::text,
		  'kind', 'FIELDS', 'prompt', 'suggest-item-fields'))`,
		id.String(), tenant.String(), askedBy.String(), entry.String()); err != nil {
		t.Fatalf("queueing the question: %v", err)
	}
	return id
}

func refusalsOf(ctx context.Context, t *testing.T, account shared.ID) int {
	t.Helper()
	var count int
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE actor_id = $1 AND outcome = 'DENIED'`, account.String(),
	).Scan(&count); err != nil {
		t.Fatalf("counting refusals: %v", err)
	}
	return count
}

func asAsker(tenant, account shared.ID) appshared.ActorContext {
	return actorIn(tenant, account, "jobs:read", "jobs:cancel", "items:read")
}

func TestThePersonWhoAskedWatchesAndStopsTheirQuestion(t *testing.T) {
	ctx := context.Background()
	scene := seedStopScene(ctx, t)
	harness := jobAcceptanceHarness(ctx, t)
	id := askedJob(ctx, t, tenantA, scene.hubOnly, scene.sharedEntry)

	// A role on one hub, and none at the workspace that GetJob and CancelJob otherwise ask.
	job, err := harness.read.Execute(ctx, asAsker(tenantA, scene.hubOnly), jobservice.Query{JobID: id})
	if err != nil || job.State != domain.StateQueued {
		t.Fatalf("the asker reading their question: %+v, %v", job, err)
	}
	stopped, err := harness.cancel.Execute(ctx, asAsker(tenantA, scene.hubOnly), jobservice.Query{JobID: id})
	if err != nil || stopped.State != domain.StateCancelled {
		t.Fatalf("the asker stopping their question: %+v, %v", stopped, err)
	}
	if denied := refusalsOf(ctx, t, scene.hubOnly); denied != 0 {
		t.Errorf("%d refusals were recorded for somebody let through", denied)
	}
}

func TestNobodyElseGainsAnythingFromAQuestion(t *testing.T) {
	ctx := context.Background()
	scene := seedStopScene(ctx, t)
	harness := jobAcceptanceHarness(ctx, t)

	// Another person of the workspace, who may write but not stop the workspace's work.
	theirs := askedJob(ctx, t, tenantA, scene.hubOnly, scene.sharedEntry)
	if _, err := harness.cancel.Execute(ctx, asAsker(tenantA, scene.member), jobservice.Query{JobID: theirs}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("another member stopping somebody's question: %v, want forbidden", err)
	}

	// The asker who cannot read what the question is about - an entry in a private hub they are
	// outside of - is no asker: the ordinary permission refuses them.
	unreadable := askedJob(ctx, t, tenantA, scene.hubOnly, scene.item)
	if _, err := harness.cancel.Execute(ctx, asAsker(tenantA, scene.hubOnly), jobservice.Query{JobID: unreadable}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("an asker without the read stopping: %v, want forbidden", err)
	}

	// The workspace's owner stops any work, as before.
	if stopped, err := harness.cancel.Execute(ctx, asAsker(tenantA, scene.administrator), jobservice.Query{JobID: theirs}); err != nil || stopped.State != domain.StateCancelled {
		t.Errorf("the owner stopping: %+v, %v", stopped, err)
	}
}

// SG-3 for the asker's read: the job query names its tenant itself, so a question of A's is
// nobody's in B - not even under the same account identifier.
func TestAQuestionIsNotThereNextDoor(t *testing.T) {
	ctx := context.Background()
	scene := seedStopScene(ctx, t)
	harness := jobAcceptanceHarness(ctx, t)
	id := askedJob(ctx, t, tenantA, scene.hubOnly, scene.sharedEntry)

	_, err := harness.cancel.Execute(ctx, asAsker(tenantB, scene.hubOnly), jobservice.Query{JobID: id})
	if err == nil || errors.Is(err, shared.ErrConflict) {
		t.Fatalf("B's caller stopping A's question: %v, want it refused", err)
	}
	if job, findErr := findJob(ctx, t, tenantA, id); findErr != nil || job.State != domain.StateQueued {
		t.Errorf("A's question after B's attempt: %+v, %v", job, findErr)
	}
}

// The fence a stopped question's answer meets (UC-AI-04 check 9), against the real queue: a claimed
// job its asker stopped is no longer the pass's to hold, so the transaction that would record its
// answer fails with queue.lease_lost and records nothing; a job still running is held, and a job
// stopped after the answer's transaction committed leaves that answer where it is.
func TestAStoppedQuestionIsNoLongerThePasssToHold(t *testing.T) {
	ctx := context.Background()
	id, claimed := claimOne(ctx, t)

	if err := systemWrite(ctx, t, func(ctx context.Context) error {
		return jobQueue(t).Hold(ctx, claimed)
	}); err != nil {
		t.Fatalf("holding a running job: %v", err)
	}

	// Stopped the way CancelJob stops it, past who may: that is the other tests' question.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := jobRepo().Cancel(ctx, id, queueClock.Now())
		return err
	}); err != nil {
		t.Fatalf("stopping the job: %v", err)
	}
	err := systemWrite(ctx, t, func(ctx context.Context) error {
		return jobQueue(t).Hold(ctx, claimed)
	})
	if got := shared.AsError(err); got == nil || got.DetailCode != "queue.lease_lost" {
		t.Fatalf("holding a stopped job answered %v, want queue.lease_lost", err)
	}
	if job, findErr := findJob(ctx, t, tenantA, id); findErr != nil || job.State != domain.StateCancelled {
		t.Errorf("the stopped job reads %+v, %v", job, findErr)
	}
}
