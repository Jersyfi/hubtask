// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// rehashAccountIn seeds an account of the test's own with a stored hash, a password moment and a
// start-password mark, so that what the rehash must leave alone is there to be left alone.
func rehashAccountIn(ctx context.Context, t *testing.T, tenant shared.ID, hash string) shared.ID {
	t.Helper()
	id := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, password_hash, status,
		                     password_set_at, start_password_at, updated_at)
		VALUES ($1, $2, 'USER', $3, 'Rehash', $4, 'ACTIVE', $5, $5, $5)`,
		id.String(), tenant.String(), "rehash-"+id.String()+"@example.org", hash, created); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
	return id
}

type storedPassword struct {
	hash             string
	setAt, startAt   time.Time
	updatedAt        time.Time
	version, history int
}

func storedPasswordOf(ctx context.Context, t *testing.T, account shared.ID) storedPassword {
	t.Helper()
	var read storedPassword
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT a.password_hash, a.password_set_at, a.start_password_at, a.updated_at, a.version,
		       (SELECT count(*) FROM account_password_history h WHERE h.account_id = a.id)
		FROM account a WHERE a.id = $1`, account.String()).Scan(
		&read.hash, &read.setAt, &read.startAt, &read.updatedAt, &read.version, &read.history); err != nil {
		t.Fatalf("reading the account: %v", err)
	}
	return read
}

// Gate SG-3: RehashPassword. Under another workspace's context the statement matches nothing; in
// its own it writes only while the stored hash is the one presented as current, and moves nothing
// but the hash and updated_at - the same password keeps its age, its mark, its history and its
// version.
func TestARehashWritesOnlyTheCurrentHashAndOnlyInItsTenant(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	_, _, signIn, uow := sessionStores(ctx, t)
	account := rehashAccountIn(ctx, t, tenantA, "old-cost-hash")
	before := storedPasswordOf(ctx, t, account)
	at := created.Add(48 * time.Hour)

	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		changed, err := signIn.RehashPassword(ctx, account, "old-cost-hash", "from-b", at)
		if err != nil || changed {
			t.Errorf("another tenant's account was rehashed from here (%v, %v)", changed, err)
		}
		return nil
	})
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		changed, err := signIn.RehashPassword(ctx, account, "a-hash-changed-meanwhile", "stale", at)
		if err != nil || changed {
			t.Errorf("a stale current hash overwrote the stored one (%v, %v)", changed, err)
		}
		return nil
	})
	if got := storedPasswordOf(ctx, t, account); got != before {
		t.Fatalf("a refused rehash changed the row: %+v, was %+v", got, before)
	}

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		changed, err := signIn.RehashPassword(ctx, account, "old-cost-hash", "current-cost-hash", at)
		if err != nil || !changed {
			t.Errorf("the own account's rehash did not write (%v, %v)", changed, err)
		}
		return nil
	})
	after := storedPasswordOf(ctx, t, account)
	if after.hash != "current-cost-hash" || !after.updatedAt.Equal(at) {
		t.Errorf("stored %q at %v, want the new hash at %v", after.hash, after.updatedAt, at)
	}
	if !after.setAt.Equal(before.setAt) || !after.startAt.Equal(before.startAt) ||
		after.version != before.version || after.history != before.history {
		t.Errorf("the rehash moved what belongs to the password: %+v, was %+v", after, before)
	}
}

// rehashSignals counts what the sign-in reports of the rehash and ignores the rest.
type rehashSignals struct{ rehashed int }

func (*rehashSignals) AuthFailure(context.Context, string)    {}
func (*rehashSignals) SignInStep(context.Context, string)     {}
func (*rehashSignals) PasswordResetRequested(context.Context) {}
func (s *rehashSignals) PasswordRehashed(context.Context)     { s.rehashed++ }

// T-02, identity.md §5.3, end to end: a hash an older build wrote under a lower cost is replaced
// by the next sign-in that proves its password - through the real Argon2id adapter, a real
// transaction and the application's role under row level security - and the new hash carries the
// build's cost and still verifies. A second sign-in finds nothing to do.
func TestASignInBringsAnOldCostHashUpToTheBuildsCost(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	writer, _, _ := realWriter(ctx, t)
	signals := &rehashSignals{}
	writer.Signals = signals

	const passphrase = "an older build hashed this one"
	salt := []byte("sixteen-byte-slt")
	cheap := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, 8*1024, 1, 1,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(argon2.IDKey([]byte(passphrase), salt, 1, 8*1024, 1, 32)))
	account := rehashAccountIn(ctx, t, tenantA, cheap)
	var email string
	if err := adminPool(ctx, t).QueryRow(ctx, `SELECT email FROM account WHERE id = $1`,
		account.String()).Scan(&email); err != nil {
		t.Fatalf("reading the address: %v", err)
	}

	signIn := func() {
		t.Helper()
		if _, err := (identityservice.SignIn{Writer: writer}).Execute(ctx, identityservice.SignInCommand{
			Email: email, Password: secret.New(passphrase), TenantHeader: tenantA.String(),
		}); err != nil {
			t.Fatalf("signing in: %v", err)
		}
	}

	signIn()
	stored := storedPasswordOf(ctx, t, account)
	if stored.hash == cheap || !strings.Contains(stored.hash, "$m=65536,t=3,p=2$") {
		t.Fatalf("the stored hash %q is not at the build's cost", stored.hash)
	}
	if ok, err := writer.Passwords.Verify(stored.hash, secret.New(passphrase)); err != nil || !ok {
		t.Fatalf("the new hash does not verify the same password (%v, %v)", ok, err)
	}
	if signals.rehashed != 1 {
		t.Errorf("counted %d rehashes, want one", signals.rehashed)
	}

	signIn()
	if again := storedPasswordOf(ctx, t, account); again.hash != stored.hash || signals.rehashed != 1 {
		t.Errorf("a current hash was rehashed again (%d counted)", signals.rehashed)
	}
}
