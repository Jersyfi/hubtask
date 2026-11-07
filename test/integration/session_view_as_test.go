// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// *View as* on the session row (UC-ID-29), against the real statements: the view is written on the
// holder's own session, the person is joined on authentication, a private hub reached through a
// group is seen, ending clears it, and another workspace's scope writes and reads nothing.

var (
	viewAsHolder  = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000139a1")
	viewAsPerson  = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000139a2")
	viewAsSession = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000139a3")
	viewAsHub     = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000139a4")
	viewAsGroup   = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000139a5")
)

func TestTheViewIsWrittenOnTheHoldersSessionAndStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	writer, _, uow := realWriter(ctx, t)
	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO account (id, tenant_id, kind, email, display_name, status)
		  VALUES ($1, $3, 'USER', 'viewer@example.org', 'Viewer', 'ACTIVE'),
		         ($2, $3, 'USER', 'viewed@example.org', 'Viewed', 'ACTIVE') ON CONFLICT (id) DO NOTHING`,
			[]any{viewAsHolder.String(), viewAsPerson.String(), tenantA.String()}},
		{`DELETE FROM session WHERE id = $1`, []any{viewAsSession.String()}},
		{`DELETE FROM membership WHERE scope_id = $1`, []any{viewAsHub.String()}},
		{`INSERT INTO session (id, tenant_id, account_id, created_at, expires_at)
		  VALUES ($1, $2, $3, now(), now() + interval '30 days')`,
			[]any{viewAsSession.String(), tenantA.String(), viewAsHolder.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	until := now.Add(identity.ViewAsLifetime)

	// Another workspace's scope matches no session of this one.
	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		started, err := writer.Sessions.StartViewingAs(ctx, viewAsSession, viewAsHolder, viewAsPerson, until, now)
		if err != nil || started {
			t.Errorf("another workspace started a view: %v, %v", started, err)
		}
		ended, err := writer.Sessions.EndViewingAs(ctx, viewAsSession, viewAsHolder)
		if err != nil || ended {
			t.Errorf("another workspace ended a view: %v, %v", ended, err)
		}
		return nil
	})

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		// Somebody else's session is not marked.
		if started, err := writer.Sessions.StartViewingAs(ctx, viewAsSession, viewAsPerson, viewAsHolder, until, now); err != nil || started {
			t.Fatalf("a session not the caller's answered %v, %v", started, err)
		}
		started, err := writer.Sessions.StartViewingAs(ctx, viewAsSession, viewAsHolder, viewAsPerson, until, now)
		if err != nil || !started {
			t.Fatalf("starting the view answered %v, %v", started, err)
		}
		credential, err := writer.Sessions.FindForAuth(ctx, viewAsSession)
		if err != nil {
			t.Fatalf("reading the session: %v", err)
		}
		if !credential.Session.IsViewingAs(now.Add(time.Minute)) || credential.Viewed.ID != viewAsPerson ||
			credential.Viewed.DisplayName != "Viewed" || credential.Viewed.Kind != identity.AccountUser {
			t.Errorf("the view read back as %+v / %+v", credential.Session, credential.Viewed)
		}
		if credential.ViewedReachesPrivateHub {
			t.Error("a person in no private hub reads as in one")
		}
		return nil
	})

	// A private hub reached through a group.
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO container (id, tenant_id, type, name, order_key, created_by, private)
		  VALUES ($1, $2, 'HUB', 'Family', 'a0', $3, true) ON CONFLICT (id) DO NOTHING`,
			[]any{viewAsHub.String(), tenantA.String(), viewAsPerson.String()}},
		{`INSERT INTO account_group (id, tenant_id, name) VALUES ($1, $2, 'Viewed group') ON CONFLICT (id) DO NOTHING`,
			[]any{viewAsGroup.String(), tenantA.String()}},
		{`INSERT INTO account_group_member (tenant_id, group_id, account_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
			[]any{tenantA.String(), viewAsGroup.String(), viewAsPerson.String()}},
		{`INSERT INTO membership (id, tenant_id, group_id, scope_type, scope_id, role)
		  VALUES (gen_random_uuid(), $1, $2, 'HUB', $3, 'MEMBER')`,
			[]any{tenantA.String(), viewAsGroup.String(), viewAsHub.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding the private hub: %v", err)
		}
	}

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		credential, err := writer.Sessions.FindForAuth(ctx, viewAsSession)
		if err != nil {
			t.Fatalf("reading the session: %v", err)
		}
		if !credential.ViewedReachesPrivateHub {
			t.Error("a private hub reached through a group is not seen")
		}
		ended, err := writer.Sessions.EndViewingAs(ctx, viewAsSession, viewAsHolder)
		if err != nil || !ended {
			t.Fatalf("ending answered %v, %v", ended, err)
		}
		credential, err = writer.Sessions.FindForAuth(ctx, viewAsSession)
		if err != nil {
			t.Fatalf("reading the session: %v", err)
		}
		if !credential.Session.ViewingAs.IsZero() || !credential.Viewed.ID.IsZero() {
			t.Errorf("after the end the session still views %+v", credential.Viewed)
		}
		return nil
	})
}
