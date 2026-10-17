// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A mark is its own workspace's: another workspace's mark at the same quota and period neither
// stops this one's nor is cleared by it.
func TestAQuotaMarkStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	marks := postgres.NewQuotaReachedRepository()
	first, second := seedWorkspace(ctx, t, "marked"), seedWorkspace(ctx, t, "unmarked")

	mark := func(workspace shared.ID) bool {
		t.Helper()
		var marked bool
		if err := uow.Within(ctx, persistence.Scope{TenantID: workspace}, func(ctx context.Context) error {
			set, err := marks.Mark(ctx, "items", "10", time.Now())
			marked = set
			return err
		}); err != nil {
			t.Fatalf("marking: %v", err)
		}
		return marked
	}
	if !mark(first) || mark(first) {
		t.Fatal("the first workspace's mark was not set once")
	}
	if !mark(second) {
		t.Error("another workspace's mark stopped this one's")
	}
	if err := uow.Within(ctx, persistence.Scope{TenantID: second}, func(ctx context.Context) error {
		return marks.Clear(ctx, "items")
	}); err != nil {
		t.Fatal(err)
	}
	if mark(first) {
		t.Error("clearing another workspace's mark cleared this one's")
	}
}
