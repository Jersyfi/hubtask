// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package event_test

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func jumbleEntry(t *testing.T) jumble.Entry {
	t.Helper()
	entry, err := jumble.NewEntry(jumble.NewEntryInput{
		ID:       shared.ID("01936f2a-7c1e-7000-8000-000000000f11"),
		TenantID: shared.ID("01936f2a-7c1e-7000-8000-000000000f12"),
		Channel:  jumble.ChannelWebhook,
		Sender:   "orders@example.org", RawSubject: "Order #42",
		RawBody:     "The customer asked for a call back.",
		Attachments: []shared.ID{"01936f2a-7c1e-7000-8000-000000000f13"},
		Now:         time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("building the entry: %v", err)
	}
	return entry
}

// The arrival's payload carries no content and no sender: an event leaves the installation, and
// the raw text of a mail is exactly what rule 10 keeps out of everything that travels.
func TestTheArrivalEventCarriesNoContent(t *testing.T) {
	entry := jumbleEntry(t)
	envelope, err := event.NewJumbleEntryReceived(
		shared.ID("01936f2a-7c1e-7000-8000-000000000f14"), entry,
		event.Actor{Kind: shared.ActorSystem}, entry.ReceivedAt, event.Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}

	if envelope.Type != event.JumbleEntryReceived {
		t.Errorf("type %s", envelope.Type)
	}
	if envelope.Subject != "jumble_entry/"+entry.ID.String() {
		t.Errorf("subject %q", envelope.Subject)
	}
	for _, name := range []string{"raw_subject", "raw_body", "sender"} {
		if _, leaked := envelope.Payload[name]; leaked {
			t.Errorf("the payload carries %s", name)
		}
	}
	if envelope.Payload["channel"] != "WEBHOOK" || envelope.Payload["attachment_count"] != 1 {
		t.Errorf("payload %v", envelope.Payload)
	}
}

// The conversion event names both halves of the provenance, and refuses an entry that was not
// converted: the writer and the event may not disagree.
func TestTheConversionEventNamesTheProvenance(t *testing.T) {
	entry := jumbleEntry(t)
	target := shared.ID("01936f2a-7c1e-7000-8000-000000000f15")
	collection := shared.ID("01936f2a-7c1e-7000-8000-000000000f16")

	if _, err := event.NewJumbleEntryConverted(
		shared.ID("01936f2a-7c1e-7000-8000-000000000f17"), entry, collection,
		event.Actor{Kind: shared.ActorUser}, entry.ReceivedAt, event.Cause{}); err == nil {
		t.Fatal("a conversion event about an unconverted entry was built")
	}

	converted, err := entry.Convert(target, entry.ReceivedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("converting: %v", err)
	}
	envelope, err := event.NewJumbleEntryConverted(
		shared.ID("01936f2a-7c1e-7000-8000-000000000f17"), converted, collection,
		event.Actor{Kind: shared.ActorUser}, entry.ReceivedAt, event.Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}
	if envelope.Payload["target_item_id"] != target.String() ||
		envelope.Payload["collection_id"] != collection.String() {
		t.Errorf("payload %v", envelope.Payload)
	}
}
