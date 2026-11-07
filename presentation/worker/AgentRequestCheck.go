// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"log/slog"

	agentservice "github.com/Jersyfi/hubtask/core/application/service/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// AgentRequestCheck is the queue's way into the check of one approved held act after its perform
// window. Detached: the check opens its own tenant transaction, and running it twice changes
// nothing the first run did not.
type AgentRequestCheck struct {
	Check agentservice.RequestCheck
}

var (
	_ queue.Handler  = AgentRequestCheck{}
	_ queue.Detached = AgentRequestCheck{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for.
func (AgentRequestCheck) OwnsItsTransactions() {}

// Run checks the request the payload names.
func (h AgentRequestCheck) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.ErrInternal.WithDetail("agents.job_without_tenant")
	}
	id, err := payloadID(job, "request_id")
	if err != nil {
		return queue.Result{}, err
	}
	interrupted, err := h.Check.Run(ctx, job.TenantID, id)
	if err != nil {
		return queue.Result{}, err
	}
	if interrupted {
		slog.WarnContext(ctx, "an approved agent act was interrupted before it answered; recorded failed",
			slog.String("request_id", id.String()))
		return queue.Result{Outcome: agentservice.OutcomeRequestInterrupted}, nil
	}
	return queue.Result{}, nil
}
