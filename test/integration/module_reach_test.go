// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

func peopleReaching(ctx context.Context, t *testing.T, tenant, hub shared.ID) int {
	t.Helper()
	var people int
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		people, err = postgres.NewMembershipRepository().PeopleReaching(ctx, hub)
		return err
	}); err != nil {
		t.Fatalf("counting: %v", err)
	}
	return people
}

// PeopleReaching against a real database, with its cross-tenant negative test (gate SG-3): two
// people given a role on A's hub are counted for it, and B, asking about A's hub, counts none.
func TestPeopleReachingCountsOnlyTheCallersPeople(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub, _ := hubWithCollection(ctx, t, tenantA, authorA)
	before := peopleReaching(ctx, t, tenantA, hub)
	memberOf(ctx, t, tenantA, hub, "MEMBER")
	memberOf(ctx, t, tenantA, hub, "VIEWER")

	if got := peopleReaching(ctx, t, tenantA, hub); got != before+2 {
		t.Errorf("A's hub counts %d, want %d", got, before+2)
	}
	if got := peopleReaching(ctx, t, tenantA, ""); got < 2 {
		t.Errorf("A's workspace counts %d", got)
	}
	if got := peopleReaching(ctx, t, tenantB, hub); got != 0 {
		t.Errorf("B counts %d people on A's hub", got)
	}
}
