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
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
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
		Name: name, CreatedAt: at, RPID: "passkeys.example",
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
			len(first.Transports) != 2 || first.Transports[0] != "internal" || first.RPID != "passkeys.example" {
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

// A data backup keeps a passkey column for column, so a restore leaves the person their way in; an
// export's copy, without identifier, key and handle, is no credential and an import skips it
// (identity.md §23.5, tenant-export.md §9).
func TestARestoredPasskeyIsTheSameCredentialAndAnExportedOneIsNone(t *testing.T) {
	ctx := context.Background()
	people := seedPasskeyPeople(ctx, t)
	repo := postgres.NewPasskeyRepository()
	at := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	original := aPasskey(t, people.ada, 0x42, "iPhone", at)
	if err := write(ctx, t, people.tenant, func(ctx context.Context) error {
		_, err := repo.Insert(ctx, original)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	var archived map[string]any
	for _, row := range exported(ctx, t, people.tenant, 100, "webauthn_credential", time.Time{}) {
		if row.ID == original.ID.String() {
			archived = row.Data
		}
	}
	if archived == nil {
		t.Fatal("the passkey was not exported")
	}

	if _, err := adminPool(ctx, t).Exec(ctx, `DELETE FROM webauthn_credential WHERE tenant_id = $1`,
		people.tenant.String()); err != nil {
		t.Fatal(err)
	}

	// The export's copy first: skipped, nothing written, no error.
	redacted := map[string]any{}
	for column, value := range archived {
		redacted[column] = value
	}
	for _, column := range []string{"credential_id", "public_key", "user_handle"} {
		delete(redacted, column)
	}
	if err := write(ctx, t, people.tenant, func(ctx context.Context) error {
		written, err := importRepo().Write(ctx, "webauthn_credential", redacted, true)
		if written {
			t.Error("an export's passkey was written as a credential")
		}
		return err
	}); err != nil {
		t.Fatalf("importing the export's copy: %v", err)
	}

	// The backup's copy: the same credential, every column.
	if err := write(ctx, t, people.tenant, func(ctx context.Context) error {
		written, err := importRepo().Write(ctx, "webauthn_credential", archived, false)
		if !written {
			t.Error("the backup's passkey was not restored")
		}
		return err
	}); err != nil {
		t.Fatalf("restoring: %v", err)
	}
	var restored map[string]any
	for _, row := range exported(ctx, t, people.tenant, 100, "webauthn_credential", time.Time{}) {
		if row.ID == original.ID.String() {
			restored = row.Data
		}
	}
	for column, before := range archived {
		if after := restored[column]; !sameJSON(before, after) {
			t.Errorf("%s came back as %#v, want %#v", column, after, before)
		}
	}

	// A credential whose account the restore did not bring back is skipped, as is a second row
	// holding a restored identifier.
	stray := map[string]any{}
	for column, value := range archived {
		stray[column] = value
	}
	stray["id"] = freshID(t).String()
	if err := write(ctx, t, people.tenant, func(ctx context.Context) error {
		if written, err := importRepo().Write(ctx, "webauthn_credential", stray, true); err != nil || written {
			t.Errorf("a second row with a held identifier: %v %v", written, err)
		}
		stray["account_id"] = freshID(t).String()
		stray["credential_id"] = "AQIDBA"
		if written, err := importRepo().Write(ctx, "webauthn_credential", stray, true); err != nil || written {
			t.Errorf("a passkey of an account not restored: %v %v", written, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// A registration's challenge is spent once, only by its session and account, only under its
// purpose, only in time; a newer one spends the older; and another workspace never reaches it.
func TestAPasskeyChallengeIsSpentOnceByItsOwnSession(t *testing.T) {
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
	hasher := security.NewPendingTokenHasher(secret.New(installationSecret))
	repo := postgres.NewPasskeyChallengeRepository(hasher)
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	at := time.Now().UTC().Truncate(time.Second)
	handle := bytes.Repeat([]byte{0x44}, identity.PasskeyUserHandleBytes)

	issue := func(fill byte) identity.Token {
		token, err := identity.NewPasskeyChallengeToken(people.tenant, bytes.Repeat([]byte{fill}, 32))
		if err != nil {
			t.Fatal(err)
		}
		inTenant(t, uow, people.tenant, func(ctx context.Context) error {
			return repo.Issue(ctx, identity.PasskeyChallenge{
				ID: freshID(t), AccountID: people.ada, SessionID: sessionA, UserHandle: handle,
				CreatedAt: at, ExpiresAt: at.Add(identity.PasskeyRegistrationLifetime),
			}, token)
		})
		return token
	}
	consume := func(tenant shared.ID, token identity.Token, session, account shared.ID, when time.Time) bool {
		var spent bool
		inTenant(t, uow, tenant, func(ctx context.Context) error {
			found, ok, err := repo.Consume(ctx, token, session, account, when)
			if ok && (!bytes.Equal(found.UserHandle, handle) || found.SessionID != session) {
				t.Errorf("spent %+v", found)
			}
			spent = ok
			return err
		})
		return spent
	}

	older := issue(0x01)
	newer := issue(0x02)
	if consume(people.tenant, older, sessionA, people.ada, at.Add(time.Second)) {
		t.Error("a superseded challenge was spent")
	}
	for name, attempt := range map[string]func() bool{
		"another session":   func() bool { return consume(people.tenant, newer, sessionB, people.ada, at.Add(time.Second)) },
		"another account":   func() bool { return consume(people.tenant, newer, sessionA, people.bea, at.Add(time.Second)) },
		"another workspace": func() bool { return consume(people.other, newer, sessionA, people.ada, at.Add(time.Second)) },
		"too late":          func() bool { return consume(people.tenant, newer, sessionA, people.ada, at.Add(5*time.Minute)) },
	} {
		if attempt() {
			t.Errorf("%s spent the challenge", name)
		}
	}
	if !consume(people.tenant, newer, sessionA, people.ada, at.Add(time.Second)) {
		t.Fatal("its own session could not spend it")
	}
	if consume(people.tenant, newer, sessionA, people.ada, at.Add(2*time.Second)) {
		t.Error("a challenge was spent twice")
	}

	// A pending sign-in's token with the same secret is no challenge: the prefix is in the hash, and
	// the purpose in the statement.
	same := bytes.Repeat([]byte{0x03}, 32)
	signIn, err := identity.NewPendingToken(people.tenant, same)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO auth_pending (id, tenant_id, account_id, token_hash, purpose, created_at, expires_at)
	    VALUES ($1, $2, $3, $4, 'TOTP', now(), now() + interval '5 minutes')`,
		freshID(t).String(), people.tenant.String(), people.ada.String(), hasher.Hash(signIn.Secret())); err != nil {
		t.Fatal(err)
	}
	asChallenge, _ := identity.NewPasskeyChallengeToken(people.tenant, same)
	if consume(people.tenant, asChallenge, sessionA, people.ada, at.Add(time.Second)) {
		t.Error("a pending sign-in was spent as a challenge")
	}
	if consume(people.tenant, signIn, sessionA, people.ada, at.Add(time.Second)) {
		t.Error("a pending sign-in's own token was spent as a challenge")
	}
}
