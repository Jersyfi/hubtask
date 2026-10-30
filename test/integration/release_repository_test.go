// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A release and its entries stay in their workspace (migration 0272, UC-PLN-08 check 6); an entry
// is in one release at a time.
func TestAReleaseStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	releases := postgres.NewPlanningRepository().Releases()
	hub, collection, tasks := seedPlan(ctx, t, tenantA)
	now := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	newRelease := func(name string) planning.Release {
		r, err := planning.NewRelease(planning.NewReleaseInput{
			ID: freshID(t), TenantID: tenantA, HubID: hub, CreatedBy: authorA, Now: now,
			Fields: planning.ReleaseFields{Name: name, ReleaseOn: planning.DayFrom(2026, time.November, 30)},
		})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	first, second := newRelease("2.4"), newRelease("2.5")
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, r := range []planning.Release{first, second} {
			if err := releases.Insert(ctx, r); err != nil {
				return err
			}
		}
		if err := releases.Add(ctx, first.ID, tasks[0], now); err != nil {
			return err
		}
		out := first
		_ = out.Release(tasks[1], now)
		return releases.Update(ctx, out, first.Version)
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return releases.Add(ctx, second.ID, tasks[0], now)
	}); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("an entry joined a second release: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		members, err := releases.Entries(ctx, first.ID, 100)
		if err != nil {
			return err
		}
		if len(members) != 1 || members[0].EntryID != tasks[0] || members[0].CollectionID != collection || members[0].Title == "" {
			t.Errorf("members = %+v", members)
		}
		of, err := releases.Of(ctx, tasks)
		if err == nil && (len(of) != 1 || of[0].ReleaseID != first.ID) {
			t.Errorf("of = %+v", of)
		}
		found, err := releases.Find(ctx, first.ID)
		if err == nil && (found.State != planning.ReleaseReleased || found.NotesPageID != tasks[1] || found.ReleasedAt == nil) {
			t.Errorf("release = %+v", found)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := releases.Find(ctx, first.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the other workspace reads the release: %v", err)
		}
		if listed, err := releases.List(ctx, hub, "", 50); err != nil || len(listed) != 0 {
			t.Errorf("the other workspace lists releases: %v %v", listed, err)
		}
		if members, err := releases.Entries(ctx, first.ID, 100); err != nil || len(members) != 0 {
			t.Errorf("the other workspace reads the entries: %v %v", members, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return releases.Delete(ctx, first.ID)
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the other workspace deleted the release: %v", err)
	}
}
