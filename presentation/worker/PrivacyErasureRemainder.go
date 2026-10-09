// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"log/slog"

	service "github.com/Jersyfi/hubtask/core/application/service/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// PrivacyErasureRemainder carries out the rest of an erasure a legal hold kept part of, once the
// hold is lifted (data-protection.md §4.1).
//
// Detached, for the erasure's own reason: it serves every storage location in the data catalogue,
// and the bytes go outside any transaction. Safe to repeat: it runs the same erasure against the
// holds in force, and what is already gone is not there to remove twice.
type PrivacyErasureRemainder struct {
	Resume service.ResumeErasure
}

var (
	_ queue.Handler  = PrivacyErasureRemainder{}
	_ queue.Detached = PrivacyErasureRemainder{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for. See the type's comment.
func (h PrivacyErasureRemainder) OwnsItsTransactions() {}

// Run carries out the rest of one case.
func (h PrivacyErasureRemainder) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.Internalf("privacy: a remainder job without a tenant")
	}
	in, err := service.RequestOf(job.Payload, job.TenantID)
	if err != nil {
		return queue.Result{}, err
	}
	if err := h.Resume.Resume(ctx, in); err != nil {
		return queue.Result{}, err
	}

	// The case and nothing about the person (rule 10).
	slog.InfoContext(ctx, "the rest of an erasure was carried out",
		slog.String("request_id", in.RequestID.String()))
	return queue.Result{}, nil
}
