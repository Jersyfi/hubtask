// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"

	service "github.com/Jersyfi/hubtask/core/application/service/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// PrivacyExtensionEntry records an installation-wide case's extension in one workspace the person
// is a member of (data-protection.md §4.1).
//
// Not detached: the entry and the job's completion are one transaction in that workspace's scope,
// so a run that fails after the entry is rolled back whole and its retry stores exactly one.
type PrivacyExtensionEntry struct {
	Record service.RecordExtensionEntry
}

var _ queue.Handler = PrivacyExtensionEntry{}

// Run writes the entry.
func (h PrivacyExtensionEntry) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.Internalf("privacy: an extension entry job without a tenant")
	}
	entry, err := service.ExtensionEntryOf(job.Payload, job.TenantID)
	if err != nil {
		return queue.Result{}, err
	}
	if _, err := h.Record.Execute(ctx, entry); err != nil {
		return queue.Result{}, err
	}
	return queue.Result{}, nil
}
