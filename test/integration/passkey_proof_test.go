// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The challenges a known account answers with its own key (identity.md §23.3, migration 0176): each
// row carries exactly what its purpose needs, and is spent once, by its own door, for its account and
// - a fresh proof's - its session.

func TestAPasskeyProofsRowCarriesWhatItsPurposeNeeds(t *testing.T) {
	ctx := context.Background()
	people := seedPasskeyPeople(ctx, t)
	admin := adminPool(ctx, t)
	session := freshID(t)
	if _, err := admin.Exec(ctx, `INSERT INTO session (id, tenant_id, account_id, created_at, last_seen_at, expires_at)
	    VALUES ($1, $2, $3, now(), now(), now() + interval '1 hour')`,
		session.String(), people.tenant.String(), people.ada.String()); err != nil {
		t.Fatalf("seeding a session: %v", err)
	}
	insert := func(purpose string, sessionID any, handle []byte) error {
		_, err := admin.Exec(ctx, `INSERT INTO auth_pending (id, tenant_id, account_id, token_hash, purpose,
		    created_at, expires_at, session_id, user_handle)
		    VALUES ($1, $2, $3, $4, $5, now(), now() + interval '5 minutes', $6, $7)`,
			freshID(t).String(), people.tenant.String(), people.ada.String(), freshID(t).String(),
			purpose, sessionID, handle)
		return err
	}
	handle := bytes.Repeat([]byte{0x22}, identity.PasskeyUserHandleBytes)
	for name, row := range map[string]struct {
		purpose string
		session any
		handle  []byte
	}{
		"a factor's challenge":  {"PASSKEY_FACTOR", nil, nil},
		"a setup's challenge":   {"PASSKEY_ENROLL", nil, handle},
		"a step-up's challenge": {"PASSKEY_STEP_UP", session.String(), nil},
		"a registration":        {"PASSKEY_REGISTER", session.String(), handle},
	} {
		if err := insert(row.purpose, row.session, row.handle); err != nil {
			t.Errorf("%s was refused: %v", name, err)
		}
	}
	for name, row := range map[string]struct {
		purpose string
		session any
		handle  []byte
	}{
		"a factor's challenge with a session":  {"PASSKEY_FACTOR", session.String(), nil},
		"a factor's challenge with a handle":   {"PASSKEY_FACTOR", nil, handle},
		"a setup's challenge without a handle": {"PASSKEY_ENROLL", nil, nil},
		"a setup's challenge with a session":   {"PASSKEY_ENROLL", session.String(), handle},
		"a setup's challenge, a short handle":  {"PASSKEY_ENROLL", nil, handle[:16]},
		"a step-up's without a session":        {"PASSKEY_STEP_UP", nil, nil},
		"a step-up's with a handle":            {"PASSKEY_STEP_UP", session.String(), handle},
		"a sign-in step with a session":        {"TOTP", session.String(), nil},
	} {
		if err := insert(row.purpose, row.session, row.handle); err == nil {
			t.Errorf("%s was stored", name)
		}
	}
}

func TestAPasskeyProofIsSpentOnceByItsOwnDoorAccountAndSession(t *testing.T) {
	ctx := context.Background()
	people := seedPasskeyPeople(ctx, t)
	admin := adminPool(ctx, t)
	sessionA, sessionB := freshID(t), freshID(t)
	for _, session := range []shared.ID{sessionA, sessionB} {
		if _, err := admin.Exec(ctx, `INSERT INTO session (id, tenant_id, account_id, created_at, last_seen_at, expires_at)
		    VALUES ($1, $2, $3, now(), now(), now() + interval '1 hour')`,
			session.String(), people.tenant.String(), people.ada.String()); err != nil {
			t.Fatalf("seeding a session: %v", err)
		}
	}
	repo := postgres.NewPasskeyProofRepository(security.NewPendingTokenHasher(secret.New(installationSecret)))
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	at := time.Now().UTC().Truncate(time.Second)
	handle := bytes.Repeat([]byte{0x55}, identity.PasskeyUserHandleBytes)

	type door struct {
		purpose identity.PendingPurpose
		mint    func(shared.ID, []byte) (identity.Token, error)
		session shared.ID
		handle  []byte
	}
	doors := map[string]door{
		"factor":  {identity.PendingPasskeyFactor, identity.NewPasskeyFactorToken, "", nil},
		"setup":   {identity.PendingPasskeyEnroll, identity.NewPasskeyEnrollToken, "", handle},
		"step-up": {identity.PendingPasskeyStepUp, identity.NewPasskeyStepUpToken, sessionA, nil},
	}
	fill := byte(0x10)
	for name, d := range doors {
		fill++
		token, err := d.mint(people.tenant, bytes.Repeat([]byte{fill}, 32))
		if err != nil {
			t.Fatal(err)
		}
		inTenant(t, uow, people.tenant, func(ctx context.Context) error {
			return repo.Issue(ctx, identity.PasskeyProofChallenge{
				ID: freshID(t), Purpose: d.purpose, AccountID: people.ada, SessionID: d.session,
				UserHandle: d.handle, CreatedAt: at, ExpiresAt: at.Add(5 * time.Minute),
			}, token)
		})
		spend := func(tenant shared.ID, purpose identity.PendingPurpose, account, session shared.ID, when time.Time) bool {
			var spent bool
			inTenant(t, uow, tenant, func(ctx context.Context) error {
				found, ok, err := repo.Spend(ctx, purpose, token, account, session, when)
				if ok && (!bytes.Equal(found.UserHandle, d.handle) || found.Purpose != purpose) {
					t.Errorf("%s: spent %+v", name, found)
				}
				spent = ok
				return err
			})
			return spent
		}
		other := identity.PendingPasskeyFactor
		if d.purpose == other {
			other = identity.PendingPasskeyStepUp
		}
		soon := at.Add(time.Second)
		for attempt, spent := range map[string]bool{
			"another door":      spend(people.tenant, other, people.ada, d.session, soon),
			"another account":   spend(people.tenant, d.purpose, people.bea, d.session, soon),
			"another session":   spend(people.tenant, d.purpose, people.ada, sessionB, soon),
			"another workspace": spend(people.other, d.purpose, people.ada, d.session, soon),
			"too late":          spend(people.tenant, d.purpose, people.ada, d.session, at.Add(5*time.Minute)),
			"a registration's":  spend(people.tenant, identity.PendingPasskeyRegister, people.ada, d.session, soon),
		} {
			if spent {
				t.Errorf("%s: %s spent the challenge", name, attempt)
			}
		}
		if !spend(people.tenant, d.purpose, people.ada, d.session, soon) {
			t.Fatalf("%s: its own door could not spend it", name)
		}
		if spend(people.tenant, d.purpose, people.ada, d.session, at.Add(2*time.Second)) {
			t.Errorf("%s: a challenge was spent twice", name)
		}
	}
}
