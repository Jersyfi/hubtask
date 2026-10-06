// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-ID-06 check 4, against the real database: *Sign out everywhere else* leaves exactly the
// session that asked, and reaches no other workspace (gate SG-3). Its own account and sessions,
// named by nobody else, because this database is shared by every file in the package.
var (
	elsewhereAccount = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000023c1")
	elsewhereHere    = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000023d1")
	elsewherePhone   = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000023d2")
	elsewhereTablet  = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000023d3")
)

func TestSigningOutEverywhereElseLeavesExactlyThisSession(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		VALUES ($1, $2, 'USER', 'elsewhere@example.org', 'Elsewhere', 'ACTIVE')
		ON CONFLICT (id) DO NOTHING`, elsewhereAccount.String(), tenantA.String()); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO session (id, tenant_id, account_id, created_at, expires_at)
		VALUES ($1, $4, $5, now(), now() + interval '30 days'),
		       ($2, $4, $5, now(), now() + interval '30 days'),
		       ($3, $4, $5, now(), now() + interval '30 days')
		ON CONFLICT (id) DO UPDATE SET revoked_at = NULL`,
		elsewhereHere.String(), elsewherePhone.String(), elsewhereTablet.String(),
		tenantA.String(), elsewhereAccount.String()); err != nil {
		t.Fatalf("seeding the sessions: %v", err)
	}
	sessions, _, _, uow := sessionStores(ctx, t)

	// From another workspace, nothing ends (SG-3).
	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		ended, err := sessions.RevokeOthers(ctx, elsewhereAccount, elsewhereHere, time.Now())
		if err != nil {
			t.Fatalf("revoking from another tenant: %v", err)
		}
		if ended != 0 {
			t.Errorf("%d of another tenant's sessions ended from here", ended)
		}
		return nil
	})

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		ended, err := sessions.RevokeOthers(ctx, elsewhereAccount, elsewhereHere, time.Now())
		if err != nil {
			t.Fatalf("signing out everywhere else: %v", err)
		}
		if ended != 2 {
			t.Errorf("%d sessions ended, want the two others", ended)
		}
		for id, open := range map[shared.ID]bool{elsewhereHere: true, elsewherePhone: false, elsewhereTablet: false} {
			credential, err := sessions.FindForAuth(ctx, id)
			if err != nil {
				t.Fatalf("reading %s back: %v", id, err)
			}
			if got := credential.Session.Verify(time.Now()) == nil; got != open {
				t.Errorf("session %s open %v, want %v", id, got, open)
			}
		}

		// Nothing to spare: a caller with no session of its own ends every one.
		ended, err = sessions.RevokeOthers(ctx, elsewhereAccount, "", time.Now())
		if err != nil || ended != 1 {
			t.Errorf("sparing nothing ended %d (%v), want the last one", ended, err)
		}
		return nil
	})
}
