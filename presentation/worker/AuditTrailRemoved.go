// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"

	service "github.com/Jersyfi/hubtask/core/application/service/audit"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// AuditTrailRemoved records in one workspace's trail that a month of it went (audit.md §3).
//
// Not detached: the entry and the job's completion are one transaction in that workspace's scope,
// so a run that fails after the entry is rolled back whole and its retry stores exactly one.
type AuditTrailRemoved struct {
	Record service.RecordTrailRemoval
}

var _ queue.Handler = AuditTrailRemoved{}

// Run writes the entry.
func (h AuditTrailRemoved) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	removal, err := service.TrailRemovalOf(job.Payload, job.TenantID)
	if err != nil {
		return queue.Result{}, err
	}
	if _, err := h.Record.Execute(ctx, removal); err != nil {
		return queue.Result{}, err
	}
	return queue.Result{}, nil
}
