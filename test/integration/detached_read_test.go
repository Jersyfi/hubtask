// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

func backendPID(ctx context.Context, t *testing.T) int {
	t.Helper()
	tx, err := postgres.FromContext(ctx)
	if err != nil {
		t.Fatalf("no transaction: %v", err)
	}
	var pid int
	if err := tx.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		t.Fatalf("reading the backend: %v", err)
	}
	return pid
}

// persistence.Detaching against PostgreSQL: a read opened from a detached context runs on a
// connection of its own, and a statement of it cancelled by its deadline leaves the caller's
// transaction usable - the figure count a container read makes inside the configuration's apply
// used to cancel or overlap the apply's own statements ("conn busy").
func TestADetachedReadLeavesTheCallersTransactionAlone(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	scope := persistence.Scope{TenantID: tenantA}

	err := unitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		outer := backendPID(ctx, t)
		detached := persistence.Detached(ctx, unitOfWork)
		bounded, cancel := context.WithTimeout(detached, 50*time.Millisecond)
		defer cancel()
		_ = unitOfWork.WithinReadOnly(bounded, scope, func(ctx context.Context) error {
			if inner := backendPID(ctx, t); inner == outer {
				t.Errorf("the detached read ran on the caller's connection %d", outer)
			}
			tx, _ := postgres.FromContext(ctx)
			_, err := tx.Exec(ctx, "SELECT pg_sleep(1)")
			return err
		})
		if again := backendPID(ctx, t); again != outer {
			t.Errorf("the caller's transaction moved from %d to %d", outer, again)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("the caller's transaction did not survive the detached read: %v", err)
	}
}
