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

// Alert A-28's number against a real database: an index PostgreSQL holds invalid is counted, read
// by the application's role in the scheduler's read-only scope.
func TestAnInvalidIndexIsCounted(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	count := func() int64 {
		t.Helper()
		var counted int64
		if err := uow.WithinReadOnly(ctx, persistence.SystemScope(), func(ctx context.Context) error {
			var err error
			counted, err = postgres.Queue{}.InvalidIndexCount(ctx)
			return err
		}); err != nil {
			t.Fatalf("counting: %v", err)
		}
		return counted
	}
	before := count()

	const mark = `UPDATE pg_index SET indisvalid = $1 WHERE indexrelid = 'public.notification_retention_idx'::regclass`
	if _, err := admin.Exec(ctx, mark, false); err != nil {
		t.Fatalf("marking the index invalid: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), mark, true); err != nil {
			t.Errorf("restoring the index: %v", err)
		}
	})
	if after := count(); after != before+1 {
		t.Errorf("counted %d invalid indexes, want %d", after, before+1)
	}
}
