// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The WebAuthn credentials as the database keeps them (identity.md §23, migration 0161): under the
// workspace's row security, once per identifier, and the limit's count held under the account lock.

type passkeyPeople struct {
	tenant, other  shared.ID
	ada, bea, stan shared.ID
}

func seedPasskeyPeople(ctx context.Context, t *testing.T) passkeyPeople {
	t.Helper()
	admin := adminPool(ctx, t)
	people := passkeyPeople{
		tenant: freshID(t), other: freshID(t), ada: freshID(t), bea: freshID(t), stan: freshID(t),
	}
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'Passkeys'), ($3, $4, 'Next door')`,
			[]any{people.tenant.String(), slugOf(people.tenant), people.other.String(), slugOf(people.other)}},
		{`INSERT INTO account (id, tenant_id, email, display_name) VALUES
		    ($1, $3, 'ada@example.org', 'Ada'), ($2, $3, 'bea@example.org', 'Bea')`,
			[]any{people.ada.String(), people.bea.String(), people.tenant.String()}},
		{`INSERT INTO account (id, tenant_id, email, display_name) VALUES ($1, $2, 'stan@example.org', 'Stan')`,
			[]any{people.stan.String(), people.other.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding: %v\n%s", err, statement.sql)
		}
	}
	return people
}

func aPasskey(t *testing.T, account shared.ID, credentialID byte, name string, at time.Time) identity.PasskeyCredential {
	return identity.PasskeyCredential{
		ID: freshID(t), AccountID: account,
		CredentialID: bytes.Repeat([]byte{credentialID}, 16), PublicKey: []byte{0xa5, 0x01, 0x02},
		Algorithm: -7, SignCount: 3, UserHandle: bytes.Repeat([]byte{0x11}, identity.PasskeyUserHandleBytes),
		BackupEligible: true, BackedUp: true, Transports: []string{"internal", "hybrid"},
		Name: name, CreatedAt: at,
	}
}

func TestAPasskeyIsKeptOnceAndReadOnlyInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	people := seedPasskeyPeople(ctx, t)
	repo := postgres.NewPasskeyRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	at := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)

	inTenant(t, uow, people.tenant, func(ctx context.Context) error {
		for _, credential := range []identity.PasskeyCredential{
			aPasskey(t, people.ada, 1, "iPhone", at),
			aPasskey(t, people.ada, 2, "Key in the drawer", at.Add(time.Minute)),
			aPasskey(t, people.bea, 3, "Laptop", at),
		} {
			if written, err := repo.Insert(ctx, credential); err != nil || !written {
				t.Fatalf("inserting %q: %v %v", credential.Name, written, err)
			}
		}
		// The same identifier again is not a second credential, whoever presents it.
		if written, err := repo.Insert(ctx, aPasskey(t, people.bea, 1, "Copy", at)); err != nil || written {
			t.Errorf("a held identifier was written again: %v %v", written, err)
		}
		return nil
	})

	inTenant(t, uow, people.tenant, func(ctx context.Context) error {
		held, err := repo.ListOf(ctx, people.ada)
		if err != nil {
			t.Fatal(err)
		}
		if len(held) != 2 || held[0].Name != "iPhone" || held[1].Name != "Key in the drawer" {
			t.Fatalf("Ada's passkeys read back as %+v", held)
		}
		first := held[0]
		if first.TenantID != people.tenant || first.AccountID != people.ada || first.Algorithm != -7 ||
			first.SignCount != 3 || !first.BackupEligible || !first.BackedUp || !first.CreatedAt.Equal(at) ||
			!first.LastUsedAt.IsZero() || len(first.UserHandle) != identity.PasskeyUserHandleBytes ||
			!bytes.Equal(first.CredentialID, bytes.Repeat([]byte{1}, 16)) ||
			len(first.Transports) != 2 || first.Transports[0] != "internal" {
			t.Errorf("the stored shape: %+v", first)
		}
		if count, err := repo.CountOf(ctx, people.ada); err != nil || count != 2 {
			t.Errorf("Ada holds %d (%v)", count, err)
		}
		if count, err := repo.CountOf(ctx, people.bea); err != nil || count != 1 {
			t.Errorf("Bea holds %d (%v)", count, err)
		}
		return nil
	})

	// Gate SG-3: next door reads nothing of ours, cannot lock our account, and cannot write a
	// credential onto it.
	inTenant(t, uow, people.other, func(ctx context.Context) error {
		if held, err := repo.ListOf(ctx, people.ada); err != nil || len(held) != 0 {
			t.Errorf("next door read %+v (%v)", held, err)
		}
		if count, err := repo.CountOf(ctx, people.ada); err != nil || count != 0 {
			t.Errorf("next door counted %d (%v)", count, err)
		}
		if err := repo.LockAccount(ctx, people.ada); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("next door locked our account: %v", err)
		}
		return nil
	})
	err := write(ctx, t, people.other, func(ctx context.Context) error {
		_, err := repo.Insert(ctx, aPasskey(t, people.ada, 9, "Planted", at))
		return err
	})
	if err == nil {
		t.Error("next door wrote a credential onto our account")
	}

	// And its own identifiers are its own: the same bytes are a different credential there.
	inTenant(t, uow, people.other, func(ctx context.Context) error {
		if written, err := repo.Insert(ctx, aPasskey(t, people.stan, 1, "Same bytes", at)); err != nil || !written {
			t.Errorf("an identifier held next door was refused here: %v %v", written, err)
		}
		return nil
	})
}

// The limit's race: a second registration that locks the account waits for the first, and then
// counts what the first wrote.
func TestASecondRegistrationWaitsForTheFirstAndCountsIt(t *testing.T) {
	ctx := context.Background()
	people := seedPasskeyPeople(ctx, t)
	repo := postgres.NewPasskeyRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	at := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)

	inTenant(t, uow, people.tenant, func(ctx context.Context) error {
		for i := range identity.MaxPasskeys - 1 {
			if _, err := repo.Insert(ctx, aPasskey(t, people.ada, byte(i+1), "Key", at)); err != nil {
				t.Fatal(err)
			}
		}
		return nil
	})

	counted := make(chan int, 1)
	failed := make(chan error, 1)
	var waited bool
	inTenant(t, uow, people.tenant, func(ctx context.Context) error {
		if err := repo.LockAccount(ctx, people.ada); err != nil {
			t.Fatal(err)
		}
		if count, err := repo.CountOf(ctx, people.ada); err != nil || count != identity.MaxPasskeys-1 {
			t.Fatalf("the first counted %d (%v)", count, err)
		}
		//nolint:contextcheck // a second transaction: the context handed in carries this one
		go func() {
			err := write(context.Background(), t, people.tenant, func(ctx context.Context) error {
				if err := repo.LockAccount(ctx, people.ada); err != nil {
					return err
				}
				count, err := repo.CountOf(ctx, people.ada)
				counted <- count
				return err
			})
			if err != nil {
				failed <- err
			}
		}()
		select {
		case <-counted:
			t.Error("the second registration counted while the first held the account")
		case <-time.After(500 * time.Millisecond):
			waited = true
		}
		_, err := repo.Insert(ctx, aPasskey(t, people.ada, 0xee, "The tenth", at))
		return err
	})
	if !waited {
		return
	}
	select {
	case count := <-counted:
		if count != identity.MaxPasskeys {
			t.Errorf("the second counted %d after the first committed, want %d", count, identity.MaxPasskeys)
		}
	case err := <-failed:
		t.Fatalf("the second transaction: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the second registration never ran")
	}
}

// The registration challenge's columns belong to its purpose and to no other (migration 0161).
func TestAPendingRegistrationCarriesItsSessionAndHandleAndNoOtherRowDoes(t *testing.T) {
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
	if err := insert("PASSKEY_REGISTER", session.String(), handle); err != nil {
		t.Errorf("a whole registration was refused: %v", err)
	}
	for name, row := range map[string]struct {
		purpose string
		session any
		handle  []byte
	}{
		"a registration without a session": {"PASSKEY_REGISTER", nil, handle},
		"a registration without a handle":  {"PASSKEY_REGISTER", session.String(), nil},
		"a registration with a short one":  {"PASSKEY_REGISTER", session.String(), handle[:16]},
		"a sign-in step with a session":    {"TOTP", session.String(), nil},
		"a sign-in step with a handle":     {"TOTP", nil, handle},
		"a session of nobody":              {"PASSKEY_REGISTER", freshID(t).String(), handle},
	} {
		if err := insert(row.purpose, row.session, row.handle); err == nil {
			t.Errorf("%s was stored", name)
		}
	}
}
