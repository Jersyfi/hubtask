// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	goalrepo "github.com/Jersyfi/hubtask/core/application/repository/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A period, a goal with its key results, linked entries and check-ins, a portfolio with its
// projects and updates, and a dashboard stay in their workspace (migration 0248, UC-GOL-01 check 7,
// UC-GOL-03 check 6, UC-GOL-04 check 6).
func TestGoalsAndInsightStayInTheirWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	store := postgres.NewGoalRepository()
	hub, collection, entries := seedPlan(ctx, t, tenantA)
	now := time.Date(2026, 11, 3, 17, 0, 0, 0, time.UTC)
	period := goal.Period{ID: freshID(t), Name: "Q4", StartOn: 20393, EndOn: 20483}
	result := freshID(t)
	g := goal.Goal{
		ID: freshID(t), PeriodID: period.ID, HubID: hub, Title: "Ship it", OwnerID: authorA, Status: goal.StatusNone,
		KeyResults: []goal.KeyResult{{ID: result, Title: "Done", Kind: goal.ResultNumber, Start: 0, Target: 10, Source: goal.SourceManual}},
	}
	portfolio := goal.Portfolio{
		ID: freshID(t), Name: "Programme", OwnerID: authorA, Status: goal.StatusNone,
		Items: []goal.PortfolioItem{{ContainerID: collection, Status: goal.StatusAtRisk}},
	}
	dashboard := goal.Dashboard{
		ID: freshID(t), OwnerID: authorA, Name: "Team", Sharing: goal.SharingGroups, GroupIDs: []shared.ID{freshID(t)},
		Widgets: []goal.Widget{{ID: freshID(t), Title: "Open", Chart: goal.ChartNumber, Source: goal.WidgetQuery,
			Query: map[string]any{"scope_container_id": collection.String()}, W: 6, H: 2},
			{ID: freshID(t), Title: "Flow", Chart: goal.ChartLine, Source: goal.WidgetQuery, Cumulative: true,
				Query:   map[string]any{"scope_container_id": collection.String()},
				Compare: &goal.Compare{FirstTitle: "Created", Title: "Resolved", Query: map[string]any{"scope_workspace": true}}, W: 6, H: 2}},
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := store.InsertPeriod(ctx, period, authorA, now); err != nil {
			return err
		}
		if err := store.InsertGoal(ctx, g, authorA, now); err != nil {
			return err
		}
		if err := store.LinkEntries(ctx, g.ID, entries[:1], authorA, now); err != nil {
			return err
		}
		if err := store.AddCheckIn(ctx, goal.CheckIn{
			ID: freshID(t), GoalID: g.ID, AuthorID: authorA, Status: goal.StatusOnTrack, Confidence: 7,
			Values: map[shared.ID]float64{result: 4}, At: now,
		}); err != nil {
			return err
		}
		if err := store.InsertPortfolio(ctx, portfolio, authorA, now); err != nil {
			return err
		}
		if err := store.AddStatusUpdate(ctx, goal.StatusUpdate{
			ID: freshID(t), PortfolioID: portfolio.ID, AuthorID: authorA, Status: goal.StatusAtRisk, Summary: "late", At: now,
		}); err != nil {
			return err
		}
		return store.InsertDashboard(ctx, dashboard, now)
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		found, err := store.Goal(ctx, g.ID)
		if err != nil || len(found.KeyResults) != 1 || found.KeyResults[0].ID != result || found.HubID != hub {
			t.Errorf("goal = %+v %v", found, err)
		}
		linked, err := store.LinkedEntries(ctx, []shared.ID{g.ID})
		if err != nil || len(linked[g.ID]) != 1 {
			t.Errorf("linked = %v %v", linked, err)
		}
		checkIns, err := store.CheckIns(ctx, g.ID, 10)
		if err != nil || len(checkIns) != 1 || checkIns[0].Values[result] != 4 {
			t.Errorf("check-ins = %+v %v", checkIns, err)
		}
		p, err := store.Portfolio(ctx, portfolio.ID)
		if err != nil || len(p.Items) != 1 || p.Items[0].Status != goal.StatusAtRisk {
			t.Errorf("portfolio = %+v %v", p, err)
		}
		d, err := store.Dashboard(ctx, dashboard.ID)
		if err != nil || len(d.Widgets) != 2 || d.Widgets[0].Query["scope_container_id"] != collection.String() ||
			!d.Widgets[1].Cumulative || d.Widgets[1].Compare == nil || d.Widgets[1].Compare.Title != "Resolved" ||
			d.Widgets[1].Compare.Query["scope_workspace"] != true ||
			len(d.GroupIDs) != 1 || d.GroupIDs[0] != dashboard.GroupIDs[0] {
			t.Errorf("dashboard = %+v %v", d, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := store.Goal(ctx, g.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the other workspace finds the goal: %v", err)
		}
		if goals, err := store.Goals(ctx, goalrepo.GoalFilter{Max: 10}); err != nil || len(goals) != 0 {
			t.Errorf("the other workspace lists goals: %v %v", goals, err)
		}
		if linked, err := store.LinkedEntries(ctx, []shared.ID{g.ID}); err != nil || len(linked) != 0 {
			t.Errorf("the other workspace reads linked entries: %v %v", linked, err)
		}
		if c, err := store.CheckIns(ctx, g.ID, 10); err != nil || len(c) != 0 {
			t.Errorf("the other workspace reads check-ins: %v %v", c, err)
		}
		if _, err := store.Portfolio(ctx, portfolio.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the other workspace finds the portfolio: %v", err)
		}
		if u, err := store.StatusUpdates(ctx, portfolio.ID, 10); err != nil || len(u) != 0 {
			t.Errorf("the other workspace reads updates: %v %v", u, err)
		}
		if d, err := store.Dashboards(ctx, 10); err != nil || len(d) != 0 {
			t.Errorf("the other workspace lists dashboards: %v %v", d, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return store.DeleteDashboard(ctx, dashboard.ID, 1)
	}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("the other workspace deleted the dashboard: %v", err)
	}
}

// A person's digest of a dashboard is kept once per dashboard and person, replaced by a second
// write, and stays in its workspace (migration 0249, UC-GOL-04 check 9).
func TestADashboardDigestStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	store := postgres.NewGoalRepository()
	now := time.Date(2026, 11, 3, 17, 0, 0, 0, time.UTC)
	dashboard := goal.Dashboard{ID: freshID(t), OwnerID: authorA, Name: "Digest", Sharing: goal.SharingPrivate}
	digest := goal.Digest{
		ID: freshID(t), DashboardID: dashboard.ID, AccountID: authorA, Cadence: goal.CadenceWeekly, Weekday: 1,
		Hour: 8, TimeZone: "Europe/Berlin", NextAt: now.Add(time.Hour),
	}
	var kept shared.ID
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := store.InsertDashboard(ctx, dashboard, now); err != nil {
			return err
		}
		if _, err := store.PutDigest(ctx, digest, now); err != nil {
			return err
		}
		again := digest
		again.ID, again.Cadence, again.Weekday = freshID(t), goal.CadenceDaily, 0
		var err error
		kept, err = store.PutDigest(ctx, again, now)
		return err
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if kept != digest.ID {
		t.Errorf("a second write kept %s, want the first row %s", kept, digest.ID)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return store.DigestSent(ctx, digest.ID, &now, now.Add(24*time.Hour))
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		found, err := store.Digest(ctx, dashboard.ID, authorA)
		if err != nil || found.Cadence != goal.CadenceDaily || found.LastSentAt == nil || !found.NextAt.Equal(now.Add(24*time.Hour)) {
			t.Errorf("digest = %+v %v", found, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := store.DigestByID(ctx, digest.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the other workspace finds the digest: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		gone, err := store.DeleteDigest(ctx, dashboard.ID, authorA)
		if gone {
			t.Errorf("the other workspace deleted the digest")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// The walk's portfolios stay in their workspace, each with its projects (offline-sync.md §3.1).
func TestThePortfolioWalkStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	store := postgres.NewGoalRepository()
	snapshot := postgres.NewSnapshotRepository()
	_, collection, _ := seedPlan(ctx, t, tenantA)
	now := time.Date(2026, 11, 3, 17, 0, 0, 0, time.UTC)
	portfolio := goal.Portfolio{
		ID: freshID(t), Name: "Walked", OwnerID: authorA, Status: goal.StatusOnTrack,
		Items: []goal.PortfolioItem{{ContainerID: collection, Status: goal.StatusOnTrack}},
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return store.InsertPortfolio(ctx, portfolio, authorA, now)
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	walked := func(tenant shared.ID) (projects int, seen bool) {
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			list, err := snapshot.Portfolios(ctx, "", 5000)
			for _, p := range list {
				if p.ID == portfolio.ID {
					seen, projects = true, len(p.Items)
				}
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return projects, seen
	}
	if projects, seen := walked(tenantA); !seen || projects != 1 {
		t.Errorf("the workspace's walk = %v with %d projects, want the portfolio with its project", seen, projects)
	}
	if _, seen := walked(tenantB); seen {
		t.Error("the workspace next door walked the portfolio")
	}
}
