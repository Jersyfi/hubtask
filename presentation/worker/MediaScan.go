// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// MediaScan is the queue's way into the scan of uploads held while the scanner could not answer
// (queue.KindMediaScan). It repeats while anything stays held and ends once nothing does - the next
// held confirmation seeds it again.
//
// Detached (queue.Detached): the pass reads each file through the scanner between its writes, and
// a transaction held across that is what observability-reliability.md §8 forbids. Running a pass
// twice settles nothing twice: a verdict is written only on a row still held.
type MediaScan struct {
	Scan media.ScanHeldMedia
	// Retry is the wait while files stay held: the scanner is down, or there are more.
	Retry time.Duration
}

var (
	_ queue.Handler  = MediaScan{}
	_ queue.Detached = MediaScan{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for.
func (s MediaScan) OwnsItsTransactions() {}

// Run does one pass for the tenant the job names.
func (s MediaScan) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.ErrInternal.WithDetail("media.scan_without_tenant")
	}
	outcome, err := s.Scan.Execute(ctx, job.TenantID)
	if err != nil {
		return queue.Result{}, err
	}
	if outcome.Held > 0 {
		retry := s.Retry
		if retry <= 0 {
			retry = 5 * time.Minute
		}
		return queue.Result{Repeat: true, RepeatAfter: retry, Outcome: "held"}, nil
	}
	return queue.Result{Outcome: "settled"}, nil
}
