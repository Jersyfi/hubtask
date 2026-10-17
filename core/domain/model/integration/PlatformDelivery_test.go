// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestAPlatformDeliveryDeadLettersAfterARoundOfAttemptsAndCountsOnAfterARedelivery(t *testing.T) {
	now := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	d, err := NewPlatformDelivery(shared.ID("d"), shared.ID("s"), shared.ID("e"), "de.hubtask.installation.workspace.created.v1", now)
	if err != nil || !d.Waiting() {
		t.Fatalf("queued %+v, %v", d, err)
	}
	for i := 1; i < MaxDeliveryAttempts; i++ {
		d = d.Missed(503, "webhooks.target_unavailable", now.Add(time.Minute), now)
		if d.Status != DeliveryFailed || !d.Waiting() || d.NextAttemptAt.IsZero() {
			t.Fatalf("attempt %d: %+v", i, d)
		}
	}
	d = d.Missed(503, "webhooks.target_unavailable", now.Add(time.Minute), now)
	if d.Status != DeliveryDeadLetter || d.Waiting() || d.Attempt != MaxDeliveryAttempts {
		t.Fatalf("after a round: %+v", d)
	}

	// A redelivery puts it back to pending; its attempts count on, and it arrives.
	d.Status = DeliveryPending
	d = d.Arrived(200, now)
	if d.Status != DeliverySucceeded || d.Attempt != MaxDeliveryAttempts+1 || d.ErrorCode != "" {
		t.Errorf("arrived %+v", d)
	}
	if expired := d.Expired(now); expired.Status != DeliveryDeadLetter || expired.ErrorCode != "webhooks.event_expired" {
		t.Errorf("expired %+v", expired)
	}
	if _, err := NewPlatformDelivery("", "s", "e", "t", now); err == nil {
		t.Error("an incomplete delivery was built")
	}
}
