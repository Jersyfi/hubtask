// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-INT-12 check 2: what each format requires of its room and language.
func TestValidChatTarget(t *testing.T) {
	cases := []struct {
		name string
		in   ChatTarget
		want ChatTarget
		code string
	}{
		{name: "empty is the CloudEvent", in: ChatTarget{}, want: ChatTarget{Format: FormatCloudEvents}},
		{name: "matrix with a room", in: ChatTarget{Format: FormatMatrix, Room: " !abc:example.org ", Locale: "de"},
			want: ChatTarget{Format: FormatMatrix, Room: "!abc:example.org", Locale: "de"}},
		{name: "matrix without a room", in: ChatTarget{Format: FormatMatrix}, code: "webhooks.room_required"},
		{name: "matrix alias", in: ChatTarget{Format: FormatMatrix, Room: "#team:example.org"}, code: "webhooks.room_malformed"},
		{name: "slack with a room", in: ChatTarget{Format: FormatSlack, Room: "!abc:example.org"}, code: "webhooks.room_not_for_format"},
		{name: "teams in German", in: ChatTarget{Format: FormatTeams, Locale: "de"}, want: ChatTarget{Format: FormatTeams, Locale: "de"}},
		{name: "mattermost with a region", in: ChatTarget{Format: FormatMattermost, Locale: "pt-BR"},
			want: ChatTarget{Format: FormatMattermost, Locale: "pt-BR"}},
		{name: "a language for the CloudEvent", in: ChatTarget{Locale: "de"}, code: "webhooks.locale_not_for_format"},
		{name: "a malformed language", in: ChatTarget{Format: FormatSlack, Locale: "german"}, code: "webhooks.locale_malformed"},
		{name: "an unknown format", in: ChatTarget{Format: "IRC"}, code: "webhooks.format_unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidChatTarget(tc.in)
			if tc.code != "" {
				var refusal *shared.Error
				if !errors.As(err, &refusal) || refusal.DetailCode != tc.code {
					t.Fatalf("want %s, got %v", tc.code, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected %v", err)
			}
			if got != tc.want {
				t.Fatalf("want %+v, got %+v", tc.want, got)
			}
		})
	}
}

func TestChatFormatsAreNotSigned(t *testing.T) {
	if FormatCloudEvents.IsChat() {
		t.Fatal("the CloudEvent is not a chat format")
	}
	for _, format := range []DeliveryFormat{FormatMatrix, FormatSlack, FormatMattermost, FormatTeams} {
		if !format.IsChat() {
			t.Fatalf("%s is a chat format", format)
		}
	}
}

func TestNewWebhookSubscriptionKeepsItsChatTarget(t *testing.T) {
	subscription, err := NewWebhookSubscription(NewWebhookSubscriptionInput{
		ID: "0190a000-0000-7000-8000-000000000001", TenantID: "0190a000-0000-7000-8000-000000000002",
		TargetURL: "https://matrix.example.org", EventTypes: []string{"de.hubtask.work.item.created.v1"},
		Chat:      ChatTarget{Format: FormatMatrix, Room: "!abc:example.org", Locale: "de"},
		CreatedBy: "0190a000-0000-7000-8000-000000000003", Now: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if subscription.Format != FormatMatrix || subscription.Room != "!abc:example.org" || subscription.Locale != "de" {
		t.Fatalf("chat target lost: %+v", subscription)
	}
}

// UC-INT-12 check 3: an incoming address is kept sealed and shown by its origin.
func TestIncomingAddress(t *testing.T) {
	address, origin, err := IncomingAddress("https://chat.example.org/hooks/abc123")
	if err != nil || address != "https://chat.example.org/hooks/abc123" || origin != "https://chat.example.org" {
		t.Fatalf("got %q %q %v", address, origin, err)
	}
	if _, _, err := IncomingAddress("https://chat.example.org/"); err == nil {
		t.Fatal("an origin alone is no room")
	}
	if !SameOrigin(address, origin) || SameOrigin("https://elsewhere.example.org/hooks/abc123", origin) {
		t.Fatal("the origin decides where a sealed address may send")
	}
	if !FormatTeams.TakesIncomingAddress() || FormatMatrix.TakesIncomingAddress() || FormatCloudEvents.TakesIncomingAddress() {
		t.Fatal("only the incoming webhook formats take an incoming address")
	}
}
