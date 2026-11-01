// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package health

import (
	"context"
	"log/slog"
	"time"

	port "github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

const (
	// DeadLettersCode is the report's warning while jobs wait in the dead letter for an operator:
	// the notice an installation without Prometheus has instead of alert A-07.
	DeadLettersCode = "queue.dead_letters"

	// backlogsEvery is how old a count may be before a report starts the next one.
	backlogsEvery = time.Minute
	// backlogsTimeout bounds one count (rule 7).
	backlogsTimeout = 10 * time.Second
)

// BacklogReader counts the installation's queues: what is due, dead-lettered, waiting to be
// dispatched and parked for a retry. SampledAt is left to the registry.
type BacklogReader func(ctx context.Context) (port.Backlogs, error)

// SetBacklogs has the report serve the queues as last counted. Without it, no figures are served.
func (r *Registry) SetBacklogs(read BacklogReader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.backlogRead = read
	r.backlogs = port.Backlogs{}
}

// SampleBacklogs counts the queues now and keeps the count; a failed count keeps the last one.
// Called at start, and by a report finding the count stale.
func (r *Registry) SampleBacklogs(ctx context.Context) {
	r.mu.RLock()
	read := r.backlogRead
	r.mu.RUnlock()
	if read == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, backlogsTimeout)
	defer cancel()
	counted, err := read(ctx)
	if err != nil {
		slog.WarnContext(ctx, "the queues could not be counted for the health report",
			slog.String("error", err.Error()))
		return
	}
	counted.SampledAt = r.now()
	r.mu.Lock()
	r.backlogs = counted
	r.mu.Unlock()
}

// lastBacklogs answers the last count at once, and starts the next one in the background when it
// is stale: a report must answer even while the database is slow to count.
func (r *Registry) lastBacklogs(ctx context.Context) port.Backlogs {
	r.mu.Lock()
	defer r.mu.Unlock()
	last := r.backlogs
	if r.backlogRead != nil && !r.backlogsRunning &&
		(last.SampledAt.IsZero() || r.now().Sub(last.SampledAt) >= backlogsEvery) {
		r.backlogsRunning = true
		concurrency.Go(context.WithoutCancel(ctx), "health.backlogs", func(ctx context.Context) {
			r.SampleBacklogs(ctx)
			r.mu.Lock()
			r.backlogsRunning = false
			r.mu.Unlock()
		})
	}
	return last
}
