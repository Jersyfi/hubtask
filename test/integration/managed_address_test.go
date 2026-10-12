// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// Migration 0131's shape: an ADDRESS row holds an address and may wait for its token; no other row
// holds an address, and no other row is without a token.
func TestAnAddressRequestIsTheOnlyPendingRowWithoutAToken(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	account := managedIn(t, tenantA, freshSignInName(t))
	pool := adminPool(ctx, t)

	insert := func(purpose string, address any, token any) error {
		_, err := pool.Exec(ctx, `
			INSERT INTO auth_pending (id, tenant_id, account_id, purpose, address, token_hash,
			                          created_at, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, now(), now() + interval '1 day')`,
			freshID(t).String(), tenantA.String(), account.ID.String(), purpose, address, token)
		return err
	}
	if err := insert("ADDRESS", "lena@example.org", nil); err != nil {
		t.Errorf("an address request without its token yet was refused: %v", err)
	}
	if err := insert("ADDRESS", nil, nil); err == nil {
		t.Error("an address request without an address was written")
	}
	if err := insert("RESET", "lena@example.org", []byte(freshID(t).String())); err == nil {
		t.Error("a reset link holding an address was written")
	}
	if err := insert("RESET", nil, nil); err == nil {
		t.Error("a reset link without a token was written")
	}
	if err := insert("RESET", nil, []byte(freshID(t).String())); err != nil {
		t.Errorf("a reset link as before was refused: %v", err)
	}
}

func addressStoreOf() postgres.AddressRepository {
	return postgres.NewAddressRepository(security.NewPendingTokenHasher(secret.New(installationSecret)))
}

func addressRequester(ctx context.Context, t *testing.T) identityservice.RequestAddress {
	t.Helper()
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	return identityservice.RequestAddress{
		Requests: addressStoreOf(), Authorizer: allowAll{}, Notifier: postgres.NewQueue(ids, clockadapter.System{}),
		Audit: postgres.NewAuditSink(generator{t}), UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)),
		Clock: clockadapter.System{}, IDs: ids,
	}
}

func addressLinkMinter(ctx context.Context, t *testing.T) identityservice.MintAddressLink {
	t.Helper()
	return identityservice.MintAddressLink{
		Requests: addressStoreOf(), Entropy: clockadapter.CryptoRandom{},
		UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)), Clock: clockadapter.System{},
	}
}

// UC-ID-20 check 7's request, as the application's role: the address waits on its own row, the job
// carries the request's identifier only, and the mail's token finds the request again.
func TestAnAddressWaitsForItsMailbox(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	lena := managedIn(t, tenantA, freshSignInName(t))
	address := freshSignInName(t) + "@example.org"

	if _, err := addressRequester(ctx, t).Execute(ctx, administratorOf(tenantA), lena.ID, address); err != nil {
		t.Fatalf("asking: %v", err)
	}
	admin := adminPool(ctx, t)
	var (
		requestID, stored, payload string
		email                      *string
	)
	if err := admin.QueryRow(ctx, `
		SELECT p.id::text, p.address, a.email, j.payload::text
		FROM auth_pending p JOIN account a ON a.id = p.account_id
		JOIN job j ON j.kind = 'notification.address_confirmation' AND j.dedupe_key = p.id::text
		WHERE p.account_id = $1 AND p.purpose = 'ADDRESS'`, lena.ID.String(),
	).Scan(&requestID, &stored, &email, &payload); err != nil {
		t.Fatalf("reading the request: %v", err)
	}
	if stored != address || email != nil {
		t.Errorf("the request holds %q and the account %v", stored, email)
	}
	if strings.Contains(payload, "example.org") || !strings.Contains(payload, requestID) {
		t.Errorf("the job carries %s", payload)
	}

	link, err := addressLinkMinter(ctx, t).MintAddressLink(ctx, tenantA, shared.MustParseID(requestID))
	if err != nil || link.Address != address || link.Token.Reveal() == "" {
		t.Fatalf("minting answered %+v, %v", link, err)
	}
	token, err := identity.ParsePendingToken(link.Token.Reveal())
	if err != nil {
		t.Fatalf("parsing the token: %v", err)
	}
	var found repository.AddressLookup
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		found, err = addressStoreOf().FindByToken(ctx, token)
		return err
	}); err != nil {
		t.Fatalf("finding by the token: %v", err)
	}
	if found.Address != address || found.Credential.AccountID != lena.ID {
		t.Errorf("found %+v", found)
	}
}

// An erasure takes the waiting address with the account's other pending steps.
func TestAnErasureTakesTheWaitingAddress(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	lena := managedIn(t, tenantA, freshSignInName(t))
	if _, err := addressRequester(ctx, t).Execute(ctx, administratorOf(tenantA), lena.ID,
		freshSignInName(t)+"@example.org"); err != nil {
		t.Fatalf("asking: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := privacyRepo().Anonymise(ctx, lena.ID, "Former user", time.Now())
		return err
	}); err != nil {
		t.Fatalf("anonymising: %v", err)
	}
	var left int
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT count(*) FROM auth_pending WHERE account_id = $1`, lena.ID.String()).Scan(&left); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if left != 0 {
		t.Errorf("%d pending rows outlived the erasure", left)
	}
}

// The cross-tenant negative test for the address store: another workspace's account is not locked,
// its requests are not counted, spent, minted or found, and its row takes no address.
func TestAnAddressRequestStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	theirs := managedIn(t, tenantB, freshSignInName(t))
	if _, err := addressRequester(ctx, t).Execute(ctx, administratorOf(tenantB), theirs.ID,
		freshSignInName(t)+"@example.org"); err != nil {
		t.Fatalf("asking in their workspace: %v", err)
	}
	var requestID string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT id::text FROM auth_pending WHERE account_id = $1`, theirs.ID.String()).Scan(&requestID); err != nil {
		t.Fatalf("reading their request: %v", err)
	}
	request := shared.MustParseID(requestID)
	theirLink, err := addressLinkMinter(ctx, t).MintAddressLink(ctx, tenantB, request)
	if err != nil {
		t.Fatalf("minting in their workspace: %v", err)
	}
	theirToken, err := identity.ParsePendingToken(theirLink.Token.Reveal())
	if err != nil {
		t.Fatalf("parsing their token: %v", err)
	}

	store := addressStoreOf()
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := store.LockAccount(ctx, theirs.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("locking their account answered %v", err)
		}
		if latest, err := store.LatestRequest(ctx, theirs.ID); err != nil || !latest.IsZero() {
			t.Errorf("their latest request read %v, %v", latest, err)
		}
		if _, err := store.Mint(ctx, request, theirToken, time.Now()); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("minting their request answered %v", err)
		}
		if _, err := store.FindByToken(ctx, theirToken); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("finding their request answered %v", err)
		}
		if spent, err := store.Consume(ctx, request, time.Now()); err != nil || spent {
			t.Errorf("spending their request answered %v, %v", spent, err)
		}
		if confirmed, err := store.Confirm(ctx, theirs.ID, "taken@example.org", time.Now()); err != nil || confirmed {
			t.Errorf("confirming on their account answered %v, %v", confirmed, err)
		}
		return store.Supersede(ctx, theirs.ID, time.Now())
	}); err != nil {
		t.Fatalf("reaching across: %v", err)
	}
	var consumed *time.Time
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT consumed_at FROM auth_pending WHERE id = $1`, requestID).Scan(&consumed); err != nil {
		t.Fatalf("reading their request: %v", err)
	}
	if consumed != nil {
		t.Error("their request was spent from another workspace")
	}
}
