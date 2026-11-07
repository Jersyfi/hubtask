// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	health "github.com/Jersyfi/hubtask/core/port/health"
)

// probeTimeout bounds the health query. Short on purpose: a probe that waits is a probe that
// turns a slow database into an unresponsive pod, because Kubernetes has its own patience and
// it is shorter than PostgreSQL's (ADR-0016).
const probeTimeout = 2 * time.Second

// probeAcquireBudget is how long the probe waits for a connection of a pool every one of whose
// connections is in use, before it calls the database busy rather than down.
const probeAcquireBudget = time.Second

// Probe reports whether PostgreSQL can serve queries. It is the only mandatory dependency
// (ADR-0003): when it is down the process is not ready, and no amount of degradation helps.
//
// What it does not do is check anything else. /healthz never touches a dependency at all -
// otherwise a database outage takes down every pod at once instead of leaving them running and
// unready (engineering-guidelines.md §5).
type Probe struct {
	pool *pgxpool.Pool
	// The readiness probe, the health report and the sampler all call Check, and they call it
	// concurrently - the state below is shared, so it is guarded.
	mu sync.Mutex
	// since records when the current status began, so the report can say how long a disruption
	// has been going on rather than only that there is one.
	since  time.Time
	status health.Status
}

func NewProbe(pool *pgxpool.Pool) *Probe {
	return &Probe{pool: pool, since: time.Now(), status: health.StatusOK}
}

func (p *Probe) Name() string   { return "postgres" }
func (p *Probe) Required() bool { return true }

func (p *Probe) Check(ctx context.Context) health.Result {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	started := time.Now()
	busy, err := p.roundTrip(ctx)
	latency := time.Since(started)

	status := health.StatusOK
	errorCode := ""
	switch {
	case busy:
		// Overload, not an outage: every connection is at work. Not ready would take every pod
		// out at once (ADR-0016); the load shedder and A-11 answer an overload instead.
		status = health.StatusDegraded
		errorCode = "postgres.pool_busy"
	case err != nil:
		status = health.StatusDown
		// A code, not the driver's message: that message names hosts and users, and this
		// report is served over HTTP (T-18, security.md §9).
		errorCode = "postgres.unreachable"
	}
	p.mu.Lock()
	if status != p.status {
		p.status, p.since = status, time.Now()
	}
	since := p.since
	p.mu.Unlock()

	result := health.Result{
		Status:    status,
		Latency:   latency,
		ErrorCode: errorCode,
		Since:     since,
	}
	if status == health.StatusDown {
		// Everything is affected. PostgreSQL is not a feature that can degrade - it is the
		// write path.
		result.Impact = []string{"all"}
	}
	return result
}

// roundTrip pings the database on a connection of the pool - a round trip, not a pool statistic:
// an idle pool looks healthy right up to the moment someone tries to use it. busy is a pool that
// had no connection to spare within the budget because every one was in use; the database said
// nothing either way then.
func (p *Probe) roundTrip(ctx context.Context) (busy bool, err error) {
	acquireCtx, cancel := context.WithTimeout(ctx, probeAcquireBudget)
	defer cancel()
	conn, err := p.pool.Acquire(acquireCtx)
	if err != nil {
		stat := p.pool.Stat()
		saturated := stat.AcquiredConns() >= stat.MaxConns()
		return saturated && ctx.Err() == nil && errors.Is(acquireCtx.Err(), context.DeadlineExceeded), err
	}
	defer conn.Release()
	return false, conn.Ping(ctx)
}
