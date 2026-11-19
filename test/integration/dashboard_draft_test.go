// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	goalservice "github.com/Jersyfi/hubtask/core/application/service/goal"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	textadapter "github.com/Jersyfi/hubtask/infrastructure/text"
)

type readsAll struct{}

func (readsAll) Permitted(_ context.Context, _ appshared.ActorContext, _ access.Request, paths [][]identity.Scope) ([]bool, error) {
	out := make([]bool, len(paths))
	for i := range out {
		out[i] = true
	}
	return out, nil
}

// countsAsked stands for POST /items:aggregate, run as whoever asks; it records the asks.
type countsAsked struct{ asked []usecase.Input }

func (c *countsAsked) Invoke(_ context.Context, _ appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	c.asked = append(c.asked, in)
	return usecase.Output{"data": []usecase.Output{{"count": 7, "group": nil, "period": nil, "value": nil}}, "truncated": false}, nil
}

// UC-GOL-04 check 23 against PostgreSQL: a draft is computed over the stored dashboard of the
// reader's workspace, never stored, and another workspace's reader finds no dashboard to draft on
// (SG-3).
func TestADashboardDraftIsComputedInItsWorkspaceAndNeverStored(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	store := postgres.NewGoalRepository()
	now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	fixed := portclock.Fixed(now)
	board := goal.Dashboard{ID: freshID(t), OwnerID: authorA, Name: "Servicedesk", Sharing: goal.SharingPrivate,
		Filter: "is: open"}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return store.InsertDashboard(ctx, board, now)
	}); err != nil {
		t.Fatalf("storing the dashboard: %v", err)
	}
	counts := &countsAsked{}
	steward := goalservice.Steward{
		Store: store, Summaries: counts, ViewSummaries: counts, Items: counts,
		Authorizer: allowAll{}, Reader: readsAll{}, Audit: postgres.NewAuditSink(clockadapter.NewUUIDv7(fixed)),
		UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)), Clock: fixed, IDs: clockadapter.NewUUIDv7(fixed),
		Text: textadapter.Forms{},
	}
	draft := goalservice.DraftDashboardFigures{Steward: steward}.Descriptor().Handler
	in := usecase.Input{"dashboard_id": board.ID.String(), "filter": "is: open created: -6w..", "widgets": []any{
		map[string]any{"title": "Offen", "chart": "NUMBER", "source": "QUERY", "w": 4.0, "h": 2.0,
			"query": map[string]any{"scope_workspace": true, "aggregate": map[string]any{"function": "COUNT"}}},
	}}
	scopes := []string{"containers:read", "items:read"}
	ana := appshared.ActorContext{TenantID: tenantA, AccountID: authorA, Kind: shared.ActorUser, Scopes: scopes}
	out, err := draft.Invoke(ctx, ana, in)
	if err != nil {
		t.Fatalf("the draft: %v", err)
	}
	rows, _ := out["widgets"].([]usecase.Output)
	if len(rows) != 1 || rows[0]["error"] != nil || len(counts.asked) != 1 {
		t.Fatalf("the draft answered %v after %d asks", rows, len(counts.asked))
	}
	var kept goal.Dashboard
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		kept, err = store.Dashboard(ctx, board.ID)
		return err
	}); err != nil {
		t.Fatalf("reading the dashboard back: %v", err)
	}
	if len(kept.Widgets) != 0 || kept.Filter != "is: open" || kept.Version != 1 {
		t.Fatalf("the draft was stored: %+v", kept)
	}

	bert := appshared.ActorContext{TenantID: tenantB, AccountID: authorB, Kind: shared.ActorUser, Scopes: scopes}
	if _, err := draft.Invoke(ctx, bert, in); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("another workspace drafted on the dashboard: %v", err)
	}
	if len(counts.asked) != 1 {
		t.Fatalf("another workspace's draft ran %d summaries", len(counts.asked)-1)
	}
}
