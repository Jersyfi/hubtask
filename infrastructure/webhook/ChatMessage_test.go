// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package webhook

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// texts renders a code as `<locale>:<code>`, and knows only the codes it was given.
type texts map[string]bool

func (t texts) Render(locale, code string, _ map[string]string) string {
	if !t[code] {
		return code
	}
	return locale + ":" + code
}

const itemID = "01936f2a-7c1e-7000-8000-000000000a07"

func completed(t *testing.T) event.Envelope {
	t.Helper()
	envelope, err := event.NewEnvelope(eventID, event.ItemCompleted, tenant, "item/"+itemID,
		event.Actor{Kind: shared.ActorUser, ID: author}, now, event.Cause{},
		map[string]any{"id": itemID, "title": "Ship <the> release", "notes": "private notes"})
	if err != nil {
		t.Fatal(err)
	}
	return envelope
}

// UC-INT-12 check 4: the sentence comes from the catalogue in the room's language, with the
// entry's title and a link - never its notes.
func TestAChatMessageCarriesTheTitleAndALinkAtMost(t *testing.T) {
	known := texts{"chat.item_completed": true, "chat.event_other": true, "chat.open": true}
	message := chatMessageOf(known, "de", "https://tasks.example.org/", completed(t))
	if message.Headline != "de:chat.item_completed" || message.Title != "Ship <the> release" ||
		message.Link != "https://tasks.example.org/items/"+itemID || message.Open != "de:chat.open" {
		t.Fatalf("message %+v", message)
	}

	other, err := event.NewEnvelope(eventID, event.LabelCreated, tenant, "label/x",
		event.Actor{Kind: shared.ActorUser, ID: author}, now, event.Cause{}, map[string]any{"name": "x"})
	if err != nil {
		t.Fatal(err)
	}
	fallback := chatMessageOf(known, "en", "https://tasks.example.org", other)
	if fallback.Headline != "en:chat.event_other" || fallback.Title != "" || fallback.Link != "https://tasks.example.org" {
		t.Fatalf("fallback %+v", fallback)
	}

	for _, format := range []domain.DeliveryFormat{domain.FormatMatrix, domain.FormatSlack, domain.FormatMattermost, domain.FormatTeams} {
		subscription := domain.WebhookSubscription{Format: format, TargetURL: "https://chat.example.org", Room: "!r:example.org"}
		_, _, _, body, err := chatRequest(subscription, "https://chat.example.org/hooks/x", completed(t), message)
		if err != nil {
			t.Fatalf("%s: %v", format, err)
		}
		if strings.Contains(string(body), "private notes") {
			t.Fatalf("%s carries the notes: %s", format, body)
		}
		if !strings.Contains(string(body), "/items/"+itemID) {
			t.Fatalf("%s carries no link: %s", format, body)
		}
		if !json.Valid(body) {
			t.Fatalf("%s is not JSON", format)
		}
	}
}

func TestChatCodes(t *testing.T) {
	for eventType, want := range map[event.Type]string{
		event.ItemCompleted:       "chat.item_completed",
		event.CommentCreated:      "chat.comment_created",
		event.RuleRunFailed:       "chat.rule_run_failed",
		event.JumbleEntryReceived: "chat.entry_received",
	} {
		if got := chatCode(eventType); got != want {
			t.Errorf("%s: got %s, want %s", eventType, got, want)
		}
	}
}

// UC-INT-12 check 1: a Matrix room is sent a notice through the client-server API with its token,
// keyed by the event so a retry is one notice, and is not signed.
func TestAMatrixRoomIsSentANoticeWithItsToken(t *testing.T) {
	h := newHarness(t, 1)
	h.store.subscription = matrixSubscription(t)
	h.deliverer.Texts = texts{"chat.item_created": true}
	h.deliverer.Source = "https://tasks.example.org"

	if _, err := h.deliverer.Run(t.Context(), job()); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if len(h.calls.requests) != 1 {
		t.Fatalf("made %d calls", len(h.calls.requests))
	}
	request := h.calls.requests[0]
	want := "https://matrix.example.org/_matrix/client/v3/rooms/%21team:example.org/send/m.room.message/hubtask-" + eventID.String()
	if request.Method != http.MethodPut || request.URL != want {
		t.Fatalf("%s %s, want PUT %s", request.Method, request.URL, want)
	}
	if request.Header["Authorization"][0] != "Bearer syt_token" {
		t.Error("the access token is not presented")
	}
	if _, signed := request.Header[security.SignatureHeader]; signed {
		t.Error("a chat room was signed")
	}
	var notice map[string]any
	if err := json.Unmarshal(request.Body, &notice); err != nil || notice["msgtype"] != "m.notice" ||
		!strings.Contains(notice["body"].(string), "Buy milk") {
		t.Fatalf("notice %s", request.Body)
	}
	if len(h.results.delivered) != 1 {
		t.Error("a delivered notice did not count as delivered")
	}
}

// UC-INT-12 check 3: a sealed incoming address that no longer matches the target's origin sends
// nothing and ends the delivery.
func TestASealedAddressOutsideItsOriginSendsNothing(t *testing.T) {
	h := newHarness(t, 1)
	subscription := h.store.subscription.Subscription
	subscription.Format, subscription.TargetURL = domain.FormatSlack, "https://chat.example.org"
	h.store.subscription = repository.StoredSubscription{
		Subscription: subscription,
		Secret:       repository.SealedSecret{KeyID: "k1", Ciphertext: []byte("https://elsewhere.example.org/hooks/x")},
	}
	h.deliverer.Texts = texts{}

	if _, err := h.deliverer.Run(t.Context(), job()); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if len(h.calls.requests) != 0 {
		t.Fatal("a mismatched address was called")
	}
	if len(h.results.failures) != 1 || h.results.failures[0] != "webhooks.target_mismatch" || h.results.terminals != 1 {
		t.Fatalf("failures %v, terminals %d", h.results.failures, h.results.terminals)
	}
}

func matrixSubscription(t *testing.T) repository.StoredSubscription {
	t.Helper()
	subscription, err := domain.NewWebhookSubscription(domain.NewWebhookSubscriptionInput{
		ID: subscriptionID, TenantID: tenant, CreatedBy: author,
		TargetURL: "https://matrix.example.org", EventTypes: []string{string(event.ItemCreated)},
		Chat: domain.ChatTarget{Format: domain.FormatMatrix, Room: "!team:example.org"}, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return repository.StoredSubscription{
		Subscription: subscription,
		Secret:       repository.SealedSecret{KeyID: "k1", Ciphertext: []byte("syt_token")},
	}
}
