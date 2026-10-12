// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The operator's hour on the session row (ADR-0070 §4, UC-INS-04 checks 3 and 4), against the real
// statements: a refresh rotates the pair on the same session and leaves the hour where it was, and a
// running hour is not written over.

var (
	elevationAccount = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000135e1")
	elevationSession = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000135e2")
	elevationRefresh = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000135e3")
)

func TestARefreshKeepsTheHourAndARunningHourIsNotReplaced(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	writer, _, uow := realWriter(ctx, t)
	admin := adminPool(ctx, t)

	presented, err := identity.NewRefreshToken(tenantA, sessionSecretOf(0x35))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	hasher := security.NewSessionRefreshHasher(secret.New(installationSecret))
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		  VALUES ($1, $2, 'USER', 'hour@example.org', 'Hour', 'ACTIVE') ON CONFLICT (id) DO NOTHING`,
			[]any{elevationAccount.String(), tenantA.String()}},
		// A run before this one left the row raised and its token rotated: start again.
		{`DELETE FROM session WHERE id = $1`, []any{elevationSession.String()}},
		{`INSERT INTO session (id, tenant_id, account_id, created_at, expires_at)
		  VALUES ($1, $2, $3, now(), now() + interval '30 days')`,
			[]any{elevationSession.String(), tenantA.String(), elevationAccount.String()}},
		{`INSERT INTO session_refresh_token (id, tenant_id, session_id, token_hash, created_at, expires_at)
		  VALUES ($1, $2, $3, $4, now(), now() + interval '30 days')`,
			[]any{elevationRefresh.String(), tenantA.String(), elevationSession.String(), hasher.Hash(presented.Secret())}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	until := now.Add(identity.ElevationLifetime)
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		raised, err := writer.Sessions.Elevate(ctx, elevationSession, elevationAccount, until, now)
		if err != nil || !raised {
			t.Fatalf("raising the session answered %v, %v", raised, err)
		}
		// A second raise while the hour runs writes nothing (check 4).
		again, err := writer.Sessions.Elevate(ctx, elevationSession, elevationAccount,
			now.Add(30*time.Minute+identity.ElevationLifetime), now.Add(30*time.Minute))
		if err != nil || again {
			t.Fatalf("a second raise inside the hour answered %v, %v", again, err)
		}
		return nil
	})

	// The refresh the web app makes every quarter of an hour (check 3).
	pair, err := identityservice.RefreshSession{Writer: writer}.Execute(ctx, identityservice.RefreshSessionCommand{
		RefreshToken: secret.New(presented.Secret()),
	})
	if err != nil {
		t.Fatalf("refreshing: %v", err)
	}
	if pair.RefreshToken.Reveal() == presented.Secret() {
		t.Fatal("the refresh did not rotate")
	}

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		kept, err := writer.Sessions.ElevatedUntil(ctx, elevationSession, elevationAccount)
		if err != nil {
			t.Fatalf("reading the hour: %v", err)
		}
		if !kept.Equal(until) {
			t.Errorf("after a refresh the hour ends at %v, want %v", kept, until)
		}
		credential, err := writer.Sessions.FindForAuth(ctx, elevationSession)
		if err != nil {
			t.Fatalf("reading the session: %v", err)
		}
		if !credential.Session.IsElevated(now.Add(time.Minute)) {
			t.Error("the refreshed session no longer carries the hour")
		}
		return nil
	})

	// Another workspace's scope reads no hour for this session.
	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		seen, err := writer.Sessions.ElevatedUntil(ctx, elevationSession, elevationAccount)
		if err != nil || !seen.IsZero() {
			t.Errorf("another workspace read the hour: %v, %v", seen, err)
		}
		return nil
	})
}
