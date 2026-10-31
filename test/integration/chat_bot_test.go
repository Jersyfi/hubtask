// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A bot is read as set up, a second one for the same room is a conflict, a run's position is
// kept, and the workspace next door finds none of it (migration 0280, UC-INT-18).
func TestAChatBotStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	admin := adminPool(ctx, t)
	hub, collection := freshID(t), freshID(t)
	if _, err := admin.Exec(ctx, `INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		VALUES ($1, $2, 'HUB', $3, 'a0', $4)`, hub.String(), tenantA.String(), freshName(t), authorA.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		VALUES ($1, $2, 'COLLECTION', $3, $4, 'a0', $5)`,
		collection.String(), tenantA.String(), hub.String(), freshName(t), authorA.String()); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	repo := postgres.NewChatBotRepository()
	bot := integration.ChatBot{
		ID: freshID(t), HubID: hub, CollectionID: collection, Homeserver: "https://matrix.example.org", Room: "!r:example.org",
		BotUser: "@hubtask:example.org", Unfurl: true, Capture: true, Enabled: true, RunAs: authorA, CreatedAt: at, UpdatedAt: at,
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := repo.Insert(ctx, bot, crypto.Sealed{KeyID: "k1", Ciphertext: []byte("token")}); err != nil {
			return err
		}
		bot.Since, bot.UpdatedAt = "s42", at.Add(time.Minute)
		return repo.Record(ctx, bot)
	}); err != nil {
		t.Fatal(err)
	}
	// The refused insert aborts its transaction, so it is the transaction's answer.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		again := bot
		again.ID = freshID(t)
		return repo.Insert(ctx, again, crypto.Sealed{KeyID: "k1", Ciphertext: []byte("t")})
	}); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a second bot for the room: %v", err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if _, _, err := repo.Find(ctx, bot.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the workspace next door read the bot: %v", err)
		}
		if list, err := repo.List(ctx, hub); err != nil || len(list) != 0 {
			t.Errorf("the workspace next door listed %v, %v", list, err)
		}
		if err := repo.Delete(ctx, bot.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the workspace next door deleted the bot: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// A Slack bot beside it keeps its platform; the Matrix one, seated without, is MATRIX.
	slack := bot
	slack.ID, slack.Platform, slack.Homeserver, slack.Room = freshID(t), integration.FormatSlack, integration.SlackOrigin, "C0123ABCD"
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Insert(ctx, slack, crypto.Sealed{KeyID: "k1", Ciphertext: []byte("bundle")})
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		found, sealed, err := repo.Find(ctx, bot.ID)
		if err != nil || found.Since != "s42" || string(sealed.Ciphertext) != "token" || !found.Capture || found.Platform != integration.FormatMatrix {
			t.Errorf("find = %+v %v", found, err)
		}
		if other, _, err := repo.Find(ctx, slack.ID); err != nil || other.Platform != integration.FormatSlack {
			t.Errorf("the Slack bot = %+v %v", other, err)
		}
		if err := repo.Delete(ctx, slack.ID); err != nil {
			return err
		}
		return repo.Delete(ctx, bot.ID)
	}); err != nil {
		t.Fatal(err)
	}
}
