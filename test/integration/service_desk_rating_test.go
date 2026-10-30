// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A request's rating is kept once and replaced by rating again; the workspace next door neither
// reads it nor rates the entry (migration 0243, UC-SVC-07).
func TestARatingStaysWithItsRequest(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	ratings := postgres.NewServiceDeskRepository().Ratings()
	now := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	item := seedWorkItemForRules(ctx, t, tenantA)

	first, err := servicedesk.NewRating(freshID(t), tenantA, item, 2, now)
	if err != nil {
		t.Fatal(err)
	}
	again, err := servicedesk.NewRating(freshID(t), tenantA, item, 5, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := ratings.Save(ctx, first); err != nil {
			return err
		}
		saved, err := ratings.Save(ctx, again)
		if err == nil && (saved.Score != 5 || saved.Version != 2 || saved.ID != first.ID) {
			t.Errorf("rating again kept %+v, want the first row with score 5", saved)
		}
		return err
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := ratings.OfItem(ctx, item); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the other workspace reads the rating: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	stray, _ := servicedesk.NewRating(freshID(t), tenantB, item, 1, now)
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := ratings.Save(ctx, stray)
		return err
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the other workspace rated the entry: %v", err)
	}
}
