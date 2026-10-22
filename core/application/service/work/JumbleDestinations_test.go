// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// collectionsInOrder is the ordered read, answering what it was given and recording the query.
type collectionsInOrder struct {
	rows  []repository.CollectionInHub
	asked repository.CollectionQuery
}

func (c *collectionsInOrder) InOrder(_ context.Context, query repository.CollectionQuery) (repository.CollectionPage, error) {
	c.asked = query
	return repository.CollectionPage{Collections: c.rows}, nil
}

func destinationsFixture() (*collectionsInOrder, shared.ID, shared.ID) {
	open := shared.MustParseID("0192f000-0000-7000-8000-000000000301")
	closed := shared.MustParseID("0192f000-0000-7000-8000-000000000302")
	return &collectionsInOrder{rows: []repository.CollectionInHub{
		{Collection: collectionFixture(open, hubID, "Orders"), HubName: "Shop"},
		{Collection: collectionFixture(closed, hubID, "Payroll"), HubName: "Shop"},
	}}, open, closed
}

// The picker's read keeps what a create would be permitted, asks what converting asks first, and
// keeps private hubs in what it scans.
func TestTheDestinationsAreWhereACreateIsPermitted(t *testing.T) {
	store, open, closed := destinationsFixture()
	guard, permits := &authorizer{}, &reader{permit: map[shared.ID]bool{open: true}}
	registry, err := usecase.NewRegistry(nil, ListJumbleDestinations{
		Collections: store, Authorizer: guard, Reader: permits, UnitOfWork: &unitOfWork{},
	}.Descriptor())
	if err != nil {
		t.Fatalf("registry: %v", err)
	}

	out, err := registry.Invoke(t.Context(), ListJumbleDestinationsName, actorFixture(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	data, _ := out["data"].([]usecase.Output)
	if len(data) != 1 || data[0].String("id") != open.String() || data[0].String("hub_name") != "Shop" {
		t.Errorf("the destinations are %v, want only %s", data, open)
	}
	for _, row := range data {
		if row.String("id") == closed.String() {
			t.Error("a collection where creating is not permitted is offered")
		}
	}
	if !store.asked.IncludePrivate || store.asked.Page.Size != destinationsPageSize {
		t.Errorf("the scan asked %+v", store.asked)
	}
	if first := guard.requests[0]; string(first.Permission) != "WRITE_ITEMS" || len(first.Path) != 1 {
		t.Errorf("the read asked %+v first, want what converting asks", first)
	}
	if string(permits.request.Permission) != "WRITE_ITEMS" || permits.request.Action != ItemCreatedAction {
		t.Errorf("each collection was asked %+v, want a create's question", permits.request)
	}
}

// Somebody who may convert nothing is refused rather than answered an empty list, and nothing is
// read.
func TestADestinationsReadThatMayNotConvertIsRefused(t *testing.T) {
	store, _, _ := destinationsFixture()
	refused := shared.ErrForbidden.WithDetail("access.not_permitted")
	_, err := ListJumbleDestinations{
		Collections: store, Authorizer: &authorizer{err: refused}, Reader: &reader{}, UnitOfWork: &unitOfWork{},
	}.Execute(t.Context(), actorFixture(), repository.Page{})
	if err == nil {
		t.Fatal("a person who may not convert was answered")
	}
	if store.asked.Page.Size != 0 {
		t.Error("the collections were read after the refusal")
	}
}
