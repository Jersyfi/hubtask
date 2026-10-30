// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

func detailOf(t *testing.T, err error) string {
	t.Helper()
	var refusal *shared.Error
	if !errors.As(err, &refusal) {
		t.Fatalf("want a refusal, got %v", err)
	}
	return refusal.DetailCode
}

// UC-INT-12 check 1 and 4: a Matrix room keeps its access token sealed, answers no secret, and
// the trail names the format and the room but never the token.
func TestAMatrixRoomSealsItsTokenAndAnswersNoSecret(t *testing.T) {
	h := newHarness()
	cmd := validCreate()
	cmd.TargetURL = "https://matrix.example.org"
	cmd.Chat = domain.ChatTarget{Format: domain.FormatMatrix, Room: "!team:example.org", Locale: "de"}
	cmd.Credential = secret.New("syt_token")

	minted, err := CreateWebhookSubscription{Writer: h.writer}.Execute(t.Context(), actor(), cmd)
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	if !minted.Secret.IsEmpty() {
		t.Error("a chat room answered a secret")
	}
	stored := h.store.rows[hookID]
	if string(stored.Secret.Ciphertext) != "sealed:syt_token" {
		t.Errorf("sealed %q, want the access token", stored.Secret.Ciphertext)
	}
	if stored.Subscription.Format != domain.FormatMatrix || stored.Subscription.Room != "!team:example.org" {
		t.Errorf("stored %+v", stored.Subscription)
	}
	out := subscriptionOutput(stored.Subscription)
	if out["format"] != "MATRIX" || out["room"] != "!team:example.org" || out["locale"] != "de" {
		t.Errorf("projection %v", out)
	}
	for _, entry := range h.sink.entries {
		if strings.Contains(fmt.Sprint(entry.Changes), "syt_token") {
			t.Fatal("the trail carries the token")
		}
	}
}

// UC-INT-12 check 3: a room's incoming webhook address is sealed, and its origin alone is listed
// and audited.
func TestAnIncomingAddressIsSealedAndShownByItsOrigin(t *testing.T) {
	h := newHarness()
	cmd := validCreate()
	cmd.TargetURL = "https://chat.example.org/hooks/room-credential"
	cmd.Chat = domain.ChatTarget{Format: domain.FormatMattermost}

	minted, err := CreateWebhookSubscription{Writer: h.writer}.Execute(t.Context(), actor(), cmd)
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	if minted.Subscription.TargetURL != "https://chat.example.org" {
		t.Errorf("target %q, want the origin", minted.Subscription.TargetURL)
	}
	if string(h.store.rows[hookID].Secret.Ciphertext) != "sealed:https://chat.example.org/hooks/room-credential" {
		t.Error("the incoming address was not sealed")
	}
	for _, entry := range h.sink.entries {
		if strings.Contains(fmt.Sprint(entry.Changes), "room-credential") {
			t.Fatal("the trail carries the incoming address")
		}
	}
}

// UC-INT-12 check 2: what a format refuses at creation.
func TestAChatRoomRefusesWhatItsFormatDoesNotTake(t *testing.T) {
	cases := []struct {
		name string
		edit func(*CreateWebhookSubscriptionCommand)
		code string
	}{
		{"matrix without a token", func(c *CreateWebhookSubscriptionCommand) {
			c.Chat = domain.ChatTarget{Format: domain.FormatMatrix, Room: "!team:example.org"}
		}, "webhooks.credential_required"},
		{"matrix without a room", func(c *CreateWebhookSubscriptionCommand) {
			c.Chat = domain.ChatTarget{Format: domain.FormatMatrix}
			c.Credential = secret.New("t")
		}, "webhooks.room_required"},
		{"a token for slack", func(c *CreateWebhookSubscriptionCommand) {
			c.Chat = domain.ChatTarget{Format: domain.FormatSlack}
			c.Credential = secret.New("t")
		}, "webhooks.credential_not_for_format"},
		{"a token for the CloudEvent", func(c *CreateWebhookSubscriptionCommand) {
			c.Credential = secret.New("t")
		}, "webhooks.credential_not_for_format"},
		{"teams at an origin alone", func(c *CreateWebhookSubscriptionCommand) {
			c.Chat = domain.ChatTarget{Format: domain.FormatTeams}
			c.TargetURL = "https://chat.example.org"
		}, "webhooks.incoming_address_incomplete"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness()
			cmd := validCreate()
			tc.edit(&cmd)
			_, err := CreateWebhookSubscription{Writer: h.writer}.Execute(t.Context(), actor(), cmd)
			if got := detailOf(t, err); got != tc.code {
				t.Fatalf("want %s, got %s", tc.code, got)
			}
			if len(h.store.rows) != 0 {
				t.Fatal("a refused room was stored")
			}
		})
	}
}

// UC-INT-12 check 3: a chat room cannot be pointed elsewhere or have its credential replaced by a
// drawn signing secret.
func TestAChatRoomKeepsItsTargetAndIsNotRotated(t *testing.T) {
	h := newHarness()
	cmd := validCreate()
	cmd.TargetURL = "https://chat.example.org/hooks/room-credential"
	cmd.Chat = domain.ChatTarget{Format: domain.FormatSlack}
	if _, err := (CreateWebhookSubscription{Writer: h.writer}).Execute(t.Context(), actor(), cmd); err != nil {
		t.Fatal(err)
	}

	elsewhere := "https://elsewhere.example.org/hooks/x"
	_, err := UpdateWebhookSubscription{Writer: h.writer}.Execute(t.Context(), actor(),
		UpdateWebhookSubscriptionCommand{ID: hookID, TargetURL: &elsewhere})
	if got := detailOf(t, err); got != "webhooks.target_fixed_for_chat" {
		t.Fatalf("update: got %s", got)
	}

	_, err = RotateWebhookSecret{Writer: h.writer}.Execute(t.Context(), actor(), hookID, time.Hour)
	if got := detailOf(t, err); got != "webhooks.chat_not_signed" {
		t.Fatalf("rotation: got %s", got)
	}
}
