// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"errors"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// reviewTimer is the name of a page's one review clock within its purpose.
const reviewTimer = "review"

// ReviewTimers arms and stops the durable timer that counts down to a page's review (UC-DOC-12
// check 4) - the calendar service's Clocks, inside the caller's transaction.
type ReviewTimers interface {
	Arm(ctx context.Context, tenant shared.ID, purpose calendar.Purpose, subject shared.ID, name string,
		cal calendar.BusinessCalendar, budget time.Duration) (calendar.Timer, error)
	Stop(ctx context.Context, purpose calendar.Purpose, subject shared.ID, name string) (calendar.Timer, error)
}

// ReviewNotifier tells a page's owner that its review fell due.
type ReviewNotifier interface {
	PageReviewDue(ctx context.Context, page domain.Page) error
}

// armReview replaces the page's review clock by one running to its new verification date. The
// date is wall time, so the calendar is continuous; a date beyond a timer's bound is reached in
// legs, the handler arming the next one when a leg elapses early.
func armReview(ctx context.Context, timers ReviewTimers, page domain.Page, now time.Time) error {
	if timers == nil {
		return nil
	}
	if _, err := timers.Stop(ctx, calendar.PurposePageReview, page.ID, reviewTimer); err != nil &&
		!errors.Is(err, shared.ErrNotFound) {
		return err
	}
	if page.VerifiedUntil == nil {
		return nil
	}
	budget := min(page.VerifiedUntil.Sub(now), calendar.MaxBudget)
	if budget < time.Second {
		return nil
	}
	_, err := timers.Arm(ctx, page.TenantID, calendar.PurposePageReview, page.ID, reviewTimer,
		calendar.Continuous(), budget)
	return err
}

// PageReviewElapsed is the PAGE_REVIEW purpose's handler in the timer pass: the page's owner is
// told, unless the page is gone, trashed or no longer verified - or its date lies beyond the leg
// that elapsed, when the next leg is armed.
type PageReviewElapsed struct {
	Pages  repository.Pages
	Timers ReviewTimers
	Notify ReviewNotifier
	Clock  clock.Clock
}

// Elapsed handles one fired review timer.
func (h PageReviewElapsed) Elapsed(ctx context.Context, _ appshared.ActorContext, timer calendar.Timer) error {
	page, err := h.Pages.Find(ctx, timer.SubjectID)
	if errors.Is(err, shared.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if page.IsTrashed() || page.VerifiedUntil == nil {
		return nil
	}
	now := h.Clock.Now()
	if !page.DueForReview(now) {
		return armReview(ctx, h.Timers, page, now)
	}
	if h.Notify == nil {
		return nil
	}
	return h.Notify.PageReviewDue(ctx, page)
}
