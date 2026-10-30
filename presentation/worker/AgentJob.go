// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"

	agentservice "github.com/Jersyfi/hubtask/core/application/service/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// AgentJob is the queue's way into one job of a shipped agent (G11). Detached: the triage asks a
// provider between its reads, and the runner owns its transactions.
//
// A job that ends refused - the agent stopped or its budget spent between the start and the run -
// is ended by the runner with the refusal's code and finishes here; only a failure to read or
// write is retried.
type AgentJob struct {
	Runner agentservice.Runner
}

var (
	_ queue.Handler  = AgentJob{}
	_ queue.Detached = AgentJob{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for.
func (h AgentJob) OwnsItsTransactions() {}

// Run runs the job the payload names.
func (h AgentJob) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.ErrInternal.WithDetail("agents.job_without_tenant")
	}
	id, err := payloadID(job, "job_id")
	if err != nil {
		return queue.Result{}, err
	}
	ended, err := h.Runner.Run(ctx, job.TenantID, id)
	if err != nil {
		return queue.Result{}, err
	}
	return queue.Result{Outcome: ended.Code}, nil
}
