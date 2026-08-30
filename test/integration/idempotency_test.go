// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/idempotency"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Neither query behind the store names a tenant: ReserveIdempotencyKey takes it from
// current_tenant_id(), and the find and the update carry no tenant condition at all. Row level
// security is the whole of the boundary here, which is why it is tested against a real database
// rather than argued about (ADR-0010, engineering-guidelines.md §1).

// keyFor keeps each test on its own key. The container is shared across the package, and a key is
// claimed exactly once by design - two tests sharing one would depend on the order they ran in.
func keyFor(name string) repository.Key {
	return repository.Key{Key: name, Endpoint: "POST /v1/containers"}
}

// compacted is how a stored body is compared. The column is jsonb, which keeps a value rather
// than the bytes it arrived as: it drops insignificant whitespace, and is free to reorder object
// keys. Comparing the raw bytes would be asserting something PostgreSQL never promised.
func compacted(t *testing.T, body []byte) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, body); err != nil {
		t.Fatalf("compacting %q: %v", body, err)
	}
	return buf.String()
}

func idempotencyStore(ctx context.Context, t *testing.T) (repository.Store, persistence.UnitOfWork) {
	t.Helper()
	return postgres.NewIdempotencyStore(), postgres.NewUnitOfWork(appPool(ctx, t))
}

// reserveAs runs one Reserve in its own transaction, as the given tenant.
func reserveAs(
	ctx context.Context,
	t *testing.T,
	tenant shared.ID,
	key repository.Key,
	hash []byte,
) (repository.Record, bool) {
	t.Helper()
	store, uow := idempotencyStore(ctx, t)

	var (
		record  repository.Record
		claimed bool
	)
	if err := uow.Within(ctx, persistence.Scope{TenantID: tenant}, func(ctx context.Context) error {
		var err error
		record, claimed, err = store.Reserve(ctx, key, hash)
		return err
	}); err != nil {
		t.Fatalf("reserving as %s: %v", tenant, err)
	}
	return record, claimed
}

// completeAs runs one Complete in its own transaction, as the given tenant.
func completeAs(
	ctx context.Context,
	t *testing.T,
	tenant shared.ID,
	key repository.Key,
	status int,
	body []byte,
) {
	t.Helper()
	store, uow := idempotencyStore(ctx, t)

	if err := uow.Within(ctx, persistence.Scope{TenantID: tenant}, func(ctx context.Context) error {
		return store.Complete(ctx, key, status, body)
	}); err != nil {
		t.Fatalf("completing as %s: %v", tenant, err)
	}
}

// The claim is what makes the guard work: exactly one attempt may proceed, and the second one is
// told it lost rather than being allowed to run the operation a second time.
func TestAKeyIsClaimedOnlyOnceInsideATenant(t *testing.T) {
	ctx := context.Background()
	key := keyFor("claimed-once")
	hash := []byte("request-hash")

	if _, claimed := reserveAs(ctx, t, tenantA, key, hash); !claimed {
		t.Fatal("the first attempt did not claim the key")
	}

	record, claimed := reserveAs(ctx, t, tenantA, key, hash)
	if claimed {
		t.Error("the second attempt claimed the same key again")
	}
	if !record.InProgress() {
		t.Errorf("the unfinished attempt reports status %d, want 0", record.Status)
	}
	if !bytes.Equal(record.RequestHash, hash) {
		t.Errorf("request hash %q, want %q", record.RequestHash, hash)
	}
}

// The same key in another tenant is another attempt. Two tenants are not coordinating their
// Idempotency-Key headers, and a claim that leaked across the boundary would refuse one tenant's
// request because an unrelated one got there first.
func TestTheSameKeyIsStillFreeInAnotherTenant(t *testing.T) {
	ctx := context.Background()
	key := keyFor("free-elsewhere")

	if _, claimed := reserveAs(ctx, t, tenantA, key, []byte("hash-a")); !claimed {
		t.Fatal("tenant A did not claim the key")
	}

	// The row of tenant A exists. If it were visible here, the insert would conflict and this
	// would come back false.
	if _, claimed := reserveAs(ctx, t, tenantB, key, []byte("hash-b")); !claimed {
		t.Error("tenant B was refused a key that only tenant A had claimed")
	}
}

// The cross-tenant negative for both methods at once. FindIdempotencyRecord and
// CompleteIdempotencyRecord match on key and endpoint alone, so with row level security removed
// this test would read - and overwrite - the other tenant's answer.
func TestAnAnswerNeitherLeaksNorIsOverwrittenAcrossTenants(t *testing.T) {
	ctx := context.Background()
	key := keyFor("answers-stay-put")
	bodyA := []byte(`{"tenant":"a"}`)
	bodyB := []byte(`{"tenant":"b"}`)

	if _, claimed := reserveAs(ctx, t, tenantA, key, []byte("hash-a")); !claimed {
		t.Fatal("tenant A did not claim the key")
	}
	completeAs(ctx, t, tenantA, key, 201, bodyA)

	// Tenant B claims the same key freshly: A's finished record must not be an answer here.
	record, claimed := reserveAs(ctx, t, tenantB, key, []byte("hash-b"))
	if !claimed {
		t.Fatalf("tenant B was handed tenant A's record: status %d, body %q", record.Status, record.Body)
	}
	completeAs(ctx, t, tenantB, key, 500, bodyB)

	// And A's answer is still A's, untouched by B's update.
	replay, claimed := reserveAs(ctx, t, tenantA, key, []byte("hash-a"))
	if claimed {
		t.Fatal("tenant A's completed record disappeared")
	}
	if replay.Status != 201 {
		t.Errorf("tenant A replays status %d, want 201 - tenant B's answer overwrote it", replay.Status)
	}
	if got, want := compacted(t, replay.Body), compacted(t, bodyA); got != want {
		t.Errorf("tenant A replays body %s, want %s", got, want)
	}
}

// A body that is not JSON is dropped rather than stored: the column is jsonb, and failing the
// request that produced it would compound the bug.
func TestANonJSONBodyIsStoredAsNoBody(t *testing.T) {
	ctx := context.Background()
	key := keyFor("not-json")

	if _, claimed := reserveAs(ctx, t, tenantA, key, []byte("hash")); !claimed {
		t.Fatal("the key was not claimed")
	}
	completeAs(ctx, t, tenantA, key, 200, []byte("this is not json"))

	record, claimed := reserveAs(ctx, t, tenantA, key, []byte("hash"))
	if claimed {
		t.Fatal("the completed record disappeared")
	}
	if record.Status != 200 {
		t.Errorf("status %d, want 200", record.Status)
	}
	if len(record.Body) != 0 {
		t.Errorf("body %q, want none", record.Body)
	}
}

// The store is a repository like any other: called outside a unit of work it must refuse rather
// than quietly reaching for the pool, which would carry no tenant at all.
func TestTheStoreRefusesToRunOutsideATransaction(t *testing.T) {
	ctx := context.Background()
	store := postgres.NewIdempotencyStore()

	if _, _, err := store.Reserve(ctx, keyFor("no-transaction"), []byte("hash")); err == nil {
		t.Error("Reserve ran without a transaction")
	}
	if err := store.Complete(ctx, keyFor("no-transaction"), 200, nil); err == nil {
		t.Error("Complete ran without a transaction")
	}
}

// releaseAs runs one Release in its own transaction, as the given tenant.
func releaseAs(ctx context.Context, t *testing.T, tenant shared.ID, key repository.Key) {
	t.Helper()
	store := postgres.NewIdempotencyStore()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))

	if err := uow.Within(ctx, persistence.Scope{TenantID: tenant}, func(ctx context.Context) error {
		return store.Release(ctx, key)
	}); err != nil {
		t.Fatalf("releasing as %s: %v", tenant, err)
	}
}

// A released claim is free again (G-09): the engine lets a failed action's reservation go so a
// replay can perform the work the first run never did.
func TestAReleasedKeyIsFreeAgain(t *testing.T) {
	ctx := context.Background()
	key := keyFor("released-and-reclaimed")

	if _, claimed := reserveAs(ctx, t, tenantA, key, []byte("hash")); !claimed {
		t.Fatal("the key was not claimed")
	}
	releaseAs(ctx, t, tenantA, key)

	if _, claimed := reserveAs(ctx, t, tenantA, key, []byte("hash")); !claimed {
		t.Error("a released key was still taken")
	}
}

// The cross-tenant negative for Release (gate SG-3): the delete matches on key and endpoint
// alone, so with row level security removed this would free the other tenant's claim - and their
// replay would act twice.
func TestAReleaseDoesNotFreeAnotherTenantsClaim(t *testing.T) {
	ctx := context.Background()
	key := keyFor("released-elsewhere")

	if _, claimed := reserveAs(ctx, t, tenantA, key, []byte("hash-a")); !claimed {
		t.Fatal("tenant A did not claim the key")
	}
	releaseAs(ctx, t, tenantB, key)

	if _, claimed := reserveAs(ctx, t, tenantA, key, []byte("hash-a")); claimed {
		t.Error("tenant B's release freed tenant A's claim")
	}
}
