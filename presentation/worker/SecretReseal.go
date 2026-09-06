// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"log/slog"

	"github.com/Jersyfi/hubtask/core/application/service/sealing"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// SecretReseal is the queue's way into one workspace's re-sealing round (ADR-0045): the job the
// operator's request seeded, translated into one call on the application layer inside the
// transaction the runner opened for the tenant. Not detached - every write is a row of the
// tenant's, and a round that fails halfway rolls back whole and runs again.
type SecretReseal struct {
	Round sealing.RunReseal
}

var _ queue.Handler = SecretReseal{}

// Run does the round for the tenant the job names.
func (h SecretReseal) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.ErrInternal.WithDetail("sealing.reseal_without_tenant")
	}

	// The system acting for a tenant, at an operator's request: the audit entry says which.
	actor := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: job.TenantID}
	outcome, err := h.Round.Execute(ctx, actor)
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
