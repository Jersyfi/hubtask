// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	service "github.com/Jersyfi/hubtask/core/application/service/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// The two halves of #207 at this adapter: a job refused for a busy target comes back rather than
// reporting success over an archive it never wrote, and a job the queue gives up on closes the run
// row that holds the target's lock.

// stubTargets answers one target and panics on anything the paths under test never reach.
type stubTargets struct{ repository.Targets }

func (stubTargets) Find(context.Context, shared.ID) (domain.Target, error) {
	return domain.Target{ID: backupTargetID, Kind: domain.KindLocal}, nil
}

// busyRuns refuses the claim: another run holds the target.
type busyRuns struct{ repository.Runs }

func (busyRuns) Start(context.Context, domain.Run) (bool, error) { return false, nil }

// closingRuns records what Abandon closes.
type closingRuns struct {
	repository.Runs
	outcomes []domain.Outcome
}

func (r *closingRuns) Finish(_ context.Context, outcome domain.Outcome) error {
	r.outcomes = append(r.outcomes, outcome)
	return nil
}

// passthroughWork runs the function and pretends there was a transaction.
type passthroughWork struct{}

func (passthroughWork) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func (passthroughWork) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

var (
	backupTargetID = shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")
	backupRunID    = shared.MustParseID("0192f000-0000-7000-8000-0000000000b2")
	backupTenantID = shared.MustParseID("0192f000-0000-7000-8000-0000000000b3")
)

func backupRunJob() queue.Job {
	return queue.Job{
		ID: jobID, TenantID: backupTenantID, Kind: queue.KindBackupRun,
		Attempts: 1, MaxAttempts: 3, Lease: now.Add(time.Minute),
		Payload: map[string]any{
			"run_id":    backupRunID.String(),
			"target_id": backupTargetID.String(),
			"mode":      string(domain.ModeFull),
		},
	}
}

// A busy target is not a success: the job was asked for an archive and has not written one, so it
// comes back when the target should be free instead of reporting SUCCEEDED over nothing (#207).
func TestABusyTargetSendsTheJobBackRatherThanSucceeding(t *testing.T) {
	handler := BackupRun{Performer: service.Performer{
		Targets: stubTargets{}, Runs: busyRuns{},
		UnitOfWork: passthroughWork{}, Clock: clock.Fixed(now),
	}}

	result, err := handler.Run(context.Background(), backupRunJob())
	if err != nil {
		t.Fatalf("a busy target failed the job: %v", err)
	}
	if !result.Repeat {
		t.Fatal("a busy target completed the job although no archive was written")
	}
	if result.RepeatAfter != busyRetryDelay {
		t.Errorf("the job comes back after %v, want %v", result.RepeatAfter, busyRetryDelay)
	}
}

// The dead letter closes the run row, which is what frees the one-run-per-target lock (#207).
func TestReleaseClosesTheAbandonedRun(t *testing.T) {
	runs := &closingRuns{}
	handler := BackupRun{Performer: service.Performer{
		Runs: runs, UnitOfWork: passthroughWork{}, Clock: clock.Fixed(now),
	}}

	handler.Release(context.Background(), backupRunJob())

	if len(runs.outcomes) != 1 {
		t.Fatalf("%d outcomes written, want the abandoned run's", len(runs.outcomes))
	}
	outcome := runs.outcomes[0]
	if outcome.ID != backupRunID || outcome.Status != domain.RunFailed {
		t.Errorf("closed %s as %s", outcome.ID, outcome.Status)
	}
	if outcome.ErrorCode != domain.CodeRunAbandoned {
		t.Errorf("closed under %q, want %q", outcome.ErrorCode, domain.CodeRunAbandoned)
	}
}
