// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// AiEmbedding keeps one workspace's vectors in step with its entries (J-10).
//
// The retention sweep's shape rather than the suggestion's: a batch, then a decision about whether
// to come straight back, and a job that is never finished for good - the next entry somebody
// renames is always coming, and a row that removed itself would leave the workspace with no pass
// until its next write.
//
// Detached, because the provider call inside it reaches somebody else's machine and a transaction
// held across that is what observability-reliability.md §8 forbids. The pass owns its two
// transactions: one to read what is owed, one to write what came back.
//
// **It stops rather than fails when there is nothing to do**, and there are three ways to have
// nothing to do: no embedding store, no provider that can embed, and nothing owed. Semantic search
// is optional at every one of those levels, and a handler that treated any of them as an error
// would fill a dead letter queue with a feature somebody switched off.
type AiEmbedding struct {
	Embed work.EmbedItems
	// Interval is the wait after a pass that found nothing left. What a quiet workspace pays for
	// having the machinery at all.
	Interval time.Duration
	// Continuation is the wait after a pass that filled its batch: there is known work left, and
	// the only reason not to do it now is that a batch is where one provider call ends.
	Continuation time.Duration
}

var (
	_ queue.Handler  = AiEmbedding{}
	_ queue.Detached = AiEmbedding{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for.
func (h AiEmbedding) OwnsItsTransactions() {}

// Run embeds one batch for the tenant the job names.
func (h AiEmbedding) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.ErrInternal.WithDetail("search.embedding_without_tenant")
	}

	// The system acting for a workspace: a tenant, and no account. An embedding is the
	// installation's own bookkeeping about entries that already exist, not anybody's act - which
	// is also why the read it performs is not narrowed to one person's visibility. It reads the
	// workspace's entries because it is the workspace's index.
	actor := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: job.TenantID}

	outcome, err := h.Embed.Execute(ctx, actor)
	if err != nil {
		return queue.Result{}, err
	}

	after := h.Interval
	if outcome.Exhausted {
		after = h.Continuation
	}
	return queue.Result{Repeat: true, RepeatAfter: after}, nil
}
