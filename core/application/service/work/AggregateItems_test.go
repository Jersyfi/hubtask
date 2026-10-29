// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"slices"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// aggregates records the search a summary handed to the adapter.
type aggregates struct{ asked repository.ItemSearch }

func (a *aggregates) Aggregate(
	_ context.Context, search repository.ItemSearch, _ view.Aggregation, _ string,
) (repository.ItemAggregateResult, error) {
	a.asked = search
	return repository.ItemAggregateResult{Rows: []repository.ItemAggregateRow{{Count: 3}}}, nil
}

func summaryActor() appshared.ActorContext {
	actor := actorFixture()
	actor.Scopes = []string{itemsRead}
	return actor
}

func aggregateFixture(permits *reader) (AggregateItems, *aggregates, *authorizer) {
	store, _, _ := destinationsFixture()
	adapter, guard := &aggregates{}, &authorizer{}
	return AggregateItems{
		Query:      QueryItems{Authorizer: guard, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now)},
		Aggregates: adapter, Collections: store, Reader: permits,
	}, adapter, guard
}

// A workspace summary reads the collections the actor may read, as one bound list, and asks no
// single anchor's permission (S-11).
func TestAWorkspaceSummaryIsNarrowedToTheReadableCollections(t *testing.T) {
	_, open, closed := destinationsFixture()
	permits := &reader{permit: map[shared.ID]bool{open: true}}
	h, adapter, guard := aggregateFixture(permits)

	registry, err := usecase.NewRegistry(nil, h.Descriptor())
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	out, err := registry.Invoke(t.Context(), AggregateItemsName, summaryActor(), usecase.Input{
		"scope_workspace": true,
		"aggregate":       map[string]any{"function": "AVG", "field": "lead_time"},
	})
	if err != nil {
		t.Fatalf("summarising: %v", err)
	}

	anchor := adapter.asked.Anchor
	if anchor.Kind != repository.AnchorCollections || !slices.Equal(anchor.CollectionIDs, []shared.ID{open}) {
		t.Errorf("the statement reads %+v, want only %s", anchor, open)
	}
	if slices.Contains(anchor.CollectionIDs, closed) {
		t.Error("a collection the actor may not read is counted")
	}
	if string(permits.request.Permission) != "READ" || permits.request.Action != ItemReadAction {
		t.Errorf("each collection was asked %+v, want a read", permits.request)
	}
	if len(guard.requests) != 0 {
		t.Errorf("a workspace summary asked an anchor's permission: %+v", guard.requests)
	}
	if data, _ := out["data"].([]usecase.Output); len(data) != 1 {
		t.Errorf("the answer is %v", out)
	}
}

// A workspace summary nobody reads anything of counts nothing rather than everything.
func TestAWorkspaceSummaryOfNothingReadableReadsNoCollection(t *testing.T) {
	h, adapter, _ := aggregateFixture(&reader{permit: map[shared.ID]bool{}})
	_, err := h.Execute(t.Context(), summaryActor(),
		view.Spec{Scope: view.Scope{Workspace: true}}, view.Aggregation{Function: view.AggregateCount})
	if err != nil {
		t.Fatal(err)
	}
	if adapter.asked.Anchor.Kind != repository.AnchorCollections || len(adapter.asked.Anchor.CollectionIDs) != 0 {
		t.Errorf("the statement reads %+v", adapter.asked.Anchor)
	}
}

// A summary that names neither an anchor nor the workspace is refused, as a query is.
func TestASummaryWithoutAScopeIsRefused(t *testing.T) {
	h, adapter, _ := aggregateFixture(&reader{})
	registry, err := usecase.NewRegistry(nil, h.Descriptor())
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	_, err = registry.Invoke(t.Context(), AggregateItemsName, actorFixture(), usecase.Input{
		"aggregate": map[string]any{"function": "COUNT"},
	})
	var refusal *shared.Error
	if !errors.As(err, &refusal) || refusal.DetailCode != "query.scope_required" {
		t.Fatalf("got %v, want query.scope_required", err)
	}
	if adapter.asked.Anchor.Kind != "" {
		t.Error("an unscoped summary reached the adapter")
	}
}
