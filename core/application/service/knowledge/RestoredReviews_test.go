// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

type underReview []domain.Page

func (u underReview) UnderReview(_ context.Context, _ time.Time, after shared.ID, _ int) ([]domain.Page, error) {
	if !after.IsZero() {
		return nil, nil
	}
	return u, nil
}

// A restored page under review gets its clock back, running to its verification date
// (backup-restore.md §8.4: the archive carries no timers).
func TestARestoredPageUnderReviewGetsItsClockBack(t *testing.T) {
	page := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000007a1")
	until := start.AddDate(0, 1, 0)
	timers := &clocks{live: map[shared.ID]calendar.Timer{}}
	restored := RestoredReviews{
		Pages: underReview{{ID: page, VerifiedUntil: &until}}, Timers: timers, Clock: clock.Fixed(start),
	}
	if err := restored.Rearm(context.Background(), tenant); err != nil {
		t.Fatal(err)
	}
	timer, armed := timers.live[page]
	if !armed || timer.Purpose != calendar.PurposePageReview || timer.TenantID != tenant || !timer.DueAt.Equal(until) {
		t.Errorf("the review clock is %+v (armed %v), want one due %s", timer, armed, until)
	}
}
