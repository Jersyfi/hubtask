// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"log/slog"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// OwnDomainCheck runs one pass of the own domains' check and comes back (ADR-0095 §4).
//
// Detached (queue.Detached): the pass waits on name servers, and a transaction held open across a
// lookup is what observability-reliability.md §8 forbids. Each move is written in its workspace's
// own short transaction, guarded on the state it was read in, so a pass run twice moves nothing twice.
type OwnDomainCheck struct {
	Check adminservice.CheckOwnDomains
}

var (
	_ queue.Handler  = OwnDomainCheck{}
	_ queue.Detached = OwnDomainCheck{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for.
func (OwnDomainCheck) OwnsItsTransactions() {}

// Run checks one pass's worth of domains.
func (h OwnDomainCheck) Run(ctx context.Context, _ queue.Job) (queue.Result, error) {
	moved, err := h.Check.Run(ctx)
	if err != nil {
		return queue.Result{}, err
	}
	if moved > 0 {
		slog.InfoContext(ctx, "own domains changed state", slog.Int("count", moved))
	}
	return queue.Result{Repeat: true, RepeatAfter: adminservice.OwnDomainCheckInterval}, nil
}
