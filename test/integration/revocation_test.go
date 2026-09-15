// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// revocationsFor is the revocation service against the real repositories (N-08), the way
// cmd/server/main.go builds it.
func revocationsFor(t *testing.T, permits access.Permitter) access.Revocations {
	t.Helper()
	hybrid, err := clockadapter.NewHybridClock(portclock.Fixed(created), "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	return access.Revocations{
		Permits:    permits,
		Grants:     postgres.NewMembershipGrantRepository(pageCursors()),
		Groups:     postgres.NewGroupRepository(pageCursors()),
		Containers: containerRepo(),
		Items:      itemRepo(),
		Changes:    postgres.NewChangeLog(),
		HLC:        hybrid,
	}
}
