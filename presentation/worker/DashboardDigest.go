// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// DigestSender sends one digest when it is owed and answers when it is owed next.
type DigestSender interface {
	RunScheduled(ctx context.Context, tenantID, digestID shared.ID) (time.Time, error)
}

// DashboardDigest is one person's digest of a dashboard by mail (UC-GOL-04): one job per digest,
// rescheduling itself to the digest's next moment and finishing when the digest or its subscriber
// is gone - the next write seeds it again.
//
// Detached: the mail goes to somebody else's server, and no transaction stays open across it.
type DashboardDigest struct {
	Digests DigestSender
	// Now is the clock the wait is measured on; nil is the system's.
	Now func() time.Time
}

var (
	_ queue.Handler  = DashboardDigest{}
	_ queue.Detached = DashboardDigest{}
)

// OwnsItsTransactions marks the handler detached.
func (DashboardDigest) OwnsItsTransactions() {}

// Run sends the digest the job names, when it is owed.
func (h DashboardDigest) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	raw, _ := job.Payload["digest_id"].(string)
	id, err := shared.ParseID(raw)
	if job.TenantID.IsZero() || err != nil {
		return queue.Result{}, shared.ErrInternal.WithDetail("dashboards.digest_job_without_digest")
	}
	next, err := h.Digests.RunScheduled(ctx, job.TenantID, id)
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
	return queue.Result{Repeat: true, RepeatAfter: waitUntil(now(), next, 7*24*time.Hour)}, nil
}
