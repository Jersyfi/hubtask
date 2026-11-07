// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"

	service "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// SAMLCertificateWatch watches one workspace's SAML certificates (identity.md §26.1), in the
// deadline watch's shape: seeded by the save of a SAML provider, rescheduled while the workspace
// has one, finished when it has none.
type SAMLCertificateWatch struct {
	Watch service.WatchSAMLCertificates
	// Queue holds this job's row before anything is read, so a provider saved meanwhile cannot
	// find the row RUNNING and leave its watch unscheduled.
	Queue queue.Queue
	Clock clock.Clock
}

var _ queue.Handler = SAMLCertificateWatch{}

var _ queue.Perpetual = SAMLCertificateWatch{}

// RepeatsForever: the job is a duty kept as one row (queue.Perpetual).
func (SAMLCertificateWatch) RepeatsForever() {}

// Run gives the warnings owed and comes back when the next one is due.
func (w SAMLCertificateWatch) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.Internalf("identity: a certificate watch without a workspace")
	}
	if err := w.Queue.Hold(ctx, job); err != nil {
		return queue.Result{}, err
	}
	actor := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: job.TenantID}
	next, err := w.Watch.Execute(ctx, actor)
	if err != nil || next == nil {
		return queue.Result{}, err
	}
	return queue.Result{Repeat: true, RepeatAfter: next.Sub(w.Clock.Now())}, nil
}
