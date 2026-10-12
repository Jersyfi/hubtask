// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/recurrence"
)

// PausedRules switches back on the rules a workspace's deletion request switched off, when the
// operator calls the deletion off (multi-tenancy.md §5, UC-INS-08 check 5).
//
// Not a use case: the operator's cancel calls it inside its own transaction, and the workspace's
// people never ask for it. It asks no permission again, and that is deliberate rather than an
// omission: every credential of a pending workspace is refused, so nothing in it - no role, no
// rule - can have changed since each rule was last switched on under the rights it needed then.
//
// It switches them on as EnableRule does: a schedule's next moment from now, never the occurrences
// missed while the workspace was pending, and the tenant's poller woken for the earliest.
type PausedRules struct {
	Paused   repository.Paused
	Expander recurrence.Expander
	Jobs     Queue
}

// Resume switches the marked rules of the transaction's workspace back on and reports how many. A
// schedule this build can no longer expand stays off and keeps its mark - switching it on with no
// moment would make a rule that looks on and never fires.
func (p PausedRules) Resume(ctx context.Context, tenantID shared.ID, now time.Time) (int, error) {
	rules, err := p.Paused.List(ctx)
	if err != nil {
		return 0, err
	}

	resumed := 0
	var earliest time.Time
	for _, rule := range rules {
		next, err := NextOccurrence(p.Expander, rule, now)
		if err != nil {
			continue
		}
		switched, err := p.Paused.Resume(ctx, rule.ID, next, now)
		if err != nil {
			return resumed, err
		}
		if !switched {
			continue
		}
		resumed++
		if !next.IsZero() && (earliest.IsZero() || next.Before(earliest)) {
			earliest = next
		}
	}

	if earliest.IsZero() || p.Jobs == nil {
		return resumed, nil
	}
	_, err = p.Jobs.Enqueue(ctx, queue.Request{
		Kind: queue.KindAutomationSchedule, TenantID: tenantID,
		DedupeKey: tenantID.String(), RunAt: earliest.UTC(),
	})
	return resumed, err
}
