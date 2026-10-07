// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The hosts a workspace answers at, against the real boundary (migration 0104).
//
// Gate SG-3 for a new table, and one invariant beyond it: **one host, one workspace,
// installation-wide**. That is the failure this table exists to make impossible, and it is the one
// an application-level check cannot hold - two workspaces claiming the same host at the same moment
// would both read "nobody has it".

func TestAWorkspacesHostsStayHomeAndAHostBelongsToOneWorkspace(t *testing.T) {
	ctx := context.Background()
	seedIdentityProviderTenants(ctx, t)

	hosts := postgres.NewTenantHostRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC()

	canonical, err := domain.NewCanonicalHost(domain.NewCanonicalHostInput{
		TenantID: idpTenantA, Host: "idp-a.hosts.example",
		Verification: domain.HostVerificationPrefix + "aaaabbbbcccc", Now: now,
	})
	if err != nil {
		t.Fatalf("building A's canonical host: %v", err)
	}

	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		if err := hosts.Insert(ctx, canonical); err != nil {
			t.Fatalf("writing A's host: %v", err)
		}
		return nil
	})

	// Gate SG-3: B sees none of it.
	inTenant(t, uow, idpTenantB, func(ctx context.Context) error {
		listed, err := hosts.List(ctx)
		if err != nil {
			t.Fatalf("B's list: %v", err)
		}
		if len(listed) != 0 {
			t.Errorf("B sees %d of A's hosts", len(listed))
		}
		return nil
	})

	// And B cannot claim it. The unique index is installation-wide, so the refusal crosses the
	// boundary although the read does not - which is the whole point: a host B could claim is a host
	// B could answer at.
	//
	// In a transaction of its own, because a unique violation poisons the one it happens in: every
	// statement after it is refused and the commit becomes a rollback. That is PostgreSQL's rule
	// rather than this repository's, and it is why a caller that means to survive a conflict has to
	// end the transaction on it.
	stolen, err := domain.NewCanonicalHost(domain.NewCanonicalHostInput{
		TenantID: idpTenantB, Host: "idp-a.hosts.example",
		Verification: domain.HostVerificationPrefix + "ddddeeeeffff", Now: now,
	})
	if err != nil {
		t.Fatalf("building B's claim: %v", err)
	}
	claimed := uow.Within(ctx, persistence.Scope{TenantID: idpTenantB}, func(ctx context.Context) error {
		return hosts.Insert(ctx, stolen)
	})
	if claimed == nil {
		t.Error("B claimed a host that belongs to A")
	} else if !isHostTaken(claimed) {
		t.Errorf("B's claim answered %v, want a conflict", claimed)
	}

	// A reads its own, canonical and verified by construction.
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		listed, err := hosts.List(ctx)
		if err != nil {
			t.Fatalf("A's list: %v", err)
		}
		if len(listed) != 1 {
			t.Fatalf("A holds %d hosts, want one", len(listed))
		}
		row := listed[0]
		if row.Host != "idp-a.hosts.example" || !row.Canonical || row.State != domain.HostActive {
			t.Errorf("A's host came back as %+v", row)
		}
		if row.TenantID != idpTenantA {
			t.Errorf("the mapped row belongs to %q, want A", row.TenantID)
		}
		if row.VerifiedAt.IsZero() {
			t.Error("an active host carries no moment")
		}

		return nil
	})

	// One canonical host per workspace, in the index: a second is refused rather than leaving two
	// rows for a mail to choose between. Its own transaction, for the reason B's claim needed one.
	second, err := domain.NewCanonicalHost(domain.NewCanonicalHostInput{
		TenantID: idpTenantA, Host: "another.hosts.example",
		Verification: domain.HostVerificationPrefix + "gggghhhhiiii", Now: now,
	})
	if err != nil {
		t.Fatalf("building a second canonical host: %v", err)
	}
	if err := uow.Within(ctx, persistence.Scope{TenantID: idpTenantA}, func(ctx context.Context) error {
		return hosts.Insert(ctx, second)
	}); err == nil {
		t.Error("a workspace took a second canonical host")
	}
}

// isHostTaken is the reading of "somebody already has it" this file gives.
func isHostTaken(err error) bool { return errors.Is(err, shared.ErrConflict) }
