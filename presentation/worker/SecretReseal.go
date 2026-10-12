// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"log/slog"

	"github.com/Jersyfi/hubtask/core/application/service/sealing"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// SecretReseal is the queue's way into one workspace's re-sealing round (ADR-0045): the job the
// operator's request seeded, translated into one call on the application layer inside the
// transaction the runner opened for the tenant. Not detached - every write is a row of the
// tenant's, and a round that fails halfway rolls back whole and runs again.
//
// A job without a tenant is the installation's own round (UC-INS-16 check 2): the runner opened it
// under the scope with none, and it moves the secrets that belong to no workspace.
type SecretReseal struct {
	Round        sealing.RunReseal
	Installation sealing.RunInstallationReseal
}

var _ queue.Handler = SecretReseal{}

// Run does the round for the tenant the job names.
func (h SecretReseal) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	var (
		outcome sealing.Outcome
		err     error
	)
	if job.TenantID.IsZero() {
		outcome, err = h.Installation.Execute(ctx)
	} else {
		// The system acting for a tenant, at an operator's request: the audit entry says which.
		outcome, err = h.Round.Execute(ctx,
			appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: job.TenantID})
	}
	if err != nil {
		return queue.Result{}, err
	}

	// Counts only (rule 10): what moved and what could not, never which value or which key.
	slog.InfoContext(ctx, "re-sealing round finished",
		slog.Int64("rewrapped", outcome.Rewrapped),
		slog.Int64("skipped", outcome.Skipped),
	)
	return queue.Result{}, nil
}
