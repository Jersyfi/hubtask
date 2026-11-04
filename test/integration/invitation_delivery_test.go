// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The people list's delivery state reads the newest invitation job per account, of the reading
// workspace only: the queue has no row level security, so the statement's own tenant condition is
// what keeps another workspace's invitations out (gate SG-3).
func TestTheLastInvitationMailPerAccountStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	admin := adminPool(ctx, t)
	invitedA := seedAccount(ctx, t, tenantA)
	invitedB := seedAccount(ctx, t, tenantB)

	seed := func(tenant shared.ID, account shared.ID, kind, state, lastError, age string) {
		t.Helper()
		if _, err := admin.Exec(ctx, `
			INSERT INTO job (id, tenant_id, kind, state, dedupe_key, last_error, created_at, finished_at, payload)
			VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), now() - $7::interval,
			        CASE WHEN $4 IN ('PENDING', 'RUNNING') THEN NULL ELSE now() - $7::interval END, '{}')`,
			freshID(t).String(), tenant.String(), kind, state, account.String(), lastError, age); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	// An older mail that went out, then a renewal that failed: the newer one is the row's.
	seed(tenantA, invitedA, "notification.invitation", "SUCCEEDED", "", "2 days")
	seed(tenantA, invitedA, "notification.invitation", "DEAD_LETTER", "mail.recipient_rejected", "1 hour")
	// Another kind under the same key is not an invitation.
	seed(tenantA, invitedA, "notification.digest", "PENDING", "", "1 minute")
	// Tenant B's invitation, keyed by its own account.
	seed(tenantB, invitedB, "notification.invitation", "PENDING", "", "1 minute")

	jobs := postgres.NewInvitationJobRepository()
	latest := func(tenant shared.ID, ids ...shared.ID) []repository.InvitationJob {
		t.Helper()
		var got []repository.InvitationJob
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			got, err = jobs.Latest(ctx, ids)
			return err
		}); err != nil {
			t.Fatalf("reading: %v", err)
		}
		return got
	}

	got := latest(tenantA, invitedA, invitedB)
	if len(got) != 1 || got[0].AccountID != invitedA || got[0].State != "DEAD_LETTER" ||
		got[0].Code != "mail.recipient_rejected" || got[0].FinishedAt.IsZero() {
		t.Fatalf("tenant A read %+v, want only its own newest invitation", got)
	}
	// Tenant B asking about tenant A's account sees nothing of it.
	if got := latest(tenantB, invitedA); len(got) != 0 {
		t.Errorf("tenant B read tenant A's invitation: %+v", got)
	}
	if got := latest(tenantB, invitedB); len(got) != 1 || got[0].State != "PENDING" {
		t.Errorf("tenant B read %+v of its own", got)
	}
}
