// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	health "github.com/Jersyfi/hubtask/core/port/health"
)

// probeTimeout bounds the health query. Short on purpose: a probe that waits is a probe that
// turns a slow database into an unresponsive pod, because Kubernetes has its own patience and
// it is shorter than PostgreSQL's (ADR-0016).
const probeTimeout = 2 * time.Second

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
	// A round trip, not just a pool statistic: an idle pool looks healthy right up to the
	// moment someone tries to use it.
	err := p.pool.Ping(ctx)
	latency := time.Since(started)

	status := health.StatusOK
	errorCode := ""
	if err != nil {
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
	if status != health.StatusOK {
		// Everything is affected. PostgreSQL is not a feature that can degrade - it is the
		// write path.
		result.Impact = []string{"all"}
	}
	return result
}
