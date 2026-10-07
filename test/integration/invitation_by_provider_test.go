// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// SC-24: an invited account accepts its invitation through the provider - ACTIVE and the invitation
// spent in one statement, only while the invitation has not run out, and only in its own workspace
// (gate SG-3). Its own accounts, named by nobody else in this shared database.
var (
	invitedFresh  = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000024c1")
	invitedLapsed = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000024c2")
)

func TestAnInvitationIsAcceptedOnceAndOnlyWhileItStands(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, status,
		                     redemption_token_hash, redemption_expires_at)
		VALUES ($1, $3, 'USER', 'invited-fresh@example.org', 'Fresh', 'INVITED', '\x01', now() + interval '7 days'),
		       ($2, $3, 'USER', 'invited-lapsed@example.org', 'Lapsed', 'INVITED', '\x02', now() - interval '1 day')
		ON CONFLICT (id) DO UPDATE SET status = 'INVITED',
		  redemption_token_hash = EXCLUDED.redemption_token_hash,
		  redemption_expires_at = EXCLUDED.redemption_expires_at`,
		invitedFresh.String(), invitedLapsed.String(), tenantA.String()); err != nil {
		t.Fatalf("seeding the invitations: %v", err)
	}
	accounts := postgres.NewAccountRepository()
	_, _, _, uow := sessionStores(ctx, t)
	now := time.Now()

	// From another workspace, nothing is accepted (SG-3).
	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		if accepted, err := accounts.AcceptInvitation(ctx, invitedFresh, now); err != nil || accepted {
			t.Errorf("another workspace accepted the invitation: (%v, %v)", accepted, err)
		}
		return nil
	})

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		accepted, err := accounts.AcceptInvitation(ctx, invitedFresh, now)
		if err != nil || !accepted {
			t.Fatalf("a standing invitation was not accepted: (%v, %v)", accepted, err)
		}
		again, err := accounts.AcceptInvitation(ctx, invitedFresh, now)
		if err != nil || again {
			t.Errorf("an invitation was accepted twice: (%v, %v)", again, err)
		}
		lapsed, err := accounts.AcceptInvitation(ctx, invitedLapsed, now)
		if err != nil || lapsed {
			t.Errorf("a lapsed invitation was accepted: (%v, %v)", lapsed, err)
		}
		return nil
	})

	var status string
	var hash []byte
	if err := admin.QueryRow(ctx, `SELECT status, redemption_token_hash FROM account WHERE id = $1`,
		invitedFresh.String()).Scan(&status, &hash); err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if status != "ACTIVE" || hash != nil {
		t.Errorf("the accepted account is %s with an invitation hash %v, want ACTIVE and spent", status, hash)
	}
}
