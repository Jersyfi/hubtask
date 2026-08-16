// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package postgres is the persistence adapter. It is the only package that holds the database
// driver (ADR-0010, enforced by depguard and by test/architecture): everything else reaches the
// database through the unit of work, which is where the tenant boundary is set.
package postgres

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// NewPool opens the connection pool for one role.
//
// The role decides the query budget: short on the interactive path, long for background work
// (engineering-guidelines.md §4). It is applied as a connection parameter rather than per query,
// so a statement that runs away is cut off by PostgreSQL even when the calling code forgot its
// own deadline.
func NewPool(ctx context.Context, cfg env.Config, role env.Role) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.Database.DSN.Reveal())
	if err != nil {
		// The DSN is a secret: it carries the password. Whatever the driver says about it
		// stays out of the message (T-18).
		return nil, shared.ErrInternal.
			WithDetail("postgres.dsn_invalid").
			WithCause(fmt.Errorf("parsing HUBTASK_DB_DSN: %w", err))
	}

	// The bounds are checked here rather than trusted from the configuration. They are checked
	// there too, but this conversion narrows to int32, and a value that arrived from the
	// environment through strconv.Atoi would wrap rather than fail - a pool size of -2147483648
	// is not a configuration error anybody would recognise in a log.
	poolCfg.MaxConns = boundedPoolSize(cfg.Database.MaxConns)
	poolCfg.MinConns = boundedPoolSize(cfg.Database.MinConns)
	poolCfg.MaxConnLifetime = cfg.Database.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.Database.MaxConnIdleTime
	poolCfg.ConnConfig.ConnectTimeout = cfg.Database.ConnectTimeout

	// A jittered fraction of the lifetime, so that a pool opened at once does not expire at
	// once and hand the database a thundering herd on the hour.
	poolCfg.MaxConnLifetimeJitter = cfg.Database.MaxConnLifetime / 10

	statementTimeout := cfg.StatementTimeoutFor(role)
	if poolCfg.ConnConfig.RuntimeParams == nil {
		poolCfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	poolCfg.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(statementTimeout.Milliseconds(), 10)
	// Visible in pg_stat_activity, which is where an operator looks first when one role is
	// holding connections.
	poolCfg.ConnConfig.RuntimeParams["application_name"] = "hubtask/" + string(role)

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.pool_unavailable").
			WithCause(fmt.Errorf("opening the pool: %w", err))
	}

	// Fail closed at startup: a pool that cannot reach the database is better discovered here
	// than by the first request (ADR-0015). The deadline is the configured connect timeout,
	// because no call goes without one (ADR-0016).
	pingCtx, cancel := context.WithTimeout(ctx, cfg.Database.ConnectTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.unreachable").
			WithCause(fmt.Errorf("first connection: %w", err))
	}

	return pool, nil
}

// boundedPoolSize narrows a configured pool size to int32 within provable bounds.
//
// Both comparisons are against constants, and both return early. That is not style: a bounds
// check against a variable proves nothing to a reader who has not read the caller, and nothing
// to CodeQL either - it only recognises comparisons with constant values, which is how the first
// attempt at this function ended up with an alert of its own.
func boundedPoolSize(value int) int32 {
	if value < 0 {
		return 0
	}
	if value > env.MaxPoolConns {
		return env.MaxPoolConns
	}
	return int32(value)
}

// PoolStats is what the health report and the metrics need, without handing either of them the
// pool itself.
type PoolStats struct {
	Total        int32
	Acquired     int32
	Idle         int32
	Max          int32
	AcquireCount int64
	// AcquireWait is the total time callers spent waiting for a free connection. Rising means
	// the pool is too small, or something is holding connections too long.
	AcquireWait time.Duration
}

func Stats(pool *pgxpool.Pool) PoolStats {
	s := pool.Stat()
	return PoolStats{
		Total:        s.TotalConns(),
		Acquired:     s.AcquiredConns(),
		Idle:         s.IdleConns(),
		Max:          s.MaxConns(),
		AcquireCount: s.AcquireCount(),
		AcquireWait:  s.AcquireDuration(),
	}
}
