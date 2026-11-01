// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// PagesUnderReview pages through the untrashed pages whose verification runs past now, in id order.
type PagesUnderReview interface {
	UnderReview(ctx context.Context, now time.Time, after shared.ID, limit int) ([]domain.Page, error)
}

// reviewPage is how many pages one read of the pass takes.
const reviewPage = 500

// RestoredReviews re-arms the review clocks of a restored workspace's pages: a restore carries no
// timers (backup-restore.md §8.4), so a page it brought back would never tell its owner its
// verification ran out. A date already past is not told late, as no past reminder is; the page
// shows itself due for review.
type RestoredReviews struct {
	Pages  PagesUnderReview
	Timers ReviewTimers
	Clock  clock.Clock
}

// Rearm re-arms the review clocks of the workspace the transaction is in.
func (r RestoredReviews) Rearm(ctx context.Context, tenant shared.ID) error {
	now := r.Clock.Now()
	var after shared.ID
	for {
		pages, err := r.Pages.UnderReview(ctx, now, after, reviewPage)
		if err != nil {
			return err
		}
		for _, page := range pages {
			if err := armReview(ctx, r.Timers, tenant, page, now); err != nil {
				return err
			}
			after = page.ID
		}
		if len(pages) < reviewPage {
			return nil
		}
	}
}
