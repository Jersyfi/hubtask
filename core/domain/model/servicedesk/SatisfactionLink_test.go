// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-SVC-07 check 6: a satisfaction link answers once, within its life.
func TestASatisfactionLinkAnswersOnceWithinItsLife(t *testing.T) {
	now := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	link, err := NewSatisfactionLink("t", "r", "i", now)
	if err != nil {
		t.Fatal(err)
	}
	if !link.ExpiresAt.Equal(now.Add(SatisfactionLinkLife)) {
		t.Fatalf("expires %v", link.ExpiresAt)
	}
	cases := map[string]struct {
		link   SatisfactionLink
		at     time.Time
		detail string
		kind   error
	}{
		"fresh":              {link, now.Add(time.Hour), "", nil},
		"at its last moment": {link, link.ExpiresAt.Add(-time.Nanosecond), "", nil},
		"expired":            {link, link.ExpiresAt, "satisfaction.link_expired", shared.ErrGone},
		"used":               {func() SatisfactionLink { l, _ := link.Use(now); return l }(), now.Add(time.Minute), "satisfaction.link_used", shared.ErrConflict},
	}
	for name, c := range cases {
		used, err := c.link.Use(c.at)
		if c.kind == nil {
			if err != nil || used.UsedAt == nil || !used.UsedAt.Equal(c.at) {
				t.Errorf("%s: %v, used %v", name, err, used.UsedAt)
			}
			continue
		}
		var typed *shared.Error
		if !errors.Is(err, c.kind) || !errors.As(err, &typed) || typed.DetailCode != c.detail {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := NewSatisfactionLink("t", "", "i", now); err == nil {
		t.Error("a link without its requester was made")
	}
}
