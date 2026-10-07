// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The installation at a glance, against the real boundary (SI-17, migration 0105).
//
// The census is the one read in this product that crosses the tenant boundary on purpose, and the
// reason it may is that it cannot bring anything back but integers. Both halves are asserted: that
// it counts at all under the scope with no tenant, and that what it answers is numbers rather than
// a way to rows.

func TestTheCensusCountsAcrossWorkspacesAndAnswersOnlyNumbers(t *testing.T) {
	ctx := context.Background()
	seedIdentityProviderTenants(ctx, t)

	installation := postgres.NewInstallationRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))

	var census adminrepo.Census
	if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		read, err := installation.Census(ctx)
		census = read
		return err
	}); err != nil {
		t.Fatalf("reading the census: %v", err)
	}

	// The two workspaces this file seeds are in it, and the account one of them holds. Greater
	// rather than equal: the integration database is shared, and a count that demanded an exact
	// total would fail whenever another file seeded a workspace of its own.
	if census.WorkspacesActive < 2 {
		t.Errorf("the census counts %d active workspaces, want at least the two seeded here",
			census.WorkspacesActive)
	}
	if census.AccountsActive < 1 {
		t.Errorf("the census counts %d active accounts", census.AccountsActive)
	}
	if census.AccountsTotal < census.AccountsActive {
		t.Errorf("the total %d is below the active %d", census.AccountsTotal, census.AccountsActive)
	}

	// And a workspace's own scope reaches the same function - it is the counts that are
	// installation-wide, not the caller. What no scope reaches is a row: the function has no
	// parameter and returns five integers, which is the whole of the exception.
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		fromInside, err := installation.Census(ctx)
		if err != nil {
			t.Fatalf("reading the census inside a workspace: %v", err)
		}
		if fromInside.WorkspacesActive != census.WorkspacesActive {
			t.Errorf("the census answers %d inside a workspace and %d outside",
				fromInside.WorkspacesActive, census.WorkspacesActive)
		}
		return nil
	})
}

// The journal is walked newest first and keyed on the moment *and* the identifier: two entries can
// share a moment, and an offset would then skip or repeat one.
func TestTheJournalIsWalkedNewestFirstOnAKeyset(t *testing.T) {
	ctx := context.Background()
	seedIdentityProviderTenants(ctx, t)

	journal := postgres.NewInstanceJournal(pageCursors())
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	// One moment for all three, which is the case a keyset has to survive and an offset does not.
	at := time.Now().UTC().Truncate(time.Microsecond)

	written := []shared.ID{
		shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe01"),
		shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe02"),
		shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe03"),
	}
	if err := uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		for i, id := range written {
			if err := journal.Record(ctx, adminrepo.InstanceEvent{
				ID: id, OccurredAt: at, Action: "tenant.provisioned",
				TenantID: idpTenantA, TenantSlug: "idp-a", ActorLabel: "The operator",
				Details: map[string]any{"seeded": i},
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("recording the journal entries: %v", err)
	}

	var page, second []adminrepo.InstanceEvent
	var info adminrepo.PageInfo
	if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		entries, read, err := journal.Page(ctx, "", 2)
		page, info = entries, read
		if err != nil || !info.HasMore {
			return err
		}
		second, _, err = journal.Page(ctx, info.NextCursor, 2)
		return err
	}); err != nil {
		t.Fatalf("reading the journal: %v", err)
	}

	if len(page) != 2 || !info.HasMore || info.NextCursor == "" {
		t.Fatalf("the first page is %d entries, more=%v, cursor=%q", len(page), info.HasMore, info.NextCursor)
	}
	// Newest first, and among entries of one moment the higher identifier first - which is what the
	// keyset orders by and what the cursor has to resume from.
	if page[0].ID != written[2] || page[1].ID != written[1] {
		t.Errorf("the first page is %s, %s", page[0].ID, page[1].ID)
	}
	if len(second) == 0 || second[0].ID != written[0] {
		t.Errorf("the second page starts at %+v, want the oldest of the three", second)
	}
	if page[0].TenantSlug != "idp-a" || page[0].ActorLabel != "The operator" {
		t.Errorf("the entry came back as %+v", page[0])
	}
	if page[0].Details["seeded"] == nil {
		t.Error("the details did not come back")
	}
}
