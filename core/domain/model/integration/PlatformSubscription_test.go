// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestAPlatformSubscriptionIsCheckedAsAWebhookIs(t *testing.T) {
	id := shared.ID("0192f000-0000-7000-8000-0000000000a1")
	now := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	cases := []struct {
		name, target, code string
	}{
		{"https", " https://shop.example/hubtask ", ""},
		{"http on a private network", "http://billing.internal/hook", ""},
		{"empty", "", "webhooks.target_required"},
		{"another scheme", "ftp://shop.example", "webhooks.target_scheme_unsupported"},
		{"credentials in the address", "https://user:pw@shop.example", "webhooks.target_carries_credentials"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sub, err := NewPlatformSubscription(id, c.target, false, "", now)
			if c.code == "" {
				if err != nil || sub.State != PlatformActive || sub.Version != 1 || sub.TargetURL[0] == ' ' {
					t.Errorf("built %+v, %v", sub, err)
				}
				return
			}
			if shared.AsError(err).DetailCode != c.code {
				t.Errorf("answered %v, want %s", err, c.code)
			}
		})
	}
	if _, err := NewPlatformSubscription("", "https://shop.example", false, "", now); err == nil {
		t.Error("a subscription without an id was built")
	}
}

func TestAPlatformSubscriptionIsPausedAndResumedAndNothingElse(t *testing.T) {
	sub, _ := NewPlatformSubscription(shared.ID("0192f000-0000-7000-8000-0000000000a1"), "https://shop.example", false, "", time.Now())
	paused, err := sub.InState(PlatformPaused)
	if err != nil || paused.State != PlatformPaused {
		t.Fatalf("pausing: %+v, %v", paused, err)
	}
	if _, err := sub.InState("DISABLED"); err == nil {
		t.Error("a platform subscription was switched off")
	}
	moved, err := sub.Retargeted("https://other.example/hook")
	if err != nil || moved.TargetURL != "https://other.example/hook" {
		t.Errorf("retargeting: %+v, %v", moved, err)
	}
	if _, err := sub.Retargeted("nope"); err == nil {
		t.Error("a malformed target was taken")
	}
}
