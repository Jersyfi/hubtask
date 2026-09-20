// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"

	"github.com/Jersyfi/hubtask/core/application/service/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// AutomationCheck is one workspace's check of its rules, as a deletion seeded it (ADR-0060).
//
// Not Detached: everything the check does is a write - findings recorded, a broken rule switched
// off with its audit entry and its notification - and those belong in the runner's transaction
// with the job's own completion, so that a process that dies halfway leaves the rules as they were.
type AutomationCheck struct {
	Check automation.CheckRules
}

var _ queue.Handler = AutomationCheck{}

// Run checks the tenant the job names.
func (h AutomationCheck) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		// Every read the check makes is made for one workspace; there is no instance-wide rule
		// for a leader to own, because automation_rule.tenant_id is NOT NULL.
		return queue.Result{}, shared.ErrInternal.WithDetail("automation.check_without_tenant")
	}
	if err := h.Check.Sweep(ctx, job.TenantID); err != nil {
		return queue.Result{}, err
	}
	return queue.Result{}, nil
}
