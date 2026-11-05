// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// countsInTurn proves a ceiling's count is taken in turn: a second transaction's count waits until
// the first one, which counted and wrote, commits, and then sees its write - so two concurrent
// creates cannot both pass the ceiling.
func countsInTurn(t *testing.T, count func(context.Context) (int, error), insert func(context.Context) error) {
	t.Helper()
	ctx := context.Background()
	counted := make(chan struct{})
	type answer struct {
		n      int
		waited time.Duration
	}
	second := make(chan answer, 1)
	failed := make(chan error, 1)
	first := make(chan error, 1)
	go func() {
		first <- write(ctx, t, tenantA, func(ctx context.Context) error {
			if _, err := count(ctx); err != nil {
				return err
			}
			close(counted)
			time.Sleep(400 * time.Millisecond)
			return insert(ctx)
		})
	}()
	<-counted
	started := time.Now()
	go func() {
		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			n, err := count(ctx)
			second <- answer{n, time.Since(started)}
			return err
		}); err != nil {
			failed <- err
		}
	}()
	if err := <-first; err != nil {
		t.Fatalf("the first create: %v", err)
	}
	select {
	case err := <-failed:
		t.Fatalf("the second count: %v", err)
	case got := <-second:
		if got.waited < 200*time.Millisecond {
			t.Errorf("the second count did not wait for the first create (%v)", got.waited)
		}
		if got.n != 1 {
			t.Errorf("the second count saw %d, want the first create's", got.n)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the second count never answered")
	}
}

func TestABoardCeilingIsCountedInTurn(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := containerRepo().Insert(ctx, containerIn(tenantA, authorA, hub, freshName(t), "a0"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	boards := postgres.NewWhiteboardRepository()
	at := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	countsInTurn(t, func(ctx context.Context) (int, error) { return boards.Count(ctx, hub) }, func(ctx context.Context) error {
		return boards.Insert(ctx, whiteboard.Board{
			ID: freshID(t), HubID: hub, Title: "Retro", CreatedBy: authorA, CreatedAt: at, UpdatedBy: authorA, UpdatedAt: at, Version: 1,
		})
	})
}

func TestABaselineCeilingIsCountedInTurn(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub, _, _ := seedPlan(ctx, t, tenantA)
	baselines := postgres.NewPlanningRepository().Baselines()
	countsInTurn(t, func(ctx context.Context) (int, error) { return baselines.Count(ctx, hub) }, func(ctx context.Context) error {
		baseline, err := planning.NewBaseline(planning.NewBaselineInput{
			ID: freshID(t), TenantID: tenantA, PlanID: hub, TakenBy: authorA, Name: "Agreed",
			Now: time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC),
		})
		if err != nil {
			return err
		}
		_, err = baselines.Take(ctx, baseline, planning.MaxBaselineEntries)
		return err
	})
}
