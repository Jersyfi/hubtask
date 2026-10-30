// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// PushRunner runs one calendar push and answers when to run next; zero for never.
type PushRunner interface {
	Run(ctx context.Context, tenantID, pushID shared.ID) (time.Time, error)
}

// CalendarPush is one calendar push's run (UC-INT-16): one job per push, rescheduling itself every
// quarter hour and finishing when the push is gone or stopped - connecting again seeds it.
//
// Detached: the events go to the provider's server, and no transaction stays open across it.
type CalendarPush struct {
	Pushes PushRunner
	// Now is the clock the wait is measured on; nil is the system's.
	Now func() time.Time
}

var (
	_ queue.Handler  = CalendarPush{}
	_ queue.Detached = CalendarPush{}
)

// OwnsItsTransactions marks the handler detached.
func (CalendarPush) OwnsItsTransactions() {}

// Run writes the push the job names.
func (h CalendarPush) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	raw, _ := job.Payload["push_id"].(string)
	id, err := shared.ParseID(raw)
	if job.TenantID.IsZero() || err != nil {
		return queue.Result{}, shared.ErrInternal.WithDetail("calendar_push.job_without_push")
	}
	next, err := h.Pushes.Run(ctx, job.TenantID, id)
	if err != nil {
		return queue.Result{}, err
	}
	if next.IsZero() {
		return queue.Result{}, nil
	}
	now := time.Now
	if h.Now != nil {
		now = h.Now
	}
	return queue.Result{Repeat: true, RepeatAfter: waitUntil(now(), next, time.Hour)}, nil
}
