// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/calendarpush"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A push is read as connected, connecting the feed again replaces its grant, what it wrote is
// kept per entry, a waiting connection is taken once, and the workspace next door finds none of it
// (migration 0278, UC-INT-16).
func TestACalendarPushStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	saved := savedViewIn(tenantA, authorA, collection, freshID(t), freshName(t))
	feed := feedIn(t, tenantA, authorA, saved.ID)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := viewRepo().Insert(ctx, saved); err != nil {
			return err
		}
		return feedRepo().Insert(ctx, feed, mintFeedToken(t, tenantA, 0x20))
	}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	repo := postgres.NewCalendarPushRepository()
	push, err := domain.NewCalendarPush(freshID(t), tenantA, authorA, feed, domain.PushGoogle, "ada@example.org", at)
	if err != nil {
		t.Fatal(err)
	}
	entry := freshID(t)
	state := []byte("a state hash of thirty-two bytes!")
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		id, err := repo.Connect(ctx, push, crypto.Sealed{KeyID: "k1", Ciphertext: []byte("first")})
		if err != nil || id != push.ID {
			t.Errorf("connect = %v, %v", id, err)
		}
		again := push
		again.ID = freshID(t)
		id, err = repo.Connect(ctx, again, crypto.Sealed{KeyID: "k1", Ciphertext: []byte("second")})
		if err != nil || id != push.ID {
			t.Errorf("connecting the feed again answered %v, %v, want the first push", id, err)
		}
		if err := repo.PutEvent(ctx, push.ID, domain.PushedEvent{EntryID: entry, RemoteID: "evt-1", Digest: "d1"}, at); err != nil {
			t.Error(err)
		}
		return repo.Await(ctx, repository.Pending{ID: freshID(t), AccountID: authorA, FeedID: feed.ID, Provider: domain.PushGoogle,
			Verifier: crypto.Sealed{KeyID: "k1", Ciphertext: []byte("v")}, ExpiresAt: at.Add(10 * time.Minute)}, state)
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if _, _, err := repo.Find(ctx, push.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the workspace next door read the push: %v", err)
		}
		if pushes, err := repo.List(ctx, authorA); err != nil || len(pushes) != 0 {
			t.Errorf("the workspace next door listed %v, %v", pushes, err)
		}
		if events, err := repo.Events(ctx, push.ID); err != nil || len(events) != 0 {
			t.Errorf("the workspace next door read the events %v, %v", events, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := repo.Take(ctx, state); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the workspace next door took the connection: %v", err)
		}
		if err := repo.Disconnect(ctx, push.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the workspace next door disconnected the push: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		found, sealed, err := repo.Find(ctx, push.ID)
		if err != nil || string(sealed.Ciphertext) != "second" || found.Address != "ada@example.org" || found.State != domain.PushActive {
			t.Errorf("found %+v, %q, %v", found, sealed.Ciphertext, err)
		}
		found.Pushed(at.Add(time.Hour), "calendar_push.grant_revoked", true)
		if err := repo.Record(ctx, found); err != nil {
			t.Error(err)
		}
		events, err := repo.Events(ctx, push.ID)
		if err != nil || len(events) != 1 || events[0].RemoteID != "evt-1" {
			t.Errorf("events %v, %v", events, err)
		}
		pending, err := repo.Take(ctx, state)
		if err != nil || pending.FeedID != feed.ID || pending.Provider != domain.PushGoogle {
			t.Errorf("taken %+v, %v", pending, err)
		}
		if _, err := repo.Take(ctx, state); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("taken twice: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		pushes, err := repo.List(ctx, authorA)
		if err != nil || len(pushes) != 1 || pushes[0].State != domain.PushFailed {
			t.Errorf("pushes %+v, %v", pushes, err)
		}
		if err := repo.Disconnect(ctx, push.ID); err != nil {
			t.Error(err)
		}
		events, err := repo.Events(ctx, push.ID)
		if err != nil || len(events) != 0 {
			t.Errorf("a disconnected push kept its events: %v, %v", events, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
