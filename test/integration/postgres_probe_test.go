// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	health "github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/test/dbtest"
)

// A request pool fully held by slow work is an overload, not an outage: the probe says busy while
// PostgreSQL itself answers, so readiness does not take every pod out at once.
func TestAProbeOnAFullyHeldPoolSaysBusyNotDown(t *testing.T) {
	ctx := context.Background()
	config, err := pgxpool.ParseConfig(dbtest.Start(t).AppDSN)
	if err != nil {
		t.Fatalf("parsing the DSN: %v", err)
	}
	config.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(pool.Close)
	probe := postgres.NewProbe(pool)

	if got := probe.Check(ctx); got.Status != health.StatusOK {
		t.Fatalf("an idle pool: %q (%s), want ok", got.Status, got.ErrorCode)
	}

	for range 2 {
		held, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatalf("holding a connection: %v", err)
		}
		defer held.Release()
	}
	got := probe.Check(ctx)
	if got.Status != health.StatusDegraded || got.ErrorCode != "postgres.pool_busy" {
		t.Errorf("a fully held pool: %q (%s), want degraded postgres.pool_busy", got.Status, got.ErrorCode)
	}
}
