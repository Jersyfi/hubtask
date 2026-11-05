// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Alerts A-36, A-37 and A-42's numbers against a real database, read by the application's role in
// the scheduler's scope: the NOTIFY queue's use, the unfrozen transaction-ID age and the database's
// size, which is never zero for a migrated schema.
func TestTheApplicationsRoleReadsTheDatabaseHorizon(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	var usage float64
	var age, size int64
	if err := uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		var err error
		usage, age, size, err = postgres.Queue{}.DatabaseHorizon(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading the horizon: %v", err)
	}
	if usage < 0 || usage > 1 || age <= 0 || size < 1<<20 {
		t.Errorf("the horizon reads usage %v, age %d, size %d", usage, age, size)
	}
}
