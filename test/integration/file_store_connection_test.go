// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/filestore"
	"github.com/Jersyfi/hubtask/core/domain/model/filelink"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A connection is read as made, signing in again as the same login replaces its grant, a waiting
// connection stays until it is forgotten, and the workspace next door finds none of it (migration
// 0279, UC-INT-17).
func TestAFileStoreConnectionStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	repo := postgres.NewFileStoreConnectionRepository()
	conn, err := filelink.NewConnection(freshID(t), tenantA, authorA, filelink.ConnectNextcloud, "https://cloud.example.org", "ada", at)
	if err != nil {
		t.Fatal(err)
	}
	state := []byte("a state hash of thirty-two bytes!")
	pendingID := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		id, err := repo.Connect(ctx, conn, crypto.Sealed{KeyID: "k1", Ciphertext: []byte("first")})
		if err != nil || id != conn.ID {
			t.Errorf("connect = %v, %v", id, err)
		}
		again := conn
		again.ID = freshID(t)
		id, err = repo.Connect(ctx, again, crypto.Sealed{KeyID: "k1", Ciphertext: []byte("second")})
		if err != nil || id != conn.ID {
			t.Errorf("signing in again answered %v, %v, want the first connection", id, err)
		}
		return repo.Await(ctx, repository.Pending{ID: pendingID, AccountID: authorA, Kind: filelink.ConnectNextcloud,
			Server: "https://cloud.example.org", PollEndpoint: "https://cloud.example.org/login/v2/poll",
			Secret: crypto.Sealed{KeyID: "k1", Ciphertext: []byte("poll")}, ExpiresAt: at.Add(10 * time.Minute)}, state)
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if _, _, err := repo.Find(ctx, conn.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the workspace next door read the connection: %v", err)
		}
		if list, err := repo.List(ctx, authorA); err != nil || len(list) != 0 {
			t.Errorf("the workspace next door listed %v, %v", list, err)
		}
		if _, err := repo.Pending(ctx, state); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the workspace next door found the waiting connection: %v", err)
		}
		if err := repo.Disconnect(ctx, conn.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the workspace next door disconnected it: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		found, sealed, err := repo.Find(ctx, conn.ID)
		if err != nil || found.Login != "ada" || string(sealed.Ciphertext) != "second" {
			t.Errorf("find = %+v %s %v", found, sealed.Ciphertext, err)
		}
		pending, err := repo.Pending(ctx, state)
		if err != nil || pending.ID != pendingID || pending.PollEndpoint == "" {
			t.Errorf("pending = %+v %v", pending, err)
		}
		if _, err := repo.Pending(ctx, state); err != nil {
			t.Errorf("a waiting connection read twice: %v", err)
		}
		if err := repo.Forget(ctx, pendingID); err != nil {
			t.Error(err)
		}
		if _, err := repo.Pending(ctx, state); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("a forgotten connection was found: %v", err)
		}
		return repo.Disconnect(ctx, conn.ID)
	}); err != nil {
		t.Fatal(err)
	}
}
