// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The statements the calendar feeds run on, against a real database (D-08): a feed round-trips,
// the token finds exactly its own row, revocation is a stamp that happens once, and the tenant
// boundary holds per method (gate SG-3).

// feedSecret is this file's installation secret. Fixed, so that a hash written by one test is the
// hash another one looks up.
const feedSecret = "an installation secret for the calendar feed integration suite"

func feedRepo() postgres.CalendarFeedRepository {
	return postgres.NewCalendarFeedRepository(security.NewFeedTokenHasher(secret.New(feedSecret)))
}

// mintFeedToken makes a token whose secret half is derived from the seed, so two calls with two
// seeds produce two different credentials and the same seed produces the same one.
func mintFeedToken(t *testing.T, tenant shared.ID, seed byte) domain.FeedToken {
	t.Helper()

	entropy := make([]byte, domain.FeedTokenSecretBytes)
	for i := range entropy {
		entropy[i] = seed + byte(i)
	}
	token, err := domain.NewFeedToken(tenant, entropy)
	if err != nil {
		t.Fatalf("minting the token: %v", err)
	}
	return token
}

func feedIn(t *testing.T, tenant, owner, view shared.ID) domain.CalendarFeed {
	t.Helper()

	feed, err := domain.NewCalendarFeed(domain.NewCalendarFeedInput{
		ID: freshID(t), TenantID: tenant, AccountID: owner, ViewID: view, Now: created,
	})
	if err != nil {
		t.Fatalf("minting the feed: %v", err)
	}
	return feed
}

func TestAFeedRoundTripsAndItsTokenFindsIt(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	saved := savedViewIn(tenantA, authorA, collection, freshID(t), freshName(t))
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return viewRepo().Insert(ctx, saved)
	}); err != nil {
		t.Fatalf("writing the view: %v", err)
	}

	feed := feedIn(t, tenantA, authorA, saved.ID)
	token := mintFeedToken(t, tenantA, 0x10)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return feedRepo().Insert(ctx, feed, token)
	}); err != nil {
		t.Fatalf("writing the feed: %v", err)
	}

	var found domain.CalendarFeed
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		found, err = feedRepo().FindByToken(ctx, token)
		return err
	}); err != nil {
		t.Fatalf("the token found nothing: %v", err)
	}
	if found.ID != feed.ID || found.AccountID != authorA || found.ViewID != saved.ID {
		t.Errorf("the feed came back as %+v", found)
	}
	if found.IsRevoked() || !found.ServesAView() {
		t.Error("a fresh feed came back revoked or serving nothing")
	}

	// The token is not stored. What is stored cannot be turned back into it.
	var stored []byte
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT token_hash FROM calendar_feed WHERE id = $1`, feed.ID.String()).
		Scan(&stored); err != nil {
		t.Fatalf("reading the row: %v", err)
	}
	if len(stored) != 32 {
		t.Errorf("the stored value is %d bytes, and an HMAC-SHA-256 is 32", len(stored))
	}
	var asText string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT token_hash::text FROM calendar_feed WHERE id = $1`, feed.ID.String()).
		Scan(&asText); err != nil {
		t.Fatalf("reading the row: %v", err)
	}
	if strings.Contains(asText, token.Secret()) {
		t.Error("the credential itself is in the table")
	}

	// Another token, however well formed, is another row - and there is no other row.
	other := mintFeedToken(t, tenantA, 0x40)
	err := read(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := feedRepo().FindByToken(ctx, other)
		return err
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("an unminted token answered %v", err)
	}
}

// Revocation is a stamp, it happens once, and the row stays: "that token was revoked on Tuesday"
// is the question this answers.
func TestRevokingAFeedHappensOnceAndKeepsTheRow(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	saved := savedViewIn(tenantA, authorA, collection, freshID(t), freshName(t))
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return viewRepo().Insert(ctx, saved)
	}); err != nil {
		t.Fatalf("writing the view: %v", err)
	}

	feed := feedIn(t, tenantA, authorA, saved.ID)
	token := mintFeedToken(t, tenantA, 0x70)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return feedRepo().Insert(ctx, feed, token)
	}); err != nil {
		t.Fatalf("writing the feed: %v", err)
	}

	tuesday := created.Add(24 * time.Hour)
	var changed bool
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		changed, err = feedRepo().Revoke(ctx, feed.ID, tuesday)
		return err
	}); err != nil {
		t.Fatalf("revoking: %v", err)
	}
	if !changed {
		t.Error("the first revocation reported no change")
	}

	var again bool
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		again, err = feedRepo().Revoke(ctx, feed.ID, created.Add(48*time.Hour))
		return err
	}); err != nil {
		t.Fatalf("revoking twice: %v", err)
	}
	if again {
		t.Error("the second revocation reported a change, and the moment would have moved")
	}

	// The row is still there, still findable by its token, and says when it stopped.
	var found domain.CalendarFeed
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		found, err = feedRepo().FindByToken(ctx, token)
		return err
	}); err != nil {
		t.Fatalf("a revoked feed vanished: %v", err)
	}
	if !found.IsRevoked() || !found.RevokedAt.Equal(tuesday) {
		t.Errorf("the revocation reads as %+v", found)
	}
}

// The list is one person's own, newest first, and nobody else's.
func TestTheFeedListIsTheAccountsOwn(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	saved := savedViewIn(tenantA, authorA, collection, freshID(t), freshName(t))
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return viewRepo().Insert(ctx, saved)
	}); err != nil {
		t.Fatalf("writing the view: %v", err)
	}

	somebodyElse := seedAccount(ctx, t, tenantA)
	mine := feedIn(t, tenantA, authorA, saved.ID)
	later := feedIn(t, tenantA, authorA, saved.ID)
	later.CreatedAt = created.Add(time.Hour)
	theirs := feedIn(t, tenantA, somebodyElse, saved.ID)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for i, feed := range []domain.CalendarFeed{mine, later, theirs} {
			if err := feedRepo().Insert(ctx, feed, mintFeedToken(t, tenantA, byte(0xa0+i))); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the feeds: %v", err)
	}

	var listed []domain.CalendarFeed
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		listed, err = feedRepo().ListForAccount(ctx, authorA)
		return err
	}); err != nil {
		t.Fatalf("listing: %v", err)
	}

	seen := map[shared.ID]int{}
	for i, feed := range listed {
		seen[feed.ID] = i
		if feed.AccountID != authorA {
			t.Errorf("the list carries %s's feed", feed.AccountID)
		}
	}
	if _, ok := seen[mine.ID]; !ok {
		t.Error("my own feed is not in my list")
	}
	if seen[later.ID] > seen[mine.ID] {
		t.Error("the list is not newest first")
	}
	if _, ok := seen[theirs.ID]; ok {
		t.Error("somebody else's feed is in my list")
	}
}

// Gate SG-3: one negative per port method.
func TestCalendarFeedsAreInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	collection := collectionFor(ctx, t, tenantA, authorA)
	saved := savedViewIn(tenantA, authorA, collection, freshID(t), freshName(t))
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return viewRepo().Insert(ctx, saved)
	}); err != nil {
		t.Fatalf("writing the view: %v", err)
	}

	feed := feedIn(t, tenantA, authorA, saved.ID)
	token := mintFeedToken(t, tenantA, 0xd0)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return feedRepo().Insert(ctx, feed, token)
	}); err != nil {
		t.Fatalf("seeding the feed: %v", err)
	}

	t.Run("find by token", func(t *testing.T) {
		// The whole of the public route's defence: a token is bound to the tenant it names, and
		// opening the transaction as another one finds nothing however right the hash is.
		err := read(ctx, t, tenantB, func(ctx context.Context) error {
			_, err := feedRepo().FindByToken(ctx, token)
			return err
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("tenant B's lookup answered %v", err)
		}
	})

	t.Run("find", func(t *testing.T) {
		err := read(ctx, t, tenantB, func(ctx context.Context) error {
			_, err := feedRepo().Find(ctx, feed.ID)
			return err
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("tenant B's find answered %v", err)
		}
	})

	t.Run("list", func(t *testing.T) {
		var listed []domain.CalendarFeed
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			listed, err = feedRepo().ListForAccount(ctx, authorA)
			return err
		}); err != nil {
			t.Fatalf("tenant B's list answered %v", err)
		}
		if len(listed) != 0 {
			t.Errorf("tenant B listed %d of tenant A's feeds", len(listed))
		}
	})

	t.Run("insert", func(t *testing.T) {
		// The account foreign key is composite, so a row naming tenant A's account from tenant B
		// cannot land at all: the boundary is the schema's here rather than only the policy's.
		foreign := feedIn(t, tenantA, authorA, saved.ID)
		err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return feedRepo().Insert(ctx, foreign, mintFeedToken(t, tenantB, 0xf0))
		})
		if err == nil {
			t.Fatal("tenant B wrote a feed for tenant A's account")
		}
	})

	t.Run("revoke", func(t *testing.T) {
		var changed bool
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			changed, err = feedRepo().Revoke(ctx, feed.ID, created)
			return err
		}); err != nil {
			t.Fatalf("tenant B's revocation answered %v", err)
		}
		if changed {
			t.Fatal("tenant B revoked tenant A's feed")
		}
		// And tenant A's feed still works.
		var found domain.CalendarFeed
		if err := read(ctx, t, tenantA, func(ctx context.Context) error {
			var err error
			found, err = feedRepo().Find(ctx, feed.ID)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if found.IsRevoked() {
			t.Error("tenant B's revocation reached the row after all")
		}
	})
}
