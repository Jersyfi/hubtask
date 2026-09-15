// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package event_test

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// An event raised online carries one instant in both fields and names no push; one a device
// brought in keeps the server's time as when it was received, takes the device's moment as when
// it occurred, and names the push (offline-sync.md §8).
func TestAPushedEventKeepsBothClocks(t *testing.T) {
	server := time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC)
	device := server.Add(-3 * 24 * time.Hour)
	push := shared.ID("01936f2a-7c1e-7000-8000-0000000000f1")

	online, err := event.NewEnvelope(
		shared.ID("01936f2a-7c1e-7000-8000-0000000000f2"), event.ItemCompleted,
		shared.ID("01936f2a-7c1e-7000-8000-0000000000f3"), "item/x",
		event.Actor{Kind: shared.ActorUser}, server, event.Cause{}, nil)
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	if !online.ReceivedAt.Equal(online.OccurredAt) || !online.PushID.IsZero() {
		t.Errorf("an online event carries %v/%v and push %q", online.OccurredAt, online.ReceivedAt, online.PushID)
	}

	pushed := online.Pushed(push, device)
	if !pushed.OccurredAt.Equal(device) || !pushed.ReceivedAt.Equal(server) || pushed.PushID != push {
		t.Errorf("a pushed event carries %v/%v and push %q", pushed.OccurredAt, pushed.ReceivedAt, pushed.PushID)
	}
	if !pushed.CausedBy().PushID.IsZero() {
		t.Error("the push travelled down the chain to an event a rule would raise")
	}

	// A mutation that carried no reading keeps the server's moment.
	unread := online.Pushed(push, time.Time{})
	if !unread.OccurredAt.Equal(server) || unread.PushID != push {
		t.Errorf("a pushed event without a reading carries %v and push %q", unread.OccurredAt, unread.PushID)
	}
}
